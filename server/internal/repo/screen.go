// Package repo：睡觉规则、屏幕时间规则、设备令牌、屏幕时间结算。
package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
	"familypoints/server/internal/scoring"
)

// Screen 封装屏幕时间与规则相关 SQL。
type Screen struct{ db *sqlx.DB }

// NewScreen 构造。
func NewScreen(db *sqlx.DB) *Screen { return &Screen{db: db} }

// ---------- 睡觉规则 ----------

// Rules 列出本组规则。
func (r *Screen) Rules(ctx context.Context, familyID uint64) ([]model.Rule, error) {
	list := []model.Rule{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, name, points, streaks, member_ids, created_at
		   FROM rules WHERE family_id = ? ORDER BY id`, familyID)
	return list, err
}

// RuleByID 取本组某规则。
func (r *Screen) RuleByID(ctx context.Context, familyID, id uint64) (*model.Rule, error) {
	var it model.Rule
	err := r.db.GetContext(ctx, &it,
		`SELECT id, family_id, name, points, streaks, member_ids, created_at
		   FROM rules WHERE id = ? AND family_id = ?`, id, familyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// RuleByFamily 取本组第一条规则（系统里每组只有一种规则：睡觉规则）。
func (r *Screen) RuleByFamily(ctx context.Context, familyID uint64) (*model.Rule, error) {
	var it model.Rule
	err := r.db.GetContext(ctx, &it,
		`SELECT id, family_id, name, points, streaks, member_ids, created_at
		   FROM rules WHERE family_id = ? ORDER BY id LIMIT 1`, familyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// SaveRule 新增或更新睡觉规则（id=0 表示新增）。
func (r *Screen) SaveRule(ctx context.Context, familyID, id uint64, name string, points int, streaksJSON string) (*model.Rule, error) {
	if id == 0 {
		res, err := r.db.ExecContext(ctx,
			`INSERT INTO rules (family_id, name, points, streaks) VALUES (?, ?, ?, ?)`,
			familyID, name, points, streaksJSON)
		if err != nil {
			return nil, err
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		return &model.Rule{ID: uint64(newID), FamilyID: familyID, Name: name, Points: points, Streaks: &streaksJSON}, nil
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE rules SET name = ?, points = ?, streaks = ? WHERE id = ? AND family_id = ?`,
		name, points, streaksJSON, id, familyID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return &model.Rule{ID: id, FamilyID: familyID, Name: name, Points: points, Streaks: &streaksJSON}, nil
}

// ---------- 屏幕时间规则（每成员一条）----------

// ScreenRuleByMember 取某成员的屏幕时间规则。
func (r *Screen) ScreenRuleByMember(ctx context.Context, familyID, memberID uint64) (*model.ScreenRule, error) {
	var it model.ScreenRule
	err := r.db.GetContext(ctx, &it,
		`SELECT id, family_id, member_id, threshold_min, bonus_points, penalty_points, enabled,
		        night_start_hour, night_end_hour, night_max_min, night_rule_id,
		        night_penalty_unit_min, night_penalty_points
		   FROM screen_rules WHERE member_id = ? AND family_id = ?`, memberID, familyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// ScreenRules 列出本组全部屏幕时间规则。
func (r *Screen) ScreenRules(ctx context.Context, familyID uint64) ([]model.ScreenRule, error) {
	list := []model.ScreenRule{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, member_id, threshold_min, bonus_points, penalty_points, enabled,
		        night_start_hour, night_end_hour, night_max_min, night_rule_id,
		        night_penalty_unit_min, night_penalty_points
		   FROM screen_rules WHERE family_id = ? ORDER BY member_id`, familyID)
	return list, err
}

// SaveScreenRule 按 member_id upsert 屏幕时间规则。
func (r *Screen) SaveScreenRule(ctx context.Context, in model.ScreenRule) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO screen_rules
		   (family_id, member_id, threshold_min, bonus_points, penalty_points, enabled,
		    night_start_hour, night_end_hour, night_max_min, night_rule_id,
		    night_penalty_unit_min, night_penalty_points)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   threshold_min = VALUES(threshold_min),
		   bonus_points = VALUES(bonus_points),
		   penalty_points = VALUES(penalty_points),
		   enabled = VALUES(enabled),
		   night_start_hour = VALUES(night_start_hour),
		   night_end_hour = VALUES(night_end_hour),
		   night_max_min = VALUES(night_max_min),
		   night_rule_id = VALUES(night_rule_id),
		   night_penalty_unit_min = VALUES(night_penalty_unit_min),
		   night_penalty_points = VALUES(night_penalty_points)`,
		in.FamilyID, in.MemberID, in.ThresholdMin, in.BonusPoints, in.PenaltyPoints, in.Enabled,
		in.NightStartHour, in.NightEndHour, in.NightMaxMin, in.NightRuleID,
		scoring.NormalizePenaltyUnit(in.NightPenaltyUnitMin), in.NightPenaltyPoints)
	return err
}

// ---------- 设备（无令牌，仅用于区分与展示） ----------

// UpsertDevice 记录/更新一台设备（按 member_id + device_key 幂等）。
func (r *Screen) UpsertDevice(ctx context.Context, familyID, memberID uint64, deviceKey, model string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO member_devices (family_id, member_id, device_key, device_model)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE device_model = VALUES(device_model), last_seen_at = NOW(3)`,
		familyID, memberID, deviceKey, model)
	return err
}

// DevicesOfMember 列出某成员上报过的设备。
func (r *Screen) DevicesOfMember(ctx context.Context, memberID uint64) ([]model.MemberDevice, error) {
	list := []model.MemberDevice{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, member_id, device_key, device_model, first_seen_at, last_seen_at
		   FROM member_devices WHERE member_id = ? ORDER BY id`, memberID)
	return list, err
}

// DevicesOfFamily 列出全组设备（家庭屏幕详情用）。
func (r *Screen) DevicesOfFamily(ctx context.Context, familyID uint64) ([]model.MemberDevice, error) {
	list := []model.MemberDevice{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, member_id, device_key, device_model, first_seen_at, last_seen_at
		   FROM member_devices WHERE family_id = ? ORDER BY member_id, id`, familyID)
	return list, err
}

// UpsertDeviceReport 写入/覆盖某设备某天的原始上报（幂等键：member+device+date）。
func (r *Screen) UpsertDeviceReport(ctx context.Context, familyID, memberID uint64, deviceKey, model string,
	date time.Time, totalMin int, nightMin *int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO screen_device_reports
		   (family_id, member_id, device_key, device_model, report_date, total_min, night_min, reported_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NOW(3))
		 ON DUPLICATE KEY UPDATE
		   device_model = VALUES(device_model), total_min = VALUES(total_min),
		   night_min = VALUES(night_min), updated_at = NOW(3)`,
		familyID, memberID, deviceKey, model, date.Format("2006-01-02"), totalMin, nightMin)
	return err
}

// AggregateDay 汇总某成员某天**全部设备**的时长（多设备相加，2026-11-05 拍板）。
func (r *Screen) AggregateDay(ctx context.Context, memberID uint64, date time.Time) (total int, night *int, err error) {
	var row struct {
		Total int  `db:"total"`
		Night *int `db:"night"`
	}
	err = r.db.GetContext(ctx, &row,
		`SELECT COALESCE(SUM(total_min), 0) AS total, SUM(night_min) AS night
		   FROM screen_device_reports WHERE member_id = ? AND report_date = ?`,
		memberID, date.Format("2006-01-02"))
	if err != nil {
		return 0, nil, err
	}
	return row.Total, row.Night, nil
}

// ReportsOfFamilyDate 取全组某天的结算行（家庭屏幕详情用）。
func (r *Screen) ReportsOfFamilyDate(ctx context.Context, familyID uint64, date time.Time) ([]model.ScreenReport, error) {
	list := []model.ScreenReport{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, member_id, report_date, total_min, night_min, night_ok, points,
		        record_id, night_record_id, source, revision, reported_at, updated_at
		   FROM screen_reports WHERE family_id = ? AND report_date = ?`,
		familyID, dateOnly(date))
	return list, err
}

// DeviceReportsOfDay 列出某成员某天各设备明细（家庭详情用）。
func (r *Screen) DeviceReportsOfDay(ctx context.Context, familyID uint64, date time.Time) ([]model.DeviceDayReport, error) {
	list := []model.DeviceDayReport{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT member_id, device_key, device_model, total_min, night_min
		   FROM screen_device_reports
		  WHERE family_id = ? AND report_date = ?
		  ORDER BY member_id, total_min DESC`, familyID, date.Format("2006-01-02"))
	return list, err
}

// DeviceReports 列出后台视图的设备原始上报（只读）。
// memberID=0 表示全组；date 为零值表示不限日期。全部条件都带 family_id，避免跨组读取。
func (r *Screen) DeviceReports(ctx context.Context, familyID, memberID uint64, date time.Time, limit int) ([]model.DeviceReport, error) {
	q := `SELECT id, family_id, member_id, device_key, device_model, report_date,
	             total_min, night_min, reported_at, updated_at
	        FROM screen_device_reports WHERE family_id = ?`
	args := []any{familyID}
	if memberID != 0 {
		q += " AND member_id = ?"
		args = append(args, memberID)
	}
	if !date.IsZero() {
		q += " AND report_date = ?"
		args = append(args, date.Format("2006-01-02"))
	}
	q += " ORDER BY report_date DESC, member_id, total_min DESC, id DESC LIMIT ?"
	args = append(args, limit)
	list := []model.DeviceReport{}
	err := r.db.SelectContext(ctx, &list, q, args...)
	return list, err
}

// ---------- 结算 ----------

// SettleParams 是一次上报结算所需的全部输入。
//
// 规则由 service 层解析后传进来：repo 只负责「在事务里把账算对」，
// 不做业务判断（哪些字段启用、档位怎么配）。
type SettleParams struct {
	FamilyID   uint64
	MemberID   uint64
	ReportDate time.Time
	TotalMin   int
	NightMin   *int
	Rule       *model.ScreenRule
	NightRule  *model.Rule // night_rule_id 指向的规则（未启用夜间可为 nil）
}

// SettleResult 是结算结果，供上层生成响应。
type SettleResult struct {
	Report     *model.ScreenReport
	Changed    bool // false = 数值未变，直接返回
	DayPoints  int
	NightOK    *bool
	StreakDays int
	Awards     []scoring.Award
}

// Settle 结算某成员某天的屏幕时间。
//
// 核心是 **upsert + 重算**（12.3）：同一天已有行时，先回滚旧积分与旧记录，再按新值结算。
// 全过程在一个事务内，避免「积分已扣、记录已删、结算行未更新」的脏状态。
func (r *Screen) Settle(ctx context.Context, p SettleParams) (*SettleResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	date := dateOnly(p.ReportDate)

	// 1. 锁定当日结算行（可能不存在）
	var old model.ScreenReport
	err = tx.GetContext(ctx, &old,
		`SELECT id, family_id, member_id, report_date, total_min, night_min, night_ok, points,
		        record_id, night_record_id, source, revision, reported_at, updated_at
		   FROM screen_reports WHERE member_id = ? AND report_date = ? FOR UPDATE`,
		p.MemberID, date)
	exists := true
	if errors.Is(err, sql.ErrNoRows) {
		exists = false
	} else if err != nil {
		return nil, err
	}

	// 2. 数值未变 → 直接返回，不产生新记录、不刷 revision（12.3 要点 2）
	if exists && old.TotalMin == p.TotalMin && equalIntPtr(old.NightMin, p.NightMin) {
		return &SettleResult{Report: &old, Changed: false}, nil
	}

	// 3. 回滚旧结果（积分与两条记录）
	if exists {
		if err := rollbackReport(ctx, tx, p.FamilyID, p.MemberID, &old); err != nil {
			return nil, err
		}
	}

	// 4. 日总量结算（双向计分）
	dayPoints := scoring.DayPoints(p.TotalMin, p.Rule.ThresholdMin, p.Rule.BonusPoints, p.Rule.PenaltyPoints)

	// 5. 夜间判定
	var (
		nightOK    *bool
		nightPoint int
		streak     int
		awards     []scoring.Award
	)
	if p.Rule.NightEnabled() && p.NightMin != nil {
		ok := *p.NightMin <= *p.Rule.NightMaxMin
		nightOK = &ok
		if !ok {
			// 未达标：超出部分按单位向上取整，每单位扣 N 分（2026-11-05）
			// 口径见 scoring.NightPenalty；points=0 时返回 0，即「只不加分 + 连续清零」
			nightPoint = scoring.NightPenalty(
				*p.NightMin, *p.Rule.NightMaxMin,
				p.Rule.NightPenaltyUnitMin, p.Rule.NightPenaltyPoints)
		}
		if ok {
			streak = 1 + countPrevStreak(ctx, tx, p.MemberID, date)
			nightPoint = 0
			if p.NightRule != nil {
				nightPoint = p.NightRule.Points
				tiers, err := scoring.ParseTiers(p.NightRule.Streaks)
				if err != nil {
					return nil, err
				}
				roundStart := date.AddDate(0, 0, -(streak - 1)).Format("2006-01-02")
				awards = scoring.NightAwards(streak, tiers, roundStart)
			}
		}
	}

	// 6. 落积分
	totalDelta := dayPoints + nightPoint
	for _, a := range awards {
		totalDelta += a.Bonus
	}
	if totalDelta != 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE members SET score = score + ? WHERE id = ? AND family_id = ?`,
			totalDelta, p.MemberID, p.FamilyID); err != nil {
			return nil, err
		}
	}

	// 7. 写两条流水（日总量 + 夜间），每条都是一次积分变动
	memberName, err := memberNameOf(ctx, tx, p.MemberID, p.FamilyID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	// occurred_at 必须是**归属日**（当天中午），否则补算历史日期会算进今天，
	// 得分榜按 occurred_at 聚合就会把旧账算到当期（11 节）。
	occurred := date.Add(12 * time.Hour)
	dayRecordID, err := insertRecord(ctx, tx, recordRow{
		FamilyID: p.FamilyID, MemberID: p.MemberID, MemberName: memberName,
		Title:  fmt.Sprintf("屏幕时间 %d 分钟（阈值 %d）", p.TotalMin, p.Rule.ThresholdMin),
		Points: dayPoints, Source: model.SourceScreen, OccurredAt: occurred, AddedAt: now,
	})
	if err != nil {
		return nil, err
	}

	var nightRecordID uint64
	nightPointsTotal := nightPoint
	for _, a := range awards {
		nightPointsTotal += a.Bonus
	}
	if nightOK != nil {
		title := fmt.Sprintf("按时睡觉（夜间使用 %d 分钟，上限 %d）", *p.NightMin, *p.Rule.NightMaxMin)
		if !*nightOK {
			over := *p.NightMin - *p.Rule.NightMaxMin
			if over < 0 {
				over = 0
			}
			unit := scoring.NormalizePenaltyUnit(p.Rule.NightPenaltyUnitMin)
			units := (over + unit - 1) / unit
			title = fmt.Sprintf("未按时睡觉：夜间使用 %d 分钟，超 %d 分钟（%d × %d 分钟）",
				*p.NightMin, over, units, unit)
		}
		var awardJSON *string
		if len(awards) > 0 {
			b, _ := json.Marshal(awards)
			str := string(b)
			awardJSON = &str
		}
		nightRecordID, err = insertRecord(ctx, tx, recordRow{
			FamilyID: p.FamilyID, MemberID: p.MemberID, MemberName: memberName,
			Title: title, Points: nightPointsTotal, Source: model.SourceNight,
			OccurredAt: occurred, AddedAt: now, StreakAward: awardJSON,
		})
		if err != nil {
			return nil, err
		}
	}

	// 8. 写/覆盖结算行
	if exists {
		if _, err := tx.ExecContext(ctx,
			`UPDATE screen_reports
			    SET total_min = ?, night_min = ?, night_ok = ?, points = ?,
			        record_id = ?, night_record_id = ?, revision = revision + 1, updated_at = NOW(3)
			  WHERE id = ? AND family_id = ?`,
			p.TotalMin, p.NightMin, nightOK, dayPoints,
			dayRecordID, nullableID(nightRecordID), old.ID, p.FamilyID); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO screen_reports
			   (family_id, member_id, report_date, total_min, night_min, night_ok, points,
			    record_id, night_record_id, source, revision, reported_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto', 1, ?)`,
			p.FamilyID, p.MemberID, date, p.TotalMin, p.NightMin, nightOK, dayPoints,
			dayRecordID, nullableID(nightRecordID), now); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// 回读结算行（拿到 revision / updated_at）
	rep, err := r.ReportByDate(ctx, p.FamilyID, p.MemberID, date)
	if err != nil {
		return nil, err
	}
	return &SettleResult{
		Report: rep, Changed: true, DayPoints: dayPoints,
		NightOK: nightOK, StreakDays: streak, Awards: awards,
	}, nil
}

// ReportByDate 取某成员某天的结算行。
func (r *Screen) ReportByDate(ctx context.Context, familyID, memberID uint64, date time.Time) (*model.ScreenReport, error) {
	var it model.ScreenReport
	err := r.db.GetContext(ctx, &it,
		`SELECT id, family_id, member_id, report_date, total_min, night_min, night_ok, points,
		        record_id, night_record_id, source, revision, reported_at, updated_at
		   FROM screen_reports WHERE member_id = ? AND family_id = ? AND report_date = ?`,
		memberID, familyID, dateOnly(date))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// Reports 列出某成员的结算历史（倒序）。
// Reports 上报历史。memberID=0 表示全组（后台跨成员查看用）。
//
// 注意：memberID 是**可选**过滤，不能直接写进 WHERE —— 传 0 会永远查不到数据。
func (r *Screen) Reports(ctx context.Context, familyID, memberID uint64, limit int) ([]model.ScreenReport, error) {
	q := `SELECT id, family_id, member_id, report_date, total_min, night_min, night_ok, points,
	             record_id, night_record_id, source, revision, reported_at, updated_at
	        FROM screen_reports WHERE family_id = ?`
	args := []any{familyID}
	if memberID != 0 {
		q += ` AND member_id = ?`
		args = append(args, memberID)
	}
	q += ` ORDER BY report_date DESC, member_id ASC LIMIT ?`
	args = append(args, limit)

	list := []model.ScreenReport{}
	err := r.db.SelectContext(ctx, &list, q, args...)
	return list, err
}

// ---------- 内部辅助 ----------

// rollbackReport 回滚某结算行产生的积分与流水（覆盖上报的前半步）。
//
// 两条记录都要回滚：日总量与夜间。漏一条就会留下孤儿记录（12.4 第四节）。
func rollbackReport(ctx context.Context, tx *sqlx.Tx, familyID, memberID uint64, old *model.ScreenReport) error {
	ids := []uint64{}
	for _, p := range []*uint64{old.RecordID, old.NightRecordID} {
		if p != nil && *p != 0 {
			ids = append(ids, *p)
		}
	}
	for _, id := range ids {
		var points int
		err := tx.GetContext(ctx, &points,
			`SELECT points FROM records WHERE id = ? AND family_id = ?`, id, familyID)
		if errors.Is(err, sql.ErrNoRows) {
			// 12.3 要点 4：记录已被单独删除时跳过，不中断事务
			continue
		}
		if err != nil {
			return err
		}
		if points != 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE members SET score = score - ? WHERE id = ? AND family_id = ?`,
				points, memberID, familyID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM records WHERE id = ? AND family_id = ?`, id, familyID); err != nil {
			return err
		}
	}
	return nil
}

// countPrevStreak 从 date 的前一天往回数，连续「夜间达标」的天数。
//
// 连续天数**不落计数器**（决策 7）：计数器在覆盖上报时极难回滚正确，
// 而按快照回溯天然幂等。缺口一天即中断。
func countPrevStreak(ctx context.Context, tx *sqlx.Tx, memberID uint64, date time.Time) int {
	var rows []struct {
		ReportDate time.Time `db:"report_date"`
		NightOK    *bool     `db:"night_ok"`
	}
	err := tx.SelectContext(ctx, &rows,
		`SELECT report_date, night_ok FROM screen_reports
		  WHERE member_id = ? AND report_date < ?
		  ORDER BY report_date DESC LIMIT 400`, memberID, date)
	if err != nil {
		return 0
	}
	streak := 0
	expect := date.AddDate(0, 0, -1)
	for _, row := range rows {
		if !sameDate(row.ReportDate, expect) {
			break // 缺了一天 → 中断
		}
		if row.NightOK == nil || !*row.NightOK {
			break // 当天未达标 → 中断
		}
		streak++
		expect = expect.AddDate(0, 0, -1)
	}
	return streak
}

type recordRow struct {
	FamilyID    uint64
	MemberID    uint64
	MemberName  string
	Title       string
	Points      int
	Source      string
	OccurredAt  time.Time // 事项发生时间：屏幕时间用**归属日**，不是写入时刻
	AddedAt     time.Time // 实际写入时间：用于「补记」判定
	StreakAward *string
}

// insertRecord 写一条流水。**所有积分变动都必须走这里**（8.1 铁律）。
func insertRecord(ctx context.Context, tx *sqlx.Tx, in recordRow) (uint64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO records (family_id, member_id, member_name, title, points,
		                      occurred_at, added_at, source, streak_award)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.FamilyID, in.MemberID, in.MemberName, in.Title, in.Points,
		in.OccurredAt, in.AddedAt, in.Source, in.StreakAward)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return uint64(id), err
}

func memberNameOf(ctx context.Context, tx *sqlx.Tx, memberID, familyID uint64) (string, error) {
	var name string
	err := tx.GetContext(ctx, &name,
		`SELECT name FROM members WHERE id = ? AND family_id = ?`, memberID, familyID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func sameDate(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func equalIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func nullableID(id uint64) any {
	if id == 0 {
		return nil
	}
	return id
}
