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
//
// username 是登录凭证，注册后**永不可改**（全仓没有任何 UPDATE username 路径）；
// nickname 是显示名，可自由修改。records.member_name 存的是当时的昵称快照，不受改名影响。
type Member struct {
	ID          uint64    `db:"id" json:"id"`
	FamilyID    *uint64   `db:"family_id" json:"family_id"`
	Nickname    string    `db:"nickname" json:"nickname"`
	Score       int       `db:"score" json:"score"`
	FrozenScore int       `db:"frozen_score" json:"frozen_score"`
	Hidden      bool      `db:"hidden" json:"hidden"`
	Avatar      *string   `db:"avatar" json:"avatar"`
	Username    string    `db:"username" json:"username"`
	PassHash    string    `db:"pass_hash" json:"-"`
	Role        string    `db:"role" json:"role"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// InFamily 表示该成员已加入某个家庭组。
func (m *Member) InFamily() bool { return m.FamilyID != nil }

// Admin 对应 admins 表。管理员不属于任何家庭组。
// 与 Member 一样：username 永不可改，nickname 可改。
type Admin struct {
	ID                 uint64    `db:"id" json:"id"`
	Username           string    `db:"username" json:"username"`
	Nickname           string    `db:"nickname" json:"nickname"`
	PassHash           string    `db:"pass_hash" json:"-"`
	MustChangePassword bool      `db:"must_change_password" json:"must_change_password"`
	Disabled           bool      `db:"disabled" json:"disabled"`
	CreatedAt          time.Time `db:"created_at" json:"created_at"`
}

// Family 对应 families 表。
type Family struct {
	ID                  uint64     `db:"id" json:"id"`
	Name                string     `db:"name" json:"name"`
	InviteCode          string     `db:"invite_code" json:"invite_code"`
	InviteCodeExpiresAt *time.Time `db:"invite_code_expires_at" json:"invite_code_expires_at"`
	CreatedAt           time.Time  `db:"created_at" json:"created_at"`
}

// InviteCodeUsable 判断邀请码是否仍可用（7 天有效期，5.2.5）。
func (f *Family) InviteCodeUsable(now time.Time) bool {
	return f.InviteCodeExpiresAt != nil && now.Before(*f.InviteCodeExpiresAt)
}

// ---------- 待确认邀请（P3.1） ----------

const (
	InvitationPending   = "pending"
	InvitationAccepted  = "accepted"
	InvitationRejected  = "rejected"
	InvitationCancelled = "cancelled"
	InvitationExpired   = "expired"
)

// InvitationUsable 判断邀请是否仍可被处理：状态 pending 且未过期。
// 过期只在这里判定（惰性），不设定时任务 —— 与任务投票清扫同一取舍。
func InvitationUsable(status string, expiresAt, now time.Time) bool {
	return status == InvitationPending && now.Before(expiresAt)
}

// FamilyInvitation 对应 family_invitations 表。
// 后三个字段是 JOIN 出的展示辅助列，不落库。
type FamilyInvitation struct {
	ID              uint64     `db:"id" json:"id"`
	FamilyID        uint64     `db:"family_id" json:"family_id"`
	InviterID       uint64     `db:"inviter_id" json:"inviter_id"`
	InviteeID       uint64     `db:"invitee_id" json:"invitee_id"`
	Status          string     `db:"status" json:"status"`
	ExpiresAt       time.Time  `db:"expires_at" json:"expires_at"`
	DecidedAt       *time.Time `db:"decided_at" json:"decided_at"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
	FamilyName      string     `db:"family_name" json:"family_name"`
	InviterNickname string     `db:"inviter_nickname" json:"inviter_nickname"`
	InviteeNickname string     `db:"invitee_nickname" json:"invitee_nickname"`
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
	ID        uint64     `db:"id" json:"id"`
	FamilyID  uint64     `db:"family_id" json:"family_id"`
	MemberID  uint64     `db:"member_id" json:"member_id"`
	ItemID    *uint64    `db:"item_id" json:"item_id"`
	ItemName  string     `db:"item_name" json:"item_name"`
	Cost      int        `db:"cost" json:"cost"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	RevokedAt *time.Time `db:"revoked_at" json:"revoked_at"`
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
	StreakAward  *string   `db:"streak_award" json:"streak_award"`
}

// 流水来源常量，与 records.source 枚举一一对应。
const (
	SourceScreen   = "screen"
	SourceNight    = "night"
	SourceTask     = "task"
	SourceTransfer = "transfer"
	SourceRedeem   = "redeem"
)

// Rule 是睡觉规则（系统里唯一一种规则）。streaks 为连续奖励档位 JSON。
type Rule struct {
	ID        uint64    `db:"id" json:"id"`
	FamilyID  uint64    `db:"family_id" json:"family_id"`
	Name      string    `db:"name" json:"name"`
	Points    int       `db:"points" json:"points"`
	Streaks   *string   `db:"streaks" json:"streaks,omitempty"`
	MemberIDs *string   `db:"member_ids" json:"member_ids,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// StreakTier 是一个连续奖励档位（rules.streaks 数组的元素）。
type StreakTier struct {
	Every int    `json:"every"` // 连续天数阈值，≥1
	Bonus int    `json:"bonus"` // 奖励分值，≠0（可为负）
	Mode  string `json:"mode"`  // repeat | once
}

// ScreenRule 是每成员一条的屏幕时间规则。
type ScreenRule struct {
	ID             uint64  `db:"id" json:"id"`
	FamilyID       uint64  `db:"family_id" json:"family_id"`
	MemberID       uint64  `db:"member_id" json:"member_id"`
	ThresholdMin   int     `db:"threshold_min" json:"threshold_min"`
	BonusPoints    int     `db:"bonus_points" json:"bonus_points"`
	PenaltyPoints  int     `db:"penalty_points" json:"penalty_points"`
	Enabled        bool    `db:"enabled" json:"enabled"`
	NightStartHour *int    `db:"night_start_hour" json:"night_start_hour"`
	NightEndHour   *int    `db:"night_end_hour" json:"night_end_hour"`
	NightMaxMin    *int    `db:"night_max_min" json:"night_max_min"`
	NightRuleID    *uint64 `db:"night_rule_id" json:"night_rule_id"`
	// 未按时睡觉的扣分口径（2026-11-05）：超出部分按单位向上取整，每单位扣 N 分
	NightPenaltyUnitMin int `db:"night_penalty_unit_min" json:"night_penalty_unit_min"`
	NightPenaltyPoints  int `db:"night_penalty_points" json:"night_penalty_points"`
}

// NightEnabled 判断是否启用了夜间判定（三项都配置了才算启用）。
func (r *ScreenRule) NightEnabled() bool {
	return r.NightStartHour != nil && r.NightEndHour != nil && r.NightMaxMin != nil
}

// Device 是设备令牌（只存 SHA-256 哈希）。
// MemberDevice 是一台上报过屏幕时间的设备（2026-11-05 起无令牌）。
type MemberDevice struct {
	ID          uint64    `db:"id" json:"id"`
	FamilyID    uint64    `db:"family_id" json:"-"`
	MemberID    uint64    `db:"member_id" json:"member_id"`
	DeviceKey   string    `db:"device_key" json:"device_key"`
	DeviceModel string    `db:"device_model" json:"device_model"`
	FirstSeenAt time.Time `db:"first_seen_at" json:"first_seen_at"`
	LastSeenAt  time.Time `db:"last_seen_at" json:"last_seen_at"`
}

// DeviceDayReport 是「某成员某天某设备」的原始上报（家庭详情用）。
type DeviceDayReport struct {
	MemberID    uint64 `db:"member_id" json:"member_id"`
	DeviceKey   string `db:"device_key" json:"device_key"`
	DeviceModel string `db:"device_model" json:"device_model"`
	TotalMin    int    `db:"total_min" json:"total_min"`
	NightMin    *int   `db:"night_min" json:"night_min"`
}

// DeviceReport 是后台「设备原始上报」列表的一行（按天 × 设备，只读展示）。
// 与 DeviceDayReport 的区别：它带日期与写入时间，供后台跨日期翻查，不参与任何结算。
type DeviceReport struct {
	ID          uint64     `db:"id" json:"id"`
	FamilyID    uint64     `db:"family_id" json:"-"`
	MemberID    uint64     `db:"member_id" json:"member_id"`
	DeviceKey   string     `db:"device_key" json:"device_key"`
	DeviceModel string     `db:"device_model" json:"device_model"`
	ReportDate  time.Time  `db:"report_date" json:"report_date"`
	TotalMin    int        `db:"total_min" json:"total_min"`
	NightMin    *int       `db:"night_min" json:"night_min"`
	ReportedAt  time.Time  `db:"reported_at" json:"reported_at"`
	UpdatedAt   *time.Time `db:"updated_at" json:"updated_at"`
}

// ScreenReport 是一天的屏幕时间结算行（幂等载体）。
type ScreenReport struct {
	ID            uint64     `db:"id" json:"id"`
	FamilyID      uint64     `db:"family_id" json:"-"`
	MemberID      uint64     `db:"member_id" json:"member_id"`
	ReportDate    time.Time  `db:"report_date" json:"report_date"`
	TotalMin      int        `db:"total_min" json:"total_min"`
	NightMin      *int       `db:"night_min" json:"night_min"`
	NightOK       *bool      `db:"night_ok" json:"night_ok"`
	Points        int        `db:"points" json:"points"`
	RecordID      *uint64    `db:"record_id" json:"record_id"`
	NightRecordID *uint64    `db:"night_record_id" json:"night_record_id"`
	Source        string     `db:"source" json:"source"`
	Revision      int        `db:"revision" json:"revision"`
	ReportedAt    time.Time  `db:"reported_at" json:"reported_at"`
	UpdatedAt     *time.Time `db:"updated_at" json:"updated_at"`
}

// Task 是悬赏任务。积分从发布者冻结后转给完成者（转移，不是生成）。
type Task struct {
	ID           uint64     `db:"id" json:"id"`
	FamilyID     uint64     `db:"family_id" json:"-"`
	Title        string     `db:"title" json:"title"`
	Points       int        `db:"points" json:"points"`
	PublisherID  uint64     `db:"publisher_id" json:"publisher_id"`
	EscrowPoints int        `db:"escrow_points" json:"escrow_points"`
	AssigneeID   *uint64    `db:"assignee_id" json:"assignee_id"`
	CompletedBy  *uint64    `db:"completed_by" json:"completed_by"`
	DueAt        *time.Time `db:"due_at" json:"due_at"`
	RepeatRule   *string    `db:"repeat_rule" json:"repeat_rule"`
	Status       string     `db:"status" json:"status"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
}

// 任务状态
const (
	TaskOpen   = "open"
	TaskTaken  = "taken"
	TaskClosed = "closed"
)

// TaskSubmission 是一次完成提交。
type TaskSubmission struct {
	ID           uint64     `db:"id" json:"id"`
	FamilyID     uint64     `db:"family_id" json:"-"`
	TaskID       uint64     `db:"task_id" json:"task_id"`
	MemberID     uint64     `db:"member_id" json:"member_id"`
	Note         string     `db:"note" json:"note"`
	Status       string     `db:"status" json:"status"`
	VoteDeadline *time.Time `db:"vote_deadline" json:"vote_deadline"`
	ReviewNote   *string    `db:"review_note" json:"review_note"`
	ReviewedAt   *time.Time `db:"reviewed_at" json:"reviewed_at"`
	RecordID     *uint64    `db:"record_id" json:"record_id"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
}

// 提交状态
const (
	SubmissionSubmitted = "submitted"
	SubmissionApproved  = "approved"
	SubmissionRejected  = "rejected"
	SubmissionExpired   = "expired"
)

// TaskVote 是一张审核票（一人一票，不可改）。
type TaskVote struct {
	ID           uint64    `db:"id" json:"id"`
	FamilyID     uint64    `db:"family_id" json:"-"`
	SubmissionID uint64    `db:"submission_id" json:"submission_id"`
	VoterID      uint64    `db:"voter_id" json:"voter_id"`
	Vote         string    `db:"vote" json:"vote"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

// AuditLog 是审计日志（append-only）。
type AuditLog struct {
	ID           uint64    `db:"id" json:"id"`
	FamilyID     *uint64   `db:"family_id" json:"family_id"`
	OperatorID   *uint64   `db:"operator_id" json:"operator_id"`
	OperatorType string    `db:"operator_type" json:"operator_type"`
	Action       string    `db:"action" json:"action"`
	TargetType   *string   `db:"target_type" json:"target_type"`
	TargetID     *uint64   `db:"target_id" json:"target_id"`
	BeforeJSON   *string   `db:"before_json" json:"before"`
	AfterJSON    *string   `db:"after_json" json:"after"`
	IP           *string   `db:"ip" json:"ip"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

// Notification 是站内通知。
type Notification struct {
	ID        uint64     `db:"id" json:"id"`
	FamilyID  uint64     `db:"family_id" json:"-"`
	MemberID  uint64     `db:"member_id" json:"member_id"`
	Type      string     `db:"type" json:"type"`
	Title     string     `db:"title" json:"title"`
	Body      *string    `db:"body" json:"body"`
	RefType   *string    `db:"ref_type" json:"ref_type"`
	RefID     *uint64    `db:"ref_id" json:"ref_id"`
	ReadAt    *time.Time `db:"read_at" json:"read_at"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
}

// RefreshToken 对应 refresh_tokens 表（只存哈希）。
type RefreshToken struct {
	ID             uint64     `db:"id" json:"id"`
	SubjectType    string     `db:"subject_type" json:"subject_type"`
	SubjectID      uint64     `db:"subject_id" json:"subject_id"`
	TokenHash      string     `db:"token_hash" json:"-"`
	ExpiresAt      time.Time  `db:"expires_at" json:"expires_at"`
	RevokedAt      *time.Time `db:"revoked_at" json:"revoked_at"`
	ReplacedByHash *string    `db:"replaced_by_hash" json:"replaced_by_hash"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
}

// Valid 判断 token 是否仍可用（未吊销且未过期）。
func (t *RefreshToken) Valid(now time.Time) bool {
	return t.RevokedAt == nil && now.Before(t.ExpiresAt)
}

// Coupon 对应 coupons 表：兑换后发出的商品券（docs 9.6）。
//
// 状态机：unused --使用--> pending --通过--> redeemed
//
//	↑                │
//	└──驳回 / 超时────┘
type Coupon struct {
	ID           uint64     `db:"id" json:"id"`
	FamilyID     uint64     `db:"family_id" json:"-"`
	MemberID     uint64     `db:"member_id" json:"member_id"`
	RedemptionID uint64     `db:"redemption_id" json:"redemption_id"`
	ItemName     string     `db:"item_name" json:"item_name"`
	Cost         int        `db:"cost" json:"cost"`
	Status       string     `db:"status" json:"status"`
	RejectCount  int        `db:"reject_count" json:"reject_count"`
	VoteDeadline *time.Time `db:"vote_deadline" json:"vote_deadline"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	SettledAt    *time.Time `db:"settled_at" json:"settled_at"`
}

// 券状态常量。
const (
	CouponUnused   = "unused"
	CouponPending  = "pending"
	CouponRedeemed = "redeemed"
)

// CouponVote 对应 coupon_votes 表：核销投票，一人一票。
type CouponVote struct {
	ID        uint64    `db:"id" json:"id"`
	FamilyID  uint64    `db:"family_id" json:"-"`
	CouponID  uint64    `db:"coupon_id" json:"coupon_id"`
	VoterID   uint64    `db:"voter_id" json:"voter_id"`
	Vote      string    `db:"vote" json:"vote"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
