package handler

import (
	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/service"
)

// DeviceHandler 设备绑定 / 解绑审批（2026-11-07，docs 15.5）。
type Device struct{ svc *service.Device }

// NewDevice 构造。
func NewDevice(svc *service.Device) *Device { return &Device{svc: svc} }

// MyDevices GET /api/app/screen/devices 已在 ScreenHandler 提供（15.4.3），这里不重复。

// RequestUnbind POST /api/app/device/unbind/request —— 普通成员发起解绑申请（需组长审批）。
func (h *Device) RequestUnbind(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		DeviceKey string `json:"device_key"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	req, err := h.svc.RequestUnbind(c.Request.Context(), m, in.DeviceKey)
	if err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "device.unbind.request", "device", &req.ID, nil, gin.H{"device_key": req.DeviceKey})
	httpx.OK(c, req)
}

// UnbindDirect POST /api/app/device/unbind —— 组长直接解绑（不需要申请）。
func (h *Device) UnbindDirect(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		DeviceKey string `json:"device_key"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	out, err := h.svc.UnbindDirect(c.Request.Context(), m, in.DeviceKey)
	if err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "device.unbind", "device", m.FamilyID, nil, gin.H{"device_key": in.DeviceKey})
	httpx.OK(c, out)
}

// MyUnbindRequests GET /api/app/device/unbind/mine —— 我发出的解绑申请。
func (h *Device) MyUnbindRequests(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	list, err := h.svc.MyUnbindRequests(c.Request.Context(), m)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": list})
}

// PendingUnbindRequests GET /api/app/device/unbind/requests —— 本组待审批（组长）。
func (h *Device) PendingUnbindRequests(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	list, err := h.svc.PendingUnbindRequests(c.Request.Context(), m)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": list})
}

// DecideUnbind POST /api/app/device/unbind/decide —— 组长审批（同意即解绑）。
func (h *Device) DecideUnbind(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		RequestID uint64 `json:"request_id"`
		Approve   bool   `json:"approve"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	req, err := h.svc.DecideUnbind(c.Request.Context(), m, in.RequestID, in.Approve)
	if err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "device.unbind.decide", "device", &req.ID, nil,
		gin.H{"approve": in.Approve, "device_key": req.DeviceKey})
	httpx.OK(c, req)
}

// CancelUnbind POST /api/app/device/unbind/cancel —— 成员撤回自己的申请。
func (h *Device) CancelUnbind(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		RequestID uint64 `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.CancelUnbind(c.Request.Context(), m, in.RequestID); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"cancelled": true})
}
