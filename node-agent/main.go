package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type NodeAgent struct {
	nodeID        string
	controllerURL string
	agentToken    string
	interval      time.Duration
	actionPoll    time.Duration
	stateDir      string

	client *http.Client
	logger *log.Logger

	cpuMinPercent float64
	numCPU        int
	lastCPUSample map[int32]cpuSample

	forceSyncMu sync.Mutex
	forceSyncOn bool

	cacheMu sync.Mutex

	localUsersCache           []NodeLocalUser
	localUsersCachedAt        time.Time
	localUsersRefreshInterval time.Duration
	localUsersCollectTimeout  time.Duration
	gpuBusMapCache            map[string]int32
	gpuBusMapCachedAt         time.Time
	gpuBusMapCacheTTL         time.Duration
	gpuInventoryModelCache    string
	gpuInventoryCountCache    int
	gpuInventoryCachedAt      time.Time
	gpuInventoryCacheTTL      time.Duration
	gpuCommandTimeout         time.Duration
}

func main() {
	agent := &NodeAgent{
		nodeID:                    strings.TrimSpace(os.Getenv("NODE_ID")),
		controllerURL:             strings.TrimSpace(os.Getenv("CONTROLLER_URL")),
		agentToken:                strings.TrimSpace(os.Getenv("AGENT_TOKEN")),
		interval:                  60 * time.Second,
		actionPoll:                1 * time.Second,
		stateDir:                  strings.TrimSpace(os.Getenv("STATE_DIR")),
		logger:                    log.New(os.Stdout, "[node-agent] ", log.LstdFlags|log.Lmicroseconds),
		cpuMinPercent:             1.0,
		numCPU:                    runtime.NumCPU(),
		lastCPUSample:             map[int32]cpuSample{},
		localUsersRefreshInterval: 15 * time.Minute,
		localUsersCollectTimeout:  8 * time.Second,
		gpuBusMapCacheTTL:         10 * time.Minute,
		gpuInventoryCacheTTL:      30 * time.Minute,
		gpuCommandTimeout:         4 * time.Second,
	}

	if sec := strings.TrimSpace(os.Getenv("INTERVAL_SECONDS")); sec != "" {
		if v, err := strconv.Atoi(sec); err == nil && v > 0 {
			agent.interval = time.Duration(v) * time.Second
		}
	}
	if sec := strings.TrimSpace(os.Getenv("ACTION_POLL_INTERVAL_SECONDS")); sec != "" {
		if v, err := strconv.Atoi(sec); err == nil && v > 0 {
			agent.actionPoll = time.Duration(v) * time.Second
		}
	}
	if v := strings.TrimSpace(os.Getenv("CPU_MIN_PERCENT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			agent.cpuMinPercent = f
		}
	}
	if sec := strings.TrimSpace(os.Getenv("LOCAL_USERS_REFRESH_SECONDS")); sec != "" {
		if v, err := strconv.Atoi(sec); err == nil && v > 0 {
			agent.localUsersRefreshInterval = time.Duration(v) * time.Second
		}
	}
	if sec := strings.TrimSpace(os.Getenv("LOCAL_USERS_COLLECT_TIMEOUT_SECONDS")); sec != "" {
		if v, err := strconv.Atoi(sec); err == nil && v > 0 {
			agent.localUsersCollectTimeout = time.Duration(v) * time.Second
		}
	}
	if sec := strings.TrimSpace(os.Getenv("GPU_BUS_MAP_CACHE_SECONDS")); sec != "" {
		if v, err := strconv.Atoi(sec); err == nil && v > 0 {
			agent.gpuBusMapCacheTTL = time.Duration(v) * time.Second
		}
	}
	if sec := strings.TrimSpace(os.Getenv("GPU_INVENTORY_CACHE_SECONDS")); sec != "" {
		if v, err := strconv.Atoi(sec); err == nil && v > 0 {
			agent.gpuInventoryCacheTTL = time.Duration(v) * time.Second
		}
	}
	if sec := strings.TrimSpace(os.Getenv("GPU_COMMAND_TIMEOUT_SECONDS")); sec != "" {
		if v, err := strconv.Atoi(sec); err == nil && v > 0 {
			agent.gpuCommandTimeout = time.Duration(v) * time.Second
		}
	}

	if agent.nodeID == "" {
		hn, _ := os.Hostname()
		agent.nodeID = hn
	}
	if agent.stateDir == "" {
		agent.stateDir = filepath.FromSlash("/var/lib/gpu-node-agent")
	}
	agent.client = agent.defaultClient()

	if err := agent.validateConfig(); err != nil {
		agent.logger.Fatalf("配置错误：%v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	agent.logger.Printf("启动：node_id=%s controller=%s interval=%s action_poll=%s", agent.nodeID, agent.controllerURL, agent.interval, agent.actionPoll)
	agent.Run(ctx)
}

func (a *NodeAgent) Run(ctx context.Context) {
	reportTicker := time.NewTicker(a.interval)
	actionTicker := time.NewTicker(a.actionPoll)
	defer reportTicker.Stop()
	defer actionTicker.Stop()

	if err := a.tick(ctx); err != nil {
		a.logger.Printf("tick 异常：%v", err)
	}
	if err := a.actionTick(ctx); err != nil {
		a.logger.Printf("action tick 异常：%v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-reportTicker.C:
			if err := a.tick(ctx); err != nil {
				a.logger.Printf("tick 异常：%v", err)
			}
		case <-actionTicker.C:
			if err := a.actionTick(ctx); err != nil {
				a.logger.Printf("action tick 异常：%v", err)
			}
		}
	}
}

func (a *NodeAgent) tick(ctx context.Context) error {
	collectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	metrics, err := a.CollectMetrics(collectCtx)
	if err != nil {
		return err
	}
	metrics.IntervalSeconds = int(a.interval.Seconds())
	metrics.ReportID = newReportID()

	reportCtx, cancel2 := context.WithTimeout(ctx, 15*time.Second)
	defer cancel2()

	resp, err := a.ReportToController(reportCtx, metrics)
	if err != nil {
		return err
	}

	a.executeActions(ctx, resp.Actions)

	return nil
}

func (a *NodeAgent) actionTick(ctx context.Context) error {
	pollTimeout := 900 * time.Millisecond
	if a.actionPoll < 900*time.Millisecond {
		pollTimeout = a.actionPoll
	}
	pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	resp, err := a.FetchActions(pollCtx)
	if err != nil {
		return err
	}
	a.executeActions(ctx, resp.Actions)
	return nil
}

func (a *NodeAgent) executeActions(ctx context.Context, actions []Action) {
	for _, act := range actions {
		actCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := a.ExecuteAction(actCtx, act); err != nil {
			a.logger.Printf("执行 action 失败：type=%s user=%s err=%v", act.Type, act.Username, err)
		}
		cancel()
	}
}

func (a *NodeAgent) triggerForceSync(reason string) {
	a.forceSyncMu.Lock()
	if a.forceSyncOn {
		a.forceSyncMu.Unlock()
		return
	}
	a.forceSyncOn = true
	a.forceSyncMu.Unlock()

	go func() {
		defer func() {
			a.forceSyncMu.Lock()
			a.forceSyncOn = false
			a.forceSyncMu.Unlock()
		}()

		a.logger.Printf("收到 force_sync 动作：%s", strings.TrimSpace(reason))
		syncCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		if err := a.tick(syncCtx); err != nil {
			a.logger.Printf("force_sync 失败：%v", err)
		}
	}()
}
