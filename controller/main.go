package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	args := parseArgs()

	cfgPath := args.configPath
	if cfgPath == "" {
		p, err := defaultConfigPath()
		if err != nil {
			log.Fatalf("加载配置失败：%v", err)
		}
		cfgPath = p
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("读取配置失败：%v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置校验失败：%v", err)
	}

	store, err := NewStore(cfg)
	if err != nil {
		log.Fatalf("连接数据库失败：%v", err)
	}
	defer func() { _ = store.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.ApplyMigrations(ctx, cfg.MigrationDir); err != nil {
		log.Fatalf("数据库迁移失败：%v", err)
	}

	srv := NewServer(cfg, store)
	srv.StartPointsMonthlyResetScheduler(context.Background())

	internalAddr := strings.TrimSpace(cfg.InternalListenAddr)
	if internalAddr == "" {
		r := srv.RouterWeb()
		log.Printf("控制器启动（单端口）：listen=%s dry_run=%v", cfg.ListenAddr, cfg.DryRun)
		if err := r.Run(cfg.ListenAddr); err != nil {
			log.Fatalf("服务启动失败：%v", err)
		}
		return
	}

	webRouter := srv.RouterWeb()
	internalRouter := srv.RouterInternal()
	webSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           webRouter,
		ReadHeaderTimeout: 10 * time.Second,
	}
	internalSrv := &http.Server{
		Addr:              internalAddr,
		Handler:           internalRouter,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("控制器启动（Web）：listen=%s dry_run=%v", cfg.ListenAddr, cfg.DryRun)
		if err := webSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("web 服务退出: %w", err)
		}
	}()
	go func() {
		log.Printf("控制器启动（Internal TLS）：listen=%s cert=%s", internalAddr, cfg.InternalTLSCertFile)
		if err := internalSrv.ListenAndServeTLS(cfg.InternalTLSCertFile, cfg.InternalTLSKeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("internal 服务退出: %w", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		log.Fatalf("服务启动失败：%v", err)
	case sig := <-sigCh:
		log.Printf("收到退出信号：%s", sig.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := webSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Web 服务关闭异常：%v", err)
	}
	if err := internalSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Internal 服务关闭异常：%v", err)
	}
}
