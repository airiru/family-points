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
	ErrScreenRuleMissing  = errors.New("该成员还没有屏幕时间规则")
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
	return s.saveRuleForFamily(ctx, *me.FamilyID, id, name, points, tiers)
}

// saveRuleForFamily 保存规则。**系统里每组只有一种规则**，所以 id=0 时先找已有的那条，
// 找不到才新建——否则会出现「后台存了一条新规则，但成员的 night_rule_id 还指着旧规则」。
func (s *Screen) saveRuleForFamily(ctx context.Context, familyID, id uint64, name string, points int, tiers []model.StreakTier) (*model.Rule, error) {
	normalized := scoring.NormalizeTiers(tiers)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if id == 0 {
		existing, err := s.screen.RuleByFamily(ctx, familyID)
		if err == nil {
			id = existing.ID
		} else if !errors.Is(err, repo.ErrNotFound) {
			return nil, err
		}
	}
	return s.screen.SaveRule(ctx, familyID, id, name, points, string(raw))
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
	NightStartHour *int    `json:"night_start_hour"`
	NightEndHour   *int    `json:"night_end_hour"`
	NightMaxMin    *int    `json:"night_max_min"`
	NightRuleID    *uint64 `json:"night_rule_id"`
	// 未按时睡觉的扣分口径（2026-11-05）：超出部分按单位向上取整，每单位扣 N 分
	NightPenaltyUnitMin *int `json:"night_penalty_unit_min"` // 30 或 60，缺省 30
	NightPenaltyPoints  *int `json:"night_penalty_points"`   // 每单位扣分，缺省 0（不扣）
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
		// night_rule_id 由**后端推导**，不要求前端传：每组只有一条规则（见 saveRuleForFamily），
		// 让 App 去知道内部 ID 既没必要，也容易传错（历史上 App 一直没传，导致保存必 400）。
		if in.NightRuleID == nil {
			existing, err := s.screen.RuleByFamily(ctx, *me.FamilyID)
			if err != nil {
				return nil, fmt.Errorf("%w：本组还没有睡觉规则，请先配置连续奖励档位", ErrNightConfigInvalid)
			}
			id := existing.ID
			in.NightRuleID = &id
		}
		if _, err := s.screen.RuleByID(ctx, *me.FamilyID, *in.NightRuleID); err != nil {
			return nil, fmt.Errorf("%w：night_rule_id 指向的规则不存在", ErrNightConfigInvalid)
		}
	}

	// 扣分口径校验（2026-11-05）：单位只能是 30/60，分值不能为负
	penaltyUnit := 30
	if in.NightPenaltyUnitMin != nil {
		if *in.NightPenaltyUnitMin != 30 && *in.NightPenaltyUnitMin != 60 {
			return nil, fmt.Errorf("%w：扣分单位只能是 30 或 60 分钟", ErrNightConfigInvalid)
		}
		penaltyUnit = *in.NightPenaltyUnitMin
	}
	penaltyPoints := 0
	if in.NightPenaltyPoints != nil {
		if *in.NightPenaltyPoints < 0 {
			return nil, fmt.Errorf("%w：每单位扣分不能为负", ErrNightConfigInvalid)
		}
		penaltyPoints = *in.NightPenaltyPoints
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
		NightPenaltyUnitMin: penaltyUnit, NightPenaltyPoints: penaltyPoints,
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
	// 设备标识（2026-11-05 起由客户端自报，不再需要设备令牌）
	DeviceKey   string `json:"device_key"`
	DeviceModel string `json:"device_model"`
	ReportDate  string `json:"report_date"` // 可选，缺省为「归属日」（见下）
	TotalMin    int    `json:"total_min"`
	NightMin    *int   `json:"night_min"`
	// Hourly 是 24 个小时桶（索引 = 小时，单位分钟），2026-11-06 起上传。
	// **只用于展示明细**；但若恰好给了 24 个桶，total_min 以桶之和为准，
	// 避免「总时长」与「逐小时加起来」两个数字对不上（同一份数据算两遍的误差）。
	Hourly []int `json:"hourly"`
}

// Report 处理一次设备上报并返回结算结果。
//
// 归属日（12.4）：夜间时段跨零点时按**开始日**归属。若当前时间早于 night_end_hour，
// 说明这次上报的夜间数据属于「前一天」，report_date 缺省值要减一天。
func (s *Screen) Report(ctx context.Context, me *model.Member, in ReportInput) (*repo.SettleResult, error) {
	if !me.InFamily() {
		return nil, ErrNotInFamily
	}
	if in.TotalMin < 0 || (in.NightMin != nil && *in.NightMin < 0) {
		return nil, fmt.Errorf("%w：分钟数不能为负", ErrInvalidInput)
	}
	if in.DeviceKey == "" {
		return nil, fmt.Errorf("%w：缺少 device_key（设备标识）", ErrInvalidInput)
	}
	if len(in.DeviceKey) > 64 || len(in.DeviceModel) > 64 {
		return nil, fmt.Errorf("%w：device_key / device_model 过长", ErrInvalidInput)
	}
	if len(in.Hourly) > 0 && len(in.Hourly) != 24 {
		return nil, fmt.Errorf("%w：hourly 必须是 24 个桶（索引 = 小时）", ErrInvalidInput)
	}
	for _, v := range in.Hourly {
		if v < 0 {
			return nil, fmt.Errorf("%w：小时分钟数不能为负", ErrInvalidInput)
		}
	}
	rule, err := s.screen.ScreenRuleByMember(ctx, *me.FamilyID, me.ID)
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
		nightRule, err = s.screen.RuleByID(ctx, *me.FamilyID, *rule.NightRuleID)
		if err != nil {
			return nil, fmt.Errorf("%w：夜间规则不存在（night_rule_id=%d）", ErrNightConfigInvalid, *rule.NightRuleID)
		}
	}

	// 1) 记录设备 + 该设备当天的原始上报（幂等：member+device+date）
	if err := s.screen.UpsertDevice(ctx, *me.FamilyID, me.ID, in.DeviceKey, in.DeviceModel); err != nil {
		return nil, err
	}
	// 24 个桶齐全时以桶之和为 total：同一份数据不该算出两个总时长。
	totalMin := in.TotalMin
	if len(in.Hourly) == 24 {
		sum := 0
		for _, v := range in.Hourly {
			sum += v
		}
		totalMin = sum
	}
	if err := s.screen.UpsertDeviceReport(ctx, *me.FamilyID, me.ID, in.DeviceKey, in.DeviceModel,
		date, totalMin, in.NightMin); err != nil {
		return nil, err
	}
	if len(in.Hourly) > 0 {
		if err := s.screen.ReplaceHourly(ctx, *me.FamilyID, me.ID, in.DeviceKey, in.DeviceModel,
			date, in.Hourly); err != nil {
			return nil, err
		}
	}

	// 2) 汇总该成员当天**所有设备**，再结算（多设备相加，2026-11-05 拍板）
	total, night, err := s.screen.AggregateDay(ctx, me.ID, date)
	if err != nil {
		return nil, err
	}

	return s.screen.Settle(ctx, repo.SettleParams{
		FamilyID: *me.FamilyID, MemberID: me.ID, ReportDate: date,
		TotalMin: total, NightMin: night, Rule: rule, NightRule: nightRule,
	})
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

// ---------- 家庭屏幕详情（2026-11-05） ----------

// FamilyMemberDay 是一个成员某天的屏幕时间汇总（含各设备明细）。
type FamilyMemberDay struct {
	MemberID uint64                  `json:"member_id"`
	Nickname string                  `json:"nickname"`
	Role     string                  `json:"role"`
	TotalMin int                     `json:"total_min"`
	NightMin *int                    `json:"night_min"`
	NightOK  *bool                   `json:"night_ok"`
	Points   int                     `json:"points"`
	Devices  []model.DeviceDayReport `json:"devices"`
}

// FamilyDayView 是全组某天的屏幕时间视图。
type FamilyDayView struct {
	Date    string            `json:"date"`
	Members []FamilyMemberDay `json:"members"`
}

// FamilyDay 返回全组某天每人的屏幕时间 + 设备明细。
//
// 组内所有成员都能看（需求：可以查看家庭成员的屏幕时间详情）。
// 没上报的人也列出来（total=0），否则界面会「少人」，看起来像数据丢了。
func (s *Screen) FamilyDay(ctx context.Context, me *model.Member, rawDate string) (*FamilyDayView, error) {
	if !me.InFamily() {
		return nil, ErrNotInFamily
	}
	date := time.Now()
	if rawDate != "" {
		d, err := time.ParseInLocation("2006-01-02", rawDate, time.Local)
		if err != nil {
			return nil, fmt.Errorf("%w：date 需为 YYYY-MM-DD", ErrInvalidInput)
		}
		date = d
	}
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

	members, err := s.family.Members(ctx, *me.FamilyID, true)
	if err != nil {
		return nil, err
	}
	reports, err := s.screen.ReportsOfFamilyDate(ctx, *me.FamilyID, date)
	if err != nil {
		return nil, err
	}
	devices, err := s.screen.DeviceReportsOfDay(ctx, *me.FamilyID, date)
	if err != nil {
		return nil, err
	}

	byMember := map[uint64]*model.ScreenReport{}
	for i := range reports {
		byMember[reports[i].MemberID] = &reports[i]
	}
	devByMember := map[uint64][]model.DeviceDayReport{}
	for _, d := range devices {
		devByMember[d.MemberID] = append(devByMember[d.MemberID], d)
	}

	view := &FamilyDayView{Date: date.Format("2006-01-02")}
	for _, m := range members {
		item := FamilyMemberDay{
			MemberID: m.ID, Nickname: m.Nickname, Role: m.Role,
			Devices: devByMember[m.ID],
		}
		if item.Devices == nil {
			item.Devices = []model.DeviceDayReport{}
		}
		// 总时长/夜间以**设备明细求和**为准，而不是结算行：
		// 当天还没到 21:00、或刚上报尚未结算时，结算行可能不存在或滞后，
		// 直接用结算行会让「今天」永远显示 0。判定与积分仍只认结算行。
		for _, d := range item.Devices {
			item.TotalMin += d.TotalMin
			if d.NightMin != nil {
				if item.NightMin == nil {
					item.NightMin = new(int)
				}
				*item.NightMin += *d.NightMin
			}
		}
		if r, ok := byMember[m.ID]; ok {
			item.NightOK = r.NightOK
			item.Points = r.Points
		}
		view.Members = append(view.Members, item)
	}
	return view, nil
}

// DevicesOfMember 列出本人上报过的设备。
func (s *Screen) DevicesOfMember(ctx context.Context, me *model.Member) ([]model.MemberDevice, error) {
	if !me.InFamily() {
		return nil, ErrNotInFamily
	}
	return s.screen.DevicesOfMember(ctx, me.ID)
}

// ---------- 今日实时快照（2026-11-06） ----------

// Snapshot 落「今天」的实时明细：设备、当日合计、逐小时桶。**不结算、不发分**。
//
// 为什么要单开一条路径：Worker 上报的是「昨天」（数据完整、可结算），
// 而家人想看的「今天」必须实时。若复用 Report，会在一天内反复重算当天积分，
// 分数早中晚各变一次，流水会很脏。所以快照只写明细，判定与积分仍只认结算行。
func (s *Screen) Snapshot(ctx context.Context, me *model.Member, in ReportInput) error {
	if !me.InFamily() {
		return ErrNotInFamily
	}
	if in.DeviceKey == "" {
		return fmt.Errorf("%w：缺少 device_key（设备标识）", ErrInvalidInput)
	}
	if len(in.DeviceKey) > 64 || len(in.DeviceModel) > 64 {
		return fmt.Errorf("%w：device_key / device_model 过长", ErrInvalidInput)
	}
	if len(in.Hourly) != 24 {
		return fmt.Errorf("%w：hourly 必须是 24 个桶（索引 = 小时）", ErrInvalidInput)
	}
	sum := 0
	for _, v := range in.Hourly {
		if v < 0 {
			return fmt.Errorf("%w：小时分钟数不能为负", ErrInvalidInput)
		}
		sum += v
	}
	var night *int
	if in.NightMin != nil {
		if *in.NightMin < 0 {
			return fmt.Errorf("%w：分钟数不能为负", ErrInvalidInput)
		}
		night = in.NightMin
	}

	date := time.Now()
	if in.ReportDate != "" {
		d, err := time.ParseInLocation("2006-01-02", in.ReportDate, time.Local)
		if err != nil {
			return fmt.Errorf("%w：report_date 需为 YYYY-MM-DD", ErrInvalidInput)
		}
		date = d
	}
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

	if err := s.screen.UpsertDevice(ctx, *me.FamilyID, me.ID, in.DeviceKey, in.DeviceModel); err != nil {
		return err
	}
	if err := s.screen.UpsertDeviceReport(ctx, *me.FamilyID, me.ID, in.DeviceKey, in.DeviceModel,
		date, sum, night); err != nil {
		return err
	}
	return s.screen.ReplaceHourly(ctx, *me.FamilyID, me.ID, in.DeviceKey, in.DeviceModel, date, in.Hourly)
}

// ---------- 成员屏幕时间详情（2026-11-06） ----------

// DayPoint 是某成员某天的合计。
type DayPoint struct {
	Date     string `json:"date"`
	TotalMin int    `json:"total_min"`
	NightMin *int   `json:"night_min"`
}

// MemberDeviceDetail 是一台设备在查询日的明细。
//
// Hourly 与 Days 都由服务端算好，前端只画，不做任何补零或重算——
// 与「前端不参与业务判定」的约定一致。
type MemberDeviceDetail struct {
	DeviceKey   string `json:"device_key"`
	DeviceModel string `json:"device_model"`
	TotalMin    int    `json:"total_min"`
	NightMin    *int   `json:"night_min"`
	Hourly      []int  `json:"hourly"` // 24，索引 = 小时
	Days        []int  `json:"days"`   // 与 MemberDetailView.Days 同序的每日分钟
}

// MemberDetailView 是「某个成员某天」的完整屏幕时间视图。
type MemberDetailView struct {
	MemberID uint64 `json:"member_id"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
	Avatar   string `json:"avatar"`
	IsSelf   bool   `json:"is_self"`
	Date     string `json:"date"`

	TotalMin int   `json:"total_min"`
	NightMin *int  `json:"night_min"`
	NightOK  *bool `json:"night_ok"`
	Points   int   `json:"points"`
	Settled  bool  `json:"settled"` // 当天是否已有结算行

	Hourly  []int                `json:"hourly"` // 24，全部设备相加
	Devices []MemberDeviceDetail `json:"devices"`
	Days    []DayPoint           `json:"days"` // 近 7 天，含查询日
}

// MemberDetail 返回某成员某天的屏幕时间详情。
//
// memberID = 0 表示自己。组内成员互相可见（需求：可以查看家庭成员的详细时间）。
// 不存在的成员返回 ErrNotFound；跨组成员返回 ErrNotFound（不泄露存在性）。
func (s *Screen) MemberDetail(ctx context.Context, me *model.Member, memberID uint64, rawDate string) (*MemberDetailView, error) {
	if !me.InFamily() {
		return nil, ErrNotInFamily
	}
	if memberID == 0 {
		memberID = me.ID
	}
	m, err := s.family.MemberInFamily(ctx, *me.FamilyID, memberID)
	if err != nil {
		return nil, err
	}

	date := time.Now()
	if rawDate != "" {
		d, perr := time.ParseInLocation("2006-01-02", rawDate, time.Local)
		if perr != nil {
			return nil, fmt.Errorf("%w：date 需为 YYYY-MM-DD", ErrInvalidInput)
		}
		date = d
	}
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	from := date.AddDate(0, 0, -6)

	devReports, err := s.screen.DeviceReportsOfDay(ctx, *me.FamilyID, date)
	if err != nil {
		return nil, err
	}
	hourlyRows, err := s.screen.HourlyOfMemberDate(ctx, memberID, date)
	if err != nil {
		return nil, err
	}
	dailyTotals, err := s.screen.DailyTotalsOfMember(ctx, memberID, from, date)
	if err != nil {
		return nil, err
	}
	deviceDaily, err := s.screen.DeviceDailyTotals(ctx, memberID, from, date)
	if err != nil {
		return nil, err
	}

	// 近 7 天：先铺满 7 天（没数据的天补 0），否则界面会「缺几根柱子」看起来像丢数据。
	avatar := ""
	if m.Avatar != nil {
		avatar = *m.Avatar
	}
	view := &MemberDetailView{
		MemberID: m.ID, Nickname: m.Nickname, Role: m.Role, Avatar: avatar,
		IsSelf: m.ID == me.ID,
		Date:   date.Format("2006-01-02"),
		Hourly: make([]int, 24),
	}
	totalByDate := map[string]model.DailyTotal{}
	for _, t := range dailyTotals {
		totalByDate[t.ReportDate] = t
	}
	dayIdx := map[string]int{}
	for i := 0; i < 7; i++ {
		d := from.AddDate(0, 0, i).Format("2006-01-02")
		dayIdx[d] = i
		p := DayPoint{Date: d}
		if t, ok := totalByDate[d]; ok {
			p.TotalMin = t.TotalMin
			p.NightMin = t.NightMin
		}
		view.Days = append(view.Days, p)
	}

	// 按设备归集小时明细
	hourByDevice := map[string][]int{}
	modelByDevice := map[string]string{}
	for _, r := range hourlyRows {
		if hourByDevice[r.DeviceKey] == nil {
			hourByDevice[r.DeviceKey] = make([]int, 24)
		}
		if r.Hour >= 0 && r.Hour < 24 {
			hourByDevice[r.DeviceKey][r.Hour] += r.Minutes
		}
		modelByDevice[r.DeviceKey] = r.DeviceModel
	}
	devDays := map[string][]int{}
	for _, r := range deviceDaily {
		if devDays[r.DeviceKey] == nil {
			devDays[r.DeviceKey] = make([]int, 7)
		}
		if i, ok := dayIdx[r.Date]; ok {
			devDays[r.DeviceKey][i] = r.TotalMin
		}
	}

	view.Devices = []MemberDeviceDetail{}
	for _, d := range devReports {
		if d.MemberID != memberID {
			continue
		}
		hours := hourByDevice[d.DeviceKey]
		if hours == nil {
			hours = make([]int, 24)
		}
		days := devDays[d.DeviceKey]
		if days == nil {
			days = make([]int, 7)
		}
		view.Devices = append(view.Devices, MemberDeviceDetail{
			DeviceKey: d.DeviceKey, DeviceModel: d.DeviceModel,
			TotalMin: d.TotalMin, NightMin: d.NightMin,
			Hourly: hours, Days: days,
		})
		for h := 0; h < 24; h++ {
			view.Hourly[h] += hours[h]
		}
		view.TotalMin += d.TotalMin
		if d.NightMin != nil {
			if view.NightMin == nil {
				view.NightMin = new(int)
			}
			*view.NightMin += *d.NightMin
		}
	}

	// 判定与积分只认结算行；当天未结算就留空，界面显示「待结算」而不是猜。
	if rep, err := s.screen.ReportByDate(ctx, *me.FamilyID, memberID, date); err == nil {
		view.NightOK = rep.NightOK
		view.Points = rep.Points
		view.Settled = true
	} else if !errors.Is(err, repo.ErrNotFound) {
		return nil, err
	}

	return view, nil
}
