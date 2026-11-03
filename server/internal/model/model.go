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
