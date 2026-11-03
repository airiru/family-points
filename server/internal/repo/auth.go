// Package repo 是 SQL 访问层：只做取数/写数，不含业务判断。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
)

// ErrNotFound 统一表示「查不到」，由 service 层决定对外是 401 还是 404。
var ErrNotFound = errors.New("记录不存在")

// Auth 封装认证相关的 SQL。
type Auth struct{ db *sqlx.DB }

// NewAuth 构造。
func NewAuth(db *sqlx.DB) *Auth { return &Auth{db: db} }

// ---------- 成员 ----------

// MemberByUsername 按用户名取成员（登录用）。
func (r *Auth) MemberByUsername(ctx context.Context, username string) (*model.Member, error) {
	var m model.Member
	err := r.db.GetContext(ctx, &m,
		`SELECT id, family_id, name, score, frozen_score, hidden, avatar,
		        username, pass_hash, role, created_at
		   FROM members WHERE username = ?`, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// MemberByID 按 id 取成员（鉴权中间件每次请求都要读，角色以库为准）。
func (r *Auth) MemberByID(ctx context.Context, id uint64) (*model.Member, error) {
	var m model.Member
	err := r.db.GetContext(ctx, &m,
		`SELECT id, family_id, name, score, frozen_score, hidden, avatar,
		        username, pass_hash, role, created_at
		   FROM members WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// CreateMember 创建成员；familyName 非空时同事务创建家庭组并把该成员设为组长。
//
// 为什么同事务：只建成账号没建成组、或建成组却没设组长，都会留下不可用的中间态。
func (r *Auth) CreateMember(ctx context.Context, username, passHash, name string, familyName *string) (*model.Member, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var familyID *uint64
	if familyName != nil {
		code, err := uniqueInviteCode(ctx, tx)
		if err != nil {
			return nil, err
		}
		// 邀请码 7 天有效期（5.2.5）
		res, err := tx.ExecContext(ctx,
			`INSERT INTO families (name, invite_code, invite_code_expires_at)
			 VALUES (?, ?, ?)`, *familyName, code, time.Now().Add(7*24*time.Hour))
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		fid := uint64(id)
		familyID = &fid
	}

	role := model.RoleMember
	if familyID != nil {
		role = model.RoleLeader
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO members (family_id, name, username, pass_hash, role)
		 VALUES (?, ?, ?, ?, ?)`, familyID, name, username, passHash, role)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	// 建组时写入一份默认配置（决策 25：不存在全局默认值，但新组必须有初始值落库）
	if familyID != nil {
		if err := seedDefaultScreenConfig(ctx, tx, *familyID, uint64(id)); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &model.Member{
		ID: uint64(id), FamilyID: familyID, Name: name,
		Username: username, PassHash: passHash, Role: role,
	}, nil
}

// UpdateMemberPassword 改密码。
func (r *Auth) UpdateMemberPassword(ctx context.Context, id uint64, passHash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE members SET pass_hash = ? WHERE id = ?`, passHash, id)
	return err
}

// ---------- 管理员 ----------

// AdminByUsername 按用户名取管理员。
func (r *Auth) AdminByUsername(ctx context.Context, username string) (*model.Admin, error) {
	var a model.Admin
	err := r.db.GetContext(ctx, &a,
		`SELECT id, username, pass_hash, must_change_password, disabled, created_at
		   FROM admins WHERE username = ?`, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AdminByID 按 id 取管理员。
func (r *Auth) AdminByID(ctx context.Context, id uint64) (*model.Admin, error) {
	var a model.Admin
	err := r.db.GetContext(ctx, &a,
		`SELECT id, username, pass_hash, must_change_password, disabled, created_at
		   FROM admins WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// CountAdmins 统计管理员数量（初始化默认账号时判断「是否已有管理员」）。
func (r *Auth) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM admins`)
	return n, err
}

// CreateAdmin 创建管理员（初始化默认账号用）。
func (r *Auth) CreateAdmin(ctx context.Context, username, passHash string, mustChange bool) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO admins (username, pass_hash, must_change_password) VALUES (?, ?, ?)`,
		username, passHash, mustChange)
	return err
}

// ListAdmins 列出全部管理员（不含 pass_hash）。
func (r *Auth) ListAdmins(ctx context.Context) ([]model.Admin, error) {
	list := []model.Admin{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, username, '' AS pass_hash, must_change_password, disabled, created_at
		   FROM admins ORDER BY id`)
	return list, err
}

// SetAdminDisabled 停用 / 启用管理员。
func (r *Auth) SetAdminDisabled(ctx context.Context, id uint64, disabled bool) error {
	res, err := r.db.ExecContext(ctx, `UPDATE admins SET disabled = ? WHERE id = ?`, disabled, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateAdminPassword 改密码，并清掉「必须改密」标记。
func (r *Auth) UpdateAdminPassword(ctx context.Context, id uint64, passHash string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE admins SET pass_hash = ?, must_change_password = 0 WHERE id = ?`, passHash, id)
	return err
}

// ---------- Refresh Token ----------

// InsertRefresh 保存 refresh token 哈希。
func (r *Auth) InsertRefresh(ctx context.Context, subjectType string, subjectID uint64, hash string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (subject_type, subject_id, token_hash, expires_at)
		 VALUES (?, ?, ?, ?)`, subjectType, subjectID, hash, expiresAt)
	return err
}

// RefreshByHash 按哈希取 refresh token。
func (r *Auth) RefreshByHash(ctx context.Context, hash string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	err := r.db.GetContext(ctx, &t,
		`SELECT id, subject_type, subject_id, token_hash, expires_at, revoked_at,
		        replaced_by_hash, created_at
		   FROM refresh_tokens WHERE token_hash = ?`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// RevokeRefresh 吊销一条 refresh token（轮换或登出）。
func (r *Auth) RevokeRefresh(ctx context.Context, hash, replacedBy string) error {
	var replaced any
	if replacedBy != "" {
		replaced = replacedBy
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW(3), replaced_by_hash = ?
		  WHERE token_hash = ? AND revoked_at IS NULL`, replaced, hash)
	return err
}

// RevokeAllForSubject 吊销某主体的全部 refresh token（改密码后必须调用）。
//
// 这是「一步到位」的核心：改密后旧 refresh 全部失效，不能再换出新的 access token。
func (r *Auth) RevokeAllForSubject(ctx context.Context, subjectType string, subjectID uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW(3)
		  WHERE subject_type = ? AND subject_id = ? AND revoked_at IS NULL`,
		subjectType, subjectID)
	return err
}

// seedDefaultScreenConfig 给新家庭组写初始规则与组长自己的屏幕时间规则。
//
// 取值来自 docs 12.4 的建议：0 点前放下手机、夜间 0–7 点、允许 0 分钟；
// 连续奖励 满 3 天 +5（repeat）、满 7 天 +10（once）；日总量阈值 120 分钟。
// **这不是「全局默认值」**——它落库并归属于具体 family_id，之后各组独立修改。
func seedDefaultScreenConfig(ctx context.Context, tx *sqlx.Tx, familyID, memberID uint64) error {
	const defaultStreaks = `[{"every":3,"bonus":5,"mode":"repeat"},{"every":7,"bonus":10,"mode":"once"}]`
	res, err := tx.ExecContext(ctx,
		`INSERT INTO rules (family_id, name, points, streaks) VALUES (?, '按时睡觉', 2, ?)`,
		familyID, defaultStreaks)
	if err != nil {
		return err
	}
	ruleID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO screen_rules
		   (family_id, member_id, threshold_min, bonus_points, penalty_points, enabled,
		    night_start_hour, night_end_hour, night_max_min, night_rule_id)
		 VALUES (?, ?, 120, 2, 3, 1, 0, 7, 0, ?)`,
		familyID, memberID, ruleID)
	return err
}

// ---------- 内部 ----------

// uniqueInviteCode 生成 8 位不易混淆的邀请码，撞库则重试。
func uniqueInviteCode(ctx context.Context, tx *sqlx.Tx) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 去掉 I/O/0/1
	for attempt := 0; attempt < 10; attempt++ {
		code, err := randomCode(alphabet, 8)
		if err != nil {
			return "", err
		}
		var n int
		if err := tx.GetContext(ctx, &n,
			`SELECT COUNT(*) FROM families WHERE invite_code = ?`, code); err != nil {
			return "", err
		}
		if n == 0 {
			return code, nil
		}
	}
	return "", fmt.Errorf("邀请码生成失败：连续 10 次冲突")
}
