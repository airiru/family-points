// Package config 负责从环境变量（含本地 .env）加载运行配置。
//
// 原则：敏感信息一律走环境变量，不写进代码、不写进文档、不入库。
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

// DB 是数据库连接参数。
type DB struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

// Config 是应用的全部运行配置。
type Config struct {
	Env       string
	Addr      string
	JWTSecret string
	DB        DB
}

// Load 读取 .env（存在则加载，不存在不算错误）后从环境变量组装配置。
func Load() (Config, error) {
	// 允许从 server/ 或仓库根目录启动
	for _, p := range []string{".env", "../.env"} {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			break
		}
	}

	port, err := strconv.Atoi(getEnv("DB_PORT", "3306"))
	if err != nil {
		return Config{}, fmt.Errorf("DB_PORT 不是合法数字: %w", err)
	}

	cfg := Config{
		Env:       getEnv("APP_ENV", "dev"),
		Addr:      getEnv("HTTP_ADDR", ":8080"),
		JWTSecret: os.Getenv("JWT_SECRET"),
		DB: DB{
			Host:     os.Getenv("DB_HOST"),
			Port:     port,
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASSWORD"),
			Name:     getEnv("DB_NAME", "family_points"),
		},
	}

	if cfg.DB.Host == "" || cfg.DB.User == "" {
		return Config{}, fmt.Errorf("缺少 DB_HOST / DB_USER，请先按 .env.example 配置 .env")
	}
	return cfg, nil
}

// DSN 返回连接指定库的 DSN；dbName 传空表示不选库（用于建库）。
func (d DB) DSN(dbName string) string {
	c := mysql.NewConfig()
	c.Net = "tcp"
	c.Addr = fmt.Sprintf("%s:%d", d.Host, d.Port)
	c.User = d.User
	c.Passwd = d.Password
	c.DBName = dbName
	c.ParseTime = true        // DATETIME → time.Time
	c.Loc = time.Local        // 与 MySQL 的 DATETIME(3) 对齐
	c.Collation = "utf8mb4_general_ci"
	c.Timeout = 10 * time.Second
	c.ReadTimeout = 30 * time.Second
	c.WriteTimeout = 30 * time.Second
	c.AllowNativePasswords = true
	return c.FormatDSN()
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
