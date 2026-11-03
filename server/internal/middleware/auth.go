// Package middleware 提供鉴权中间件。
//
// 两条铁律：
//  1. 两套 token 互不通用 —— /api/admin/* 只认 typ=admin，/api/app/* 只认 typ=member，
//     打错了统一 401（验收 6、7）。
//  2. **角色每次都从数据库读**，不从 token 里取 —— 组长被换掉后旧 token 立即失去组长权限。
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/auth"
	"familypoints/server/internal/httpx"
	"familypoints/server/internal/model"
	"familypoints/server/internal/service"
)

// 上下文键
const (
	CtxAdmin  = "ctx_admin"
	CtxMember = "ctx_member"
)

// Auth 持有 token 管理器与业务服务。
type Auth struct {
	token *auth.Manager
	svc   *service.Auth
}

// NewAuth 构造。
func NewAuth(tm *auth.Manager, svc *service.Auth) *Auth {
	return &Auth{token: tm, svc: svc}
}

// RequireAdmin 校验后台 token。未改密的默认账号只允许访问改密接口。
func (m *Auth) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := m.parse(c, model.SubjectAdmin)
		if !ok {
			return
		}
		admin, err := m.svc.AdminByID(c.Request.Context(), claims.Sub)
		if err != nil {
			httpx.Unauthorized(c, "账号不存在或已失效")
			c.Abort()
			return
		}
		if admin.Disabled {
			httpx.Unauthorized(c, "账号已停用")
			c.Abort()
			return
		}
		// 5.4.2：未改密前除改密接口外全部拒绝
		if admin.MustChangePassword && c.FullPath() != "/api/admin/password" {
			httpx.Fail(c, http.StatusForbidden, "请先修改初始密码")
			c.Abort()
			return
		}
		c.Set(CtxAdmin, admin)
		c.Next()
	}
}

// RequireMember 校验前台 token。
func (m *Auth) RequireMember() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := m.parse(c, model.SubjectMember)
		if !ok {
			return
		}
		member, err := m.svc.MemberByID(c.Request.Context(), claims.Sub)
		if err != nil {
			httpx.Unauthorized(c, "账号不存在或已失效")
			c.Abort()
			return
		}
		c.Set(CtxMember, member)
		c.Next()
	}
}

// RequireFamily 在 RequireMember 之后使用：要求已加入家庭组。
func RequireFamily() gin.HandlerFunc {
	return func(c *gin.Context) {
		m, ok := CurrentMember(c)
		if !ok || !m.InFamily() {
			httpx.Forbidden(c, "请先加入或创建家庭组")
			c.Abort()
			return
		}
		c.Next()
	}
}

// parse 从 Authorization 头解析 token 并校验 typ。
func (m *Auth) parse(c *gin.Context, wantTyp string) (*auth.Claims, bool) {
	raw := bearer(c.GetHeader("Authorization"))
	if raw == "" {
		httpx.Unauthorized(c, "缺少访问令牌")
		c.Abort()
		return nil, false
	}
	claims, err := m.token.ParseAccess(raw, wantTyp)
	if err != nil {
		if errors.Is(err, auth.ErrWrongType) {
			// 拿另一套系统的 token 打本接口：401，且不泄露「这个 token 其实有效」
			httpx.Unauthorized(c, "令牌类型不匹配")
		} else {
			httpx.Unauthorized(c, "令牌无效或已过期")
		}
		c.Abort()
		return nil, false
	}
	return claims, true
}

func bearer(h string) string {
	const prefix = "Bearer "
	if strings.HasPrefix(h, prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// CurrentMember 取当前成员（必须在 RequireMember 之后调用）。
func CurrentMember(c *gin.Context) (*model.Member, bool) {
	v, ok := c.Get(CtxMember)
	if !ok {
		return nil, false
	}
	m, ok := v.(*model.Member)
	return m, ok
}

// CurrentAdmin 取当前管理员。
func CurrentAdmin(c *gin.Context) (*model.Admin, bool) {
	v, ok := c.Get(CtxAdmin)
	if !ok {
		return nil, false
	}
	a, ok := v.(*model.Admin)
	return a, ok
}
