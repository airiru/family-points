package middleware

import (
	"github.com/gin-gonic/gin"

	"familypoints/server/internal/auth"
	"familypoints/server/internal/httpx"
	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
)

// CtxDevice 是设备上下文的键。
const CtxDevice = "ctx_device"

// Device 校验设备令牌（`X-Device-Token` 头）。
//
// 屏幕时间**只有这一条写入路径**（19 条）：设备自动上报，不接受成员 JWT。
// 令牌原文不落库，只存 SHA-256，这里按哈希查。
type Device struct{ devices *repo.Screen }

// NewDevice 构造。
func NewDevice(d *repo.Screen) *Device { return &Device{devices: d} }

// Require 校验通过后把设备放进上下文。
func (m *Device) Require() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("X-Device-Token")
		if raw == "" {
			httpx.Unauthorized(c, "缺少设备令牌")
			c.Abort()
			return
		}
		dev, err := m.devices.DeviceByTokenHash(c.Request.Context(), auth.HashRefresh(raw))
		if err != nil {
			httpx.Unauthorized(c, "设备令牌无效或已吊销")
			c.Abort()
			return
		}
		c.Set(CtxDevice, dev)
		c.Next()
	}
}

// CurrentDevice 取当前设备。
func CurrentDevice(c *gin.Context) (*model.Device, bool) {
	v, ok := c.Get(CtxDevice)
	if !ok {
		return nil, false
	}
	d, ok := v.(*model.Device)
	return d, ok
}
