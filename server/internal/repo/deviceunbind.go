// Package repo：设备解绑申请（2026-11-07）的 SQL。
//
// 普通成员解绑设备需组长审批；组长可直接解绑（不走这张表）。
// 过期不设定时任务：pending 不过期（与 family_invitations 不同——设备没有时效性），
// 由组长审批或成员换绑时自然终结；同一人同时最多一条 pending（生成列唯一键兜底）。
package repo

import (
	"context"

	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
)

// DeviceUnbind 封装 device_unbind_requests。
type DeviceUnbind struct{ db *sqlx.DB }

// NewDeviceUnbind 构造。
func NewDeviceUnbind(db *sqlx.DB) *DeviceUnbind { return &DeviceUnbind{db: db} }

const unbindCols = `r.id, r.family_id, r.member_id, r.device_key, r.device_model, r.status,
       r.created_at, r.decided_at, m.nickname AS member_nickname, m.username AS member_username`

// Create 建立一条解绑申请（uk_pending_member 保证一人同时一条 pending，撞键返回 1062）。
func (r *DeviceUnbind) Create(ctx context.Context, familyID, memberID uint64, deviceKey, deviceModel string) (uint64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO device_unbind_requests (family_id, member_id, device_key, device_model)
		 VALUES (?, ?, ?, ?)`, familyID, memberID, deviceKey, deviceModel)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return uint64(id), err
}

// ByID 取一条申请（含申请人昵称）。
func (r *DeviceUnbind) ByID(ctx context.Context, id uint64) (*model.DeviceUnbindRequest, error) {
	var row model.DeviceUnbindRequest
	err := r.db.GetContext(ctx, &row,
		`SELECT `+unbindCols+`
		   FROM device_unbind_requests r JOIN members m ON m.id = r.member_id
		  WHERE r.id = ?`, id)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// MineOf 我发出的解绑申请（含已处理，最新的在前）。
func (r *DeviceUnbind) MineOf(ctx context.Context, memberID uint64) ([]model.DeviceUnbindRequest, error) {
	list := []model.DeviceUnbindRequest{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT `+unbindCols+`
		   FROM device_unbind_requests r JOIN members m ON m.id = r.member_id
		  WHERE r.member_id = ? ORDER BY r.id DESC LIMIT 20`, memberID)
	return list, err
}

// PendingOfFamily 本组待审批的解绑申请（组长看）。
func (r *DeviceUnbind) PendingOfFamily(ctx context.Context, familyID uint64) ([]model.DeviceUnbindRequest, error) {
	list := []model.DeviceUnbindRequest{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT `+unbindCols+`
		   FROM device_unbind_requests r JOIN members m ON m.id = r.member_id
		  WHERE r.family_id = ? AND r.status = 'pending'
		  ORDER BY r.id DESC LIMIT 100`, familyID)
	return list, err
}

// Decide 审批一条申请：置状态 + decided_at。受影响行数为 0 表示已被处理（幂等）。
func (r *DeviceUnbind) Decide(ctx context.Context, familyID, id uint64, approve bool) error {
	status := "rejected"
	if approve {
		status = "approved"
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE device_unbind_requests SET status = ?, decided_at = NOW(3)
		  WHERE id = ? AND family_id = ? AND status = 'pending'`, status, id, familyID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// HasPendingFor 校验某人是否已有 pending（Create 前的友好检查；并发由唯一键兜底）。
func (r *DeviceUnbind) HasPendingFor(ctx context.Context, memberID uint64) (bool, error) {
	var n int
	err := r.db.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM device_unbind_requests WHERE member_id = ? AND status = 'pending'`, memberID)
	return n > 0, err
}

// CancelMine 成员撤回自己的一条 pending 申请。
func (r *DeviceUnbind) CancelMine(ctx context.Context, memberID, id uint64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE device_unbind_requests SET status = 'cancelled', decided_at = NOW(3)
		  WHERE id = ? AND member_id = ? AND status = 'pending'`, id, memberID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolvePendingForDevice 设备被组长直接解绑时，把它的 pending 申请落为 approved
// （设备已经解了，申请自动算同意）。返回是否有改动。
func (r *DeviceUnbind) ResolvePendingForDevice(ctx context.Context, familyID uint64, deviceKey string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE device_unbind_requests SET status = 'approved', decided_at = NOW(3)
		  WHERE family_id = ? AND device_key = ? AND status = 'pending'`, familyID, deviceKey)
	return err
}

// UnbindDevice 删除一条设备绑定（组长直接解绑 / 申请通过时共用）。
// 返回是否真的删了行（设备可能已不存在）。
func (r *Screen) UnbindDevice(ctx context.Context, familyID, memberID uint64, deviceKey string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM member_devices WHERE family_id = ? AND member_id = ? AND device_key = ?`,
		familyID, memberID, deviceKey)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
