// Package repo：家庭组相关的 SQL。
//
// 铁律（4.3 第 2 条）：前台查询必须带 family_id = ?，
// 且 family_id 只能来自 token 主体，绝不能来自请求参数。
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
)

// Family 封装家庭组 SQL。
type Family struct{ db *sqlx.DB }

// NewFamily 构造。
func NewFamily(db *sqlx.DB) *Family { return &Family{db: db} }

// ByID 按 id 取家庭组。查不到返回 ErrNotFound（handler 会转成 404）。
func (r *Family) ByID(ctx context.Context, id uint64) (*model.Family, error) {
	var f model.Family
	err := r.db.GetContext(ctx, &f,
		`SELECT id, name, invite_code, invite_code_expires_at, created_at
		   FROM families WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// Members 取某组的全部成员。
//
// includeHidden=false 时过滤掉被隐藏的成员（隐藏只影响展示，不影响数据，见 10.4）。
// **SQL 一定带 family_id = ?** —— 这是多租户隔离的唯一屏障。
func (r *Family) Members(ctx context.Context, familyID uint64, includeHidden bool) ([]model.Member, error) {
	q := `SELECT id, family_id, name, score, frozen_score, hidden, avatar,
	             username, pass_hash, role, created_at
	        FROM members WHERE family_id = ?`
	if !includeHidden {
		q += ` AND hidden = 0`
	}
	q += ` ORDER BY id`

	var list []model.Member
	if err := r.db.SelectContext(ctx, &list, q, familyID); err != nil {
		return nil, err
	}
	return list, nil
}
