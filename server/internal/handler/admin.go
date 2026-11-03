package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/model"
	"familypoints/server/internal/service"
)

// Admin 是后台管理 handler。
type Admin struct{ svc *service.Admin }

// NewAdmin 构造。
func NewAdmin(svc *service.Admin) *Admin { return &Admin{svc: svc} }

// Families GET /api/admin/families —— 所有家庭组（运营方视角）。
func (h *Admin) Families(c *gin.Context) {
	list, err := h.svc.Families(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"families": list})
}

// State GET /api/admin/state —— 后台首页概览。
func (h *Admin) State(c *gin.Context) {
	st, err := h.svc.State(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, st)
}

// Invite POST /api/admin/family/:family_id/invite —— 按用户名拉人进组。
func (h *Admin) Invite(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	var in struct {
		Username string `json:"username"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.Invite(c.Request.Context(), fid, in.Username); err != nil {
		fail(c, err)
		return
	}
	markAuditFamily(c, fid)
	recordAudit(c, "member.invite", "family", &fid, nil, gin.H{"username": in.Username})
	httpx.OK(c, gin.H{"joined": true, "username": in.Username})
}

// Kick POST /api/admin/family/:family_id/member/kick —— 踢出成员。
func (h *Admin) Kick(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	memberID, ok := bodyID(c, "member_id")
	if !ok {
		return
	}
	if err := h.svc.Kick(c.Request.Context(), fid, memberID); err != nil {
		fail(c, err)
		return
	}
	markAuditFamily(c, fid)
	recordAudit(c, "member.kick", "member", &memberID, nil, gin.H{"family_id": fid})
	httpx.OK(c, gin.H{"removed": true, "member_id": memberID})
}

// SetHidden POST /api/admin/family/:family_id/member/hidden
func (h *Admin) SetHidden(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	var in struct {
		MemberID uint64 `json:"member_id"`
		Hidden   bool   `json:"hidden"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.SetHidden(c.Request.Context(), fid, in.MemberID, in.Hidden); err != nil {
		fail(c, err)
		return
	}
	markAuditFamily(c, fid)
	recordAudit(c, "member.hidden", "member", &in.MemberID, nil, gin.H{"hidden": in.Hidden})
	httpx.OK(c, gin.H{"member_id": in.MemberID, "hidden": in.Hidden})
}

// TransferLeader POST /api/admin/family/:family_id/member/leader —— 指定新组长。
func (h *Admin) TransferLeader(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	memberID, ok := bodyID(c, "member_id")
	if !ok {
		return
	}
	if err := h.svc.TransferLeader(c.Request.Context(), fid, memberID); err != nil {
		fail(c, err)
		return
	}
	markAuditFamily(c, fid)
	recordAudit(c, "leader.transfer", "member", &memberID, nil, gin.H{"family_id": fid})
	httpx.OK(c, gin.H{"leader_id": memberID})
}

// RenameFamily POST /api/admin/family/:family_id/rename
func (h *Admin) RenameFamily(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	name, err := h.svc.RenameFamily(c.Request.Context(), fid, in.Name)
	if err != nil {
		fail(c, err)
		return
	}
	markAuditFamily(c, fid)
	recordAudit(c, "family.rename", "family", &fid, nil, gin.H{"name": name})
	httpx.OK(c, gin.H{"name": name})
}

// SaveRule POST /api/admin/family/:family_id/rule/save —— 与组长同权（14.3）。
func (h *Admin) SaveRule(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	var in struct {
		ID      uint64             `json:"id"`
		Name    string             `json:"name"`
		Points  int                `json:"points"`
		Streaks []model.StreakTier `json:"streaks"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	rule, err := h.svc.SaveRule(c.Request.Context(), fid, in.ID, in.Name, in.Points, in.Streaks)
	if err != nil {
		fail(c, err)
		return
	}
	markAuditFamily(c, fid)
	recordAudit(c, "rule.save", "rule", &rule.ID, nil, gin.H{
		"name": rule.Name, "points": rule.Points, "streaks": in.Streaks,
	})
	httpx.OK(c, rule)
}

// SaveItem POST /api/admin/family/:family_id/item/save —— 与组长同权（14.5）。
func (h *Admin) SaveItem(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
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
	item, err := h.svc.SaveItem(c.Request.Context(), fid, in.ID, in.Name, in.Cost)
	if err != nil {
		fail(c, err)
		return
	}
	markAuditFamily(c, fid)
	recordAudit(c, "item.save", "item", &item.ID, nil, gin.H{"name": item.Name, "cost": item.Cost})
	httpx.OK(c, item)
}

// ExportRecords GET /api/admin/export/records?family_id=&from=&to= —— CSV。
func (h *Admin) ExportRecords(c *gin.Context) {
	fid, err := strconv.ParseUint(c.Query("family_id"), 10, 64)
	if err != nil || fid == 0 {
		httpx.BadRequest(c, "缺少 family_id")
		return
	}
	data, name, err := h.svc.ExportRecords(c.Request.Context(), fid, c.Query("from"), c.Query("to"))
	if err != nil {
		fail(c, err)
		return
	}
	markAuditFamily(c, fid)
	recordAudit(c, "records.export", "family", &fid, nil, gin.H{"bytes": len(data)})
	c.Header("Content-Disposition", "attachment; filename="+name)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}

// ListAdmins GET /api/admin/accounts
func (h *Admin) ListAdmins(c *gin.Context) {
	list, err := h.svc.ListAdmins(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"admins": list})
}

// CreateAdmin POST /api/admin/accounts
func (h *Admin) CreateAdmin(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	admin, err := h.svc.CreateAdmin(c.Request.Context(), in.Username, in.Password)
	if err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "admin.create", "admin", &admin.ID, nil, gin.H{"username": admin.Username})
	httpx.OK(c, gin.H{"id": admin.ID, "username": admin.Username})
}

// DisableAdmin POST /api/admin/accounts/disable —— 停用 / 启用。
func (h *Admin) DisableAdmin(c *gin.Context) {
	op, ok := middleware.CurrentAdmin(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		AdminID  uint64 `json:"admin_id"`
		Disabled bool   `json:"disabled"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.SetAdminDisabled(c.Request.Context(), op.ID, in.AdminID, in.Disabled); err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "admin.disable", "admin", &in.AdminID, nil, gin.H{"disabled": in.Disabled})
	httpx.OK(c, gin.H{"admin_id": in.AdminID, "disabled": in.Disabled})
}

// markAuditFamily 让 recordAudit 知道本次操作属于哪个家庭组（管理员跨组时必须显式标记）。
func markAuditFamily(c *gin.Context, familyID uint64) {
	c.Set("audit_family_id", familyID)
}
