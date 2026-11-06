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
	"context"
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
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"

	"familypoints/server/internal/config"
	"familypoints/server/internal/repo"
	"familypoints/server/migrations"
)

func main() {
	down := flag.Int("down", 0, "回滚 N 步；0 表示不执行")
	version := flag.Bool("version", false, "只打印当前版本")
	list := flag.Bool("list", false, "打印当前账号可访问的数据库后退出")
	verify := flag.Bool("verify", false, "迁移后核对表结构关键约束")
	seedAdmin := flag.Bool("seed-admin", false, "初始化默认管理员（已有管理员则跳过）")
	grants := flag.Bool("grants", false, "打印当前账号的授权（排查权限问题）")
	force := flag.Int("force", 0, "把迁移版本强制设为 N（N=-1 表示清空），用于修复 dirty 状态")
	reset := flag.Bool("reset", false, "【仅 dev】删除全部表与迁移记录，从零重建")
	checkDDL := flag.Bool("check-ddl", false, "验证账号能否执行 ALTER / INDEX（迁移演进的前提）")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fail("配置加载失败", err)
	}

	if *checkDDL {
		if err := checkDDLPrivileges(cfg.DB); err != nil {
			fail("DDL 权限不足", err)
		}
		return
	}

	if *reset {
		if cfg.Env != "dev" {
			fail("拒绝执行", fmt.Errorf("-reset 只允许在 APP_ENV=dev 下使用，当前为 %q", cfg.Env))
		}
		if err := resetAll(cfg.DB); err != nil {
			fail("重置失败", err)
		}
		return
	}

	if *grants {
		if err := showGrants(cfg.DB); err != nil {
			fail("读取授权失败", err)
		}
		return
	}

	if *force != 0 {
		if err := forceVersion(cfg.DB, *force); err != nil {
			fail("设置版本失败", err)
		}
		return
	}

	if *seedAdmin {
		if err := seedDefaultAdmin(cfg); err != nil {
			fail("初始化管理员失败", err)
		}
		return
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
	c.MultiStatements = true // 一个 .sql 文件里有多个 CREATE TABLE
	c.Timeout = 10 * time.Second
	dsn := "mysql://" + c.FormatDSN()

	return migrate.NewWithSourceInstance("iofs", src, dsn)
}

// expectedTables 是第 3 节定义的全部表；少一张或多一张都算结构漂移。
var expectedTables = []string{
	"families", "family_bans", "admins", "members", "rules", "items",
	"records", "transfers", "tasks", "task_submissions", "task_votes",
	"audit_logs", "notifications", "screen_rules", "screen_reports",
	"screen_device_reports", "member_devices",
	"refresh_tokens", "redemptions", "coupons", "coupon_votes",
	"family_invitations",
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

	// 5. families.invite_code_expires_at（邀请码 7 天有效期）
	var cnt int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = 'families' AND column_name = 'invite_code_expires_at'`,
		cfg.Name).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		return fmt.Errorf("families 缺少 invite_code_expires_at")
	}
	fmt.Println("✅ families.invite_code_expires_at 存在")

	// 6. username / nickname 分离：两张账号表都要有 nickname，members 不得残留旧 name 列
	for _, table := range []string{"members", "admins"} {
		var n int
		if err := conn.QueryRow(
			`SELECT COUNT(*) FROM information_schema.columns
			 WHERE table_schema = ? AND table_name = ? AND column_name = 'nickname'`,
			cfg.Name, table).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%s 缺少 nickname（可改显示名）列", table)
		}
	}
	var oldName int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = 'members' AND column_name = 'name'`,
		cfg.Name).Scan(&oldName); err != nil {
		return err
	}
	if oldName != 0 {
		return fmt.Errorf("members 仍残留旧 name 列，应已改名为 nickname")
	}
	fmt.Println("✅ username / nickname 分离：members + admins 均有 nickname，旧 name 已清除")

	// 7. 待确认邀请：pending 生成列 + 唯一键，保证同组同一人最多一条待确认
	var invExtra string
	if err := conn.QueryRow(
		`SELECT extra FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = 'family_invitations' AND column_name = 'pending_invitee'`,
		cfg.Name).Scan(&invExtra); err != nil {
		return fmt.Errorf("family_invitations.pending_invitee 不存在: %w", err)
	}
	if !strings.Contains(strings.ToUpper(invExtra), "STORED GENERATED") {
		return fmt.Errorf("pending_invitee 不是生成列：%s", invExtra)
	}
	var invIdx int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM information_schema.statistics
		 WHERE table_schema = ? AND table_name = 'family_invitations' AND index_name = 'uk_pending_invitee'`,
		cfg.Name).Scan(&invIdx); err != nil {
		return err
	}
	if invIdx == 0 {
		return fmt.Errorf("缺少唯一索引 uk_pending_invitee")
	}
	fmt.Println("✅ family_invitations：生成列 + uk_pending_invitee 生效")

	fmt.Println("\n结构核对全部通过")
	return nil
}

// checkDDLPrivileges 用一个可回滚的探针验证 ALTER / INDEX 权限。
//
// 为什么需要：只有 CREATE/DROP 时，给已有表加字段会 1142，
// 而迁移的常态恰恰是「改已有表」。开发库权限变动后跑一次即可确认。
func checkDDLPrivileges(cfg config.DB) error {
	conn, err := sql.Open("mysql", cfg.DSN(cfg.Name))
	if err != nil {
		return err
	}
	defer conn.Close()

	const probe = "priv_probe"
	steps := []struct {
		name string
		sql  string
	}{
		{"ADD COLUMN", "ALTER TABLE `screen_rules` ADD COLUMN `" + probe + "` TINYINT NULL"},
		{"ADD INDEX", "ALTER TABLE `screen_rules` ADD INDEX `idx_" + probe + "` (`" + probe + "`)"},
		{"DROP COLUMN", "ALTER TABLE `screen_rules` DROP COLUMN `" + probe + "`"},
	}
	for _, st := range steps {
		if _, err := conn.Exec(st.sql); err != nil {
			// 失败时尽力清理，避免留下探针列
			_, _ = conn.Exec("ALTER TABLE `screen_rules` DROP COLUMN `" + probe + "`")
			return fmt.Errorf("%s 失败: %w", st.name, err)
		}
		fmt.Printf("✅ %s 可用\n", st.name)
	}
	fmt.Println("\nALTER / INDEX 权限正常，迁移可以演进已有表")
	return nil
}

// resetAll 删除全部业务表与 schema_migrations。
//
// 用途：开发库账号没有 ALTER 权限（只有 CREATE/DROP），
// 因此「给已有表加字段」只能靠「全删重建」。**仅限 dev**。
func resetAll(cfg config.DB) error {
	conn, err := sql.Open("mysql", cfg.DSN(cfg.Name))
	if err != nil {
		return err
	}
	defer conn.Close()

	// 先关外键检查，避免将来加了外键后顺序问题
	if _, err := conn.Exec("SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		return err
	}
	tables := append([]string{}, expectedTables...)
	tables = append(tables, "schema_migrations")
	for _, t := range tables {
		if _, err := conn.Exec("DROP TABLE IF EXISTS `" + t + "`"); err != nil {
			return fmt.Errorf("删除表 %s: %w", t, err)
		}
	}
	slog.Info("已删除全部表，可重新执行迁移", "tables", len(tables))
	return nil
}

// showGrants 打印当前账号的授权，用于判断「是权限不足还是口令/主机问题」。
func showGrants(cfg config.DB) error {
	conn, err := sql.Open("mysql", cfg.DSN(""))
	if err != nil {
		return err
	}
	defer conn.Close()

	rows, err := conn.Query("SHOW GRANTS FOR CURRENT_USER()")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return err
		}
		fmt.Println(g)
	}
	return rows.Err()
}

// forceVersion 修复 dirty 状态：把 schema_migrations 的版本强制设回 N。
func forceVersion(cfg config.DB, n int) error {
	m, err := newMigrator(cfg)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Force(n); err != nil {
		return err
	}
	slog.Info("版本已强制设置", "version", n)
	return nil
}

// seedDefaultAdmin 初始化默认管理员（5.4.1）。
//
// 幂等：已有任何管理员就直接跳过——避免重复部署时把密码重置回默认值。
// 新账号一律 must_change_password=1，首次登录必须先改密（5.4.2）。
func seedDefaultAdmin(cfg config.Config) error {
	conn, err := sqlx.Connect("mysql", cfg.DB.DSN(cfg.DB.Name))
	if err != nil {
		return err
	}
	defer conn.Close()

	r := repo.NewAuth(conn)
	n, err := r.CountAdmins(context.Background())
	if err != nil {
		return err
	}
	if n > 0 {
		slog.Info("已存在管理员，跳过初始化", "count", n)
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := r.CreateAdmin(context.Background(), cfg.AdminUsername, string(hash), true); err != nil {
		return err
	}
	slog.Info("默认管理员已创建，请登录后立即改密", "username", cfg.AdminUsername)
	return nil
}

func fail(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
