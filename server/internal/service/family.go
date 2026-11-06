// Package service：家庭组相关业务。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"familypoints/server/internal/model"
	"familypoints/server/internal/notify"
	"familypoints/server/internal/repo"
)

// 家庭组相关的业务错误。handler 统一映射成状态码。
var (
	ErrNotLeader          = errors.New("只有组长可以执行该操作")
	ErrAlreadyInFamily    = errors.New("你已经加入了家庭组")
	ErrInviteCodeInvalid  = errors.New("邀请码无效或已过期")
	ErrBanned             = errors.New("你已被该家庭组禁止加入")
	ErrLeaderMustTransfer = errors.New("组长不能直接退出，请先转让组长")
	ErrConfirmRequired    = errors.New("该操作不可撤销，需要二次确认")
	ErrCannotTargetSelf   = errors.New("不能对自己执行该操作")
	ErrInvitationExpired  = errors.New("邀请已过期")
)

// Family 是家庭组服务。
type Family struct {
	families    *repo.Family
	auth        *repo.Auth
	invitations *repo.Invitation
	notifier    *notify.Service
}

// NewFamily 构造。notifier 是旁路，允许为 nil（如单测）。
func NewFamily(r *repo.Family, a *repo.Auth, inv *repo.Invitation, n *notify.Service) *Family {
	return &Family{families: r, auth: a, invitations: inv, notifier: n}
}

// MemberBrief 是对外暴露的成员信息：**绝不包含 pass_hash**。
// username 不可改（登录凭证），nickname 可改（显示名）。
type MemberBrief struct {
	ID          uint64    `json:"id"`
	Nickname    string    `json:"nickname"`
	Username    string    `json:"username"`
	Role        string    `json:"role"`
	Avatar      *string   `json:"avatar"`
	Score       int       `json:"score"`
	FrozenScore int       `json:"frozen_score"`
	Hidden      bool      `json:"hidden"`
	CreatedAt   time.Time `json:"created_at"`
}

// Info 返回家庭组基本信息。
func (s *Family) Info(ctx context.Context, familyID uint64) (*model.Family, error) {
	return s.families.ByID(ctx, familyID)
}

// Members 返回组成员。includeHidden 仅后台可传 true。
func (s *Family) Members(ctx context.Context, familyID uint64, includeHidden bool) ([]MemberBrief, error) {
	// 组不存在时返回 404，而不是空列表——空列表会让人以为「组在但没人」
	if _, err := s.families.ByID(ctx, familyID); err != nil {
		return nil, err
	}
	list, err := s.families.Members(ctx, familyID, includeHidden)
	if err != nil {
		return nil, err
	}
	out := make([]MemberBrief, 0, len(list))
	for _, m := range list {
		out = append(out, MemberBrief{
			ID: m.ID, Nickname: m.Nickname, Username: m.Username, Role: m.Role, Avatar: m.Avatar,
			Score: m.Score, FrozenScore: m.FrozenScore, Hidden: m.Hidden, CreatedAt: m.CreatedAt,
		})
	}
	return out, nil
}

// FamilyCode 返回本组家庭码与隐藏状态。
//
// **全员可读**：家庭码不是密钥，成员都可能需要把它告诉家人；
// 只有「改隐藏」是组长权限（SetCodeHidden）。
func (s *Family) FamilyCode(ctx context.Context, me *model.Member) (code string, hidden bool, err error) {
	if !me.InFamily() {
		return "", false, ErrNotInFamily
	}
	f, err := s.families.ByID(ctx, *me.FamilyID)
	if err != nil {
		return "", false, err
	}
	return f.InviteCode, f.CodeHidden, nil
}

// SetCodeHidden 组长设置家庭码是否可被搜索到。
//
// 隐藏只影响「按码搜索」，**不影响凭码直接加入**——否则隐藏等于关闭入组，
// 而需求只是「不想被陌生人搜到」。
func (s *Family) SetCodeHidden(ctx context.Context, me *model.Member, hidden bool) error {
	if err := requireLeader(me); err != nil {
		return err
	}
	return s.families.SetCodeHidden(ctx, *me.FamilyID, hidden)
}

// FamilySearchView 是按家庭码搜索的结果。
//
// 只暴露「组名 + 成员数」，不返回邀请码本身、不返回成员名单：
// 搜索的用途只是确认「是不是这个家」，加入后自然能看到全部信息。
type FamilySearchView struct {
	Found       bool   `json:"found"`
	FamilyID    uint64 `json:"family_id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
}

// SearchByCode 按家庭码查找家庭（仅限**尚未入组**的成员）。
//
// 家庭码被组长设为隐藏时，返回 found=false 且不带任何提示——
// 不能返回「存在但隐藏」，那等于泄露了码的有效性。
func (s *Family) SearchByCode(ctx context.Context, me *model.Member, code string) (*FamilySearchView, error) {
	if me.InFamily() {
		return nil, ErrAlreadyInFamily
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, fmt.Errorf("%w：请填写家庭码", ErrInvalidInput)
	}
	f, err := s.families.ByInviteCode(ctx, code)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return &FamilySearchView{Found: false}, nil
		}
		return nil, err
	}
	if f.CodeHidden {
		return &FamilySearchView{Found: false}, nil
	}
	members, err := s.families.Members(ctx, f.ID, false)
	if err != nil {
		return nil, err
	}
	return &FamilySearchView{
		Found: true, FamilyID: f.ID, Name: f.Name, MemberCount: len(members),
	}, nil
}

// CreateFamily 给尚未入组的成员新建家庭，创建者成为组长。
func (s *Family) CreateFamily(ctx context.Context, me *model.Member, name string) (*model.Family, error) {
	if me.InFamily() {
		return nil, ErrAlreadyInFamily
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return nil, fmt.Errorf("%w：家庭组名需 1–64 个字符", ErrInvalidInput)
	}
	code, err := s.families.NewInviteCode(ctx)
	if err != nil {
		return nil, err
	}
	fid, err := s.families.CreateFamilyForMember(ctx, me.ID, name, code)
	if err != nil {
		return nil, err
	}
	return s.families.ByID(ctx, fid)
}

// Join 凭邀请码加入家庭组。
//
// 四条校验缺一不可：未入组、码存在、码未过期、未被该组禁入。
func (s *Family) Join(ctx context.Context, me *model.Member, code string) (*model.Family, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if me.InFamily() {
		return nil, ErrAlreadyInFamily
	}
	if code == "" {
		return nil, fmt.Errorf("%w：请填写邀请码", ErrInvalidInput)
	}

	f, err := s.families.ByInviteCode(ctx, code)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrInviteCodeInvalid
		}
		return nil, err
	}
	banned, err := s.families.IsBanned(ctx, f.ID, me.ID)
	if err != nil {
		return nil, err
	}
	if banned {
		return nil, ErrBanned
	}
	if err := s.families.Join(ctx, f.ID, me.ID); err != nil {
		return nil, err
	}
	return f, nil
}

// Leave 自行退出家庭组。
//
// 规则：
//   - **必须二次确认**（前端弹窗 + 后端 confirm 标记），因为积分清零不可逆
//   - **组长不能直接退出**，必须先转让组长，否则组内会没人能管理
func (s *Family) Leave(ctx context.Context, me *model.Member, confirm bool) error {
	if !me.InFamily() {
		return ErrNotInFamily
	}
	if !confirm {
		return ErrConfirmRequired
	}
	if me.Role == model.RoleLeader {
		return ErrLeaderMustTransfer
	}
	return s.families.Leave(ctx, *me.FamilyID, me.ID, "left")
}

// Kick 组长踢出成员。同样需要二次确认，且不能踢自己。
func (s *Family) Kick(ctx context.Context, me *model.Member, targetID uint64, confirm bool) error {
	if err := requireLeader(me); err != nil {
		return err
	}
	if targetID == me.ID {
		return ErrCannotTargetSelf
	}
	if !confirm {
		return ErrConfirmRequired
	}
	// 先确认目标确实在本组，避免跨组踢人
	if _, err := s.families.MemberInFamily(ctx, *me.FamilyID, targetID); err != nil {
		return err
	}
	return s.families.Leave(ctx, *me.FamilyID, targetID, "kicked")
}

// TransferLeader 组长转让（组内原子操作：旧组长降级 + 新组长升级）。
func (s *Family) TransferLeader(ctx context.Context, me *model.Member, targetID uint64) error {
	if err := requireLeader(me); err != nil {
		return err
	}
	if targetID == me.ID {
		return ErrCannotTargetSelf
	}
	if _, err := s.families.MemberInFamily(ctx, *me.FamilyID, targetID); err != nil {
		return err
	}
	return s.families.TransferLeader(ctx, *me.FamilyID, me.ID, targetID)
}

// SetHidden 隐藏 / 显示成员（组长操作，只影响展示）。
func (s *Family) SetHidden(ctx context.Context, me *model.Member, targetID uint64, hidden bool) error {
	if err := requireLeader(me); err != nil {
		return err
	}
	if targetID == me.ID {
		return ErrCannotTargetSelf
	}
	return s.families.SetHidden(ctx, *me.FamilyID, targetID, hidden)
}

// RenameFamily 改家庭组名（组长）。
func (s *Family) RenameFamily(ctx context.Context, me *model.Member, name string) (string, error) {
	if err := requireLeader(me); err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return "", fmt.Errorf("%w：家庭组名需 1–64 个字符", ErrInvalidInput)
	}
	if err := s.families.UpdateName(ctx, *me.FamilyID, name); err != nil {
		return "", err
	}
	return name, nil
}

// UpdateMyProfile 更新自己的资料（昵称 / 头像）。**只改 nickname，username 永不可改**。
// RenameMember 成员改昵称（本人或组长可改）。records.member_name 是快照，历史不受影响。
//
// 昵称长度上限与注册一致（32 字符），头像只接受**预设标识**（≤16 字节，通常是 emoji）。
func (s *Family) UpdateMyProfile(ctx context.Context, me *model.Member, nickname, avatar string) (*model.Member, error) {
	nickname = strings.TrimSpace(nickname)
	avatar = strings.TrimSpace(avatar)
	if nickname != "" {
		n := utf8.RuneCountInString(nickname)
		if n == 0 || n > 32 {
			return nil, fmt.Errorf("%w：昵称需 1–32 个字符", ErrInvalidInput)
		}
	}
	if avatar != "" && len(avatar) > 16 {
		return nil, fmt.Errorf("%w：头像标识过长", ErrInvalidInput)
	}
	if nickname == "" && avatar == "" {
		return nil, fmt.Errorf("%w：昵称与头像至少改一项", ErrInvalidInput)
	}
	if err := s.families.UpdateProfile(ctx, me.ID, nickname, avatar); err != nil {
		return nil, err
	}
	return s.families.MemberInFamily(ctx, *me.FamilyID, me.ID)
}

func (s *Family) RenameMember(ctx context.Context, me *model.Member, targetID uint64, nickname string) error {
	if targetID != me.ID && me.Role != model.RoleLeader {
		return ErrNotLeader
	}
	nickname = strings.TrimSpace(nickname)
	if nickname == "" || utf8.RuneCountInString(nickname) > 32 {
		return fmt.Errorf("%w：昵称需 1–32 个字符", ErrInvalidInput)
	}
	if _, err := s.families.MemberInFamily(ctx, *me.FamilyID, targetID); err != nil {
		return err
	}
	return s.families.RenameMember(ctx, *me.FamilyID, targetID, nickname)
}

// requireLeader 校验「已入组且是组长」。
func requireLeader(m *model.Member) error {
	if !m.InFamily() {
		return ErrNotInFamily
	}
	if m.Role != model.RoleLeader {
		return ErrNotLeader
	}
	return nil
}

// ---------- 待确认邀请（P3.1） ----------

// 邀请有效期与邀请码一致：7 天。
const invitationTTL = 7 * 24 * time.Hour

// InviteByUsername 组长按用户名发起邀请（**不直接入组**，等对方确认）。
//
// 与后台 15.2.3 的「按用户名直拉」是两种语义：这里必须经过被邀请人同意。
// 校验顺序与 Join 一致：用户名存在 → 不能邀自己 → 未入组 → 未禁入。
func (s *Family) InviteByUsername(ctx context.Context, me *model.Member, username string) (*model.FamilyInvitation, error) {
	if err := requireLeader(me); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("%w：请填写用户名", ErrInvalidInput)
	}
	target, err := s.auth.MemberByUsername(ctx, username)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, fmt.Errorf("%w：该用户名不存在", ErrInvalidInput)
	}
	if err != nil {
		return nil, err
	}
	if target.ID == me.ID {
		return nil, ErrCannotTargetSelf
	}
	if target.InFamily() {
		return nil, ErrAlreadyInFamily
	}
	banned, err := s.families.IsBanned(ctx, *me.FamilyID, target.ID)
	if err != nil {
		return nil, err
	}
	if banned {
		return nil, ErrBanned
	}

	f, err := s.families.ByID(ctx, *me.FamilyID)
	if err != nil {
		return nil, err
	}
	id, err := s.invitations.Create(ctx, *me.FamilyID, me.ID, target.ID, repo.DirectionInvite, time.Now().Add(invitationTTL))
	if err != nil {
		return nil, err
	}
	// 通知是旁路：失败只记日志，不影响已经创建的邀请
	s.notifier.InviteReceived(ctx, *me.FamilyID, target.ID, id, f.Name, me.Nickname)
	return s.invitations.ByID(ctx, id)
}

// MyInvitations 我收到的邀请。**未入组也能查**（接口挂在 app 组而非 inFamily）。
func (s *Family) MyInvitations(ctx context.Context, me *model.Member) ([]model.FamilyInvitation, error) {
	return s.invitations.ForInvitee(ctx, me.ID, time.Now())
}

// FamilyInvitations 本组发出的邀请（组长可见）。
func (s *Family) FamilyInvitations(ctx context.Context, me *model.Member) ([]model.FamilyInvitation, error) {
	if err := requireLeader(me); err != nil {
		return nil, err
	}
	return s.invitations.ForFamily(ctx, *me.FamilyID, time.Now())
}

// DecideInvitation 接受 / 拒绝邀请。接受时复用与邀请码完全相同的入组事务。
func (s *Family) DecideInvitation(ctx context.Context, me *model.Member, invitationID uint64, accept bool) (*model.FamilyInvitation, error) {
	if invitationID == 0 {
		return nil, fmt.Errorf("%w：缺少 invitation_id", ErrInvalidInput)
	}
	inv, err := s.invitations.ByID(ctx, invitationID)
	if err != nil {
		return nil, err
	}
	// 不是发给我的 → 按「不存在」处理，不泄露他人邀请（4.3 第 2 条）
	if inv.InviteeID != me.ID || inv.Status != model.InvitationPending {
		return nil, repo.ErrNotFound
	}
	if !model.InvitationUsable(inv.Status, inv.ExpiresAt, time.Now()) {
		return nil, fmt.Errorf("%w：邀请已过期", ErrInvitationExpired)
	}
	if me.InFamily() {
		return nil, ErrAlreadyInFamily
	}
	banned, err := s.families.IsBanned(ctx, inv.FamilyID, me.ID)
	if err != nil {
		return nil, err
	}
	if banned {
		return nil, ErrBanned
	}

	if err := s.invitations.Decide(ctx, invitationID, me.ID, accept); err != nil {
		return nil, err
	}
	s.notifier.InviteDecided(ctx, inv.FamilyID, inv.InviterID, invitationID, accept, me.Nickname)
	return s.invitations.ByID(ctx, invitationID)
}

// RequestJoin 凭家庭码申请加入（**需组长审批**，不是直接入组）。
//
// 与 Join 的区别：Join 是「凭码直接入组」，那是后台/老版本行为；
// App 侧 2026-11-06 起统一走申请-审批，避免任何人拿到码就能进门。
// 家庭码被隐藏时不允许申请——隐藏的语义就是「不对外可见」。
func (s *Family) RequestJoin(ctx context.Context, me *model.Member, code string) (*model.FamilyInvitation, error) {
	if me.InFamily() {
		return nil, ErrAlreadyInFamily
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, fmt.Errorf("%w：请填写家庭码", ErrInvalidInput)
	}
	f, err := s.families.ByInviteCode(ctx, code)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrInviteCodeInvalid
		}
		return nil, err
	}
	if f.CodeHidden {
		return nil, ErrInviteCodeInvalid
	}
	banned, err := s.families.IsBanned(ctx, f.ID, me.ID)
	if err != nil {
		return nil, err
	}
	if banned {
		return nil, ErrBanned
	}
	id, err := s.invitations.Create(ctx, f.ID, me.ID, me.ID, repo.DirectionRequest, time.Now().Add(invitationTTL))
	if err != nil {
		return nil, err
	}
	s.notifier.JoinRequested(ctx, f.ID, me.ID, me.Nickname, f.Name)
	return s.invitations.ByID(ctx, id)
}

// MyJoinRequests 我发出的入组申请（未入组时看「等待审批」）。
func (s *Family) MyJoinRequests(ctx context.Context, me *model.Member) ([]model.FamilyInvitation, error) {
	return s.invitations.MyRequests(ctx, me.ID, time.Now())
}

// PendingJoinRequests 本组待审批的入组申请（组长）。
func (s *Family) PendingJoinRequests(ctx context.Context, me *model.Member) ([]model.FamilyInvitation, error) {
	if err := requireLeader(me); err != nil {
		return nil, err
	}
	return s.invitations.PendingRequests(ctx, *me.FamilyID, time.Now())
}

// DecideJoinRequest 组长审批入组申请。同意时复用同一段入组事务。
func (s *Family) DecideJoinRequest(ctx context.Context, me *model.Member, invitationID uint64, accept bool) (*model.FamilyInvitation, error) {
	if err := requireLeader(me); err != nil {
		return nil, err
	}
	if invitationID == 0 {
		return nil, fmt.Errorf("%w：缺少 invitation_id", ErrInvalidInput)
	}
	inv, err := s.invitations.ByID(ctx, invitationID)
	if err != nil {
		return nil, err
	}
	// 不是本组的申请 → 按「不存在」处理，不泄露别组数据
	if inv.FamilyID != *me.FamilyID || inv.Direction != repo.DirectionRequest ||
		inv.Status != model.InvitationPending {
		return nil, repo.ErrNotFound
	}
	if !model.InvitationUsable(inv.Status, inv.ExpiresAt, time.Now()) {
		return nil, fmt.Errorf("%w：申请已过期", ErrInvitationExpired)
	}
	if err := s.invitations.DecideByFamily(ctx, *me.FamilyID, invitationID, accept); err != nil {
		return nil, err
	}
	s.notifier.JoinDecided(ctx, inv.FamilyID, inv.InviteeID, accept)
	return s.invitations.ByID(ctx, invitationID)
}

// CancelInvitation 组长取消本组的一条待确认邀请。
func (s *Family) CancelInvitation(ctx context.Context, me *model.Member, invitationID uint64) error {
	if err := requireLeader(me); err != nil {
		return err
	}
	if invitationID == 0 {
		return fmt.Errorf("%w：缺少 invitation_id", ErrInvalidInput)
	}
	return s.invitations.Cancel(ctx, *me.FamilyID, invitationID)
}
