// Package audit 是操作审计的写入端。
//
// 三条铁律（17 条）：
//  1. **append-only**：只提供写入，不提供 UPDATE / DELETE
//  2. **必须脱敏**：before/after 里绝不能出现 pass_hash
//  3. **写入失败不得阻断主流程**：审计是旁路，失败只记应用日志
package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"familypoints/server/internal/repo"
)

// Entry 是一条待写入的审计记录。
type Entry struct {
	FamilyID     *uint64
	OperatorID   *uint64
	OperatorType string // member / device / system
	Action       string // 如 'item.redeem' / 'task.approve'
	TargetType   string
	TargetID     *uint64
	Before       any
	After        any
	IP           string
}

// Writer 把审计写进 audit_logs。
type Writer struct{ repo *repo.Audit }

// New 构造。
func New(r *repo.Audit) *Writer { return &Writer{repo: r} }

// Write 写一条审计。**永不返回错误**——调用方不需要处理，也不该因此中断业务。
func (w *Writer) Write(ctx context.Context, e Entry) {
	if w == nil || w.repo == nil {
		return
	}
	if e.OperatorType == "" {
		e.OperatorType = "member"
	}
	err := w.repo.Insert(ctx, repo.AuditRow{
		FamilyID:     e.FamilyID,
		OperatorID:   e.OperatorID,
		OperatorType: e.OperatorType,
		Action:       e.Action,
		TargetType:   e.TargetType,
		TargetID:     e.TargetID,
		BeforeJSON:   sanitizeJSON(e.Before),
		AfterJSON:    sanitizeJSON(e.After),
		IP:           e.IP,
	})
	if err != nil {
		slog.Error("审计写入失败（已忽略，不影响主流程）", "action", e.Action, "err", err)
	}
}

// 敏感字段：写进可查询的表就等于泄露
var sensitiveKeys = map[string]bool{
	"pass_hash":            true,
	"password":             true,
	"old_password":         true,
	"new_password":         true,
	"token":                true,
	"refresh_token":        true,
	"token_hash":           true,
	"jwt_secret":           true,
	"must_change_password": false, // 布尔标记本身不敏感，保留便于排查
}

// sanitizeJSON 把任意值转成 JSON，并剔除敏感字段。
func sanitizeJSON(v any) *string {
	if v == nil {
		return nil
	}
	// 先转成通用结构再清理，避免结构体里藏着哈希
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil
	}
	cleaned := stripSensitive(generic)
	out, err := json.Marshal(cleaned)
	if err != nil {
		return nil
	}
	s := string(out)
	return &s
}

func stripSensitive(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k := range val {
			if sensitiveKeys[k] {
				delete(val, k)
				continue
			}
			val[k] = stripSensitive(val[k])
		}
		return val
	case []any:
		for i := range val {
			val[i] = stripSensitive(val[i])
		}
		return val
	default:
		return v
	}
}
