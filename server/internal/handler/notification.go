package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/repo"
)

// Notification 是站内通知 handler。
//
// 写入端统一在 internal/notify.Service（由 service 层调用）；handler 只读。
// 早期这里还有一条直写 repo 的 notify(...) 路径，会与 service 层各发一条，
// 导致赠送 / 任务审核重复推送，2026-11-06 已整体移除。
type Notification struct{ repo *repo.Notification }

// NewNotification 构造。
func NewNotification(r *repo.Notification) *Notification { return &Notification{repo: r} }

// AppList GET /api/app/notification/list?unread=1&limit=
func (h *Notification) AppList(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	onlyUnread := c.Query("unread") == "1"
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	list, err := h.repo.List(c.Request.Context(), *m.FamilyID, m.ID, onlyUnread, limit)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": list})
}

// AppRead POST /api/app/notification/read —— 标记单条或全部已读。
func (h *Notification) AppRead(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		ID  uint64 `json:"id"`
		All bool   `json:"all"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if in.All {
		n, err := h.repo.MarkAllRead(c.Request.Context(), *m.FamilyID, m.ID)
		if err != nil {
			fail(c, err)
			return
		}
		httpx.OK(c, gin.H{"marked": n})
		return
	}
	if in.ID == 0 {
		httpx.BadRequest(c, "缺少 id（或传 all=true）")
		return
	}
	if err := h.repo.MarkRead(c.Request.Context(), *m.FamilyID, m.ID, in.ID); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"marked": 1})
}
