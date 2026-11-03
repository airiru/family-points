// 服务入口：加载配置 → 连库 → 组装依赖 → 起 HTTP。
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

	"familypoints/server/internal/auth"
	"familypoints/server/internal/config"
	"familypoints/server/internal/db"
	"familypoints/server/internal/handler"
	"familypoints/server/internal/middleware"
	"familypoints/server/internal/repo"
	"familypoints/server/internal/router"
	"familypoints/server/internal/service"
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

	tokenMgr, err := auth.NewManager(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	if err != nil {
		slog.Error("token 管理器初始化失败", "err", err)
		os.Exit(1)
	}

	authRepo := repo.NewAuth(conn)
	authSvc := service.NewAuth(authRepo, tokenMgr)
	authHandler := handler.NewAuth(authSvc)
	authMW := middleware.NewAuth(tokenMgr, authSvc)

	familyRepo := repo.NewFamily(conn)
	familySvc := service.NewFamily(familyRepo)
	familyHandler := handler.NewFamily(familySvc)

	ledgerSvc := service.NewLedger(repo.NewLedger(conn), familyRepo)
	ledgerHandler := handler.NewLedger(ledgerSvc)

	r := router.Build(cfg.Env, conn, authHandler, familyHandler, ledgerHandler, authMW)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("HTTP 服务已启动", "addr", cfg.Addr, "env", cfg.Env,
			"access_ttl", cfg.AccessTTL, "refresh_ttl", cfg.RefreshTTL)
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
