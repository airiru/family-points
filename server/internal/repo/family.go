// Package repo：家庭组相关的 SQL。
//
// 铁律（4.3 第 2 条）：前台查询必须带 family_id = ?，
// 且 family_id 只能来自 token 主体，绝不能来自请求参数。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

// MemberInFamily 取组内某成员；**必须同时匹配 family_id**，否则就是跨组读取。
func (r *Family) MemberInFamily(ctx context.Context, familyID, memberID uint64) (*model.Member, error) {
	var m model.Member
	err := r.db.GetContext(ctx, &m,
		`SELECT id, family_id, name, score, frozen_score, hidden, avatar,
		        username, pass_hash, role, created_at
		   FROM members WHERE id = ? AND family_id = ?`, memberID, familyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ByInviteCode 按邀请码找组（加入家庭组用）。
func (r *Family) ByInviteCode(ctx context.Context, code string) (*model.Family, error) {
	var f model.Family
	err := r.db.GetContext(ctx, &f,
		`SELECT id, name, invite_code, invite_code_expires_at, created_at
		   FROM families WHERE invite_code = ?`, code)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// UpdateName 改家庭组名。
func (r *Family) UpdateName(ctx context.Context, familyID uint64, name string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE families SET name = ? WHERE id = ?`, name, familyID)
	return err
}

// ResetInviteCode 重置邀请码并刷新有效期。
func (r *Family) ResetInviteCode(ctx context.Context, familyID uint64, code string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE families SET invite_code = ?, invite_code_expires_at = ? WHERE id = ?`,
		code, expiresAt, familyID)
	return err
}

// InviteCodeTaken 判断邀请码是否已被别的组占用。
func (r *Family) InviteCodeTaken(ctx context.Context, code string) (bool, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM families WHERE invite_code = ?`, code)
	return n > 0, err
}

// NewInviteCode 生成一个未被占用的 8 位邀请码（去掉 I/O/0/1 避免口头传错）。
func (r *Family) NewInviteCode(ctx context.Context) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for attempt := 0; attempt < 10; attempt++ {
		code, err := randomCode(alphabet, 8)
		if err != nil {
			return "", err
		}
		taken, err := r.InviteCodeTaken(ctx, code)
		if err != nil {
			return "", err
		}
		if !taken {
			return code, nil
		}
	}
	return "", errors.New("邀请码生成失败：连续 10 次冲突")
}

// IsBanned 判断某成员是否被某组禁入（5.3.4）。
func (r *Family) IsBanned(ctx context.Context, familyID, memberID uint64) (bool, error) {
	var n int
	err := r.db.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM family_bans WHERE family_id = ? AND member_id = ?`, familyID, memberID)
	return n > 0, err
}

// Join 把成员加入家庭组（调用方需先校验：未入组、未被禁入、码有效）。
func (r *Family) Join(ctx context.Context, familyID, memberID uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE members SET family_id = ?, role = ? WHERE id = ? AND family_id IS NULL`,
		familyID, model.RoleMember, memberID)
	return err
}

// Leave 退出 / 踢出：积分清零 + family_id 置空 + 角色回落 + 写禁入名单。
//
// **同一事务**：任何一步失败都不能留下「积分清了但还在组里」这种中间态。
// reason 只能是 kicked（踢出）或 left（自行退出）。
func (r *Family) Leave(ctx context.Context, familyID, memberID uint64, reason string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 清零 + 离组（必须带 family_id，防止跨组误改）
	res, err := tx.ExecContext(ctx,
		`UPDATE members SET score = 0, frozen_score = 0, family_id = NULL, role = ?
		  WHERE id = ? AND family_id = ?`,
		model.RoleMember, memberID, familyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	// 2. 写入禁入名单（同一人重复退出不重复写）
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO family_bans (family_id, member_id, reason) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE reason = VALUES(reason)`,
		familyID, memberID, reason); err != nil {
		return err
	}

	return tx.Commit()
}

// SetHidden 隐藏 / 显示成员（只影响展示）。
func (r *Family) SetHidden(ctx context.Context, familyID, memberID uint64, hidden bool) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE members SET hidden = ? WHERE id = ? AND family_id = ?`,
		hidden, memberID, familyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TransferLeader 组长转让。
//
// **顺序不可交换**：先把旧组长降为 member，再升新组长。
// members 表有 UNIQUE(leader_family_id)（每组仅一名 leader），
// 反过来先升新组长会直接撞唯一键。
func (r *Family) TransferLeader(ctx context.Context, familyID, fromID, toID uint64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 旧组长降级（带 family_id 与 role 双条件，防误改）
	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET role = ? WHERE id = ? AND family_id = ? AND role = ?`,
		model.RoleMember, fromID, familyID, model.RoleLeader); err != nil {
		return err
	}

	// 2. 新组长升级
	res, err := tx.ExecContext(ctx,
		`UPDATE members SET role = ? WHERE id = ? AND family_id = ?`,
		model.RoleLeader, toID, familyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	return tx.Commit()
}

// RenameMember 成员改名（records.member_name 是快照，历史不受影响）。
func (r *Family) RenameMember(ctx context.Context, familyID, memberID uint64, name string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE members SET name = ? WHERE id = ? AND family_id = ?`, name, memberID, familyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
