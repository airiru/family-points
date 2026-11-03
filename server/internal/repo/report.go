// Package repo：查询与榜单（只读）。
package repo

import (
	"context"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
)

// Report 封装只读查询。
type Report struct{ db *sqlx.DB }

// NewReport 构造。
func NewReport(db *sqlx.DB) *Report { return &Report{db: db} }

// RecordFilter 是记录查询条件。
type RecordFilter struct {
	FamilyID uint64
	MemberID uint64    // 0 = 全组
	Source   string    // 空 = 全部
	Keyword  string    // 标题模糊搜索
	From     time.Time // 零值 = 不限
	To       time.Time
	Limit    int
	Offset   int
}

// Records 按条件分页查询记录。
func (r *Report) Records(ctx context.Context, f RecordFilter) ([]model.Record, int, error) {
	where := []string{"family_id = ?"}
	args := []any{f.FamilyID}

	if f.MemberID != 0 {
		where = append(where, "member_id = ?")
		args = append(args, f.MemberID)
	}
	if f.Source != "" {
		where = append(where, "source = ?")
		args = append(args, f.Source)
	}
	if f.Keyword != "" {
		where = append(where, "title LIKE ?")
		args = append(args, "%"+f.Keyword+"%")
	}
	if !f.From.IsZero() {
		where = append(where, "occurred_at >= ?")
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		where = append(where, "occurred_at < ?")
		args = append(args, f.To)
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := r.db.GetContext(ctx, &total,
		`SELECT COUNT(*) FROM records WHERE `+cond, args...); err != nil {
		return nil, 0, err
	}

	q := `SELECT id, family_id, member_id, member_name, title, points,
	             occurred_at, added_at, source, transfer_id, redemption_id, streak_award
	        FROM records WHERE ` + cond + `
	       ORDER BY occurred_at DESC, id DESC LIMIT ? OFFSET ?`
	list := []model.Record{}
	if err := r.db.SelectContext(ctx, &list, q, append(args, f.Limit, f.Offset)...); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// MemberScore 是积分榜/得分榜的一行。
type MemberScore struct {
	MemberID    uint64  `db:"member_id" json:"member_id"`
	Name        string  `db:"name" json:"name"`
	Avatar      *string `db:"avatar" json:"avatar"`
	Score       int     `db:"score" json:"score"`
	FrozenScore int     `db:"frozen_score" json:"frozen_score"`
	PeriodScore int     `db:"period_score" json:"period_score"`
}

// RankingPoints 积分榜：按 `score + frozen_score` 降序（净额，冻结也计入）。
//
// 隐藏成员不展示（隐藏只影响展示，不影响数据）。
func (r *Report) RankingPoints(ctx context.Context, familyID uint64) ([]MemberScore, error) {
	list := []MemberScore{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT id AS member_id, name, avatar, score, frozen_score, 0 AS period_score
		   FROM members
		  WHERE family_id = ? AND hidden = 0
		  ORDER BY (score + frozen_score) DESC, id ASC`, familyID)
	return list, err
}

// RankingScore 得分榜：区间内 `points > 0` 之和，按 `occurred_at` 聚合。
//
// 用 occurred_at 而不是 added_at —— 否则补记的旧日期记录会算到当前周期（11.2）。
func (r *Report) RankingScore(ctx context.Context, familyID uint64, from, to time.Time) ([]MemberScore, error) {
	list := []MemberScore{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT m.id AS member_id, m.name, m.avatar, m.score, m.frozen_score,
		        COALESCE(SUM(r.points), 0) AS period_score
		   FROM members m
		   LEFT JOIN records r
		          ON r.member_id = m.id AND r.family_id = m.family_id
		         AND r.points > 0 AND r.occurred_at >= ? AND r.occurred_at < ?
		  WHERE m.family_id = ? AND m.hidden = 0
		  GROUP BY m.id, m.name, m.avatar, m.score, m.frozen_score
		  ORDER BY period_score DESC, m.id ASC`, from, to, familyID)
	return list, err
}

// TrendPoint 是趋势图上的一天。
type TrendPoint struct {
	Date   string `db:"d" json:"date"`
	Earned int    `db:"earned" json:"earned"` // 当天正分之和
	Net    int    `db:"net" json:"net"`       // 当天净变动（含负分）
}

// Trend 按天聚合某成员（或全组）的积分变动。
func (r *Report) Trend(ctx context.Context, familyID, memberID uint64, from, to time.Time) ([]TrendPoint, error) {
	cond := "family_id = ? AND occurred_at >= ? AND occurred_at < ?"
	args := []any{familyID, from, to}
	if memberID != 0 {
		cond += " AND member_id = ?"
		args = append(args, memberID)
	}
	list := []TrendPoint{}
	err := r.db.SelectContext(ctx, &list,
		`SELECT DATE_FORMAT(occurred_at, '%Y-%m-%d') AS d,
		        COALESCE(SUM(CASE WHEN points > 0 THEN points ELSE 0 END), 0) AS earned,
		        COALESCE(SUM(points), 0) AS net
		   FROM records WHERE `+cond+`
		  GROUP BY d ORDER BY d`, args...)
	return list, err
}

// CurrentStreak 计算某成员当前连续「夜间达标」天数。
//
// 口径（12.4）：最近一条结算行必须是今天或昨天且达标，否则视为中断（当日未上报 = 未达标）。
func (r *Report) CurrentStreak(ctx context.Context, familyID, memberID uint64, today time.Time) (int, error) {
	var rows []struct {
		ReportDate time.Time `db:"report_date"`
		NightOK    *bool     `db:"night_ok"`
	}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT report_date, night_ok FROM screen_reports
		  WHERE member_id = ? AND family_id = ?
		  ORDER BY report_date DESC LIMIT 400`, memberID, familyID); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	// 最近一条必须是今天或昨天，否则已经断了
	latest := rows[0].ReportDate
	yesterday := today.AddDate(0, 0, -1)
	if !sameDate(latest, today) && !sameDate(latest, yesterday) {
		return 0, nil
	}
	streak := 0
	expect := latest
	for _, row := range rows {
		if !sameDate(row.ReportDate, expect) {
			break
		}
		if row.NightOK == nil || !*row.NightOK {
			break
		}
		streak++
		expect = expect.AddDate(0, 0, -1)
	}
	return streak, nil
}

// UnreadNotifications 未读通知数（概览页用）。
func (r *Report) UnreadNotifications(ctx context.Context, familyID, memberID uint64) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM notifications
		  WHERE family_id = ? AND member_id = ? AND read_at IS NULL`, familyID, memberID)
	return n, err
}
