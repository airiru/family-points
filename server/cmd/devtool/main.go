// 开发脚手架：给成员直接加/减积分余额。
//
// ⚠️ **这不是产品功能，是测试脚手架**。
// 按 8.1，积分只能由「睡觉打卡」生成、由「任务/赠送」转移、由「兑换」消耗，
// 任何一条真实路径都必须写 records。本工具**只改余额、不写流水**，
// 唯一用途是给 P4/P5/P6 的接口测试准备初始余额。
//
// 保护措施：APP_ENV 必须是 dev；必须显式传 -member 与 -points；
// 每次执行都会打一条醒目的 WARN，避免被误当成「加分接口」。
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
	member := flag.String("member", "", "成员用户名")
	points := flag.Int("points", 0, "增加（可为负）的积分")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fail("配置加载失败", err)
	}
	if cfg.Env != "dev" {
		fail("拒绝执行", fmt.Errorf("该工具只允许在 APP_ENV=dev 下使用，当前为 %q", cfg.Env))
	}
	if *member == "" || *points == 0 {
		fail("参数不完整", fmt.Errorf("用法: go run ./cmd/devtool -member <用户名> -points <N>"))
	}

	conn, err := sql.Open("mysql", cfg.DB.DSN(cfg.DB.Name))
	if err != nil {
		fail("连接失败", err)
	}
	defer conn.Close()

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

func fail(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
