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
func Build(env string, db *sqlx.DB, authHandler *handler.Auth, familyHandler *handler.Family,
	ledgerHandler *handler.Ledger, screenHandler *handler.Screen, taskHandler *handler.Task,
	reportHandler *handler.Report, auditHandler *handler.Audit,
	notifHandler *handler.Notification, adminHandler *handler.Admin,
	couponHandler *handler.Coupon,
	mw *middleware.Auth) *gin.Engine {
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

		// 前台登出：只要 access 有效就能调，不要求已入组
		appAuth := r.Group("/api/app", mw.RequireMember())
		appAuth.POST("/logout", authHandler.LogoutMember)
	}

	// 后台：只认 admin token
	admin := r.Group("/api/admin", mw.RequireAdmin())
	{
		admin.GET("/me", authHandler.MeAdmin)
		admin.POST("/logout", authHandler.LogoutAdmin)
		admin.POST("/password", authHandler.ChangeAdminPassword)
		// 管理员资料：只改昵称，username 永不可改
		admin.POST("/me/profile", authHandler.UpdateAdminProfile)

		// 管理员跨组：family_id 只能走路径参数（4.3 第 3 条）
		admin.GET("/family/:family_id", familyHandler.AdminInfo)
		admin.GET("/family/:family_id/members", familyHandler.AdminMembers)

		// 屏幕时间上报已改为「成员登录态 + 客户端自报 device_key」，不再有设备令牌（2026-11-05）。

		// 审计日志（append-only，只读查询）
		admin.GET("/audit/list", auditHandler.AdminList)

		// 全站视图与后台管理（family_id 只能走路径参数）
		admin.GET("/families", adminHandler.Families)
		admin.GET("/state", adminHandler.State)
		admin.POST("/family/:family_id/invite", adminHandler.Invite)
		admin.POST("/family/:family_id/member/kick", adminHandler.Kick)
		admin.POST("/family/:family_id/member/hidden", adminHandler.SetHidden)
		admin.POST("/family/:family_id/member/leader", adminHandler.TransferLeader)
		admin.POST("/family/:family_id/rename", adminHandler.RenameFamily)
		admin.POST("/family/:family_id/rule/save", adminHandler.SaveRule)
		admin.POST("/family/:family_id/item/save", adminHandler.SaveItem)
		admin.GET("/export/records", adminHandler.ExportRecords)

		// 只读视图（2026-11-04 补：后台 12 个页面需要读到现值）
		admin.GET("/family/:family_id/rule", adminHandler.Rule)
		admin.GET("/family/:family_id/items", adminHandler.Items)
		admin.GET("/family/:family_id/tasks", adminHandler.Tasks)
		admin.GET("/family/:family_id/records", adminHandler.Records)
		admin.GET("/family/:family_id/transfers", adminHandler.Transfers)
		admin.GET("/family/:family_id/screen-reports", adminHandler.ScreenReports)
		admin.GET("/family/:family_id/screen-device-reports", adminHandler.ScreenDeviceReports)
		admin.GET("/family/:family_id/stats/ranking", adminHandler.Ranking)
		admin.GET("/family/:family_id/stats/trend", adminHandler.Trend)

		// 管理员账号
		admin.GET("/accounts", adminHandler.ListAdmins)
		admin.POST("/accounts", adminHandler.CreateAdmin)
		admin.POST("/accounts/disable", adminHandler.DisableAdmin)
	}

	// 前台：只认 member token；组内接口额外要求已加入家庭组
	app := r.Group("/api/app", mw.RequireMember())
	{
		app.GET("/me", authHandler.MeMember)
		app.POST("/password", authHandler.ChangeMemberPassword)

		// 加入家庭组：此时还没入组，不能挂在 inFamily 下
		app.POST("/family/join", familyHandler.AppJoin)
		// 未入组者的两条路：凭家庭码搜索加入 / 自己建一个家
		app.GET("/family/search", familyHandler.AppSearchFamily)
		app.POST("/family/create", familyHandler.AppCreateFamily)
		// 凭码申请加入 + 查看自己的申请状态（都要求未入组，挂在 app 组）
		app.POST("/family/request", familyHandler.AppRequestJoin)
		app.GET("/family/requests/mine", familyHandler.AppMyJoinRequests)

		// P3.1 待确认邀请（被邀请人侧）：未入组也必须能查、能确认/拒绝
		app.GET("/invitations", familyHandler.AppMyInvitations)
		app.POST("/invitation/accept", familyHandler.AppAcceptInvitation)
		app.POST("/invitation/reject", familyHandler.AppRejectInvitation)
	}

	// 组内接口：family_id 一律取自 token 主体，不接受任何请求参数
	inFamily := app.Group("", middleware.RequireFamily())
	{
		inFamily.GET("/family", familyHandler.AppInfo)
		inFamily.GET("/family/members", familyHandler.AppMembers)
		inFamily.POST("/family/rename", familyHandler.AppRenameFamily)

		// 邀请码：查看 / 重置（组长）
		// 家庭码：全员可读（不是密钥），只有「改隐藏」是组长权限
		inFamily.GET("/family/code", familyHandler.AppInviteCode)
		inFamily.POST("/family/code/hidden", familyHandler.AppSetCodeHidden)
		// 入组申请的审批（组长）
		inFamily.GET("/family/requests", familyHandler.AppPendingJoinRequests)
		inFamily.POST("/family/request/decide", familyHandler.AppDecideJoinRequest)

		// P3.1 待确认邀请（组长侧）：发起 / 查看本组 / 取消
		inFamily.POST("/member/invite", familyHandler.AppInviteByUsername)
		inFamily.GET("/member/invitations", familyHandler.AppFamilyInvitations)
		inFamily.POST("/member/invite/cancel", familyHandler.AppCancelInvitation)

		// 不可逆操作：均要求 confirm=true
		inFamily.POST("/family/leave", familyHandler.AppLeave)
		inFamily.POST("/member/remove", familyHandler.AppKick)

		// 组长转让 / 成员管理
		inFamily.POST("/family/transfer-leader", familyHandler.AppTransferLeader)
		inFamily.POST("/member/hidden", familyHandler.AppSetHidden)
		inFamily.POST("/member/rename", familyHandler.AppRenameMember)
		// 个人资料：名称 / 头像（2026-11-05）
		inFamily.POST("/me/profile", familyHandler.AppUpdateProfile)

		// 商城
		inFamily.GET("/item/list", ledgerHandler.AppItems)
		inFamily.POST("/item/save", ledgerHandler.AppSaveItem)
		inFamily.POST("/item/delete", ledgerHandler.AppDeleteItem)
		inFamily.POST("/item/redeem", ledgerHandler.AppRedeem)

		// 赠送
		inFamily.POST("/transfer/send", ledgerHandler.AppSendTransfer)
		inFamily.GET("/transfer/list", ledgerHandler.AppTransfers)

		// 概览 / 记录 / 榜单 / 趋势
		inFamily.GET("/state", reportHandler.AppState)
		inFamily.GET("/records", reportHandler.AppRecords)
		inFamily.GET("/stats/ranking", reportHandler.AppRanking)
		inFamily.GET("/stats/trend", reportHandler.AppTrend)

		// 睡觉规则与屏幕时间配置
		inFamily.GET("/rule", screenHandler.AppRules)
		inFamily.POST("/rule/save", screenHandler.AppSaveRule)
		inFamily.GET("/screen-rule", screenHandler.AppScreenRules)
		inFamily.POST("/screen-rule/save", screenHandler.AppSaveScreenRule)
		inFamily.GET("/screen/reports", screenHandler.AppScreenReports)
		// 上报改用成员登录态，不再需要设备令牌（2026-11-05）
		inFamily.POST("/screen/report", screenHandler.AppReport)
		inFamily.GET("/screen/family", screenHandler.AppFamilyDay)
		// 今日实时快照（只落明细、不结算）与成员明细（含逐小时 / 各设备 / 近 7 天）
		inFamily.POST("/screen/snapshot", screenHandler.AppSnapshot)
		inFamily.GET("/screen/member/:member_id", screenHandler.AppMemberDetail)
		inFamily.GET("/screen/devices", screenHandler.AppDevices)

		// 任务：发布（冻结）→ 接取 → 提交 → 全员投票
		inFamily.POST("/task/create", taskHandler.AppCreateTask)
		inFamily.GET("/task/list", taskHandler.AppTasks)
		inFamily.GET("/task/detail", taskHandler.AppTaskDetail)
		inFamily.POST("/task/claim", taskHandler.AppClaimTask)
		inFamily.POST("/task/abandon", taskHandler.AppAbandonTask)
		inFamily.POST("/task/close", taskHandler.AppCloseTask)
		inFamily.POST("/task/submit", taskHandler.AppSubmitTask)
		inFamily.POST("/task/vote", taskHandler.AppVoteTask)

		// 商品券与核销投票（docs 9.6）
		inFamily.GET("/coupons", couponHandler.AppList)
		inFamily.GET("/coupons/pending", couponHandler.AppPending)
		inFamily.GET("/coupon/detail", couponHandler.AppDetail)
		inFamily.POST("/coupon/use", couponHandler.AppUse)
		inFamily.POST("/coupon/vote", couponHandler.AppVote)

		// 站内通知
		inFamily.GET("/notification/list", notifHandler.AppList)
		inFamily.POST("/notification/read", notifHandler.AppRead)
	}

	return r
}
