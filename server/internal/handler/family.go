package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/service"
)

// Family 是家庭组 handler。
type Family struct{ svc *service.Family }

// NewFamily 构造。
func NewFamily(svc *service.Family) *Family { return &Family{svc: svc} }

// AppInfo GET /api/app/family —— 只能看自己所在的组。
//
// family_id 来自 token 主体的 members.family_id，**不接受任何请求参数**。
func (h *Family) AppInfo(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok || !m.InFamily() {
		httpx.Forbidden(c, "请先加入或创建家庭组")
		return
	}
	f, err := h.svc.Info(c.Request.Context(), *m.FamilyID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"id":                     f.ID,
		"name":                   f.Name,
		"invite_code":            f.InviteCode,
		"invite_code_expires_at": f.InviteCodeExpiresAt,
	})
}

// AppMembers GET /api/app/family/members —— 本组成员，过滤被隐藏的。
func (h *Family) AppMembers(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok || !m.InFamily() {
		httpx.Forbidden(c, "请先加入或创建家庭组")
		return
	}
	list, err := h.svc.Members(c.Request.Context(), *m.FamilyID, false)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"members": list})
}

// AdminInfo GET /api/admin/family/:family_id —— 管理员跨组查看。
//
// 这是**唯一**允许从请求中取 family_id 的入口（4.3 第 3 条）：
// 必须是路径参数，且要写审计日志（审计在 P8 实现，此处先记应用日志）。
func (h *Family) AdminInfo(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	f, err := h.svc.Info(c.Request.Context(), fid)
	if err != nil {
		fail(c, err)
		return
	}
	auditCrossFamily(c, "admin.family.view", fid)
	httpx.OK(c, gin.H{
		"id":                     f.ID,
		"name":                   f.Name,
		"invite_code":            f.InviteCode,
		"invite_code_expires_at": f.InviteCodeExpiresAt,
	})
}

// AdminMembers GET /api/admin/family/:family_id/members —— 管理员看全部（含隐藏）。
func (h *Family) AdminMembers(c *gin.Context) {
	fid, ok := pathFamilyID(c)
	if !ok {
		return
	}
	list, err := h.svc.Members(c.Request.Context(), fid, true)
	if err != nil {
		fail(c, err)
		return
	}
	auditCrossFamily(c, "admin.family.members", fid)
	httpx.OK(c, gin.H{"members": list})
}

// pathFamilyID 解析路径里的 family_id。非法值按「资源不存在」处理（404），
// 不返回 400 —— 避免通过错误码差异探测 id 是否合法。
func pathFamilyID(c *gin.Context) (uint64, bool) {
	raw := c.Param("family_id")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		httpx.NotFound(c, "资源不存在")
		return 0, false
	}
	return id, true
}

// auditCrossFamily 记录管理员跨组访问。
//
// P8 会替换成写 audit_logs；现在先落到应用日志，保证「跨组操作可追溯」这条不被跳过。
func auditCrossFamily(c *gin.Context, action string, familyID uint64) {
	admin, _ := middleware.CurrentAdmin(c)
	var adminID uint64
	var username string
	if admin != nil {
		adminID, username = admin.ID, admin.Username
	}
	logger().Info("管理员跨组访问",
		"action", action, "admin_id", adminID, "admin", username,
		"family_id", familyID, "path", c.FullPath())
}
