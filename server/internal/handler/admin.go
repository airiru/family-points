package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
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
	httpx.OK(c, gin.H{"id": admin.ID, "username": admin.Username, "nickname": admin.Nickname})
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

// ---------- 只读视图（2026-11-04 补：后台 12 个页面需要读到现值）----------

func qInt(c *gin.Context, key string, def int) int {
	if v := c.Query(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// Rule GET /api/admin/family/:family_id/rule —— 本组睡觉规则（未配置返回 null）。
func (h *Admin) Rule(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	r, err := h.svc.Rule(c.Request.Context(), fid)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"rule": r})
}

// Items GET /api/admin/family/:family_id/items —— 本组商品。
func (h *Admin) Items(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	list, err := h.svc.Items(c.Request.Context(), fid)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": list})
}

// Tasks GET /api/admin/family/:family_id/tasks —— 本组全部任务（含已关闭）。
func (h *Admin) Tasks(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	list, err := h.svc.Tasks(c.Request.Context(), fid)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"tasks": list})
}

// Transfers GET /api/admin/family/:family_id/transfers —— 赠送记录。
func (h *Admin) Transfers(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	list, err := h.svc.Transfers(c.Request.Context(), fid, qInt(c, "limit", 50))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"transfers": list})
}

// ScreenReports GET /api/admin/family/:family_id/screen-reports —— 上报历史（只读）。
func (h *Admin) ScreenReports(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	list, err := h.svc.ScreenReports(c.Request.Context(), fid, uint64(qInt(c, "member_id", 0)), qInt(c, "limit", 50))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"reports": list})
}

// ScreenDeviceReports GET /api/admin/family/:family_id/screen-device-reports —— 设备原始上报明细（只读）。
// date=YYYY-MM-DD 可省略（不限日期）；member_id=0 表示全组。
func (h *Admin) ScreenDeviceReports(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	var day time.Time
	if v := c.Query("date"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			httpx.BadRequest(c, "日期格式应为 YYYY-MM-DD")
			return
		}
		day = t
	}
	reports, devices, err := h.svc.ScreenDeviceReports(
		c.Request.Context(), fid, uint64(qInt(c, "member_id", 0)), day, qInt(c, "limit", 50))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"reports": reports, "devices": devices})
}

// Bans GET /api/admin/family/:family_id/bans —— 本组封禁名单（只读）。
func (h *Admin) Bans(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	list, err := h.svc.Bans(c.Request.Context(), fid)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"bans": list})
}

// JoinAttempts GET /api/admin/family/:family_id/invitations —— 本组全部邀请与申请（只读）。
func (h *Admin) JoinAttempts(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	list, err := h.svc.JoinAttempts(c.Request.Context(), fid, qInt(c, "limit", 50))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"attempts": list})
}

// Ranking GET /api/admin/family/:family_id/stats/ranking?mode=points|score&range=week|month
func (h *Admin) Ranking(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	list, err := h.svc.Ranking(c.Request.Context(), fid, c.DefaultQuery("mode", "points"), c.DefaultQuery("range", "week"), time.Now())
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"ranking": list})
}

// Trend GET /api/admin/family/:family_id/stats/trend?days=30&member_id=0
func (h *Admin) Trend(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	list, err := h.svc.Trend(c.Request.Context(), fid, uint64(qInt(c, "member_id", 0)), qInt(c, "days", 30), time.Now())
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"trend": list})
}

// Records GET /api/admin/family/:family_id/records —— 本组流水（只读、分页、可筛选）。
func (h *Admin) Records(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	f := repo.RecordFilter{
		FamilyID: fid,
		MemberID: uint64(qInt(c, "member_id", 0)),
		Source:   c.Query("source"),
		Keyword:  c.Query("keyword"),
		Limit:    qInt(c, "limit", 50),
		Offset:   qInt(c, "offset", 0),
	}
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			f.From = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			f.To = t.AddDate(0, 0, 1) // 含当天
		}
	}
	list, total, err := h.svc.Records(c.Request.Context(), f)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"records": list, "total": total})
}
