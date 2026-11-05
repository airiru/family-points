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
		"created_at":             f.CreatedAt,
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
		"created_at":             f.CreatedAt,
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

// ---------- 邀请码 ----------

// AppInviteCode GET /api/app/family/invite-code —— 组长查看当前邀请码。
func (h *Family) AppInviteCode(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	code, expires, err := h.svc.InviteCode(c.Request.Context(), m)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"invite_code": code, "expires_at": expires})
}

// AppResetInviteCode POST /api/app/family/invite-code —— 重置邀请码（旧码立即失效）。
func (h *Family) AppResetInviteCode(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	code, expires, err := h.svc.ResetInviteCode(c.Request.Context(), m)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"invite_code": code, "expires_at": expires})
}

// AppJoin POST /api/app/family/join —— 凭邀请码加入（需前台 token，且尚未入组）。
func (h *Family) AppJoin(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		InviteCode string `json:"invite_code"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	f, err := h.svc.Join(c.Request.Context(), m, in.InviteCode)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"family_id": f.ID, "family_name": f.Name, "role": "member"})
}

// ---------- 退出 / 踢出（不可逆，必须二次确认）----------

// AppLeave POST /api/app/family/leave —— 自行退出。
//
// 前端必须先弹窗二次确认，再把 confirm=true 传上来；后端不信任「前端已经问过」，
// 没有 confirm 一律 400 —— 这条路径会把积分清零，误触代价太大。
func (h *Family) AppLeave(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.Leave(c.Request.Context(), m, in.Confirm); err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "member.leave", "member", &m.ID, gin.H{"score": m.Score, "frozen": m.FrozenScore}, gin.H{"score": 0, "frozen": 0})
	httpx.OK(c, gin.H{"left": true, "score_reset": true})
}

// AppKick POST /api/app/member/remove —— 组长踢出成员（二次确认）。
func (h *Family) AppKick(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		MemberID uint64 `json:"member_id"`
		Confirm  bool   `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.Kick(c.Request.Context(), m, in.MemberID, in.Confirm); err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "member.kick", "member", &in.MemberID, nil, gin.H{"removed": true})
	httpx.OK(c, gin.H{"removed": true, "member_id": in.MemberID})
}

// ---------- 组长转让 / 隐藏 / 改名 ----------

// AppTransferLeader POST /api/app/family/transfer-leader —— 组长转让。
func (h *Family) AppTransferLeader(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		MemberID uint64 `json:"member_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.TransferLeader(c.Request.Context(), m, in.MemberID); err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "leader.transfer", "member", &in.MemberID, gin.H{"leader_id": m.ID}, gin.H{"leader_id": in.MemberID})
	httpx.OK(c, gin.H{"leader_id": in.MemberID})
}

// AppSetHidden POST /api/app/member/hidden —— 隐藏 / 显示成员。
func (h *Family) AppSetHidden(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		MemberID uint64 `json:"member_id"`
		Hidden   bool   `json:"hidden"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if err := h.svc.SetHidden(c.Request.Context(), m, in.MemberID, in.Hidden); err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "member.hidden", "member", &in.MemberID, nil, gin.H{"hidden": in.Hidden})
	httpx.OK(c, gin.H{"member_id": in.MemberID, "hidden": in.Hidden})
}

// AppUpdateProfile POST /api/app/me/profile —— 改自己的名称 / 头像。
func (h *Family) AppUpdateProfile(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	updated, err := h.svc.UpdateMyProfile(c.Request.Context(), m, in.Name, in.Avatar)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"id": updated.ID, "name": updated.Name, "avatar": updated.Avatar,
		"role": updated.Role, "family_id": updated.FamilyID,
	})
}

// AppRenameFamily POST /api/app/family/rename —— 组长改家庭组名。
func (h *Family) AppRenameFamily(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	name, err := h.svc.RenameFamily(c.Request.Context(), m, in.Name)
	if err != nil {
		fail(c, err)
		return
	}
	recordAudit(c, "family.rename", "family", m.FamilyID, nil, gin.H{"name": name})
	httpx.OK(c, gin.H{"name": name})
}

// AppRenameMember POST /api/app/member/rename —— 改昵称（本人或组长）。
func (h *Family) AppRenameMember(c *gin.Context) {
	m, ok := middleware.CurrentMember(c)
	if !ok {
		httpx.Unauthorized(c, "未登录")
		return
	}
	var in struct {
		MemberID uint64 `json:"member_id"`
		Name     string `json:"name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "请求体格式不正确")
		return
	}
	if in.MemberID == 0 {
		in.MemberID = m.ID // 不传就是改自己
	}
	if err := h.svc.RenameMember(c.Request.Context(), m, in.MemberID, in.Name); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"member_id": in.MemberID, "name": in.Name})
}
