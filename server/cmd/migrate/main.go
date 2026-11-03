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
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fail("配置加载失败", err)
	}

	if err := ensureDatabase(cfg.DB); err != nil {
		fail("建库失败", err)
	}

	m, err := newMigrator(cfg.DB)
	if err != nil {
		fail("初始化迁移器失败", err)
	}
	defer m.Close()

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
		return fmt.Errorf("执行 %q: %w", stmt, err)
	}
	return nil
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

func fail(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
