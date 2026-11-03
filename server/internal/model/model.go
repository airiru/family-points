// Package model 存放与数据库表一一对应的结构体。
package model

import "time"

// 角色常量：前台只有组长与成员两种，后台是独立的 admins 表。
const (
	RoleLeader = "leader"
	RoleMember = "member"

	SubjectAdmin  = "admin"
	SubjectMember = "member"
)

// Member 对应 members 表。family_id 可空：注册后未入组时为空。
type Member struct {
	ID           uint64     `db:"id"`
	FamilyID     *uint64    `db:"family_id"`
	Name         string     `db:"name"`
	Score        int        `db:"score"`
	FrozenScore  int        `db:"frozen_score"`
	Hidden       bool       `db:"hidden"`
	Avatar       *string    `db:"avatar"`
	Username     string     `db:"username"`
	PassHash     string     `db:"pass_hash"`
	Role         string     `db:"role"`
	CreatedAt    time.Time  `db:"created_at"`
}

// InFamily 表示该成员已加入某个家庭组。
func (m *Member) InFamily() bool { return m.FamilyID != nil }

// Admin 对应 admins 表。管理员不属于任何家庭组。
type Admin struct {
	ID                 uint64    `db:"id"`
	Username           string    `db:"username"`
	PassHash           string    `db:"pass_hash"`
	MustChangePassword bool      `db:"must_change_password"`
	Disabled           bool      `db:"disabled"`
	CreatedAt          time.Time `db:"created_at"`
}

// Family 对应 families 表。
type Family struct {
	ID                 uint64     `db:"id"`
	Name               string     `db:"name"`
	InviteCode         string     `db:"invite_code"`
	InviteCodeExpiresAt *time.Time `db:"invite_code_expires_at"`
	CreatedAt          time.Time  `db:"created_at"`
}

// InviteCodeUsable 判断邀请码是否仍可用（7 天有效期，5.2.5）。
func (f *Family) InviteCodeUsable(now time.Time) bool {
	return f.InviteCodeExpiresAt != nil && now.Before(*f.InviteCodeExpiresAt)
}

// Item 是商城商品。
type Item struct {
	ID        uint64    `db:"id" json:"id"`
	FamilyID  uint64    `db:"family_id" json:"family_id"`
	Name      string    `db:"name" json:"name"`
	Cost      int       `db:"cost" json:"cost"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// Redemption 是兑换单（撤销状态载体；积分流水在 records）。
type Redemption struct {
	ID        uint64     `db:"id"`
	FamilyID  uint64     `db:"family_id"`
	MemberID  uint64     `db:"member_id"`
	ItemID    *uint64    `db:"item_id"`
	ItemName  string     `db:"item_name"`
	Cost      int        `db:"cost"`
	CreatedAt time.Time  `db:"created_at"`
	RevokedAt *time.Time `db:"revoked_at"`
}

// Transfer 是赠送单。
type Transfer struct {
	ID           uint64     `db:"id" json:"id"`
	FamilyID     uint64     `db:"family_id" json:"family_id"`
	FromMemberID uint64     `db:"from_member_id" json:"from_member_id"`
	ToMemberID   uint64     `db:"to_member_id" json:"to_member_id"`
	Amount       int        `db:"amount" json:"amount"`
	Note         *string    `db:"note" json:"note"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	RevokedAt    *time.Time `db:"revoked_at" json:"revoked_at"`
}

// Record 是一条积分流水。**所有积分变动都必须写这里**（用户 2026-11-03 明确）。
type Record struct {
	ID           uint64    `db:"id" json:"id"`
	FamilyID     uint64    `db:"family_id" json:"-"`
	MemberID     uint64    `db:"member_id" json:"member_id"`
	MemberName   string    `db:"member_name" json:"member_name"`
	Title        string    `db:"title" json:"title"`
	Points       int       `db:"points" json:"points"`
	OccurredAt   time.Time `db:"occurred_at" json:"occurred_at"`
	AddedAt      time.Time `db:"added_at" json:"added_at"`
	Source       string    `db:"source" json:"source"`
	TransferID   *uint64   `db:"transfer_id" json:"transfer_id"`
	RedemptionID *uint64   `db:"redemption_id" json:"redemption_id"`
}

// 流水来源常量，与 records.source 枚举一一对应。
const (
	SourceScreen   = "screen"
	SourceNight    = "night"
	SourceTask     = "task"
	SourceTransfer = "transfer"
	SourceRedeem   = "redeem"
)

// RefreshToken 对应 refresh_tokens 表（只存哈希）。
type RefreshToken struct {
	ID            uint64     `db:"id"`
	SubjectType   string     `db:"subject_type"`
	SubjectID     uint64     `db:"subject_id"`
	TokenHash     string     `db:"token_hash"`
	ExpiresAt     time.Time  `db:"expires_at"`
	RevokedAt     *time.Time `db:"revoked_at"`
	ReplacedByHash *string   `db:"replaced_by_hash"`
	CreatedAt     time.Time  `db:"created_at"`
}

// Valid 判断 token 是否仍可用（未吊销且未过期）。
func (t *RefreshToken) Valid(now time.Time) bool {
	return t.RevokedAt == nil && now.Before(t.ExpiresAt)
}
