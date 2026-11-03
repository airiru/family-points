// Package repo：审计日志与站内通知。
package repo

import (
	"context"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
)

// Audit 封装审计日志 SQL（只有 INSERT 与 SELECT，没有 UPDATE / DELETE）。
type Audit struct{ db *sqlx.DB }

// NewAudit 构造。
func NewAudit(db *sqlx.DB) *Audit { return &Audit{db: db} }

// AuditRow 是一条审计记录的入库形态。
type AuditRow struct {
	FamilyID     *uint64
	OperatorID   *uint64
	OperatorType string
	Action       string
	TargetType   string
	TargetID     *uint64
	BeforeJSON   *string
	AfterJSON    *string
	IP           string
}

// Insert 追加一条审计。
func (r *Audit) Insert(ctx context.Context, a AuditRow) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_logs
		   (family_id, operator_id, operator_type, action, target_type, target_id,
		    before_json, after_json, ip)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.FamilyID, a.OperatorID, a.OperatorType, a.Action,
		nullableStr(a.TargetType), a.TargetID, a.BeforeJSON, a.AfterJSON, nullableStr(a.IP))
	return err
}

// AuditFilter 是后台查询条件。
type AuditFilter struct {
	FamilyID uint64
	Action   string
	Operator uint64
	From     time.Time
	To       time.Time
	Limit    int
	Offset   int
}

// List 分页查询审计（新→旧）。
func (r *Audit) List(ctx context.Context, f AuditFilter) ([]model.AuditLog, int, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if f.FamilyID != 0 {
		where = append(where, "family_id = ?")
		args = append(args, f.FamilyID)
	}
	if f.Action != "" {
		where = append(where, "action = ?")
		args = append(args, f.Action)
	}
	if f.Operator != 0 {
		where = append(where, "operator_id = ?")
		args = append(args, f.Operator)
	}
	if !f.From.IsZero() {
		where = append(where, "created_at >= ?")
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		where = append(where, "created_at < ?")
		args = append(args, f.To)
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM audit_logs WHERE `+cond, args...); err != nil {
		return nil, 0, err
	}
	list := []model.AuditLog{}
	if err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, operator_id, operator_type, action, target_type, target_id,
		        before_json, after_json, ip, created_at
		   FROM audit_logs WHERE `+cond+`
		  ORDER BY id DESC LIMIT ? OFFSET ?`, append(args, f.Limit, f.Offset)...); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// Notification 封装站内通知 SQL。
type Notification struct{ db *sqlx.DB }

// NewNotification 构造。
func NewNotification(db *sqlx.DB) *Notification { return &Notification{db: db} }

// Notify 写一条通知（旁路：失败只记日志，不阻断业务）。
func (r *Notification) Notify(ctx context.Context, familyID, memberID uint64, typ, title, body, refType string, refID *uint64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notifications (family_id, member_id, type, title, body, ref_type, ref_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		familyID, memberID, typ, title, nullableStr(body), nullableStr(refType), refID)
	return err
}

// List 列出某成员的通知（新→旧）。
func (r *Notification) List(ctx context.Context, familyID, memberID uint64, onlyUnread bool, limit int) ([]model.Notification, error) {
	q := `SELECT id, family_id, member_id, type, title, body, ref_type, ref_id, read_at, created_at
	        FROM notifications WHERE family_id = ? AND member_id = ?`
	if onlyUnread {
		q += ` AND read_at IS NULL`
	}
	q += ` ORDER BY id DESC LIMIT ?`
	list := []model.Notification{}
	err := r.db.SelectContext(ctx, &list, q, familyID, memberID, limit)
	return list, err
}

// MarkRead 标记某条已读（只能标记自己的）。
func (r *Notification) MarkRead(ctx context.Context, familyID, memberID, id uint64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = NOW(3)
		  WHERE id = ? AND family_id = ? AND member_id = ? AND read_at IS NULL`,
		id, familyID, memberID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// 已读或不属于自己：都按「不存在」处理，不泄露他人通知
		return ErrNotFound
	}
	return nil
}

// MarkAllRead 全部标记已读。
func (r *Notification) MarkAllRead(ctx context.Context, familyID, memberID uint64) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = NOW(3)
		  WHERE family_id = ? AND member_id = ? AND read_at IS NULL`, familyID, memberID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
