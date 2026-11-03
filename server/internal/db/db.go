// Package db 负责建立并校验 MySQL 连接池。
package db

import (
	"context"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/config"
)

// Open 打开连接池并做一次 Ping 校验。
//
// 连接池参数按「家庭场景（并发极低）+ 远端库（延迟高）」取舍：
// 连接数不必大，但空闲连接要留住，避免每次请求都重新握手。
func Open(cfg config.DB) (*sqlx.DB, error) {
	conn, err := sqlx.Connect("mysql", cfg.DSN(cfg.Name))
	if err != nil {
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}

	conn.SetMaxOpenConns(20)
	conn.SetMaxIdleConns(10)
	conn.SetConnMaxLifetime(30 * time.Minute)
	conn.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("Ping MySQL 失败: %w", err)
	}
	return conn, nil
}
