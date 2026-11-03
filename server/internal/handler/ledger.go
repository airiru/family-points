package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/service"
)

// Ledger 是账目 handler（商品、兑换、赠送、流水）。
type Ledger struct{ svc *service.Ledger }

// NewLedger 构造。
func NewLedger(svc *service.Ledger) *Ledger { return &Ledger{svc: svc} }

// ---------- 商品 ----------

// AppItems GET /api/app/item/list
func (h *Ledger) AppItems(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	items, err := h.svc.Items(c.Request.Context(), *m.FamilyID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// AppSaveItem POST /api/app/item/save —— 新增或更新（组长）。
func (h *Ledger) AppSaveItem(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		ID   uint64 `json:"id"`
		Name string `json:"name"`
		Cost int    `json:"cost"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	item, err := h.svc.SaveItem(c.Request.Context(), m, in.ID, in.Name, in.Cost)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, item)
}

// AppDeleteItem POST /api/app/item/delete —— 删除商品（组长）。
func (h *Ledger) AppDeleteItem(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		ID uint64 `json:"id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.DeleteItem(c.Request.Context(), m, in.ID); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true, "id": in.ID})
}

// ---------- 兑换 ----------

// AppRedeem POST /api/app/item/redeem
func (h *Ledger) AppRedeem(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		ItemID uint64 `json:"item_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	rd, err := h.svc.Redeem(c.Request.Context(), m, in.ItemID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"redemption_id": rd.ID,
		"item_name":     rd.ItemName,
		"cost":          rd.Cost,
	})
}

// ---------- 赠送 ----------

// AppSendTransfer POST /api/app/transfer/send
func (h *Ledger) AppSendTransfer(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		ToMemberID uint64 `json:"to_member_id"`
		Amount     int    `json:"amount"`
		Note       string `json:"note"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	t, err := h.svc.Send(c.Request.Context(), m, in.ToMemberID, in.Amount, in.Note)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"transfer_id": t.ID,
		"to_member_id": t.ToMemberID,
		"amount":      t.Amount,
	})
}

// AppTransfers GET /api/app/transfer/list
func (h *Ledger) AppTransfers(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	list, err := h.svc.Transfers(c.Request.Context(), *m.FamilyID, limit)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"transfers": list})
}

// ---------- 流水 ----------
