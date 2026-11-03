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
	"familypoints/server/internal/repo"
)

// 家庭组相关的业务错误。handler 统一映射成状态码。
var (
	ErrNotLeader        = errors.New("只有组长可以执行该操作")
	ErrAlreadyInFamily  = errors.New("你已经加入了家庭组")
	ErrInviteCodeInvalid = errors.New("邀请码无效或已过期")
	ErrBanned           = errors.New("你已被该家庭组禁止加入")
	ErrLeaderMustTransfer = errors.New("组长不能直接退出，请先转让组长")
	ErrConfirmRequired  = errors.New("该操作不可撤销，需要二次确认")
	ErrCannotTargetSelf = errors.New("不能对自己执行该操作")
)

// Family 是家庭组服务。
type Family struct {
	families *repo.Family
}

// NewFamily 构造。
func NewFamily(r *repo.Family) *Family { return &Family{families: r} }

// MemberBrief 是对外暴露的成员信息：**绝不包含 pass_hash**。
type MemberBrief struct {
	ID          uint64  `json:"id"`
	Name        string  `json:"name"`
	Role        string  `json:"role"`
	Avatar      *string `json:"avatar"`
	Score       int     `json:"score"`
	FrozenScore int     `json:"frozen_score"`
	Hidden      bool    `json:"hidden,omitempty"`
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
			ID: m.ID, Name: m.Name, Role: m.Role, Avatar: m.Avatar,
			Score: m.Score, FrozenScore: m.FrozenScore, Hidden: m.Hidden,
		})
	}
	return out, nil
}

// InviteCode 返回本组邀请码及其有效期（组长可见）。
func (s *Family) InviteCode(ctx context.Context, me *model.Member) (code string, expiresAt *time.Time, err error) {
	if err := requireLeader(me); err != nil {
		return "", nil, err
	}
	f, err := s.families.ByID(ctx, *me.FamilyID)
	if err != nil {
		return "", nil, err
	}
	return f.InviteCode, f.InviteCodeExpiresAt, nil
}

// ResetInviteCode 重置邀请码（旧码立即失效，5.2.5）。
func (s *Family) ResetInviteCode(ctx context.Context, me *model.Member) (string, time.Time, error) {
	if err := requireLeader(me); err != nil {
		return "", time.Time{}, err
	}
	code, err := s.families.NewInviteCode(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(7 * 24 * time.Hour)
	if err := s.families.ResetInviteCode(ctx, *me.FamilyID, code, expires); err != nil {
		return "", time.Time{}, err
	}
	return code, expires, nil
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
	if !f.InviteCodeUsable(time.Now()) {
		return nil, ErrInviteCodeInvalid
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

// RenameMember 成员改名（本人或组长可改）。records.member_name 是快照，历史不受影响。
func (s *Family) RenameMember(ctx context.Context, me *model.Member, targetID uint64, name string) error {
	if targetID != me.ID && me.Role != model.RoleLeader {
		return ErrNotLeader
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 32 {
		return fmt.Errorf("%w：昵称需 1–32 个字符", ErrInvalidInput)
	}
	if _, err := s.families.MemberInFamily(ctx, *me.FamilyID, targetID); err != nil {
		return err
	}
	return s.families.RenameMember(ctx, *me.FamilyID, targetID, name)
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
