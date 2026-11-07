// Package repo：用户反馈（2026-11-07）的 SQL。
package repo

import (
	"context"

	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
)

// Feedback 封装 feedbacks。
type Feedback struct{ db *sqlx.DB }

// NewFeedback 构造。
func NewFeedback(db *sqlx.DB) *Feedback { return &Feedback{db: db} }

// Create 提交一条反馈（content 由 service 层校验长度）。
func (r *Feedback) Create(ctx context.Context, memberID uint64, username, content string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO feedbacks (member_id, username, content) VALUES (?, ?, ?)`,
		memberID, username, content)
	return err
}

// List 后台反馈列表（handled 0=未处理 1=已处理 2=全部；新的在前）。
func (r *Feedback) List(ctx context.Context, handled int, limit int) ([]model.Feedback, error) {
	list := []model.Feedback{}
	q := `SELECT id, member_id, username, content, handled, created_at
		    FROM feedbacks`
	args := []any{}
	if handled != 2 {
		q += ` WHERE handled = ?`
		args = append(args, handled)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	err := r.db.SelectContext(ctx, &list, q, args...)
	return list, err
}

// MarkHandled 标记已处理。受影响行数 0 = 不存在或已处理。
func (r *Feedback) MarkHandled(ctx context.Context, id uint64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE feedbacks SET handled = 1 WHERE id = ? AND handled = 0`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
