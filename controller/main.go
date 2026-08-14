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

	"github.com/gin-gonic/gin"
)

func main() {
	setDefaultTimezone()
	initControllerBuildInfo()
	// 默认以 release 模式运行：避免 debug 模式打印路由表与警告。
	// 需要调试时显式设置 GIN_MODE=debug。
	if strings.TrimSpace(os.Getenv("GIN_MODE")) == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	args := parseArgs()
	if args.showVer {
		printControllerVersion()
		return
	}

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
	if args.healthCheck {
		// 容器镜像不含 shell/curl，因此健康检查由二进制自身完成。
		os.Exit(runHealthCheck(healthCheckURL(cfg.ListenAddr)))
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
	if assigned, err := store.BackfillMissingPlatformUIDs(ctx, 200000); err != nil {
		log.Fatalf("平台 UID 回填失败：%v", err)
	} else if assigned > 0 {
		log.Printf("平台 UID 回填完成：新增分配 %d 个账号", assigned)
	}
	if deletedAssigned, err := store.BackfillMissingDeletedPlatformUIDs(ctx, 200000); err != nil {
		log.Fatalf("删除账号平台 UID 回填失败：%v", err)
	} else if deletedAssigned > 0 {
		log.Printf("删除账号平台 UID 回填完成：新增分配 %d 条记录", deletedAssigned)
	}

	srv := NewServer(cfg, store)
	srv.StartPointsMonthlyResetScheduler(context.Background())
	srv.StartUsageAutoDeleteScheduler(context.Background())
	srv.StartHASyncScheduler(context.Background())
	go func() {
		syncCtx, syncCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer syncCancel()
		srv.enqueueHistoricalNodeIdentitySyncs(syncCtx)
		srv.ensureHistoricalSharedWorkspaceDirs(syncCtx)
	}()

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

// runHealthCheck 探测本机 /readyz，返回进程退出码（0 就绪 / 1 未就绪）。
func runHealthCheck(url string) int {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck 请求失败：%v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck 状态异常：%s\n", resp.Status)
		return 1
	}
	return 0
}
