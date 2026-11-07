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

	"math/rand"

	"golang.org/x/crypto/bcrypt"

	"familypoints/server/internal/auth"
	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
)

// 业务错误：handler 按这些值映射 HTTP 状态码，不要在 handler 里判断错误文案。
var (
	ErrInvalidCredentials = errors.New("用户名或密码不正确")
	ErrMemberDisabled     = errors.New("账号已被停用")
	ErrUsernameTaken      = errors.New("用户名已被占用")
	ErrPhoneTaken         = errors.New("该手机号已被注册")
	ErrEmailTaken         = errors.New("该邮箱已被注册")
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
	repo        *repo.Auth
	siteConfigs *repo.SiteConfig
	token       *auth.Manager
}

// NewAuth 构造。
func NewAuth(r *repo.Auth, sc *repo.SiteConfig, tm *auth.Manager) *Auth {
	return &Auth{repo: r, siteConfigs: sc, token: tm}
}

// RegisterInput 是注册入参。CreateFamily 为 true 时顺带建组并成为组长。
type RegisterInput struct {
	// Mode：username（默认）/ phone / email（2026-11-07；后两者受后台开关控制）
	Mode         string `json:"mode"`
	Username     string `json:"username"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	Nickname     string `json:"nickname"`
	CreateFamily bool   `json:"create_family"`
	FamilyName   string `json:"family_name"`
}

// 手机号 / 邮箱格式（开发阶段只做格式校验；短信/邮件验证码属生产化 checklist）
var (
	phonePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)
	emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// regSwitch 读注册方式开关（site_configs；未配置视为关闭）。

// RegPhoneEnabled 手机号注册是否开放。
func (s *Auth) RegPhoneEnabled(ctx context.Context) bool { return s.regSwitch(ctx, "reg_phone") }

// RegEmailEnabled 邮箱注册是否开放。
func (s *Auth) RegEmailEnabled(ctx context.Context) bool { return s.regSwitch(ctx, "reg_email") }
func (s *Auth) regSwitch(ctx context.Context, name string) bool {
	v, _ := s.siteConfigs.Get(ctx, name)
	return v == "1"
}

// Register 注册前台账号。**绝不会产生管理员**（管理员只能由初始化/后台创建）。
func (s *Auth) Register(ctx context.Context, in RegisterInput) (*model.Member, *TokenPair, error) {
	in.Mode = strings.TrimSpace(strings.ToLower(in.Mode))
	if in.Mode == "" {
		in.Mode = "username"
	}
	in.Username = strings.TrimSpace(in.Username)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Email = strings.TrimSpace(in.Email)
	in.Nickname = strings.TrimSpace(in.Nickname)
	in.FamilyName = strings.TrimSpace(in.FamilyName)

	switch in.Mode {
	case "username":
		// 用户名注册：始终开放
	case "phone":
		if !s.regSwitch(ctx, "reg_phone") {
			return nil, nil, fmt.Errorf("%w：手机号注册未开放", ErrInvalidInput)
		}
		if !phonePattern.MatchString(in.Phone) {
			return nil, nil, fmt.Errorf("%w：手机号格式不正确", ErrInvalidInput)
		}
		if p, _ := s.repo.MemberByIdentifier(ctx, in.Phone); p != nil {
			return nil, nil, ErrPhoneTaken
		}
		if in.Username == "" {
			in.Username = "u" + strings.ToLower(randString(10))
		}
	case "email":
		if !s.regSwitch(ctx, "reg_email") {
			return nil, nil, fmt.Errorf("%w：邮箱注册未开放", ErrInvalidInput)
		}
		if !emailPattern.MatchString(in.Email) {
			return nil, nil, fmt.Errorf("%w：邮箱格式不正确", ErrInvalidInput)
		}
		if p, _ := s.repo.MemberByIdentifier(ctx, in.Email); p != nil {
			return nil, nil, ErrEmailTaken
		}
		if in.Username == "" {
			in.Username = "u" + strings.ToLower(randString(10))
		}
	default:
		return nil, nil, fmt.Errorf("%w：不支持的注册方式", ErrInvalidInput)
	}

	if !usernamePattern.MatchString(in.Username) {
		return nil, nil, fmt.Errorf("%w：用户名需 3–20 位，仅限字母、数字、下划线", ErrInvalidInput)
	}
	if err := checkPassword(in.Password); err != nil {
		return nil, nil, err
	}
	if in.Nickname == "" {
		in.Nickname = in.Username
	}
	if utf8.RuneCountInString(in.Nickname) > 32 {
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
			name = in.Nickname + "的家"
		}
		familyName = &name
	}

	member, err := s.repo.CreateMember(ctx, in.Username, string(hash), in.Nickname, familyName, phoneOrNil(in), emailOrNil(in))
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
	// 2026-11-07：identifier 可以是 用户名 / 手机号 / 邮箱
	m, err := s.repo.MemberByIdentifier(ctx, strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(m.PassHash), []byte(password)) != nil {
		return nil, nil, ErrInvalidCredentials
	}
	if m.Disabled {
		return nil, nil, ErrMemberDisabled
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

// UpdateAdminNickname 只改管理员昵称。**没有任何改 username 的方法**，这是设计约束。
func (s *Auth) UpdateAdminNickname(ctx context.Context, adminID uint64, nickname string) error {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" || utf8.RuneCountInString(nickname) > 32 {
		return fmt.Errorf("%w：昵称需 1–32 个字符", ErrInvalidInput)
	}
	return s.repo.UpdateAdminNickname(ctx, adminID, nickname)
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

// ErrNotFamilyFreeForCancel 注销前必须先退出家庭组。
var ErrNotFamilyFreeForCancel = errors.New("请先退出家庭组再注销账号")

// CancelMember 注销账号（2026-11-07）：需密码二次确认，仅限未入组成员。
// 成功后该账号即从系统中删除（历史流水保留），客户端应清空本地会话。
func (s *Auth) CancelMember(ctx context.Context, me *model.Member, password string) error {
	if password == "" {
		return fmt.Errorf("%w：请输入密码确认注销", ErrInvalidInput)
	}
	if me.InFamily() {
		return ErrNotFamilyFreeForCancel
	}
	// 用带 pass_hash 的完整行校验密码（middleware 里的 me 不一定带）
	full, err := s.repo.MemberByID(ctx, me.ID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(full.PassHash), []byte(password)) != nil {
		return ErrInvalidCredentials
	}
	return s.repo.CancelAccount(ctx, me.ID)
}

// randString 生成 n 位小写字母数字随机串（手机/邮箱注册时自动生成用户名用）。
func randString(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

func phoneOrNil(in RegisterInput) *string {
	if in.Phone == "" {
		return nil
	}
	return &in.Phone
}

func emailOrNil(in RegisterInput) *string {
	if in.Email == "" {
		return nil
	}
	return &in.Email
}
