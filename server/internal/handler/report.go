package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/service"
)

// Report 是概览 / 记录 / 榜单 / 趋势 handler。
type Report struct{ svc *service.Report }

// NewReport 构造。
func NewReport(svc *service.Report) *Report { return &Report{svc: svc} }

// AppState GET /api/app/state —— 首页概览（**不含全量记录**）。
// AppSiteConfig GET /api/app/site-config —— 站点级配置（如 ICP 备案号），登录即可读。
func (h *Report) AppSiteConfig(c *gin.Context) {
	icp, err := h.svc.SiteICP(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"icp": icp})
}

func (h *Report) AppState(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	st, err := h.svc.State(c.Request.Context(), m)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, st)
}

// AppRecords GET /api/app/records —— 记录分页 + 筛选 + 搜索。
func (h *Report) AppRecords(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	memberID, _ := strconv.ParseUint(c.DefaultQuery("member_id", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	page, err := h.svc.Records(c.Request.Context(), m, service.RecordQuery{
		MemberID: memberID,
		Source:   c.Query("source"),
		Keyword:  c.Query("keyword"),
		From:     c.Query("from"),
		To:       c.Query("to"),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, page)
}

// AppRanking GET /api/app/stats/ranking?type=points|score&period=week|month
func (h *Report) AppRanking(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	rows, period, err := h.svc.Ranking(c.Request.Context(), m,
		c.DefaultQuery("type", "points"), c.DefaultQuery("period", "week"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"type": c.DefaultQuery("type", "points"), "period": period, "rows": rows})
}

// AppTrend GET /api/app/stats/trend?member_id=&period=
func (h *Report) AppTrend(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	memberID, _ := strconv.ParseUint(c.DefaultQuery("member_id", "0"), 10, 64)
	points, period, err := h.svc.Trend(c.Request.Context(), m, memberID, c.DefaultQuery("period", "week"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"period": period, "points": points})
}
