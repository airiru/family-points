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

// AdminRevokeRedemption POST /api/admin/family/:family_id/item/redeem/revoke —— 仅管理员（9.4）。
func (h *Ledger) AdminRevokeRedemption(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	var in struct {
		RedemptionID uint64 `json:"redemption_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.RevokeRedemption(c.Request.Context(), fid, in.RedemptionID); err != nil {
		fail(c, err)
		return
	}
	auditCrossFamily(c, "admin.redeem.revoke", fid)
	httpx.OK(c, gin.H{"revoked": true, "redemption_id": in.RedemptionID})
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

// AdminRevokeTransfer POST /api/admin/family/:family_id/transfer/revoke —— 仅管理员。
func (h *Ledger) AdminRevokeTransfer(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	var in struct {
		TransferID uint64 `json:"transfer_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.RevokeTransfer(c.Request.Context(), fid, in.TransferID); err != nil {
		fail(c, err)
		return
	}
	auditCrossFamily(c, "admin.transfer.revoke", fid)
	httpx.OK(c, gin.H{"revoked": true, "transfer_id": in.TransferID})
}

// ---------- 流水 ----------

// AppRecords GET /api/app/records —— 本组流水分页。
func (h *Ledger) AppRecords(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	page, err := h.svc.Records(c.Request.Context(), *m.FamilyID, limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, page)
}
