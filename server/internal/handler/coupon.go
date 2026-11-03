package handler

import (
	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/service"
)

// Coupon 是商品券 handler。
type Coupon struct{ svc *service.Coupon }

// NewCoupon 构造。
func NewCoupon(svc *service.Coupon) *Coupon { return &Coupon{svc: svc} }

// AppList GET /api/app/coupons —— 我的券（三种状态）。
func (h *Coupon) AppList(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	list, err := h.svc.List(c.Request.Context(), m)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": list})
}

// AppPending GET /api/app/coupons/pending —— 本组待我核销的券。
func (h *Coupon) AppPending(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	list, err := h.svc.ListPending(c.Request.Context(), m)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": list})
}

// AppUse POST /api/app/coupon/use —— 使用券，发起核销投票。
func (h *Coupon) AppUse(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		CouponID uint64 `json:"coupon_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.CouponID == 0 {
		httpx.BadRequest(c, "缺少 coupon_id")
		return
	}
	cp, err := h.svc.Use(c.Request.Context(), m, in.CouponID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, cp)
}

// AppVote POST /api/app/coupon/vote —— 核销投票（同意 / 反对）。
func (h *Coupon) AppVote(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		CouponID uint64 `json:"coupon_id"`
		Vote     string `json:"vote"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.CouponID == 0 {
		httpx.BadRequest(c, "缺少 coupon_id")
		return
	}
	res, err := h.svc.Vote(c.Request.Context(), m, in.CouponID, in.Vote)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"status":  res.Status,
		"approve": res.Approve,
		"reject":  res.Reject,
		"voters":  res.Voters,
	})
}

// AppDetail GET /api/app/coupon/detail?coupon_id= —— 券详情与投票明细。
func (h *Coupon) AppDetail(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	id, ok := queryID(c, "coupon_id")
	if !ok {
		return
	}
	cp, votes, err := h.svc.Detail(c.Request.Context(), m, id)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"coupon": cp, "votes": votes})
}
