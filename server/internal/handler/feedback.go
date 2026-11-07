package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/service"
)

// FeedbackHandler 用户反馈：成员提交 + 后台查看。
type Feedback struct{ svc *service.FeedbackService }

// NewFeedback 构造。
func NewFeedback(svc *service.FeedbackService) *Feedback { return &Feedback{svc: svc} }

// Submit POST /api/app/feedback —— 成员提交反馈（1–500 字，登录态即可，无需入组）。
func (h *Feedback) Submit(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.Submit(c.Request.Context(), m, in.Content); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"submitted": true})
}

// List GET /api/admin/feedbacks?handled=0|1|2&limit= —— 后台反馈列表。
func (h *Feedback) List(c *gin.Context) {
	handled, _ := strconv.Atoi(c.DefaultQuery("handled", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	list, err := h.svc.List(c.Request.Context(), handled, limit)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": list})
}

// Handle POST /api/admin/feedback/:id/handle —— 标记已处理。
func (h *Feedback) Handle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		httpx.BadRequest(c, "无效的反馈 id")
		return
	}
	if err := h.svc.Handle(c.Request.Context(), id); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"handled": true})
}
