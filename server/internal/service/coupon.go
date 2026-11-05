// Package service：商品券与核销投票（docs 9.6）。
package service

import (
	"context"
	"fmt"
	"time"

	"familypoints/server/internal/model"
	"familypoints/server/internal/notify"
	"familypoints/server/internal/repo"
)

// 核销投票有效期。
const couponVoteWindow = 24 * time.Hour

// Coupon 是商品券服务。
type Coupon struct {
	coupons  *repo.Coupon
	notifier *notify.Service
}

// NewCoupon 构造。
func NewCoupon(c *repo.Coupon, n *notify.Service) *Coupon { return &Coupon{coupons: c, notifier: n} }

// List 返回「我的券」（含三种状态）。
func (s *Coupon) List(ctx context.Context, me *model.Member) ([]model.Coupon, error) {
	if !me.InFamily() {
		return nil, ErrNotInFamily
	}
	return s.coupons.ListByMember(ctx, *me.FamilyID, me.ID)
}

// ListPending 返回本组「待我核销」的券。
func (s *Coupon) ListPending(ctx context.Context, me *model.Member) ([]model.Coupon, error) {
	if !me.InFamily() {
		return nil, ErrNotInFamily
	}
	return s.coupons.ListPending(ctx, *me.FamilyID, me.ID)
}

// Use 使用券：发起核销，进入投票。
//
// 只有本人能用自己的券；有投票权为 0（1 人家庭）时直接核销，不卡死。
func (s *Coupon) Use(ctx context.Context, me *model.Member, couponID uint64) (*model.Coupon, error) {
	if !me.InFamily() {
		return nil, ErrNotInFamily
	}
	familyID := *me.FamilyID

	voters, err := s.coupons.CountVoters(ctx, familyID, me.ID)
	if err != nil {
		return nil, err
	}
	// 1 人家庭：除自己外无人可投，直接核销
	if voters == 0 {
		return s.coupons.RedeemDirect(ctx, familyID, couponID, me.ID)
	}
	c, err := s.coupons.Use(ctx, familyID, couponID, me.ID, time.Now().Add(couponVoteWindow))
	if err != nil {
		return nil, err
	}
	s.notifier.CouponPending(ctx, familyID, me.ID, c)
	return c, nil
}

// Vote 核销投票。申请人没有投票权，一人一票。
func (s *Coupon) Vote(ctx context.Context, me *model.Member, couponID uint64, vote string) (*repo.CouponVoteResult, error) {
	if !me.InFamily() {
		return nil, ErrNotInFamily
	}
	if vote != "approve" && vote != "reject" {
		return nil, fmt.Errorf("%w：vote 只能是 approve 或 reject", ErrInvalidInput)
	}
	res, err := s.coupons.Vote(ctx, *me.FamilyID, couponID, me.ID, vote)
	if err != nil {
		return nil, err
	}
	if res.Status == "redeemed" || res.Status == "unused" {
		if c, err := s.coupons.ByID(ctx, *me.FamilyID, couponID); err == nil {
			s.notifier.CouponDecided(ctx, *me.FamilyID, c.MemberID, couponID, c.ItemName, res.Status)
		}
	}
	return res, nil
}

// Detail 返回某张券及其投票明细。
func (s *Coupon) Detail(ctx context.Context, me *model.Member, couponID uint64) (*model.Coupon, []model.CouponVote, error) {
	if !me.InFamily() {
		return nil, nil, ErrNotInFamily
	}
	c, err := s.coupons.ByID(ctx, *me.FamilyID, couponID)
	if err != nil {
		return nil, nil, err
	}
	votes, err := s.coupons.Votes(ctx, *me.FamilyID, couponID)
	if err != nil {
		return nil, nil, err
	}
	return c, votes, nil
}
