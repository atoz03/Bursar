package main

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// 控制器内置最小监控指标（Prometheus 文本格式的子集）
// 目标：上线可观测性，不引入额外依赖。
type controllerMetrics struct {
	reportsTotal          atomic.Int64
	reportsDuplicateTotal atomic.Int64
	usageRecordsTotal     atomic.Int64

	actionsNotifyTotal      atomic.Int64
	actionsBlockUserTotal   atomic.Int64
	actionsUnblockUserTotal atomic.Int64
	actionsKillTotal        atomic.Int64
	actionsCPUQuotaTotal    atomic.Int64

	lastReportUnix atomic.Int64
}

func (m *controllerMetrics) observeReport(receivedAt time.Time, duplicate bool, usageRecords int, actions []Action) {
	if duplicate {
		m.reportsDuplicateTotal.Add(1)
		return
	}
	m.reportsTotal.Add(1)
	m.usageRecordsTotal.Add(int64(usageRecords))
	m.lastReportUnix.Store(receivedAt.Unix())

	for _, a := range actions {
		switch a.Type {
		case "notify":
			m.actionsNotifyTotal.Add(1)
		case "block_user":
			m.actionsBlockUserTotal.Add(1)
		case "unblock_user":
			m.actionsUnblockUserTotal.Add(1)
		case "kill_process":
			m.actionsKillTotal.Add(1)
		case "set_cpu_quota":
			m.actionsCPUQuotaTotal.Add(1)
		}
	}
}

func (m *controllerMetrics) render(queueLen int) string {
	var b strings.Builder
	write := func(name string, v int64) {
		_, _ = fmt.Fprintf(&b, "%s %d\n", name, v)
	}
	write("gpuops_controller_reports_total", m.reportsTotal.Load())
	write("gpuops_controller_reports_duplicate_total", m.reportsDuplicateTotal.Load())
	write("gpuops_controller_usage_records_total", m.usageRecordsTotal.Load())

	write("gpuops_controller_actions_notify_total", m.actionsNotifyTotal.Load())
	write("gpuops_controller_actions_block_user_total", m.actionsBlockUserTotal.Load())
	write("gpuops_controller_actions_unblock_user_total", m.actionsUnblockUserTotal.Load())
	write("gpuops_controller_actions_kill_process_total", m.actionsKillTotal.Load())
	write("gpuops_controller_actions_set_cpu_quota_total", m.actionsCPUQuotaTotal.Load())

	write("gpuops_controller_queue_length", int64(queueLen))
	write("gpuops_controller_last_report_unix", m.lastReportUnix.Load())
	return b.String()
}
