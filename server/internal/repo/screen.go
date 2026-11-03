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
	var list []model.Rule
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
		        night_start_hour, night_end_hour, night_max_min, night_rule_id
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
	var list []model.ScreenRule
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, member_id, threshold_min, bonus_points, penalty_points, enabled,
		        night_start_hour, night_end_hour, night_max_min, night_rule_id
		   FROM screen_rules WHERE family_id = ? ORDER BY member_id`, familyID)
	return list, err
}

// SaveScreenRule 按 member_id upsert 屏幕时间规则。
func (r *Screen) SaveScreenRule(ctx context.Context, in model.ScreenRule) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO screen_rules
		   (family_id, member_id, threshold_min, bonus_points, penalty_points, enabled,
		    night_start_hour, night_end_hour, night_max_min, night_rule_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   threshold_min = VALUES(threshold_min),
		   bonus_points = VALUES(bonus_points),
		   penalty_points = VALUES(penalty_points),
		   enabled = VALUES(enabled),
		   night_start_hour = VALUES(night_start_hour),
		   night_end_hour = VALUES(night_end_hour),
		   night_max_min = VALUES(night_max_min),
		   night_rule_id = VALUES(night_rule_id)`,
		in.FamilyID, in.MemberID, in.ThresholdMin, in.BonusPoints, in.PenaltyPoints, in.Enabled,
		in.NightStartHour, in.NightEndHour, in.NightMaxMin, in.NightRuleID)
	return err
}

// ---------- 设备令牌 ----------

// CreateDevice 保存设备（只存哈希）。
func (r *Screen) CreateDevice(ctx context.Context, familyID, memberID uint64, name, tokenHash string) (*model.Device, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO devices (family_id, member_id, name, token_hash) VALUES (?, ?, ?, ?)`,
		familyID, memberID, name, tokenHash)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &model.Device{ID: uint64(id), FamilyID: familyID, MemberID: memberID, Name: name}, nil
}

// DeviceByTokenHash 用令牌哈希查设备（只返回未吊销的）。
func (r *Screen) DeviceByTokenHash(ctx context.Context, hash string) (*model.Device, error) {
	var d model.Device
	err := r.db.GetContext(ctx, &d,
		`SELECT id, family_id, member_id, name, token_hash, revoked, created_at, last_seen_at
		   FROM devices WHERE token_hash = ? AND revoked = 0`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// TouchDevice 记录设备最近活跃时间。
func (r *Screen) TouchDevice(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE devices SET last_seen_at = NOW(3) WHERE id = ?`, id)
	return err
}

// Devices 列出本组设备。
func (r *Screen) Devices(ctx context.Context, familyID uint64) ([]model.Device, error) {
	var list []model.Device
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, member_id, name, token_hash, revoked, created_at, last_seen_at
		   FROM devices WHERE family_id = ? ORDER BY id`, familyID)
	return list, err
}

// RevokeDevice 吊销设备令牌。
func (r *Screen) RevokeDevice(ctx context.Context, familyID, id uint64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE devices SET revoked = 1 WHERE id = ? AND family_id = ?`, id, familyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
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
		Title:   fmt.Sprintf("屏幕时间 %d 分钟（阈值 %d）", p.TotalMin, p.Rule.ThresholdMin),
		Points:  dayPoints, Source: model.SourceScreen, OccurredAt: occurred, AddedAt: now,
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
			title = fmt.Sprintf("未按时睡觉：夜间使用 %d 分钟，超过 %d 分钟", *p.NightMin, *p.Rule.NightMaxMin)
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
func (r *Screen) Reports(ctx context.Context, familyID, memberID uint64, limit int) ([]model.ScreenReport, error) {
	var list []model.ScreenReport
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, member_id, report_date, total_min, night_min, night_ok, points,
		        record_id, night_record_id, source, revision, reported_at, updated_at
		   FROM screen_reports WHERE member_id = ? AND family_id = ?
		  ORDER BY report_date DESC LIMIT ?`, memberID, familyID, limit)
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
