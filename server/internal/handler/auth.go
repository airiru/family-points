// Package handler 是 HTTP 层：只做参数绑定、调用 service、映射响应。
// 业务判断一律不写在这里。
package handler

import (
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
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"member": gin.H{
			"id":        member.ID,
			"username":  member.Username,
			"nickname":  member.Nickname,
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
		fail(c, err)
		return
	}
	// 登录是公开接口，中间件主体还没注入，须显式传；失败路径不记审计。
	aid := admin.ID
	recordAuditAs(c, "auth.login", "admin", &aid, aid, "system", nil,
		gin.H{"username": admin.Username})
	httpx.OK(c, gin.H{
		"admin": gin.H{
			"id":       admin.ID,
			"username": admin.Username,
			"nickname": admin.Nickname,
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
		fail(c, err)
		return
	}
	// 同上：显式用刚认证通过的主体，family_id 取服务端查到的归属（可为 nil）。
	mid := member.ID
	recordAuditAs(c, "auth.login", "member", &mid, mid, "member", member.FamilyID,
		gin.H{"username": member.Username})
	httpx.OK(c, gin.H{
		"member": gin.H{
			"id":        member.ID,
			"username":  member.Username,
			"nickname":  member.Nickname,
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
		fail(c, err)
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
		fail(c, err)
		return
	}
	// 改密是受保护接口，中间件已注入主体，直接复用 recordAudit。
	// 不往 before/after 里放任何密码字段：after 只记结果标记。
	if m, ok := middleware.CurrentMember(c); ok {
		id := m.ID
		recordAudit(c, "auth.password", "member", &id, nil, gin.H{"relogin_required": true})
	} else if a, ok := middleware.CurrentAdmin(c); ok {
		id := a.ID
		recordAudit(c, "auth.password", "admin", &id, nil, gin.H{"relogin_required": true})
	}
	// 改密后旧 refresh 全部失效，客户端必须重新登录
	httpx.OK(c, gin.H{"relogin_required": true})
}

// LogoutAdmin POST /api/admin/logout —— 吊销该管理员全部 refresh token。
func (h *Auth) LogoutAdmin(c *gin.Context) {
	a, ok := middleware.CurrentAdmin(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	if err := h.svc.Logout(c.Request.Context(), model.SubjectAdmin, a.ID); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"revoked": true})
}

// LogoutMember POST /api/app/logout —— 吊销该成员全部 refresh token。
func (h *Auth) LogoutMember(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	if err := h.svc.Logout(c.Request.Context(), model.SubjectMember, m.ID); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"revoked": true})
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
		"nickname":             admin.Nickname,
		"must_change_password": admin.MustChangePassword,
	})
}

// UpdateAdminProfile POST /api/admin/me/profile —— 只改昵称。
//
// username 是登录凭证，**永不可改**：若请求体带了 username（无论值是什么）一律 400，
// 而不是静默忽略 —— 让「不可改」成为可验证的契约，也防止将来有人误加写路径。
func (h *Auth) UpdateAdminProfile(c *gin.Context) {
	admin, ok := middleware.CurrentAdmin(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		Nickname string `json:"nickname"`
		Username string `json:"username"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if in.Username != "" {
		httpx.BadRequest(c, "用户名注册后不可修改")
		return
	}
	if err := h.svc.UpdateAdminNickname(c.Request.Context(), admin.ID, in.Nickname); err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "auth.profile", "admin", &admin.ID, nil, gin.H{"nickname": in.Nickname})
	httpx.OK(c, gin.H{"id": admin.ID, "username": admin.Username, "nickname": in.Nickname})
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
		"nickname":     m.Nickname,
		"role":         m.Role,
		"family_id":    m.FamilyID,
		"score":        m.Score,
		"frozen_score": m.FrozenScore,
		"avatar":       m.Avatar,
	})
}
