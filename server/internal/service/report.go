// Package service：概览、记录查询、榜单、趋势。
package service

import (
	"context"
	"fmt"
	"time"

	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
)

// Report 是查询服务。
type Report struct {
	report   *repo.Report
	families *repo.Family
}

// NewReport 构造。
func NewReport(r *repo.Report, f *repo.Family) *Report {
	return &Report{report: r, families: f}
}

// State 是前台首页的概览数据。
//
// **不含全量记录**（13 条约定：`/api/app/state` 必须服务端分页）——
// 记录走 `/api/app/records`。
type State struct {
	Family      FamilyBrief   `json:"family"`
	Me          MemberBrief   `json:"me"`
	Members     []MemberBrief `json:"members"`
	UnreadCount int           `json:"unread_count"`
}

// FamilyBrief 是概览里的家庭组信息。
type FamilyBrief struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// State 组装概览。
func (s *Report) State(ctx context.Context, me *model.Member) (*State, error) {
	f, err := s.families.ByID(ctx, *me.FamilyID)
	if err != nil {
		return nil, err
	}
	members, err := s.families.Members(ctx, *me.FamilyID, false)
	if err != nil {
		return nil, err
	}
	briefs := make([]MemberBrief, 0, len(members))
	for _, m := range members {
		briefs = append(briefs, MemberBrief{
			ID: m.ID, Name: m.Name, Role: m.Role, Avatar: m.Avatar,
			Score: m.Score, FrozenScore: m.FrozenScore, Hidden: m.Hidden,
		})
	}
	unread, err := s.report.UnreadNotifications(ctx, *me.FamilyID, me.ID)
	if err != nil {
		return nil, err
	}
	return &State{
		Family: FamilyBrief{ID: f.ID, Name: f.Name},
		Me: MemberBrief{
			ID: me.ID, Name: me.Name, Role: me.Role, Avatar: me.Avatar,
			Score: me.Score, FrozenScore: me.FrozenScore, Hidden: me.Hidden,
		},
		Members:     briefs,
		UnreadCount: unread,
	}, nil
}

// RecordQuery 是记录查询入参（对应 11.2.5 / 11.2.6）。
type RecordQuery struct {
	MemberID uint64 `json:"member_id"`
	Source   string `json:"source"`
	Keyword  string `json:"keyword"`
	From     string `json:"from"` // YYYY-MM-DD
	To       string `json:"to"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
}

// RecordPage 是分页结果。
type RecordPage struct {
	Items  []model.Record `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// Records 分页查询本组记录。
func (s *Report) Records(ctx context.Context, me *model.Member, q RecordQuery) (*RecordPage, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	f := repo.RecordFilter{
		FamilyID: *me.FamilyID, MemberID: q.MemberID, Source: q.Source,
		Keyword: q.Keyword, Limit: q.Limit, Offset: q.Offset,
	}
	if q.From != "" {
		t, err := time.ParseInLocation("2006-01-02", q.From, time.Local)
		if err != nil {
			return nil, fmt.Errorf("%w：from 需为 YYYY-MM-DD", ErrInvalidInput)
		}
		f.From = t
	}
	if q.To != "" {
		t, err := time.ParseInLocation("2006-01-02", q.To, time.Local)
		if err != nil {
			return nil, fmt.Errorf("%w：to 需为 YYYY-MM-DD", ErrInvalidInput)
		}
		f.To = t.AddDate(0, 0, 1) // 含当天
	}
	items, total, err := s.report.Records(ctx, f)
	if err != nil {
		return nil, err
	}
	return &RecordPage{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

// RankingRow 是榜单的一行（含连续打卡天数）。
type RankingRow struct {
	MemberID    uint64  `json:"member_id"`
	Name        string  `json:"name"`
	Avatar      *string `json:"avatar"`
	Score       int     `json:"score"`
	FrozenScore int     `json:"frozen_score"`
	PeriodScore int     `json:"period_score"`
	Streak      int     `json:"streak"` // 仅睡觉规则的连续天数（11.2.3）
}

// Ranking 返回积分榜或得分榜。
//
//   - type=points：按 `score + frozen_score` 降序（净额，回答「还剩多少」）
//   - type=score ：区间内 `points > 0` 之和降序（毛额，回答「这段时间谁表现好」）
func (s *Report) Ranking(ctx context.Context, me *model.Member, typ, period string) ([]RankingRow, string, error) {
	from, to, label := periodRange(period)
	var (
		rows []repo.MemberScore
		err  error
	)
	switch typ {
	case "", "points":
		typ = "points"
		rows, err = s.report.RankingPoints(ctx, *me.FamilyID)
	case "score":
		rows, err = s.report.RankingScore(ctx, *me.FamilyID, from, to)
	default:
		return nil, "", fmt.Errorf("%w：type 只能是 points 或 score", ErrInvalidInput)
	}
	if err != nil {
		return nil, "", err
	}

	// 连续天数一次算全组（原来在循环里逐成员查，是 N+1）
	streaks, err := s.report.Streaks(ctx, *me.FamilyID, time.Now())
	if err != nil {
		return nil, "", err
	}

	out := make([]RankingRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, RankingRow{
			MemberID: r.MemberID, Name: r.Name, Avatar: r.Avatar,
			Score: r.Score, FrozenScore: r.FrozenScore,
			PeriodScore: r.PeriodScore, Streak: streaks[r.MemberID],
		})
	}
	return out, label, nil
}

// TrendPoint 是趋势图上的一天。
type TrendPoint = repo.TrendPoint

// Trend 返回某成员（memberID=0 表示全组）的积分趋势。
func (s *Report) Trend(ctx context.Context, me *model.Member, memberID uint64, period string) ([]TrendPoint, string, error) {
	if memberID != 0 && memberID != me.ID && me.Role != model.RoleLeader {
		return nil, "", ErrNotLeader
	}
	from, to, label := periodRange(period)
	points, err := s.report.Trend(ctx, *me.FamilyID, memberID, from, to)
	if err != nil {
		return nil, "", err
	}
	return points, label, nil
}

// periodRange 把 week / month 解析成 [from, to) 区间。
//
// 周边界取**周一 00:00**（中国习惯），月边界取 1 日 00:00。
func periodRange(period string) (from, to time.Time, label string) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch period {
	case "month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(0, 1, 0), "month"
	default: // week
		weekday := int(today.Weekday())
		if weekday == 0 {
			weekday = 7 // 周日归到上一周的末尾
		}
		start := today.AddDate(0, 0, -(weekday - 1))
		return start, start.AddDate(0, 0, 7), "week"
	}
}
