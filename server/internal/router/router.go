// Package router 组装路由。
//
// 前缀与鉴权严格对应（15 节）：
//   - /api/admin/* 只认 typ=admin
//   - /api/app/*   只认 typ=member
//   - /api/register、两个 login、两个 refresh 为公开接口
package router

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/handler"
	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
)

// Build 组装 gin 引擎。
func Build(env string, db *sqlx.DB, authHandler *handler.Auth, familyHandler *handler.Family, mw *middleware.Auth) *gin.Engine {
	if env != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	// 健康检查：直接 Ping 库，能反映「服务活着但库挂了」这种最常见的故障
	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			httpx.Fail(c, http.StatusServiceUnavailable, "数据库不可用")
			return
		}
		httpx.OK(c, gin.H{"env": env, "db": "up"})
	})

	// 公开接口
	api := r.Group("/api")
	{
		api.POST("/register", authHandler.Register)
		api.POST("/admin/login", authHandler.LoginAdmin)
		api.POST("/admin/refresh", authHandler.RefreshAdmin)
		api.POST("/app/login", authHandler.LoginMember)
		api.POST("/app/refresh", authHandler.RefreshMember)
	}

	// 后台：只认 admin token
	admin := r.Group("/api/admin", mw.RequireAdmin())
	{
		admin.GET("/me", authHandler.MeAdmin)
		admin.POST("/password", authHandler.ChangeAdminPassword)

		// 管理员跨组：family_id 只能走路径参数（4.3 第 3 条）
		admin.GET("/family/:family_id", familyHandler.AdminInfo)
		admin.GET("/family/:family_id/members", familyHandler.AdminMembers)
	}

	// 前台：只认 member token；组内接口额外要求已加入家庭组
	app := r.Group("/api/app", mw.RequireMember())
	{
		app.GET("/me", authHandler.MeMember)
		app.POST("/password", authHandler.ChangeMemberPassword)

		// 加入家庭组：此时还没入组，不能挂在 inFamily 下
		app.POST("/family/join", familyHandler.AppJoin)
	}

	// 组内接口：family_id 一律取自 token 主体，不接受任何请求参数
	inFamily := app.Group("", middleware.RequireFamily())
	{
		inFamily.GET("/family", familyHandler.AppInfo)
		inFamily.GET("/family/members", familyHandler.AppMembers)
		inFamily.POST("/family/rename", familyHandler.AppRenameFamily)

		// 邀请码：查看 / 重置（组长）
		inFamily.GET("/family/invite-code", familyHandler.AppInviteCode)
		inFamily.POST("/family/invite-code", familyHandler.AppResetInviteCode)

		// 不可逆操作：均要求 confirm=true
		inFamily.POST("/family/leave", familyHandler.AppLeave)
		inFamily.POST("/member/remove", familyHandler.AppKick)

		// 组长转让 / 成员管理
		inFamily.POST("/family/transfer-leader", familyHandler.AppTransferLeader)
		inFamily.POST("/member/hidden", familyHandler.AppSetHidden)
		inFamily.POST("/member/rename", familyHandler.AppRenameMember)
	}

	return r
}
