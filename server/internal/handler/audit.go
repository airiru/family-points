package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/audit"
	"familypoints/server/internal/httpx"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/repo"
)

// auditor 是进程级的审计写入器（横切关注点，不想穿透到每个 handler 的构造参数里）。
var auditor *audit.Writer

// SetAuditor 在启动时注入。
func SetAuditor(w *audit.Writer) { auditor = w }

// recordAudit 记录一条审计。
//
// 调用约定：**在业务成功后**调用，失败路径不记；写入失败由 audit.Writer 吞掉，
// 不影响已经成功的业务（17 条铁律 ③）。
func recordAudit(c *gin.Context, action, targetType string, targetID *uint64, before, after any) {
	if auditor == nil {
		return
	}
	e := audit.Entry{
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Before:     before,
		After:      after,
		IP:         c.ClientIP(),
	}
	if m, ok := middleware.CurrentMember(c); ok && m.InFamily() {
		e.FamilyID = m.FamilyID
		id := m.ID
		e.OperatorID = &id
		e.OperatorType = "member"
	}
	if a, ok := middleware.CurrentAdmin(c); ok {
		id := a.ID
		e.OperatorID = &id
		e.OperatorType = "system" // 管理员是运营者，不属于家庭组
		if fid, ok := c.Get("audit_family_id"); ok {
			if v, ok := fid.(uint64); ok {
				e.FamilyID = &v
			}
		}
	}
	auditor.Write(c.Request.Context(), e)
}

// recordAuditAs 在**公开接口**里显式指定主体写审计。
//
// 登录时 JWT 中间件还没跑，CurrentMember / CurrentAdmin 都取不到，
// recordAudit 会写出一条 operator_id 为空的记录。这里由 handler 用刚认证
// 通过的主体构造 Entry；脱敏、旁路、append-only 等约定与 recordAudit 完全一致。
// familyID 用服务端查出来的成员归属，绝不从请求体里取。
func recordAuditAs(c *gin.Context, action, targetType string, targetID *uint64,
	operatorID uint64, operatorType string, familyID *uint64, after any) {
	if auditor == nil {
		return
	}
	id := operatorID
	auditor.Write(c.Request.Context(), audit.Entry{
		FamilyID:     familyID,
		OperatorID:   &id,
		OperatorType: operatorType,
		Action:       action,
		TargetType:   targetType,
		TargetID:     targetID,
		After:        after,
		IP:           c.ClientIP(),
	})
}

// Audit 是审计查询 handler。
type Audit struct{ repo *repo.Audit }

// NewAudit 构造。
func NewAudit(r *repo.Audit) *Audit { return &Audit{repo: r} }

// AdminList GET /api/admin/audit/list —— 跨组查询审计（仅管理员）。
func (h *Audit) AdminList(c *gin.Context) {
	familyID, _ := strconv.ParseUint(c.DefaultQuery("family_id", "0"), 10, 64)
	operator, _ := strconv.ParseUint(c.DefaultQuery("operator_id", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	f := repo.AuditFilter{
		FamilyID: familyID, Action: c.Query("action"), Operator: operator,
		Limit: limit, Offset: offset,
	}
	if from := c.Query("from"); from != "" {
		t, err := time.ParseInLocation("2006-01-02", from, time.Local)
		if err != nil {
			httpx.BadRequest(c, "from 需为 YYYY-MM-DD")
			return
		}
		f.From = t
	}
	if to := c.Query("to"); to != "" {
		t, err := time.ParseInLocation("2006-01-02", to, time.Local)
		if err != nil {
			httpx.BadRequest(c, "to 需为 YYYY-MM-DD")
			return
		}
		f.To = t.AddDate(0, 0, 1)
	}

	list, total, err := h.repo.List(c.Request.Context(), f)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": list, "total": total, "limit": limit, "offset": offset})
}
