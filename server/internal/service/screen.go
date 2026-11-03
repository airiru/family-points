// Package service：睡觉规则与屏幕时间。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
	"familypoints/server/internal/scoring"
)

// 屏幕时间相关业务错误。
var (
	ErrScreenRuleMissing = errors.New("该成员还没有屏幕时间规则")
	ErrNightConfigInvalid = errors.New("夜间时段配置不合法")
	ErrDeviceTokenInvalid = errors.New("设备令牌无效或已吊销")
)

// Screen 是屏幕时间服务。
type Screen struct {
	screen *repo.Screen
	family *repo.Family
}

// NewScreen 构造。
func NewScreen(s *repo.Screen, f *repo.Family) *Screen {
	return &Screen{screen: s, family: f}
}

// ---------- 规则配置 ----------

// SaveRule 保存睡觉规则（组长）。streaks 由服务端校验后序列化。
func (s *Screen) SaveRule(ctx context.Context, me *model.Member, id uint64, name string, points int, tiers []model.StreakTier) (*model.Rule, error) {
	if err := requireLeader(me); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "按时睡觉"
	}
	if utf8.RuneCountInString(name) > 64 {
		return nil, fmt.Errorf("%w：规则名最多 64 个字符", ErrInvalidInput)
	}
	if err := scoring.ValidateTiers(tiers); err != nil {
		return nil, fmt.Errorf("%w：%v", ErrInvalidInput, err)
	}
	normalized := scoring.NormalizeTiers(tiers)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return s.screen.SaveRule(ctx, *me.FamilyID, id, name, points, string(raw))
}

// Rules 列出本组规则。
func (s *Screen) Rules(ctx context.Context, familyID uint64) ([]model.Rule, error) {
	return s.screen.Rules(ctx, familyID)
}

// SaveScreenRuleInput 是屏幕时间规则的入参。
type SaveScreenRuleInput struct {
	MemberID      uint64 `json:"member_id"`
	ThresholdMin  int    `json:"threshold_min"`
	BonusPoints   int    `json:"bonus_points"`
	PenaltyPoints int    `json:"penalty_points"`
	Enabled       *bool  `json:"enabled"`
	// 三项同时传或同时不传；不传 = 不启用夜间判定
	NightStartHour *int   `json:"night_start_hour"`
	NightEndHour   *int   `json:"night_end_hour"`
	NightMaxMin    *int   `json:"night_max_min"`
	NightRuleID    *uint64 `json:"night_rule_id"`
}

// SaveScreenRule 保存某成员的屏幕时间规则（组长）。
func (s *Screen) SaveScreenRule(ctx context.Context, me *model.Member, in SaveScreenRuleInput) (*model.ScreenRule, error) {
	if err := requireLeader(me); err != nil {
		return nil, err
	}
	if in.MemberID == 0 {
		return nil, fmt.Errorf("%w：缺少 member_id", ErrInvalidInput)
	}
	if _, err := s.family.MemberInFamily(ctx, *me.FamilyID, in.MemberID); err != nil {
		return nil, err
	}
	if in.ThresholdMin < 0 || in.BonusPoints < 0 || in.PenaltyPoints < 0 {
		return nil, fmt.Errorf("%w：阈值与分值不能为负", ErrInvalidInput)
	}

	// 夜间三项必须同时给或同时不给（(五) 边界规则）
	given := 0
	for _, p := range []*int{in.NightStartHour, in.NightEndHour, in.NightMaxMin} {
		if p != nil {
			given++
		}
	}
	if given != 0 && given != 3 {
		return nil, fmt.Errorf("%w：night_start_hour / night_end_hour / night_max_min 必须同时提供", ErrNightConfigInvalid)
	}
	if given == 3 {
		if *in.NightStartHour < 0 || *in.NightStartHour > 23 ||
			*in.NightEndHour < 0 || *in.NightEndHour > 23 {
			return nil, fmt.Errorf("%w：小时必须在 0–23 之间", ErrNightConfigInvalid)
		}
		if *in.NightStartHour == *in.NightEndHour {
			return nil, fmt.Errorf("%w：开始与结束小时不能相同", ErrNightConfigInvalid)
		}
		if *in.NightMaxMin < 0 {
			return nil, fmt.Errorf("%w：允许分钟数不能为负", ErrNightConfigInvalid)
		}
		if in.NightRuleID == nil {
			return nil, fmt.Errorf("%w：启用夜间判定必须指定 night_rule_id", ErrNightConfigInvalid)
		}
		if _, err := s.screen.RuleByID(ctx, *me.FamilyID, *in.NightRuleID); err != nil {
			return nil, fmt.Errorf("%w：night_rule_id 指向的规则不存在", ErrNightConfigInvalid)
		}
	}

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	rule := model.ScreenRule{
		FamilyID: *me.FamilyID, MemberID: in.MemberID,
		ThresholdMin: in.ThresholdMin, BonusPoints: in.BonusPoints,
		PenaltyPoints: in.PenaltyPoints, Enabled: enabled,
		NightStartHour: in.NightStartHour, NightEndHour: in.NightEndHour,
		NightMaxMin: in.NightMaxMin, NightRuleID: in.NightRuleID,
	}
	if err := s.screen.SaveScreenRule(ctx, rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

// ScreenRules 列出本组全部屏幕时间规则。
func (s *Screen) ScreenRules(ctx context.Context, familyID uint64) ([]model.ScreenRule, error) {
	return s.screen.ScreenRules(ctx, familyID)
}

// ---------- 上报 ----------

// ReportInput 是设备上报的入参。total_min 是**当天累计总时长**，不是增量。
type ReportInput struct {
	ReportDate string `json:"report_date"` // 可选，缺省为「归属日」（见下）
	TotalMin   int    `json:"total_min"`
	NightMin   *int   `json:"night_min"`
}

// Report 处理一次设备上报并返回结算结果。
//
// 归属日（12.4）：夜间时段跨零点时按**开始日**归属。若当前时间早于 night_end_hour，
// 说明这次上报的夜间数据属于「前一天」，report_date 缺省值要减一天。
func (s *Screen) Report(ctx context.Context, d *model.Device, in ReportInput) (*repo.SettleResult, error) {
	if in.TotalMin < 0 || (in.NightMin != nil && *in.NightMin < 0) {
		return nil, fmt.Errorf("%w：分钟数不能为负", ErrInvalidInput)
	}
	rule, err := s.screen.ScreenRuleByMember(ctx, d.FamilyID, d.MemberID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrScreenRuleMissing
		}
		return nil, err
	}
	if !rule.Enabled {
		return nil, fmt.Errorf("%w：该成员的屏幕时间规则已停用", ErrInvalidInput)
	}

	date, err := s.resolveReportDate(rule, in.ReportDate)
	if err != nil {
		return nil, err
	}

	var nightRule *model.Rule
	if rule.NightEnabled() && rule.NightRuleID != nil {
		nightRule, err = s.screen.RuleByID(ctx, d.FamilyID, *rule.NightRuleID)
		if err != nil {
			return nil, fmt.Errorf("%w：夜间规则不存在（night_rule_id=%d）", ErrNightConfigInvalid, *rule.NightRuleID)
		}
	}

	res, err := s.screen.Settle(ctx, repo.SettleParams{
		FamilyID: d.FamilyID, MemberID: d.MemberID, ReportDate: date,
		TotalMin: in.TotalMin, NightMin: in.NightMin, Rule: rule, NightRule: nightRule,
	})
	if err != nil {
		return nil, err
	}
	_ = s.screen.TouchDevice(ctx, d.ID)
	return res, nil
}

// resolveReportDate 解析上报归属日。
func (s *Screen) resolveReportDate(rule *model.ScreenRule, raw string) (time.Time, error) {
	if raw != "" {
		d, err := time.ParseInLocation("2006-01-02", raw, time.Local)
		if err != nil {
			return time.Time{}, fmt.Errorf("%w：report_date 需为 YYYY-MM-DD", ErrInvalidInput)
		}
		return d, nil
	}
	now := time.Now()
	// 跨零点归属：若当前小时早于夜间结束小时，数据属于前一天
	if rule.NightEnabled() && rule.NightEndHour != nil && now.Hour() < *rule.NightEndHour {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
			AddDate(0, 0, -1), nil
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
}

// Reports 列出某成员的结算历史（本人或组长可看）。
func (s *Screen) Reports(ctx context.Context, me *model.Member, memberID uint64, limit int) ([]model.ScreenReport, error) {
	if memberID == 0 {
		memberID = me.ID
	}
	if memberID != me.ID && me.Role != model.RoleLeader {
		return nil, ErrNotLeader
	}
	if limit <= 0 || limit > 90 {
		limit = 30
	}
	return s.screen.Reports(ctx, *me.FamilyID, memberID, limit)
}

// ---------- 设备 ----------

// CreateDevice 生成设备令牌（管理员，跨组）。返回的明文令牌只出现这一次。
func (s *Screen) CreateDevice(ctx context.Context, familyID, memberID uint64, name, rawToken, tokenHash string) (*model.Device, error) {
	if name == "" {
		name = "设备"
	}
	return s.screen.CreateDevice(ctx, familyID, memberID, name, tokenHash)
}

// Devices 列出本组设备。
func (s *Screen) Devices(ctx context.Context, familyID uint64) ([]model.Device, error) {
	return s.screen.Devices(ctx, familyID)
}

// RevokeDevice 吊销设备。
func (s *Screen) RevokeDevice(ctx context.Context, familyID, id uint64) error {
	return s.screen.RevokeDevice(ctx, familyID, id)
}
