// Package httpx 统一 HTTP 响应格式。
//
// 全站响应形如 { "ok": true, "data": ... } 或 { "ok": false, "error": "..." }，
// 与 docs/features.md 第 15 节一致。任何 handler 都必须走这两个函数，
// 不要直接 c.JSON —— 否则前端要处理多种形状。
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope 是统一响应体。
type Envelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// OK 返回 200 与数据。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{OK: true, Data: data})
}

// Fail 返回指定状态码与错误文案。
func Fail(c *gin.Context, status int, msg string) {
	c.JSON(status, Envelope{OK: false, Error: msg})
}

// 常用错误快捷方式：跨组访问一律 404（不泄露资源是否存在），见约定第 9 条。
func BadRequest(c *gin.Context, msg string) { Fail(c, http.StatusBadRequest, msg) }
func Unauthorized(c *gin.Context, msg string) { Fail(c, http.StatusUnauthorized, msg) }
func Forbidden(c *gin.Context, msg string)  { Fail(c, http.StatusForbidden, msg) }
func NotFound(c *gin.Context, msg string)   { Fail(c, http.StatusNotFound, msg) }
func Conflict(c *gin.Context, msg string)   { Fail(c, http.StatusConflict, msg) }
func Internal(c *gin.Context, msg string)   { Fail(c, http.StatusInternalServerError, msg) }
