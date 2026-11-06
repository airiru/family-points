// 开发脚手架：给成员直接加/减积分余额；或把待确认邀请的过期时间改到过去。
//
// ⚠️ **这不是产品功能，是测试脚手架**。
// 按 8.1，积分只能由「睡觉打卡」生成、由「任务/赠送」转移、由「兑换」消耗，
// 任何一条真实路径都必须写 records。加分模式**只改余额、不写流水**，
// 唯一用途是给 P4/P5/P6 的接口测试准备初始余额。
// -expire-invitation 只用于 P3.1 验收：邀请过期是**读路径惰性判定**，
// 没有定时任务可等，只能把 expires_at 改到过去来走真实判定代码。
//
// 保护措施：APP_ENV 必须是 dev；两种模式都要求显式传参；
// 每次执行都会打一条醒目的 WARN，避免被误当成产品接口。
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/go-sql-driver/mysql"

	"familypoints/server/internal/config"
)

func main() {
	member := flag.String("member", "", "成员用户名（加分模式）")
	points := flag.Int("points", 0, "增加（可为负）的积分（加分模式）")
	expireInv := flag.Uint64("expire-invitation", 0, "把指定 pending 邀请的 expires_at 改到过去（P3.1 验收）")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fail("配置加载失败", err)
	}
	if cfg.Env != "dev" {
		fail("拒绝执行", fmt.Errorf("该工具只允许在 APP_ENV=dev 下使用，当前为 %q", cfg.Env))
	}
	if (*member == "" || *points == 0) && *expireInv == 0 {
		fail("参数不完整", fmt.Errorf("用法: go run ./cmd/devtool -member <用户名> -points <N>  或  -expire-invitation <ID>"))
	}

	conn, err := sql.Open("mysql", cfg.DB.DSN(cfg.DB.Name))
	if err != nil {
		fail("连接失败", err)
	}
	defer conn.Close()

	if *expireInv != 0 {
		expireInvitation(conn, *expireInv)
		return
	}

	res, err := conn.ExecContext(context.Background(),
		`UPDATE members SET score = score + ? WHERE username = ?`, *points, *member)
	if err != nil {
		fail("更新失败", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail("未找到成员", fmt.Errorf("username=%s", *member))
	}

	var score int
	if err := conn.QueryRowContext(context.Background(),
		`SELECT score FROM members WHERE username = ?`, *member).Scan(&score); err != nil {
		fail("回查失败", err)
	}

	slog.Warn("⚠️ 测试脚手架：已直接修改余额，**未写流水**（不属于任何真实积分来源）",
		"member", *member, "delta", *points, "score_now", score)
}

// expireInvitation 把一条 pending 邀请的过期时间改到昨天。
// 不改 status：过期状态按设计**不落库**，由读路径用 expires_at 判定（5.2.6）。
func expireInvitation(conn *sql.DB, id uint64) {
	res, err := conn.ExecContext(context.Background(),
		`UPDATE family_invitations SET expires_at = DATE_SUB(NOW(3), INTERVAL 1 DAY)
		  WHERE id = ? AND status = 'pending'`, id)
	if err != nil {
		fail("更新邀请过期时间失败", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail("未找到待确认邀请", fmt.Errorf("id=%d", id))
	}
	slog.Warn("⚠️ 测试脚手架：已把邀请过期时间改到过去，status 仍为 pending（读路径应判为过期）",
		"invitation_id", id)
}

func fail(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
