// Package service 是业务逻辑层：参数校验、规则判断、事务编排。
package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"familypoints/server/internal/auth"
	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
)

// 业务错误：handler 按这些值映射 HTTP 状态码，不要在 handler 里判断错误文案。
var (
	ErrInvalidCredentials = errors.New("用户名或密码不正确")
	ErrUsernameTaken      = errors.New("用户名已被占用")
	ErrInvalidInput       = errors.New("参数不合法")
	ErrTokenInvalid       = errors.New("登录状态已失效，请重新登录")
	ErrAdminDisabled      = errors.New("该管理员账号已停用")
	ErrNotInFamily        = errors.New("尚未加入家庭组")
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,20}$`)

// TokenPair 是登录/刷新成功后返回给客户端的一对 token。
type TokenPair struct {
	AccessToken      string `json:"access_token"`
	AccessExpiresIn  int64  `json:"access_expires_in"` // 秒
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"` // 秒
	TokenType        string `json:"token_type"`         // 固定 "Bearer"
	// 仅后台有意义：true 表示必须先改密码（5.4.2）。
	// 注意**不能加 omitempty** —— false 与「字段不存在」对客户端含义完全不同。
	MustChangePassword bool `json:"must_change_password"`
}

// Auth 是认证服务。
type Auth struct {
	repo  *repo.Auth
	token *auth.Manager
}

// NewAuth 构造。
func NewAuth(r *repo.Auth, tm *auth.Manager) *Auth {
	return &Auth{repo: r, token: tm}
}

// RegisterInput 是注册入参。CreateFamily 为 true 时顺带建组并成为组长。
type RegisterInput struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	Name         string `json:"name"`
	CreateFamily bool   `json:"create_family"`
	FamilyName   string `json:"family_name"`
}

// Register 注册前台账号。**绝不会产生管理员**（管理员只能由初始化/后台创建）。
func (s *Auth) Register(ctx context.Context, in RegisterInput) (*model.Member, *TokenPair, error) {
	in.Username = strings.TrimSpace(in.Username)
	in.Name = strings.TrimSpace(in.Name)
	in.FamilyName = strings.TrimSpace(in.FamilyName)

	if !usernamePattern.MatchString(in.Username) {
		return nil, nil, fmt.Errorf("%w：用户名需 3–20 位，仅限字母、数字、下划线", ErrInvalidInput)
	}
	if err := checkPassword(in.Password); err != nil {
		return nil, nil, err
	}
	if in.Name == "" {
		in.Name = in.Username
	}
	if utf8.RuneCountInString(in.Name) > 32 {
		return nil, nil, fmt.Errorf("%w：昵称最多 32 个字符", ErrInvalidInput)
	}
	if in.CreateFamily && utf8.RuneCountInString(in.FamilyName) > 64 {
		return nil, nil, fmt.Errorf("%w：家庭组名最多 64 个字符", ErrInvalidInput)
	}

	if _, err := s.repo.MemberByUsername(ctx, in.Username); err == nil {
		return nil, nil, ErrUsernameTaken
	} else if !errors.Is(err, repo.ErrNotFound) {
		return nil, nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, err
	}

	var familyName *string
	if in.CreateFamily {
		name := in.FamilyName
		if name == "" {
			name = in.Name + "的家"
		}
		familyName = &name
	}

	member, err := s.repo.CreateMember(ctx, in.Username, string(hash), in.Name, familyName)
	if err != nil {
		return nil, nil, err
	}

	pair, err := s.issue(ctx, model.SubjectMember, member.ID, false)
	if err != nil {
		return nil, nil, err
	}
	return member, pair, nil
}

// LoginMember 前台登录。
func (s *Auth) LoginMember(ctx context.Context, username, password string) (*model.Member, *TokenPair, error) {
	m, err := s.repo.MemberByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(m.PassHash), []byte(password)) != nil {
		return nil, nil, ErrInvalidCredentials
	}
	pair, err := s.issue(ctx, model.SubjectMember, m.ID, false)
	if err != nil {
		return nil, nil, err
	}
	return m, pair, nil
}

// LoginAdmin 后台登录。停用账号直接拒绝；未改密的仍发 token，但带标记。
func (s *Auth) LoginAdmin(ctx context.Context, username, password string) (*model.Admin, *TokenPair, error) {
	a, err := s.repo.AdminByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}
	if a.Disabled {
		return nil, nil, ErrAdminDisabled
	}
	if bcrypt.CompareHashAndPassword([]byte(a.PassHash), []byte(password)) != nil {
		return nil, nil, ErrInvalidCredentials
	}
	pair, err := s.issue(ctx, model.SubjectAdmin, a.ID, a.MustChangePassword)
	if err != nil {
		return nil, nil, err
	}
	return a, pair, nil
}

// Refresh 用 refresh token 换新的一对 token（轮换：旧的立即失效）。
//
// 用「轮换 + 落库」而不是纯无状态，是为了改密/登出后能立刻断掉旧会话。
func (s *Auth) Refresh(ctx context.Context, rawRefresh, subjectType string) (*TokenPair, error) {
	if rawRefresh == "" {
		return nil, ErrTokenInvalid
	}
	hash := auth.HashRefresh(rawRefresh)

	t, err := s.repo.RefreshByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrTokenInvalid
		}
		return nil, err
	}
	if t.SubjectType != subjectType {
		return nil, ErrTokenInvalid
	}
	if !t.Valid(time.Now()) {
		return nil, ErrTokenInvalid
	}

	// 主体可能已被停用/删除，这里要重新确认
	mustChange := false
	switch subjectType {
	case model.SubjectAdmin:
		a, err := s.repo.AdminByID(ctx, t.SubjectID)
		if err != nil || a.Disabled {
			return nil, ErrTokenInvalid
		}
		mustChange = a.MustChangePassword
	case model.SubjectMember:
		if _, err := s.repo.MemberByID(ctx, t.SubjectID); err != nil {
			return nil, ErrTokenInvalid
		}
	default:
		return nil, ErrTokenInvalid
	}

	rawNew, hashNew, err := auth.NewRefresh()
	if err != nil {
		return nil, err
	}
	if err := s.repo.RevokeRefresh(ctx, hash, hashNew); err != nil {
		return nil, err
	}
	if err := s.repo.InsertRefresh(ctx, subjectType, t.SubjectID, hashNew,
		time.Now().Add(s.token.RefreshTTL())); err != nil {
		return nil, err
	}

	access, err := s.token.SignAccess(t.SubjectID, subjectType)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:        access,
		AccessExpiresIn:    int64(s.token.AccessTTL().Seconds()),
		RefreshToken:       rawNew,
		RefreshExpiresIn:   int64(s.token.RefreshTTL().Seconds()),
		TokenType:          "Bearer",
		MustChangePassword: mustChange,
	}, nil
}

// ChangeMemberPassword 改前台密码：校验旧密码 → 写新哈希 → **吊销该成员全部 refresh token**。
func (s *Auth) ChangeMemberPassword(ctx context.Context, memberID uint64, oldPassword, newPassword string) error {
	m, err := s.repo.MemberByID(ctx, memberID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(m.PassHash), []byte(oldPassword)) != nil {
		return ErrInvalidCredentials
	}
	if err := checkPassword(newPassword); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateMemberPassword(ctx, memberID, string(hash)); err != nil {
		return err
	}
	return s.repo.RevokeAllForSubject(ctx, model.SubjectMember, memberID)
}

// Logout 登出：吊销该主体的**全部** refresh token。
//
// 为什么不是「只吊销本次传上来的那一条」：
//
//	refresh 是无状态的 30 天凭证，前端「丢 token」并不等于服务端失效——
//	拿到过 refresh 的人仍能换出新的 access（文档 5.4.4 此前正是这个缺口）。
//	按主体全量吊销，语义上「退出登录 = 该账号所有会话下线」，
//	单设备多开也不会留下可用凭证。
//
// 调用方必须是已认证的主体（access token 未过期），所以不需要再验密码。
func (s *Auth) Logout(ctx context.Context, subjectType string, subjectID uint64) error {
	if subjectType != model.SubjectAdmin && subjectType != model.SubjectMember {
		return ErrTokenInvalid
	}
	return s.repo.RevokeAllForSubject(ctx, subjectType, subjectID)
}

// ChangeAdminPassword 改后台密码；同时清掉 must_change_password。
func (s *Auth) ChangeAdminPassword(ctx context.Context, adminID uint64, oldPassword, newPassword string) error {
	a, err := s.repo.AdminByID(ctx, adminID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(a.PassHash), []byte(oldPassword)) != nil {
		return ErrInvalidCredentials
	}
	if err := checkPassword(newPassword); err != nil {
		return err
	}
	if oldPassword == newPassword {
		return fmt.Errorf("%w：新密码不能与旧密码相同", ErrInvalidInput)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateAdminPassword(ctx, adminID, string(hash)); err != nil {
		return err
	}
	return s.repo.RevokeAllForSubject(ctx, model.SubjectAdmin, adminID)
}

// MemberByID / AdminByID 供鉴权中间件使用。
func (s *Auth) MemberByID(ctx context.Context, id uint64) (*model.Member, error) {
	return s.repo.MemberByID(ctx, id)
}

// AdminByID 同上。
func (s *Auth) AdminByID(ctx context.Context, id uint64) (*model.Admin, error) {
	return s.repo.AdminByID(ctx, id)
}

// issue 签发一对 token（登录/注册后调用）。
func (s *Auth) issue(ctx context.Context, subjectType string, subjectID uint64, mustChange bool) (*TokenPair, error) {
	access, err := s.token.SignAccess(subjectID, subjectType)
	if err != nil {
		return nil, err
	}
	rawRefresh, hashRefresh, err := auth.NewRefresh()
	if err != nil {
		return nil, err
	}
	if err := s.repo.InsertRefresh(ctx, subjectType, subjectID, hashRefresh,
		time.Now().Add(s.token.RefreshTTL())); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:        access,
		AccessExpiresIn:    int64(s.token.AccessTTL().Seconds()),
		RefreshToken:       rawRefresh,
		RefreshExpiresIn:   int64(s.token.RefreshTTL().Seconds()),
		TokenType:          "Bearer",
		MustChangePassword: mustChange,
	}, nil
}

// checkPassword 校验密码强度。bcrypt 只处理前 72 字节，超长必须挡掉。
func checkPassword(p string) error {
	n := len([]byte(p))
	if n < 8 {
		return fmt.Errorf("%w：密码至少 8 位", ErrInvalidInput)
	}
	if n > 72 {
		return fmt.Errorf("%w：密码最长 72 字节", ErrInvalidInput)
	}
	return nil
}
