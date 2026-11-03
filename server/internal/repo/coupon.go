// Package repo：商品券与核销投票的 SQL（docs 9.6）。
//
// 与 task_votes 同构的判定口径：
//   - 有投票权 = 家庭组成员数 − 申请人
//   - 通过 = 同意票 > 有投票权人数 ÷ 2
//   - 提前驳回 = 同意票 + 剩余未投票 < 通过所需票数
package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
)

// Coupon 封装商品券 SQL。
type Coupon struct{ db *sqlx.DB }

// NewCoupon 构造。
func NewCoupon(db *sqlx.DB) *Coupon { return &Coupon{db: db} }

// 券相关业务错误。
var (
	ErrCouponNotUsable = errors.New("券当前不可使用")
	ErrForbidden       = errors.New("没有权限执行该操作")
)

const couponCols = `id, family_id, member_id, redemption_id, item_name, cost,
       status, reject_count, vote_deadline, created_at, settled_at`

// CreateInTx 在兑换事务内发券。与扣分同事务，避免「扣了分但没券」。
func (r *Coupon) CreateInTx(ctx context.Context, tx *sqlx.Tx, familyID, memberID, redemptionID uint64,
	itemName string, cost int) (uint64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO coupons (family_id, member_id, redemption_id, item_name, cost)
		 VALUES (?, ?, ?, ?, ?)`,
		familyID, memberID, redemptionID, itemName, cost)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return uint64(id), nil
}

// ListByMember 列出某成员的全部券（新到旧）。
func (r *Coupon) ListByMember(ctx context.Context, familyID, memberID uint64) ([]model.Coupon, error) {
	var list []model.Coupon
	err := r.db.SelectContext(ctx, &list,
		`SELECT `+couponCols+` FROM coupons
		  WHERE family_id = ? AND member_id = ?
		  ORDER BY id DESC`, familyID, memberID)
	return list, err
}

// ListPending 列出本组「待我核销」的券：pending 且申请人不是自己。
//
// 没有这个查询，别人兑换的券就没人能投票——核销机制等于空转。
func (r *Coupon) ListPending(ctx context.Context, familyID, excludeMemberID uint64) ([]model.Coupon, error) {
	var list []model.Coupon
	err := r.db.SelectContext(ctx, &list,
		`SELECT `+couponCols+` FROM coupons
		  WHERE family_id = ? AND status = ? AND member_id <> ?
		  ORDER BY id DESC`, familyID, model.CouponPending, excludeMemberID)
	return list, err
}

// ByID 取本组某张券。
func (r *Coupon) ByID(ctx context.Context, familyID, id uint64) (*model.Coupon, error) {
	var c model.Coupon
	err := r.db.GetContext(ctx, &c,
		`SELECT `+couponCols+` FROM coupons WHERE id = ? AND family_id = ?`, id, familyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CountVoters 有投票权人数 = 家庭成员数 − 申请人。
func (r *Coupon) CountVoters(ctx context.Context, familyID, excludeMemberID uint64) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM members WHERE family_id = ? AND id <> ?`,
		familyID, excludeMemberID)
	return n, err
}

// Use 发起核销：unused → pending。返回更新后的券。
//
// 只能在事务内锁行操作：并发点两次「使用」不能都成功。
func (r *Coupon) Use(ctx context.Context, familyID, couponID, memberID uint64, deadline time.Time) (*model.Coupon, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var c model.Coupon
	if err := tx.GetContext(ctx, &c,
		`SELECT `+couponCols+` FROM coupons
		  WHERE id = ? AND family_id = ? FOR UPDATE`, couponID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	// 只能本人使用自己的券
	if c.MemberID != memberID {
		return nil, ErrForbidden
	}
	if c.Status != model.CouponUnused {
		return nil, ErrCouponNotUsable
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE coupons SET status = ?, vote_deadline = ? WHERE id = ? AND family_id = ?`,
		model.CouponPending, deadline, couponID, familyID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	c.Status = model.CouponPending
	c.VoteDeadline = &deadline
	return &c, nil
}

// RedeemDirect 直接核销：仅用于「有投票权为 0」的 1 人家庭，
// 无人可投时不该把券永久卡在 unused。
func (r *Coupon) RedeemDirect(ctx context.Context, familyID, couponID, memberID uint64) (*model.Coupon, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var c model.Coupon
	if err := tx.GetContext(ctx, &c,
		`SELECT `+couponCols+` FROM coupons
		  WHERE id = ? AND family_id = ? FOR UPDATE`, couponID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if c.MemberID != memberID {
		return nil, ErrForbidden
	}
	if c.Status != model.CouponUnused {
		return nil, ErrCouponNotUsable
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE coupons SET status = ?, settled_at = NOW(3) WHERE id = ? AND family_id = ?`,
		model.CouponRedeemed, couponID, familyID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	now := time.Now()
	c.Status = model.CouponRedeemed
	c.SettledAt = &now
	return &c, nil
}

// CouponVoteResult 是一次核销投票后的状态。
type CouponVoteResult struct {
	Status  string // pending / redeemed
	Approve int
	Reject  int
	Voters  int
}

// Vote 投票并即时判定。
//
// 通过 = approve > voters/2；提前驳回 = approve + 剩余 < 通过所需。
func (r *Coupon) Vote(ctx context.Context, familyID, couponID, voterID uint64, vote string) (*CouponVoteResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var c model.Coupon
	if err := tx.GetContext(ctx, &c,
		`SELECT `+couponCols+` FROM coupons
		  WHERE id = ? AND family_id = ? FOR UPDATE`, couponID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if c.Status != model.CouponPending {
		return nil, ErrCouponNotUsable
	}
	// 申请人没有投票权
	if c.MemberID == voterID {
		return nil, ErrForbidden
	}
	// 超时：券回到 unused，本次投票无效
	if c.VoteDeadline != nil && time.Now().After(*c.VoteDeadline) {
		if _, err := tx.ExecContext(ctx,
			`UPDATE coupons SET status = ?, vote_deadline = NULL WHERE id = ? AND family_id = ?`,
			model.CouponUnused, couponID, familyID); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, ErrVoteExpired
	}

	// 一人一票：唯一键冲突即已投过
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO coupon_votes (family_id, coupon_id, voter_id, vote) VALUES (?, ?, ?, ?)`,
		familyID, couponID, voterID, vote); err != nil {
		if isDuplicate(err) {
			return nil, ErrAlreadyVoted
		}
		return nil, err
	}

	var approve, reject int
	if err := tx.GetContext(ctx, &approve,
		`SELECT COUNT(*) FROM coupon_votes WHERE coupon_id = ? AND vote = 'approve'`, couponID); err != nil {
		return nil, err
	}
	if err := tx.GetContext(ctx, &reject,
		`SELECT COUNT(*) FROM coupon_votes WHERE coupon_id = ? AND vote = 'reject'`, couponID); err != nil {
		return nil, err
	}
	var voters int
	if err := tx.GetContext(ctx, &voters,
		`SELECT COUNT(*) FROM members WHERE family_id = ? AND id <> ?`,
		familyID, c.MemberID); err != nil {
		return nil, err
	}

	need := voters/2 + 1
	status := model.CouponPending
	switch {
	case approve >= need:
		status = model.CouponRedeemed
		if _, err := tx.ExecContext(ctx,
			`UPDATE coupons SET status = ?, settled_at = NOW(3), vote_deadline = NULL
			  WHERE id = ? AND family_id = ?`, model.CouponRedeemed, couponID, familyID); err != nil {
			return nil, err
		}
	case voters-reject < need:
		// 剩余未投票 + 已同意 < 通过所需 ⇒ 不可能通过，驳回并回到 unused
		if _, err := tx.ExecContext(ctx,
			`UPDATE coupons SET status = ?, reject_count = reject_count + 1, vote_deadline = NULL
			  WHERE id = ? AND family_id = ?`, model.CouponUnused, couponID, familyID); err != nil {
			return nil, err
		}
		status = model.CouponUnused
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &CouponVoteResult{Status: status, Approve: approve, Reject: reject, Voters: voters}, nil
}

// Votes 列出某张券的投票。
func (r *Coupon) Votes(ctx context.Context, familyID, couponID uint64) ([]model.CouponVote, error) {
	var list []model.CouponVote
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, coupon_id, voter_id, vote, created_at
		   FROM coupon_votes WHERE family_id = ? AND coupon_id = ? ORDER BY id`,
		familyID, couponID)
	return list, err
}

// isDuplicate 判断是否唯一键冲突（MySQL 1062）。
func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "1062") || strings.Contains(msg, "Duplicate entry")
}
