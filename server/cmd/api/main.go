// 服务入口：加载配置 → 连库 → 起 HTTP。
//
// P0 阶段只有健康检查，业务路由在后续阶段逐个挂上来。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"familypoints/server/internal/config"
	"familypoints/server/internal/db"
	"familypoints/server/internal/httpx"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("配置加载失败", "err", err)
		os.Exit(1)
	}

	conn, err := db.Open(cfg.DB)
	if err != nil {
		slog.Error("数据库不可用", "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	if cfg.Env != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	// 健康检查：直接 Ping 库，能反映「服务活着但库挂了」这种最常见的故障
	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := conn.PingContext(ctx); err != nil {
			httpx.Fail(c, http.StatusServiceUnavailable, "数据库不可用")
			return
		}
		httpx.OK(c, gin.H{"env": cfg.Env, "db": "up"})
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("HTTP 服务已启动", "addr", cfg.Addr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP 服务异常退出", "err", err)
			os.Exit(1)
		}
	}()

	// 优雅退出：容器/服务器收到 SIGTERM 时先把在途请求处理完
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("关闭 HTTP 服务超时", "err", err)
	}
	slog.Info("服务已退出")
}
