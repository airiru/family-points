// Package repo：站点级配置（键值对，2026-11-07）。
package repo

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// SiteConfig 封装 site_configs（name → value，如 icp=备案号）。
type SiteConfig struct{ db *sqlx.DB }

// NewSiteConfig 构造。
func NewSiteConfig(db *sqlx.DB) *SiteConfig { return &SiteConfig{db: db} }

// Get 取单个配置值；不存在返回空串。
func (r *SiteConfig) Get(ctx context.Context, name string) (string, error) {
	var v string
	err := r.db.GetContext(ctx, &v,
		`SELECT value FROM site_configs WHERE name = ?`, name)
	if err != nil {
		return "", nil // 没配置过 = 空串，不算错误
	}
	return v, nil
}

// Set 写入/更新一个配置项。
func (r *SiteConfig) Set(ctx context.Context, name, value string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO site_configs (name, value) VALUES (?, ?)
		 ON DUPLICATE KEY UPDATE value = VALUES(value)`, name, value)
	return err
}
