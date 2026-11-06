// Package repo：待确认邀请（P3.1）的 SQL。
//
// 状态流转只在这里落库：pending → accepted / rejected / cancelled。
// expired 不落库，由读路径用服务端传入的 now 判断（惰性清扫，与任务投票同一取舍）。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
)

// ErrInvitationPending 同一家庭组对同一人已有待确认邀请。
var ErrInvitationPending = errors.New("该用户已有待确认的邀请")

// Invitation 封装 family_invitations。
type Invitation struct{ db *sqlx.DB }

// NewInvitation 构造。
func NewInvitation(db *sqlx.DB) *Invitation { return &Invitation{db: db} }

const invitationCols = `i.id, i.family_id, i.inviter_id, i.invitee_id, i.direction, i.status,
       i.expires_at, i.decided_at, i.created_at`

// 两个方向
const (
	DirectionInvite  = "invite"  // 组长邀请成员
	DirectionRequest = "request" // 成员凭家庭码申请加入
)

// Create 发一条待确认邀请。
// uk_pending_invitee（family_id + pending 生成列）保证同组同一人最多一条 pending；
// 撞唯一键转成业务错误而不是 500。
//
// **必须先落地已过期的 pending**：过期按设计只在读路径判定、不设定时任务，
// 但生成列只看 status，一条过期的 pending 会永远占着唯一键，
// 导致组长再也无法邀请这个人（列表里看不到过期条目，也就拿不到 ID 去取消）。
// 这里在同事务里把本组对该人已过期的 pending 落为 expired，再插入新行。
// clock 与 service 写入 expires_at 用的是同一个 Go 时钟，避免 DB 时区差异。
func (r *Invitation) Create(ctx context.Context, familyID, inviterID, inviteeID uint64, direction string, expiresAt time.Time) (uint64, error) {
	now := time.Now()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE family_invitations SET status = 'expired', decided_at = ?
		  WHERE family_id = ? AND invitee_id = ? AND status = 'pending' AND expires_at <= ?`,
		now, familyID, inviteeID, now); err != nil {
		return 0, err
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO family_invitations (family_id, inviter_id, invitee_id, direction, status, expires_at)
		 VALUES (?, ?, ?, ?, 'pending', ?)`, familyID, inviterID, inviteeID, direction, expiresAt)
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return 0, ErrInvitationPending
		}
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return uint64(id), nil
}

// ByID 按 id 取邀请。与列表用同一组展示列，保证「发起 / 接受 / 拒绝」
// 的返回体和列表里的行形状完全一致，客户端不需要两套 DTO。
func (r *Invitation) ByID(ctx context.Context, id uint64) (*model.FamilyInvitation, error) {
	var inv model.FamilyInvitation
	err := r.db.GetContext(ctx, &inv,
		`SELECT `+invitationCols+`,
		        f.name AS family_name, m1.nickname AS inviter_nickname,
		        m2.nickname AS invitee_nickname
		   FROM family_invitations i
		   JOIN families f  ON f.id  = i.family_id
		   JOIN members  m1 ON m1.id = i.inviter_id
		   JOIN members  m2 ON m2.id = i.invitee_id
		  WHERE i.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// ForInvitee 我收到的邀请：只列仍有效的 pending 与历史处理记录。
// 过期的 pending 不显示（不落库，纯读路径判定）；now 由服务端传，避免 DB 时区差异。
func (r *Invitation) ForInvitee(ctx context.Context, inviteeID uint64, now time.Time) ([]model.FamilyInvitation, error) {
	list := []model.FamilyInvitation{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT `+invitationCols+`,
		        f.name AS family_name, m1.nickname AS inviter_nickname,
		        m2.nickname AS invitee_nickname
		   FROM family_invitations i
		   JOIN families f  ON f.id  = i.family_id
		   JOIN members  m1 ON m1.id = i.inviter_id
		   JOIN members  m2 ON m2.id = i.invitee_id
		  WHERE i.invitee_id = ? AND i.direction = 'invite'
		    AND (i.status <> 'pending' OR i.expires_at > ?)
		  ORDER BY i.id DESC LIMIT 100`, inviteeID, now)
	return list, err
}

// ForFamily 本组发出的邀请（组长看），带被邀请人昵称。
func (r *Invitation) ForFamily(ctx context.Context, familyID uint64, now time.Time) ([]model.FamilyInvitation, error) {
	list := []model.FamilyInvitation{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT `+invitationCols+`,
		        f.name AS family_name, m1.nickname AS inviter_nickname,
		        m2.nickname AS invitee_nickname
		   FROM family_invitations i
		   JOIN families f  ON f.id  = i.family_id
		   JOIN members  m1 ON m1.id = i.inviter_id
		   JOIN members  m2 ON m2.id = i.invitee_id
		  WHERE i.family_id = ? AND i.direction = 'invite'
		    AND (i.status <> 'pending' OR i.expires_at > ?)
		  ORDER BY i.id DESC LIMIT 100`, familyID, now)
	return list, err
}

// PendingRequests 本组待审批的入组申请（组长看）。
func (r *Invitation) PendingRequests(ctx context.Context, familyID uint64, now time.Time) ([]model.FamilyInvitation, error) {
	list := []model.FamilyInvitation{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT `+invitationCols+`,
		        f.name AS family_name, m1.nickname AS inviter_nickname,
		        m2.nickname AS invitee_nickname
		   FROM family_invitations i
		   JOIN families f  ON f.id  = i.family_id
		   JOIN members  m1 ON m1.id = i.inviter_id
		   JOIN members  m2 ON m2.id = i.invitee_id
		  WHERE i.family_id = ? AND i.direction = 'request'
		    AND i.status = 'pending' AND i.expires_at > ?
		  ORDER BY i.id DESC LIMIT 100`, familyID, now)
	return list, err
}

// MyRequests 我发出的入组申请（未入组时看「等待审批」）。
func (r *Invitation) MyRequests(ctx context.Context, inviteeID uint64, now time.Time) ([]model.FamilyInvitation, error) {
	list := []model.FamilyInvitation{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT `+invitationCols+`,
		        f.name AS family_name, m1.nickname AS inviter_nickname,
		        m2.nickname AS invitee_nickname
		   FROM family_invitations i
		   JOIN families f  ON f.id  = i.family_id
		   JOIN members  m1 ON m1.id = i.inviter_id
		   JOIN members  m2 ON m2.id = i.invitee_id
		  WHERE i.invitee_id = ? AND i.direction = 'request'
		    AND (i.status <> 'pending' OR i.expires_at > ?)
		  ORDER BY i.id DESC LIMIT 20`, inviteeID, now)
	return list, err
}

// Decide 原子处理邀请：状态流转 +（接受时）入组 + 取消其它 pending。
//
// 调用方（service）已校验归属 / 状态 / 过期 / 未入组 / 未禁入；这里再用
// FOR UPDATE + 条件 UPDATE 兜住并发，失败一律按「资源不存在」返回 404。
func (r *Invitation) Decide(ctx context.Context, invitationID, inviteeID uint64, accept bool) error {
	return r.decide(ctx, invitationID, &inviteeID, nil, accept)
}

// DecideByFamily 组长审批入组申请：归属用 family_id 兜住，而不是 invitee_id。
func (r *Invitation) DecideByFamily(ctx context.Context, familyID, invitationID uint64, accept bool) error {
	return r.decide(ctx, invitationID, nil, &familyID, accept)
}

func (r *Invitation) decide(ctx context.Context, invitationID uint64, inviteeID, familyID *uint64, accept bool) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var inv model.FamilyInvitation
	// 两个方向用不同的归属列兜住；注意先判空再解引用，否则 nil 指针直接 panic
	var cond string
	var arg uint64
	if familyID != nil {
		cond, arg = "family_id = ?", *familyID
	} else {
		cond, arg = "invitee_id = ?", *inviteeID
	}
	if err := tx.GetContext(ctx, &inv,
		`SELECT id, family_id, inviter_id, invitee_id, direction, status,
		        expires_at, decided_at, created_at
		   FROM family_invitations WHERE id = ? AND `+cond+` FOR UPDATE`,
		invitationID, arg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if inv.Status != model.InvitationPending {
		return ErrNotFound
	}

	status := model.InvitationRejected
	if accept {
		status = model.InvitationAccepted
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE family_invitations SET status = ?, decided_at = NOW(3)
		  WHERE id = ? AND status = 'pending'`, status, invitationID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	if accept {
		// 与邀请码 / 后台直邀完全同一段入组 SQL（含补屏幕时间规则）。
		// 入组的永远是 invitee_id：邀请方向是「被邀请人」，申请方向是「申请人」。
		if err := joinTx(ctx, tx, inv.FamilyID, inv.InviteeID); err != nil {
			return err
		}
		// 接受后其它组发来的 pending 一并作废，避免还能再点
		if _, err := tx.ExecContext(ctx,
			`UPDATE family_invitations SET status = 'cancelled', decided_at = NOW(3)
			  WHERE invitee_id = ? AND status = 'pending' AND id <> ?`,
			inv.InviteeID, invitationID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Cancel 组长取消本组的一条待确认邀请。
func (r *Invitation) Cancel(ctx context.Context, familyID, invitationID uint64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE family_invitations SET status = 'cancelled', decided_at = NOW(3)
		  WHERE id = ? AND family_id = ? AND status = 'pending'`, invitationID, familyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
