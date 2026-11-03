// Package handler 是 HTTP 层：只做参数绑定、调用 service、映射响应。
// 业务判断一律不写在这里。
package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/model"
	"familypoints/server/internal/service"
)

// Auth 是认证相关 handler。
type Auth struct{ svc *service.Auth }

// NewAuth 构造。
func NewAuth(svc *service.Auth) *Auth { return &Auth{svc: svc} }

// Register POST /api/register
func (h *Auth) Register(c *gin.Context) {
	var in service.RegisterInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	member, pair, err := h.svc.Register(c.Request.Context(), in)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"member": gin.H{
			"id":        member.ID,
			"username":  member.Username,
			"name":      member.Name,
			"role":      member.Role,
			"family_id": member.FamilyID,
		},
		"token": pair,
	})
}

// LoginAdmin POST /api/admin/login
func (h *Auth) LoginAdmin(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	admin, pair, err := h.svc.LoginAdmin(c.Request.Context(), in.Username, in.Password)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"admin": gin.H{
			"id":       admin.ID,
			"username": admin.Username,
		},
		"token": pair,
	})
}

// LoginMember POST /api/app/login
func (h *Auth) LoginMember(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	member, pair, err := h.svc.LoginMember(c.Request.Context(), in.Username, in.Password)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"member": gin.H{
			"id":        member.ID,
			"username":  member.Username,
			"name":      member.Name,
			"role":      member.Role,
			"family_id": member.FamilyID,
		},
		"token": pair,
	})
}

// RefreshAdmin POST /api/admin/refresh
func (h *Auth) RefreshAdmin(c *gin.Context) {
	h.refresh(c, model.SubjectAdmin)
}

// RefreshMember POST /api/app/refresh
func (h *Auth) RefreshMember(c *gin.Context) {
	h.refresh(c, model.SubjectMember)
}

func (h *Auth) refresh(c *gin.Context, subjectType string) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), in.RefreshToken, subjectType)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, pair)
}

// ChangeAdminPassword POST /api/admin/password
func (h *Auth) ChangeAdminPassword(c *gin.Context) {
	admin, ok := middleware.CurrentAdmin(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	h.changePassword(c, func(oldPwd, newPwd string) error {
		return h.svc.ChangeAdminPassword(c.Request.Context(), admin.ID, oldPwd, newPwd)
	})
}

// ChangeMemberPassword POST /api/app/password
func (h *Auth) ChangeMemberPassword(c *gin.Context) {
	member, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	h.changePassword(c, func(oldPwd, newPwd string) error {
		return h.svc.ChangeMemberPassword(c.Request.Context(), member.ID, oldPwd, newPwd)
	})
}

func (h *Auth) changePassword(c *gin.Context, do func(oldPwd, newPwd string) error) {
	var in struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := do(in.OldPassword, in.NewPassword); err != nil {
		h.fail(c, err)
		return
	}
	// 改密后旧 refresh 全部失效，客户端必须重新登录
	httpx.OK(c, gin.H{"relogin_required": true})
}

// MeAdmin GET /api/admin/me
func (h *Auth) MeAdmin(c *gin.Context) {
	admin, ok := middleware.CurrentAdmin(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	httpx.OK(c, gin.H{
		"id":                   admin.ID,
		"username":             admin.Username,
		"must_change_password": admin.MustChangePassword,
	})
}

// MeMember GET /api/app/me
func (h *Auth) MeMember(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	httpx.OK(c, gin.H{
		"id":           m.ID,
		"username":     m.Username,
		"name":         m.Name,
		"role":         m.Role,
		"family_id":    m.FamilyID,
		"score":        m.Score,
		"frozen_score": m.FrozenScore,
	})
}

// fail 把 service 错误映射成 HTTP 状态码。跨组/不存在一律 404（约定 9）。
func (h *Auth) fail(c *gin.Context, err error) {
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
	default:
		httpx.Internal(c, "服务器内部错误")
	}
}
