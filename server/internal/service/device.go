// Package service：设备绑定与解绑审批（2026-11-07）。
//
// 绑定语义：一台设备（device_key）全局只能绑定一个账号；
// 上报时未绑定 → 自动绑定上报者；绑定他人 → repo.ErrDeviceBound（409）。
// 解绑语义：组长可直接解绑本组任何设备；普通成员对自己的设备发起申请，
// 组长审批通过才解绑（防「孩子自己解绑逃避统计」）。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"

	"familypoints/server/internal/model"
	"familypoints/server/internal/notify"
	"familypoints/server/internal/repo"
)

// ErrUnbindPending 同一成员同时只能有一条解绑申请。
var ErrUnbindPending = errors.New("你已有一条待审批的解绑申请")

// Device 封装设备绑定 / 解绑审批。
type Device struct {
	screen   *repo.Screen
	families *repo.Family
	unbinds  *repo.DeviceUnbind
	notifier *notify.Service // 旁路，允许为 nil
}

// NewDevice 构造。
func NewDevice(s *repo.Screen, f *repo.Family, u *repo.DeviceUnbind, n *notify.Service) *Device {
	return &Device{screen: s, families: f, unbinds: u, notifier: n}
}

func requireDeviceKey(deviceKey string) (string, error) {
	deviceKey = strings.TrimSpace(deviceKey)
	if deviceKey == "" {
		return "", fmt.Errorf("%w：缺少 device_key", ErrInvalidInput)
	}
	return deviceKey, nil
}

// MyDevices 我绑定的设备（设备管理页列表）。
func (s *Device) MyDevices(ctx context.Context, me *model.Member) ([]model.MemberDevice, error) {
	return s.screen.DevicesOfMember(ctx, me.ID)
}

// requestUnbind 普通成员：对自己的设备发起解绑申请（组长审批）。
func (s *Device) RequestUnbind(ctx context.Context, me *model.Member, deviceKey string) (*model.DeviceUnbindRequest, error) {
	deviceKey, err := requireDeviceKey(deviceKey)
	if err != nil {
		return nil, err
	}
	if me.Role == model.RoleLeader {
		return nil, ErrNotLeader // 组长不需要申请，走直接解绑接口
	}
	// 只能申请解绑**绑定在自己名下**的设备
	if !me.InFamily() {
		return nil, ErrNotInFamily
	}
	devices, err := s.screen.DevicesOfMember(ctx, me.ID)
	if err != nil {
		return nil, err
	}
	deviceModel := ""
	found := false
	for _, d := range devices {
		if d.DeviceKey == deviceKey {
			found, deviceModel = true, d.DeviceModel
			break
		}
	}
	if !found {
		return nil, repo.ErrNotFound
	}
	if pending, err := s.unbinds.HasPendingFor(ctx, me.ID); err != nil {
		return nil, err
	} else if pending {
		return nil, ErrUnbindPending
	}
	id, err := s.unbinds.Create(ctx, *me.FamilyID, me.ID, deviceKey, deviceModel)
	if err != nil {
		var me2 *mysql.MySQLError
		if errors.As(err, &me2) && me2.Number == 1062 {
			return nil, ErrUnbindPending
		}
		return nil, err
	}
	// 通知组长（旁路：失败只记日志）
	if s.notifier != nil {
		if leaderID, e := s.families.LeaderID(ctx, *me.FamilyID); e == nil {
			s.notifier.UnbindRequested(ctx, *me.FamilyID, leaderID, me.ID, me.Nickname, deviceModel)
		}
	}
	return s.unbinds.ByID(ctx, id)
}

// UnbindDirect 组长：直接解绑本组任何成员的设备（含自己的），不需要申请。
func (s *Device) UnbindDirect(ctx context.Context, me *model.Member, deviceKey string) (map[string]any, error) {
	deviceKey, err := requireDeviceKey(deviceKey)
	if err != nil {
		return nil, err
	}
	if me.Role != model.RoleLeader || !me.InFamily() {
		return nil, ErrNotLeader
	}
	// 归属必须在本组：按 (family_id, device_key) 删，顺带取 member_id 通知
	var ownerID uint64
	var ownerModel string
	err = s.screen.DeviceInfoInFamily(ctx, *me.FamilyID, deviceKey, &ownerID, &ownerModel)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, repo.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	removed, err := s.screen.UnbindDevice(ctx, *me.FamilyID, ownerID, deviceKey)
	if err != nil {
		return nil, err
	}
	// 该设备上的 pending 申请视为已处理（设备都解了）
	_ = s.unbinds.ResolvePendingForDevice(ctx, *me.FamilyID, deviceKey)
	if s.notifier != nil && ownerID != me.ID {
		s.notifier.UnbindDecided(ctx, *me.FamilyID, ownerID, ownerModel, true)
	}
	return map[string]any{"unbound": removed, "device_key": deviceKey}, nil
}

// MyUnbindRequests 我发出的解绑申请（设备管理页显示状态）。
func (s *Device) MyUnbindRequests(ctx context.Context, me *model.Member) ([]model.DeviceUnbindRequest, error) {
	return s.unbinds.MineOf(ctx, me.ID)
}

// PendingUnbindRequests 本组待审批的解绑申请（组长设备管理页）。
func (s *Device) PendingUnbindRequests(ctx context.Context, me *model.Member) ([]model.DeviceUnbindRequest, error) {
	if me.Role != model.RoleLeader || !me.InFamily() {
		return nil, ErrNotLeader
	}
	return s.unbinds.PendingOfFamily(ctx, *me.FamilyID)
}

// DecideUnbind 组长审批解绑申请：同意 → 立即解绑。
func (s *Device) DecideUnbind(ctx context.Context, me *model.Member, requestID uint64, approve bool) (*model.DeviceUnbindRequest, error) {
	if me.Role != model.RoleLeader || !me.InFamily() {
		return nil, ErrNotLeader
	}
	if requestID == 0 {
		return nil, fmt.Errorf("%w：缺少 request_id", ErrInvalidInput)
	}
	req, err := s.unbinds.ByID(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if req.FamilyID != *me.FamilyID {
		return nil, repo.ErrNotFound // 不泄露别组的申请
	}
	if err := s.unbinds.Decide(ctx, *me.FamilyID, requestID, approve); err != nil {
		return nil, err
	}
	if approve {
		// 同意 → 立即解绑（设备可能已被组长先解了，删 0 行不算错）
		_, _ = s.screen.UnbindDevice(ctx, *me.FamilyID, req.MemberID, req.DeviceKey)
	}
	if s.notifier != nil {
		s.notifier.UnbindDecided(ctx, *me.FamilyID, req.MemberID, req.DeviceModel, approve)
	}
	return s.unbinds.ByID(ctx, requestID)
}

// CancelUnbind 成员撤回自己的 pending 申请。
func (s *Device) CancelUnbind(ctx context.Context, me *model.Member, requestID uint64) error {
	if requestID == 0 {
		return fmt.Errorf("%w：缺少 request_id", ErrInvalidInput)
	}
	return s.unbinds.CancelMine(ctx, me.ID, requestID)
}
