package handler

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/repo"
	"familypoints/server/internal/service"
)

// fail 是全局的错误 → 状态码映射。所有 handler 都走这里，
// 保证「同一类错误在任何接口上的状态码一致」。
//
// 关键约定（4.3 第 2 条）：跨组 / 不存在一律 404，不用 403 —— 403 会泄露资源存在性。
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials),
		errors.Is(err, service.ErrTokenInvalid):
		httpx.Unauthorized(c, err.Error())
	case errors.Is(err, service.ErrAdminDisabled):
		httpx.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrUsernameTaken):
		httpx.Conflict(c, err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		httpx.BadRequest(c, err.Error())
	case errors.Is(err, repo.ErrNotFound):
		httpx.NotFound(c, "资源不存在")
	case errors.Is(err, service.ErrNotInFamily),
		errors.Is(err, service.ErrNotLeader),
		errors.Is(err, service.ErrBanned):
		httpx.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrAlreadyInFamily),
		errors.Is(err, service.ErrLeaderMustTransfer):
		httpx.Conflict(c, err.Error())
	case errors.Is(err, service.ErrInviteCodeInvalid),
		errors.Is(err, service.ErrConfirmRequired),
		errors.Is(err, service.ErrCannotTargetSelf),
		errors.Is(err, service.ErrSameMember),
		errors.Is(err, service.ErrNotEnoughScore):
		httpx.BadRequest(c, err.Error())
	case errors.Is(err, service.ErrAlreadyRevoked):
		httpx.Conflict(c, err.Error())
	default:
		httpx.Internal(c, "服务器内部错误")
	}
}

// logger 返回默认结构化日志器（跨组访问等旁路记录用）。
func logger() *slog.Logger { return slog.Default() }
