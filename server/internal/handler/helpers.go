package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
)

// itoa 是 strconv.Itoa 的短别名，handler 里拼文案用。
func itoa(n int) string { return strconv.Itoa(n) }

// queryID 从 query 里取正整数 id；缺失或非法按「资源不存在」返回 404。
func queryID(c *gin.Context, key string) (uint64, bool) {
	raw := c.Query(key)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		httpx.NotFound(c, "资源不存在")
		return 0, false
	}
	return id, true
}

// bodyID 从 JSON 请求体里取正整数 id（只读一个字段，便于小接口省掉结构体）。
func bodyID(c *gin.Context, key string) (uint64, bool) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return 0, false
	}
	raw, ok := body[key]
	if !ok {
		httpx.BadRequest(c, "缺少 "+key)
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		if v <= 0 {
			httpx.BadRequest(c, key+" 必须为正整数")
			return 0, false
		}
		return uint64(v), true
	default:
		httpx.BadRequest(c, key+" 必须为数字")
		return 0, false
	}
}
