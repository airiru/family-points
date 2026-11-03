package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/repo"
)

// notifier 是进程级通知写入器（横切关注点）。
var notifier *repo.Notification

// SetNotifier 在启动时注入。
func SetNotifier(n *repo.Notification) { notifier = n }

// notify 写一条站内通知。**旁路**：失败只记日志，不影响已成功的业务。
func notify(c *gin.Context, familyID, memberID uint64, typ, title, body, refType string, refID *uint64) {
	if notifier == nil || memberID == 0 {
		return
	}
	if err := notifier.Notify(c.Request.Context(), familyID, memberID, typ, title, body, refType, refID); err != nil {
		logger().Error("通知写入失败（已忽略）", "type", typ, "member_id", memberID, "err", err)
	}
}

// Notification 是站内通知 handler。
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
