package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/service"
)

// Task 是任务 handler。
type Task struct{ svc *service.Task }

// NewTask 构造。
func NewTask(svc *service.Task) *Task { return &Task{svc: svc} }

// AppCreateTask POST /api/app/task/create —— 发布任务（冻结积分）。
func (h *Task) AppCreateTask(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		Title  string     `json:"title"`
		Points int        `json:"points"`
		DueAt  *time.Time `json:"due_at"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	t, err := h.svc.Create(c.Request.Context(), m, in.Title, in.Points, in.DueAt)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, t)
}

// AppTasks GET /api/app/task/list
func (h *Task) AppTasks(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	list, err := h.svc.List(c.Request.Context(), m)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"tasks": list})
}

// AppTaskDetail GET /api/app/task/detail?task_id=
func (h *Task) AppTaskDetail(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	taskID, ok := queryID(c, "task_id")
	if !ok {
		return
	}
	d, err := h.svc.Detail(c.Request.Context(), m, taskID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, d)
}

// AppClaimTask POST /api/app/task/claim
func (h *Task) AppClaimTask(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	taskID, ok := bodyID(c, "task_id")
	if !ok {
		return
	}
	t, err := h.svc.Claim(c.Request.Context(), m, taskID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, t)
}

// AppAbandonTask POST /api/app/task/abandon
func (h *Task) AppAbandonTask(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	taskID, ok := bodyID(c, "task_id")
	if !ok {
		return
	}
	if err := h.svc.Abandon(c.Request.Context(), m, taskID); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"abandoned": true, "task_id": taskID})
}

// AppCloseTask POST /api/app/task/close —— 发布者关闭，冻结退回。
func (h *Task) AppCloseTask(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	taskID, ok := bodyID(c, "task_id")
	if !ok {
		return
	}
	t, err := h.svc.Close(c.Request.Context(), m, taskID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, t)
}

// AppSubmitTask POST /api/app/task/submit —— 提交完成（说明必填）。
func (h *Task) AppSubmitTask(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		TaskID uint64 `json:"task_id"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if in.TaskID == 0 {
		httpx.BadRequest(c, "缺少 task_id")
		return
	}
	sub, err := h.svc.Submit(c.Request.Context(), m, in.TaskID, in.Note)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, sub)
}

// AppVoteTask POST /api/app/task/vote —— 全员投票，过半即通过。
func (h *Task) AppVoteTask(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		SubmissionID uint64 `json:"submission_id"`
		Vote         string `json:"vote"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if in.SubmissionID == 0 {
		httpx.BadRequest(c, "缺少 submission_id")
		return
	}
	res, err := h.svc.Vote(c.Request.Context(), m, in.SubmissionID, in.Vote)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"status":      res.Status,
		"approve":     res.Approve,
		"reject":      res.Reject,
		"voters":      res.Voters,
		"transferred": res.Transferred,
	})
}
