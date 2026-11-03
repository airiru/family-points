// Package migrations 把 *.sql 打进二进制，避免部署时还要拷目录。
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
