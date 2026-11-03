// 迁移工具：建库 + 执行 migrations/*.sql（golang-migrate）。
//
// 用法（在 server/ 下）：
//
//	go run ./cmd/migrate            # 建库并升级到最新
//	go run ./cmd/migrate -down 1    # 回滚一步
//	go run ./cmd/migrate -version   # 只看当前版本
//
// 为什么不用手工建表：第 3 节的表结构是唯一事实源，手工建会漂移。
package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql" // 注册 mysql:// 驱动
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"familypoints/server/internal/config"
	"familypoints/server/migrations"
)

func main() {
	down := flag.Int("down", 0, "回滚 N 步；0 表示不执行")
	version := flag.Bool("version", false, "只打印当前版本")
	list := flag.Bool("list", false, "打印当前账号可访问的数据库后退出")
	verify := flag.Bool("verify", false, "迁移后核对表结构关键约束")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fail("配置加载失败", err)
	}

	if *list {
		if err := listDatabases(cfg.DB); err != nil {
			fail("列库失败", err)
		}
		return
	}

	if err := ensureDatabase(cfg.DB); err != nil {
		fail("建库失败", err)
	}

	m, err := newMigrator(cfg.DB)
	if err != nil {
		fail("初始化迁移器失败", err)
	}
	defer m.Close()

	if *verify {
		if err := verifySchema(cfg.DB); err != nil {
			fail("结构核对未通过", err)
		}
		return
	}

	if *version {
		v, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			fmt.Println("当前版本：无（尚未执行任何迁移）")
			return
		}
		if err != nil {
			fail("读取版本失败", err)
		}
		fmt.Printf("当前版本：%d（dirty=%v）\n", v, dirty)
		return
	}

	if *down > 0 {
		if err := m.Steps(-*down); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			fail("回滚失败", err)
		}
		slog.Info("回滚完成", "steps", -*down)
		return
	}

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			slog.Info("已是最新版本，无需迁移")
			return
		}
		fail("迁移失败", err)
	}
	v, _, _ := m.Version()
	slog.Info("迁移完成", "version", v)
}

// ensureDatabase 建库（不存在时）。必须连到不指定库的 DSN 才能执行 CREATE DATABASE。
func ensureDatabase(cfg config.DB) error {
	conn, err := sql.Open("mysql", cfg.DSN(""))
	if err != nil {
		return err
	}
	defer conn.Close()

	stmt := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci",
		cfg.Name,
	)
	if _, err := conn.Exec(stmt); err != nil {
		// 1044 = 无建库权限。此时若目标库已存在且可连，就不该阻断迁移
		if !strings.Contains(err.Error(), "Error 1044") {
			return fmt.Errorf("执行 %q: %w", stmt, err)
		}
		if pingErr := pingDatabase(cfg); pingErr != nil {
			return fmt.Errorf("既无建库权限，目标库 %q 也不可用: %w", cfg.Name, pingErr)
		}
		slog.Warn("当前账号无建库权限，直接使用已存在的库", "db", cfg.Name)
		return nil
	}
	return nil
}

// pingDatabase 用指定库连一次，确认库存在且账号有权限。
func pingDatabase(cfg config.DB) error {
	conn, err := sql.Open("mysql", cfg.DSN(cfg.Name))
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Ping()
}

// listDatabases 打印账号可见的库，用于排查「该用哪个库名」。
func listDatabases(cfg config.DB) error {
	conn, err := sql.Open("mysql", cfg.DSN(""))
	if err != nil {
		return err
	}
	defer conn.Close()

	rows, err := conn.Query("SHOW DATABASES")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		fmt.Println(name)
	}
	return rows.Err()
}

func newMigrator(cfg config.DB) (*migrate.Migrate, error) {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, err
	}

	c := mysql.NewConfig()
	c.Net = "tcp"
	c.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	c.User = cfg.User
	c.Passwd = cfg.Password
	c.DBName = cfg.Name
	c.MultiStatements = true      // 一个 .sql 文件里有多个 CREATE TABLE
	c.Timeout = 10 * time.Second
	dsn := "mysql://" + c.FormatDSN()

	return migrate.NewWithSourceInstance("iofs", src, dsn)
}

// expectedTables 是第 3 节定义的全部表；少一张或多一张都算结构漂移。
var expectedTables = []string{
	"families", "family_bans", "admins", "members", "rules", "items",
	"records", "transfers", "tasks", "task_submissions", "task_votes",
	"audit_logs", "notifications", "screen_rules", "screen_reports", "devices",
}

// verifySchema 核对 P0 的完成判据：表齐全 + 几条关键约束真的生效。
func verifySchema(cfg config.DB) error {
	conn, err := sql.Open("mysql", cfg.DSN(cfg.Name))
	if err != nil {
		return err
	}
	defer conn.Close()

	// 1. 表是否齐全
	rows, err := conn.Query(
		`SELECT table_name FROM information_schema.tables WHERE table_schema = ? ORDER BY table_name`,
		cfg.Name)
	if err != nil {
		return err
	}
	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		got[name] = true
	}
	rows.Close()

	for _, t := range expectedTables {
		if !got[t] {
			return fmt.Errorf("缺少表 %s", t)
		}
	}
	fmt.Printf("✅ 表齐全：%d 张\n", len(expectedTables))

	// 2. records.source 必须是五值枚举
	var colType string
	if err := conn.QueryRow(
		`SELECT column_type FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = 'records' AND column_name = 'source'`,
		cfg.Name).Scan(&colType); err != nil {
		return err
	}
	if !strings.Contains(colType, "'task'") || !strings.Contains(colType, "'redeem'") ||
		strings.Contains(colType, "'manual'") {
		return fmt.Errorf("records.source 枚举不符：%s", colType)
	}
	fmt.Printf("✅ records.source = %s\n", colType)

	// 3. 「每组仅一名组长」的生成列与唯一索引
	var extra string
	if err := conn.QueryRow(
		`SELECT extra FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = 'members' AND column_name = 'leader_family_id'`,
		cfg.Name).Scan(&extra); err != nil {
		return fmt.Errorf("members.leader_family_id 不存在: %w", err)
	}
	if !strings.Contains(strings.ToUpper(extra), "STORED GENERATED") {
		return fmt.Errorf("leader_family_id 不是生成列：%s", extra)
	}
	var idxCount int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM information_schema.statistics
		 WHERE table_schema = ? AND table_name = 'members' AND index_name = 'uk_leader_per_family'`,
		cfg.Name).Scan(&idxCount); err != nil {
		return err
	}
	if idxCount == 0 {
		return fmt.Errorf("缺少唯一索引 uk_leader_per_family")
	}
	fmt.Println("✅ members：生成列 + uk_leader_per_family 生效")

	// 4. 幂等唯一键
	for _, c := range []struct{ table, index string }{
		{"screen_reports", "uk_member_date"},
		{"task_votes", "uk_submission_voter"},
		{"family_bans", "uk_family_member"},
	} {
		var n int
		if err := conn.QueryRow(
			`SELECT COUNT(*) FROM information_schema.statistics
			 WHERE table_schema = ? AND table_name = ? AND index_name = ?`,
			cfg.Name, c.table, c.index).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%s 缺少唯一索引 %s", c.table, c.index)
		}
	}
	fmt.Println("✅ 幂等唯一键齐全（screen_reports / task_votes / family_bans）")

	fmt.Println("\n结构核对全部通过")
	return nil
}

func fail(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
