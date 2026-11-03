package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/auth"
	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/model"
	"familypoints/server/internal/service"
)

// Screen 是睡觉规则 / 屏幕时间 handler。
type Screen struct{ svc *service.Screen }

// NewScreen 构造。
func NewScreen(svc *service.Screen) *Screen { return &Screen{svc: svc} }

// ---------- 规则配置（组长）----------

// AppRules GET /api/app/rule —— 本组规则（含连续奖励档位）。
func (h *Screen) AppRules(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	rules, err := h.svc.Rules(c.Request.Context(), *m.FamilyID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"rules": rules})
}

// AppSaveRule POST /api/app/rule/save —— 保存睡觉规则（组长）。
func (h *Screen) AppSaveRule(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
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
	rule, err := h.svc.SaveRule(c.Request.Context(), m, in.ID, in.Name, in.Points, in.Streaks)
	if err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "rule.save", "rule", &rule.ID, nil, gin.H{
		"name": rule.Name, "points": rule.Points, "streaks": in.Streaks,
	})
	httpx.OK(c, rule)
}

// AppScreenRules GET /api/app/screen-rule —— 本组屏幕时间规则。
func (h *Screen) AppScreenRules(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	list, err := h.svc.ScreenRules(c.Request.Context(), *m.FamilyID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"screen_rules": list})
}

// AppSaveScreenRule POST /api/app/screen-rule/save —— 保存某成员的规则（组长）。
func (h *Screen) AppSaveScreenRule(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in service.SaveScreenRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	rule, err := h.svc.SaveScreenRule(c.Request.Context(), m, in)
	if err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "screen_rule.save", "member", &in.MemberID, nil, gin.H{
		"threshold_min": in.ThresholdMin, "night_start": in.NightStartHour,
		"night_end": in.NightEndHour, "night_max_min": in.NightMaxMin,
	})
	httpx.OK(c, rule)
}

// AppScreenReports GET /api/app/screen/reports?member_id=&limit=
func (h *Screen) AppScreenReports(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	memberID, _ := strconv.ParseUint(c.DefaultQuery("member_id", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	list, err := h.svc.Reports(c.Request.Context(), m, memberID, limit)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"reports": list})
}

// ---------- 设备（管理员，跨组）----------

// AdminCreateDevice POST /api/admin/family/:family_id/device —— 生成设备令牌。
//
// **明文令牌只在这里返回一次**，库里只有 SHA-256。
func (h *Screen) AdminCreateDevice(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	var in struct {
		MemberID uint64 `json:"member_id"`
		Name     string `json:"name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if in.MemberID == 0 {
		httpx.BadRequest(c, "缺少 member_id")
		return
	}
	raw, hash, err := auth.NewRefresh() // 复用随机串生成：32 字节 → hex
	if err != nil {
		fail(c, err)
		return
	}
	dev, err := h.svc.CreateDevice(c.Request.Context(), fid, in.MemberID, in.Name, raw, hash)
	if err != nil {
		fail(c, err)
		return
	}
	auditCrossFamily(c, "admin.device.create", fid)
	c.Set("audit_family_id", fid)
	recordAudit(c, "device.create", "device", &dev.ID, nil, gin.H{"member_id": dev.MemberID})
	httpx.OK(c, gin.H{
		"device_id": dev.ID,
		"member_id": dev.MemberID,
		"name":      dev.Name,
		"token":     raw, // 只此一次
	})
}

// AdminDevices GET /api/admin/family/:family_id/device
func (h *Screen) AdminDevices(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	list, err := h.svc.Devices(c.Request.Context(), fid)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"devices": list})
}

// AdminRevokeDevice POST /api/admin/family/:family_id/device/revoke
func (h *Screen) AdminRevokeDevice(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	var in struct {
		DeviceID uint64 `json:"device_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.RevokeDevice(c.Request.Context(), fid, in.DeviceID); err != nil {
		fail(c, err)
		return
	}
	auditCrossFamily(c, "admin.device.revoke", fid)
	c.Set("audit_family_id", fid)
	recordAudit(c, "device.revoke", "device", &in.DeviceID, nil, nil)
	httpx.OK(c, gin.H{"revoked": true, "device_id": in.DeviceID})
}

// ---------- 上报（设备令牌）----------

// DeviceReport POST /api/device/screen/report
func (h *Screen) DeviceReport(c *gin.Context) {
	dev, ok := middleware.CurrentDevice(c)
	if !ok {
		httpx.Unauthorized(c, "缺少设备上下文")
		return
	}
	var in service.ReportInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	res, err := h.svc.Report(c.Request.Context(), dev, in)
	if err != nil {
		fail(c, err)
		return
	}
	body := gin.H{
		"changed":     res.Changed,
		"report_date": res.Report.ReportDate.Format("2006-01-02"),
		"total_min":   res.Report.TotalMin,
		"points":      res.Report.Points,
		"revision":    res.Report.Revision,
	}
	if res.Changed {
		body["day_points"] = res.DayPoints
		body["night_ok"] = res.NightOK
		body["streak_days"] = res.StreakDays
		body["awards"] = res.Awards
	}
	httpx.OK(c, body)
}
