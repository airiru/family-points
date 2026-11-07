// Package service：后台（管理员）跨组操作。
//
// 管理员是网站运营者：不属于任何家庭组，但**可管理所有家庭组的全部内容**（4.2）。
// 因此这里的每个方法都显式接收 familyID —— 它只能来自路径参数，
// 与前台「family_id 只从 token 取」形成两条互不混淆的路径（4.3 第 3 条）。
package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
	"familypoints/server/internal/scoring"
)

// Admin 是后台服务。
type Admin struct {
	families    *repo.Family
	invitations *repo.Invitation
	auth        *repo.Auth
	screen      *repo.Screen
	ledger      *repo.Ledger
	report      *repo.Report
	task        *repo.Task
}

// NewAdmin 构造。
func NewAdmin(f *repo.Family, inv *repo.Invitation, a *repo.Auth, s *repo.Screen, l *repo.Ledger, r *repo.Report, t *repo.Task) *Admin {
	return &Admin{families: f, invitations: inv, auth: a, screen: s, ledger: l, report: r, task: t}
}

// ---------- 只读视图（2026-11-04 补：后台 12 个页面需要能读到现值）----------

// Rule 本组睡觉规则；未配置返回 nil（不是错误）。
func (a *Admin) Rule(ctx context.Context, familyID uint64) (*model.Rule, error) {
	r, err := a.screen.RuleByFamily(ctx, familyID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil
	}
	return r, err
}

// Items 本组商品列表。
func (a *Admin) Items(ctx context.Context, familyID uint64) ([]model.Item, error) {
	return a.ledger.Items(ctx, familyID)
}

// Tasks 本组全部任务（含已关闭）。
func (a *Admin) Tasks(ctx context.Context, familyID uint64) ([]model.Task, error) {
	if err := a.task.ExpireDue(ctx, familyID); err != nil { // 顺手把超时的置 expired
		return nil, err
	}
	return a.task.List(ctx, familyID)
}

// Transfers 赠送记录（按时间倒序）。
func (a *Admin) Transfers(ctx context.Context, familyID uint64, limit int) ([]model.Transfer, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return a.ledger.Transfers(ctx, familyID, limit)
}

// ScreenReports 屏幕时间上报历史（只读）。memberID=0 表示全组。
func (a *Admin) ScreenReports(ctx context.Context, familyID, memberID uint64, limit int) ([]model.ScreenReport, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return a.screen.Reports(ctx, familyID, memberID, limit)
}

// ScreenDeviceReports 后台设备明细：原始上报列表 + 已登记设备（均只读）。
// 原始上报是「每台设备每天一行」；结算行仍是「每人每天一行」（多设备相加，见 12.4），
// 这里只做展示，不重新汇总、不重新判定。
func (a *Admin) ScreenDeviceReports(ctx context.Context, familyID, memberID uint64, date time.Time, limit int) ([]model.DeviceReport, []model.MemberDevice, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	reports, err := a.screen.DeviceReports(ctx, familyID, memberID, date, limit)
	if err != nil {
		return nil, nil, err
	}
	devices, err := a.screen.DevicesOfFamily(ctx, familyID)
	if err != nil {
		return nil, nil, err
	}
	if memberID != 0 {
		kept := devices[:0]
		for _, d := range devices {
			if d.MemberID == memberID {
				kept = append(kept, d)
			}
		}
		devices = kept
	}
	return reports, devices, nil
}

// Records 本组流水（只读，服务端分页与筛选）。
func (a *Admin) Records(ctx context.Context, f repo.RecordFilter) ([]model.Record, int, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	return a.report.Records(ctx, f)
}

// Bans 本组封禁名单（后台只读；解除只有「组长邀请入组时自动清除」一条路径）。
func (a *Admin) Bans(ctx context.Context, familyID uint64) ([]repo.BanRow, error) {
	if _, err := a.families.ByID(ctx, familyID); err != nil {
		return nil, err
	}
	return a.families.Bans(ctx, familyID)
}

// JoinAttempts 本组全部邀请与申请（后台运营视角，含已处理与已过期，只读）。
func (a *Admin) JoinAttempts(ctx context.Context, familyID uint64, limit int) ([]repo.AdminAttemptRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if _, err := a.families.ByID(ctx, familyID); err != nil {
		return nil, err
	}
	return a.invitations.ForAdmin(ctx, familyID, limit)
}

// Ranking 积分榜（mode=points）或得分榜（mode=score）。range 只对得分榜生效。
func (a *Admin) Ranking(ctx context.Context, familyID uint64, mode, rng string, now time.Time) ([]repo.MemberScore, error) {
	if mode == "score" {
		from, to := adminPeriodRange(rng, now)
		return a.report.RankingScore(ctx, familyID, from, to)
	}
	return a.report.RankingPoints(ctx, familyID)
}

// Trend 按天聚合（memberID=0 表示全组）。
func (a *Admin) Trend(ctx context.Context, familyID, memberID uint64, days int, now time.Time) ([]repo.TrendPoint, error) {
	if days <= 0 || days > 180 {
		days = 30
	}
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -days)
	return a.report.Trend(ctx, familyID, memberID, from, to)
}

// adminPeriodRange 与前台口径一致（避免与 report.go 的 periodRange 重名）：周从周一 00:00 起，月按自然月（11.1）。
func adminPeriodRange(rng string, now time.Time) (time.Time, time.Time) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if rng == "month" {
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return from, from.AddDate(0, 1, 0)
	}
	offset := (int(day.Weekday()) + 6) % 7 // 周一=0
	from := day.AddDate(0, 0, -offset)
	return from, from.AddDate(0, 0, 7)
}

// FamilyRow 是家庭组列表的一行（复用仓储层类型，避免两处结构漂移）。
type FamilyRow = repo.FamilyRow

// Families 列出所有家庭组（含成员数）。
func (a *Admin) Families(ctx context.Context) ([]FamilyRow, error) {
	return a.families.AllWithCounts(ctx)
}

// AdminState 是后台首页概览。
type AdminState = repo.AdminStats

// State 统计全站数据。
func (a *Admin) State(ctx context.Context) (*AdminState, error) {
	return a.families.AdminStats(ctx)
}

// Invite 按用户名把已注册用户拉进家庭组（管理员代邀请）。
func (a *Admin) Invite(ctx context.Context, familyID uint64, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("%w：请填写用户名", ErrInvalidInput)
	}
	if _, err := a.families.ByID(ctx, familyID); err != nil {
		return err
	}
	m, err := a.auth.MemberByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return fmt.Errorf("%w：该用户名不存在", ErrInvalidInput)
		}
		return err
	}
	if m.InFamily() {
		return ErrAlreadyInFamily
	}
	banned, err := a.families.IsBanned(ctx, familyID, m.ID)
	if err != nil {
		return err
	}
	if banned {
		return ErrBanned
	}
	return a.families.Join(ctx, familyID, m.ID)
}

// Kick 踢出成员（后台）。与前台同一套语义：积分清零 + 写禁入名单 + 历史保留。
func (a *Admin) Kick(ctx context.Context, familyID, memberID uint64) error {
	if _, err := a.families.MemberInFamily(ctx, familyID, memberID); err != nil {
		return err
	}
	// 组长不能直接踢出：否则组内无人可管理，必须先换组长
	m, err := a.families.MemberInFamily(ctx, familyID, memberID)
	if err != nil {
		return err
	}
	if m.Role == model.RoleLeader {
		return ErrLeaderMustTransfer
	}
	return a.families.Leave(ctx, familyID, memberID, "kicked")
}

// SetHidden 隐藏 / 显示成员。
func (a *Admin) SetHidden(ctx context.Context, familyID, memberID uint64, hidden bool) error {
	return a.families.SetHidden(ctx, familyID, memberID, hidden)
}

// TransferLeader 后台指定新组长（同事务降旧升新）。
func (a *Admin) TransferLeader(ctx context.Context, familyID, memberID uint64) error {
	target, err := a.families.MemberInFamily(ctx, familyID, memberID)
	if err != nil {
		return err
	}
	if target.Role == model.RoleLeader {
		return fmt.Errorf("%w：该成员已经是组长", ErrInvalidInput)
	}
	members, err := a.families.Members(ctx, familyID, true)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.Role == model.RoleLeader {
			return a.families.TransferLeader(ctx, familyID, m.ID, memberID)
		}
	}
	return fmt.Errorf("%w：该组还没有组长", ErrInvalidInput)
}

// RenameFamily 改家庭组名。
func (a *Admin) RenameFamily(ctx context.Context, familyID uint64, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return "", fmt.Errorf("%w：家庭组名需 1–64 个字符", ErrInvalidInput)
	}
	if err := a.families.UpdateName(ctx, familyID, name); err != nil {
		return "", err
	}
	return name, nil
}

// SaveRule 后台保存睡觉规则（与组长同权，14.3）。
func (a *Admin) SaveRule(ctx context.Context, familyID, id uint64, name string, points int, tiers []model.StreakTier) (*model.Rule, error) {
	if err := scoring.ValidateTiers(tiers); err != nil {
		return nil, fmt.Errorf("%w：%v", ErrInvalidInput, err)
	}
	normalized := scoring.NormalizeTiers(tiers)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = "按时睡觉"
	}
	// 每组只有一条规则：id=0 时先找已有的那条，避免新建后没人引用
	if id == 0 {
		if existing, err := a.screen.RuleByFamily(ctx, familyID); err == nil {
			id = existing.ID
		} else if !errors.Is(err, repo.ErrNotFound) {
			return nil, err
		}
	}
	return a.screen.SaveRule(ctx, familyID, id, name, points, string(raw))
}

// SaveItem 后台保存商品（与组长同权，14.5）。
func (a *Admin) SaveItem(ctx context.Context, familyID, id uint64, name string, cost int) (*model.Item, error) {
	name = strings.TrimSpace(name)
	if name == "" || cost <= 0 {
		return nil, fmt.Errorf("%w：商品名必填且价格为正整数", ErrInvalidInput)
	}
	return a.ledger.SaveItem(ctx, familyID, id, name, cost)
}

// ExportRecords 导出记录为 CSV（11.2.8 / 15.2.10）。
//
// 加 UTF-8 BOM：否则 Excel 打开中文会乱码——这是导出功能最常见的投诉。
func (a *Admin) ExportRecords(ctx context.Context, familyID uint64, from, to string) ([]byte, string, error) {
	if _, err := a.families.ByID(ctx, familyID); err != nil {
		return nil, "", err
	}
	f := repo.RecordFilter{FamilyID: familyID, Limit: 100000, Offset: 0}
	if from != "" {
		t, err := time.ParseInLocation("2006-01-02", from, time.Local)
		if err != nil {
			return nil, "", fmt.Errorf("%w：from 需为 YYYY-MM-DD", ErrInvalidInput)
		}
		f.From = t
	}
	if to != "" {
		t, err := time.ParseInLocation("2006-01-02", to, time.Local)
		if err != nil {
			return nil, "", fmt.Errorf("%w：to 需为 YYYY-MM-DD", ErrInvalidInput)
		}
		f.To = t.AddDate(0, 0, 1)
	}
	rows, _, err := a.report.Records(ctx, f)
	if err != nil {
		return nil, "", err
	}

	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"时间", "成员", "标题", "分值", "来源"})
	for _, r := range rows {
		_ = w.Write([]string{
			r.OccurredAt.Format("2006-01-02 15:04"),
			r.MemberName,
			r.Title,
			strconv.Itoa(r.Points),
			r.Source,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}
	name := fmt.Sprintf("records_family%d_%s.csv", familyID, time.Now().Format("20060102"))
	return buf.Bytes(), name, nil
}

// CreateAdmin 新增管理员（19.19：运营方可新增其他管理员）。
func (a *Admin) CreateAdmin(ctx context.Context, username, password string) (*model.Admin, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 20 {
		return nil, fmt.Errorf("%w：用户名需 3–20 位", ErrInvalidInput)
	}
	if len(password) < 8 || len(password) > 72 {
		return nil, fmt.Errorf("%w：密码需 8–72 字节", ErrInvalidInput)
	}
	if _, err := a.auth.AdminByUsername(ctx, username); err == nil {
		return nil, ErrUsernameTaken
	} else if !errors.Is(err, repo.ErrNotFound) {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	// 新管理员同样强制首次改密
	if err := a.auth.CreateAdmin(ctx, username, string(hash), true); err != nil {
		return nil, err
	}
	return a.auth.AdminByUsername(ctx, username)
}

// ListAdmins 管理员列表。
func (a *Admin) ListAdmins(ctx context.Context) ([]model.Admin, error) {
	return a.auth.ListAdmins(ctx)
}

// SetAdminDisabled 停用 / 启用管理员。**不允许停用自己**。
func (a *Admin) SetAdminDisabled(ctx context.Context, operatorID, targetID uint64, disabled bool) error {
	if operatorID == targetID {
		return fmt.Errorf("%w：不能停用自己", ErrInvalidInput)
	}
	return a.auth.SetAdminDisabled(ctx, targetID, disabled)
}
