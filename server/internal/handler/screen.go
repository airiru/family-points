package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

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

// ---------- 上报（成员登录态，无需设备令牌）----------

// AppReport POST /api/app/screen/report
//
// 2026-11-05 起不再要求设备令牌：登录成员即可上报，设备用 device_key + 型号区分。
// 一个成员可以有多台设备，各自上报；服务端按「成员×日」汇总后再结算。
func (h *Screen) AppReport(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in service.ReportInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	res, err := h.svc.Report(c.Request.Context(), m, in)
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

// AppFamilyDay GET /api/app/screen/family?date=YYYY-MM-DD
//
// 全组当天屏幕时间：每人汇总 + 各自设备明细。用于「查看家庭成员的屏幕时间」。
func (h *Screen) AppFamilyDay(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	date := c.DefaultQuery("date", "")
	view, err := h.svc.FamilyDay(c.Request.Context(), m, date)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// AppDevices GET /api/app/screen/devices —— 本人上报过的设备。
func (h *Screen) AppDevices(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	list, err := h.svc.DevicesOfMember(c.Request.Context(), m)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"devices": list})
}

// AppSnapshot POST /api/app/screen/snapshot
//
// 「今天」的实时快照：只落设备、当日合计与逐小时桶，**不结算、不发分**。
// 判定与积分仍由 Worker 在次日上报「昨天」时统一结算，避免一天内反复改分。
func (h *Screen) AppSnapshot(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in service.ReportInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.Snapshot(c.Request.Context(), m, in); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"saved": true})
}

// AppMemberDetail GET /api/app/screen/member/:member_id?date=YYYY-MM-DD
//
// 某成员某天的完整明细：总时长、逐小时（24 桶）、各设备、近 7 天。
// member_id = 0 表示自己；组内成员互相可见。
func (h *Screen) AppMemberDetail(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var memberID uint64
	if raw := c.Param("member_id"); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			httpx.BadRequest(c, "member_id 必须是数字")
			return
		}
		memberID = v
	}
	date := c.DefaultQuery("date", "")
	view, err := h.svc.MemberDetail(c.Request.Context(), m, memberID, date)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, view)
}
