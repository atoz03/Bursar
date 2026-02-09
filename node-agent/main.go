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
}

func main() {
	agent := &NodeAgent{
		nodeID:        strings.TrimSpace(os.Getenv("NODE_ID")),
		controllerURL: strings.TrimSpace(os.Getenv("CONTROLLER_URL")),
		agentToken:    strings.TrimSpace(os.Getenv("AGENT_TOKEN")),
		interval:      60 * time.Second,
		actionPoll:    1 * time.Second,
		stateDir:      strings.TrimSpace(os.Getenv("STATE_DIR")),
		logger:        log.New(os.Stdout, "[node-agent] ", log.LstdFlags|log.Lmicroseconds),
		cpuMinPercent: 1.0,
		numCPU:        runtime.NumCPU(),
		lastCPUSample: map[int32]cpuSample{},
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
