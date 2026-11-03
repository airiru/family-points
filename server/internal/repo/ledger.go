// Package repo：账目相关 SQL（商品、兑换、赠送、流水）。
//
// 三条铁律（见 docs 8、9、10 节）：
//  1. **每一次积分变动都要写 records**（用户 2026-11-03 明确：所有变动都留记录）
//  2. 余额校验必须在**事务内**，且用 `FOR UPDATE` 锁住成员行
//  3. 涉及两个成员时**按 id 升序加锁**，否则互赠并发会死锁
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
)

// Ledger 封装账目 SQL。
type Ledger struct{ db *sqlx.DB }

// NewLedger 构造。
func NewLedger(db *sqlx.DB) *Ledger { return &Ledger{db: db} }

// ---------- 商品 ----------

// Items 列出本组商品。
func (r *Ledger) Items(ctx context.Context, familyID uint64) ([]model.Item, error) {
	var list []model.Item
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, name, cost, created_at
		   FROM items WHERE family_id = ? ORDER BY id`, familyID)
	return list, err
}

// ItemByID 取本组某商品。
func (r *Ledger) ItemByID(ctx context.Context, familyID, id uint64) (*model.Item, error) {
	var it model.Item
	err := r.db.GetContext(ctx, &it,
		`SELECT id, family_id, name, cost, created_at
		   FROM items WHERE id = ? AND family_id = ?`, id, familyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// SaveItem 新增或更新商品（id 为 0 表示新增）。
func (r *Ledger) SaveItem(ctx context.Context, familyID uint64, id uint64, name string, cost int) (*model.Item, error) {
	if id == 0 {
		res, err := r.db.ExecContext(ctx,
			`INSERT INTO items (family_id, name, cost) VALUES (?, ?, ?)`, familyID, name, cost)
		if err != nil {
			return nil, err
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		return &model.Item{ID: uint64(newID), FamilyID: familyID, Name: name, Cost: cost, CreatedAt: time.Now()}, nil
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE items SET name = ?, cost = ? WHERE id = ? AND family_id = ?`,
		name, cost, id, familyID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return &model.Item{ID: id, FamilyID: familyID, Name: name, Cost: cost, CreatedAt: time.Now()}, nil
}

// DeleteItem 删除商品。历史兑换单有 item_name 快照，不受影响。
func (r *Ledger) DeleteItem(ctx context.Context, familyID, id uint64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM items WHERE id = ? AND family_id = ?`, id, familyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- 兑换 ----------

// Redeem 兑换商品：锁成员行 → 校验余额 → 扣分 → 写兑换单 → 写流水。
func (r *Ledger) Redeem(ctx context.Context, familyID, memberID uint64, item *model.Item) (*model.Redemption, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 锁住成员行：并发下防止两个请求同时看到「余额够」
	var m struct {
		ID       uint64 `db:"id"`
		Name     string `db:"name"`
		Score    int    `db:"score"`
		FamilyID uint64 `db:"family_id"`
	}
	if err := tx.GetContext(ctx, &m,
		`SELECT id, name, score, family_id FROM members WHERE id = ? AND family_id = ? FOR UPDATE`,
		memberID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	// 2. 余额校验必须在锁内
	if m.Score < item.Cost {
		return nil, ErrInsufficient
	}

	// 3. 扣分（只用可用余额，冻结积分不动）
	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET score = score - ? WHERE id = ? AND family_id = ?`,
		item.Cost, memberID, familyID); err != nil {
		return nil, err
	}

	// 4. 兑换单（商品名与价格都存快照）
	res, err := tx.ExecContext(ctx,
		`INSERT INTO redemptions (family_id, member_id, item_id, item_name, cost)
		 VALUES (?, ?, ?, ?, ?)`, familyID, memberID, item.ID, item.Name, item.Cost)
	if err != nil {
		return nil, err
	}
	redemptionID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	// 5. 流水：兑换是负分记录（source='redeem'）
	now := time.Now()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO records (family_id, member_id, member_name, title, points,
		                      occurred_at, added_at, source, redemption_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		familyID, memberID, m.Name, "兑换 "+item.Name, -item.Cost,
		now, now, model.SourceRedeem, redemptionID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &model.Redemption{
		ID: uint64(redemptionID), FamilyID: familyID, MemberID: memberID,
		ItemID: &item.ID, ItemName: item.Name, Cost: item.Cost, CreatedAt: now,
	}, nil
}

// RedemptionByID 取兑换单。
func (r *Ledger) RedemptionByID(ctx context.Context, familyID, id uint64) (*model.Redemption, error) {
	var rd model.Redemption
	err := r.db.GetContext(ctx, &rd,
		`SELECT id, family_id, member_id, item_id, item_name, cost, created_at, revoked_at
		   FROM redemptions WHERE id = ? AND family_id = ?`, id, familyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rd, nil
}

// RevokeRedemption 撤销兑换：退回积分 + 写反向流水 + 标记 revoked_at。
func (r *Ledger) RevokeRedemption(ctx context.Context, familyID, redemptionID uint64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 锁兑换单，防止并发重复撤销
	var rd model.Redemption
	if err := tx.GetContext(ctx, &rd,
		`SELECT id, family_id, member_id, item_id, item_name, cost, created_at, revoked_at
		   FROM redemptions WHERE id = ? AND family_id = ? FOR UPDATE`,
		redemptionID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if rd.RevokedAt != nil {
		return ErrAlreadyRevoked
	}

	// 2. 锁成员行并退回积分
	var m struct {
		Name  string `db:"name"`
		Score int    `db:"score"`
	}
	if err := tx.GetContext(ctx, &m,
		`SELECT name, score FROM members WHERE id = ? AND family_id = ? FOR UPDATE`,
		rd.MemberID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET score = score + ? WHERE id = ? AND family_id = ?`,
		rd.Cost, rd.MemberID, familyID); err != nil {
		return err
	}

	// 3. 标记已撤销
	if _, err := tx.ExecContext(ctx,
		`UPDATE redemptions SET revoked_at = NOW(3) WHERE id = ? AND family_id = ?`,
		redemptionID, familyID); err != nil {
		return err
	}

	// 4. 反向流水（不是删原记录——流水只增不删）
	now := time.Now()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO records (family_id, member_id, member_name, title, points,
		                      occurred_at, added_at, source, redemption_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		familyID, rd.MemberID, m.Name, "撤销兑换 "+rd.ItemName, rd.Cost,
		now, now, model.SourceRedeem, redemptionID); err != nil {
		return err
	}

	return tx.Commit()
}

// ---------- 赠送 ----------

// SendTransfer 赠送积分。**按 id 升序加锁**，避免 A↔B 互赠并发死锁。
func (r *Ledger) SendTransfer(ctx context.Context, familyID, fromID, toID uint64, amount int, note string) (*model.Transfer, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 按 id 升序加锁（ORDER BY id 是死锁防护的关键）
	type memberRow struct {
		ID    uint64 `db:"id"`
		Name  string `db:"name"`
		Score int    `db:"score"`
	}
	var rows []memberRow
	if err := tx.SelectContext(ctx, &rows,
		`SELECT id, name, score FROM members
		  WHERE id IN (?, ?) AND family_id = ? ORDER BY id FOR UPDATE`,
		fromID, toID, familyID); err != nil {
		return nil, err
	}
	if len(rows) != 2 {
		// 有一个不在本组 —— 按「资源不存在」处理，不泄露对方是否存在
		return nil, ErrNotFound
	}
	var from, to memberRow
	for _, row := range rows {
		switch row.ID {
		case fromID:
			from = row
		case toID:
			to = row
		}
	}

	// 2. 事务内校验余额
	if from.Score < amount {
		return nil, ErrInsufficient
	}

	// 3. 变动余额
	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET score = score - ? WHERE id = ? AND family_id = ?`,
		amount, fromID, familyID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET score = score + ? WHERE id = ? AND family_id = ?`,
		amount, toID, familyID); err != nil {
		return nil, err
	}

	// 4. 赠送单
	var noteVal any
	if note != "" {
		noteVal = note
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO transfers (family_id, from_member_id, to_member_id, amount, note)
		 VALUES (?, ?, ?, ?, ?)`, familyID, fromID, toID, amount, noteVal)
	if err != nil {
		return nil, err
	}
	transferID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	// 5. 两条流水（转出 -N / 转入 +N），共享 transfer_id
	now := time.Now()
	tid := uint64(transferID)
	for _, row := range []struct {
		memberID uint64
		name     string
		title    string
		points   int
	}{
		{fromID, from.Name, "赠送给 " + to.Name, -amount},
		{toID, to.Name, "来自 " + from.Name + " 的赠送", amount},
	} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO records (family_id, member_id, member_name, title, points,
			                      occurred_at, added_at, source, transfer_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			familyID, row.memberID, row.name, row.title, row.points,
			now, now, model.SourceTransfer, tid); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	t := &model.Transfer{
		ID: tid, FamilyID: familyID, FromMemberID: fromID, ToMemberID: toID,
		Amount: amount, CreatedAt: now,
	}
	if note != "" {
		t.Note = &note
	}
	return t, nil
}

// Transfers 列出本组赠送记录。
func (r *Ledger) Transfers(ctx context.Context, familyID uint64, limit int) ([]model.Transfer, error) {
	var list []model.Transfer
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, from_member_id, to_member_id, amount, note, created_at, revoked_at
		   FROM transfers WHERE family_id = ? ORDER BY id DESC LIMIT ?`, familyID, limit)
	return list, err
}

// RevokeTransfer 撤销赠送：受赠方退回 N，转出方收回 N，写两条反向流水。
//
// 余额约束在**受赠方**身上：他可能已经把这 N 分花掉，此时无法回滚（10.5）。
func (r *Ledger) RevokeTransfer(ctx context.Context, familyID, transferID uint64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var t model.Transfer
	if err := tx.GetContext(ctx, &t,
		`SELECT id, family_id, from_member_id, to_member_id, amount, note, created_at, revoked_at
		   FROM transfers WHERE id = ? AND family_id = ? FOR UPDATE`, transferID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if t.RevokedAt != nil {
		return ErrAlreadyRevoked
	}

	// 按 id 升序加锁（与赠送同一顺序，避免与并发赠送互相死锁）
	type memberRow struct {
		ID    uint64 `db:"id"`
		Name  string `db:"name"`
		Score int    `db:"score"`
	}
	var rows []memberRow
	if err := tx.SelectContext(ctx, &rows,
		`SELECT id, name, score FROM members
		  WHERE id IN (?, ?) AND family_id = ? ORDER BY id FOR UPDATE`,
		t.FromMemberID, t.ToMemberID, familyID); err != nil {
		return err
	}
	if len(rows) != 2 {
		return ErrNotFound
	}
	var from, to memberRow
	for _, row := range rows {
		switch row.ID {
		case t.FromMemberID:
			from = row
		case t.ToMemberID:
			to = row
		}
	}
	if to.Score < t.Amount {
		return ErrInsufficient
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET score = score - ? WHERE id = ? AND family_id = ?`,
		t.Amount, t.ToMemberID, familyID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET score = score + ? WHERE id = ? AND family_id = ?`,
		t.Amount, t.FromMemberID, familyID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE transfers SET revoked_at = NOW(3) WHERE id = ? AND family_id = ?`,
		transferID, familyID); err != nil {
		return err
	}

	// 反向流水：受赠方 -N、转出方 +N，仍挂在同一 transfer_id 上
	now := time.Now()
	for _, row := range []struct {
		memberID uint64
		name     string
		title    string
		points   int
	}{
		{to.ID, to.Name, "撤销赠送（退回给 " + from.Name + "）", -t.Amount},
		{from.ID, from.Name, "撤销赠送（收回）", t.Amount},
	} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO records (family_id, member_id, member_name, title, points,
			                      occurred_at, added_at, source, transfer_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			familyID, row.memberID, row.name, row.title, row.points,
			now, now, model.SourceTransfer, t.ID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ---------- 流水 ----------

// Records 分页列出本组流水（按发生时间倒序）。
func (r *Ledger) Records(ctx context.Context, familyID uint64, limit, offset int) ([]model.Record, error) {
	var list []model.Record
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, member_id, member_name, title, points,
		        occurred_at, added_at, source, transfer_id, redemption_id
		   FROM records WHERE family_id = ?
		  ORDER BY occurred_at DESC, id DESC LIMIT ? OFFSET ?`, familyID, limit, offset)
	return list, err
}

// CountRecords 本组流水总数（分页用）。
func (r *Ledger) CountRecords(ctx context.Context, familyID uint64) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM records WHERE family_id = ?`, familyID)
	return n, err
}
