package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type NodeMonitorStatus struct {
	NodeID                  string            `json:"node_id"`
	ReportTS                time.Time         `json:"monitor_report_ts"`
	MonitorMetricsAvailable bool              `json:"monitor_metrics_available"`
	HostCPUPercent          float64           `json:"host_cpu_percent"`
	HostMemoryTotalMB       float64           `json:"host_memory_total_mb"`
	HostMemoryUsedMB        float64           `json:"host_memory_used_mb"`
	HostLoad1               float64           `json:"host_load_1"`
	HostLoad5               float64           `json:"host_load_5"`
	HostLoad15              float64           `json:"host_load_15"`
	HostUptimeSeconds       uint64            `json:"host_uptime_seconds"`
	AgentMemoryMB           float64           `json:"agent_memory_mb"`
	GPUDevices              []GPUDeviceStatus `json:"gpu_devices"`
}

type NodeMonitorView struct {
	NodeStatus
	MonitorReportTS         *time.Time        `json:"monitor_report_ts,omitempty"`
	MonitorMetricsAvailable bool              `json:"monitor_metrics_available"`
	HostCPUPercent          float64           `json:"host_cpu_percent"`
	HostMemoryTotalMB       float64           `json:"host_memory_total_mb"`
	HostMemoryUsedMB        float64           `json:"host_memory_used_mb"`
	HostLoad1               float64           `json:"host_load_1"`
	HostLoad5               float64           `json:"host_load_5"`
	HostLoad15              float64           `json:"host_load_15"`
	HostUptimeSeconds       uint64            `json:"host_uptime_seconds"`
	AgentMemoryMB           float64           `json:"agent_memory_mb"`
	GPUDevices              []GPUDeviceStatus `json:"gpu_devices"`
}

func finiteNonNegative(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}

func monitorPercent(v float64) float64 {
	v = finiteNonNegative(v)
	if v > 100 {
		return 100
	}
	return v
}

func normalizeMonitorGPUDevices(in []GPUDeviceStatus) []GPUDeviceStatus {
	if len(in) == 0 {
		return []GPUDeviceStatus{}
	}
	seen := make(map[int32]struct{}, len(in))
	out := make([]GPUDeviceStatus, 0, len(in))
	for _, item := range in {
		if item.Index < 0 || item.Index > 255 {
			continue
		}
		if _, ok := seen[item.Index]; ok {
			continue
		}
		seen[item.Index] = struct{}{}
		item.UUID = monitorText(item.UUID, 160)
		item.Name = monitorText(item.Name, 160)
		item.BusID = monitorText(item.BusID, 80)
		item.UtilizationPct = monitorPercent(item.UtilizationPct)
		item.MemoryUsedMB = finiteNonNegative(item.MemoryUsedMB)
		item.MemoryTotalMB = finiteNonNegative(item.MemoryTotalMB)
		if item.MemoryTotalMB > 0 && item.MemoryUsedMB > item.MemoryTotalMB {
			item.MemoryUsedMB = item.MemoryTotalMB
		}
		item.TemperatureC = finiteNonNegative(item.TemperatureC)
		item.PowerDrawW = finiteNonNegative(item.PowerDrawW)
		item.PowerLimitW = finiteNonNegative(item.PowerLimitW)
		if item.ComputeProcesses < 0 {
			item.ComputeProcesses = 0
		}
		out = append(out, item)
		if len(out) >= 64 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func monitorText(raw string, maxLen int) string {
	value := strings.TrimSpace(raw)
	if maxLen > 0 && len(value) > maxLen {
		return value[:maxLen]
	}
	return value
}

func (s *Store) UpsertNodeMonitorStatusTx(ctx context.Context, tx *sql.Tx, data MetricsData, reportTS time.Time) error {
	if tx == nil {
		return errors.New("tx 不能为空")
	}
	nodeID := strings.TrimSpace(data.NodeID)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	devicesJSON, err := json.Marshal(normalizeMonitorGPUDevices(data.GPUDevices))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO node_monitor_status(
  node_id, report_ts, metrics_available, host_cpu_percent,
  host_memory_total_mb, host_memory_used_mb,
  host_load_1, host_load_5, host_load_15, host_uptime_seconds,
  agent_memory_mb, gpu_devices, updated_at
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,NOW())
ON CONFLICT (node_id) DO UPDATE SET
  report_ts=EXCLUDED.report_ts,
  metrics_available=EXCLUDED.metrics_available,
  host_cpu_percent=EXCLUDED.host_cpu_percent,
  host_memory_total_mb=EXCLUDED.host_memory_total_mb,
  host_memory_used_mb=EXCLUDED.host_memory_used_mb,
  host_load_1=EXCLUDED.host_load_1,
  host_load_5=EXCLUDED.host_load_5,
  host_load_15=EXCLUDED.host_load_15,
  host_uptime_seconds=EXCLUDED.host_uptime_seconds,
  agent_memory_mb=EXCLUDED.agent_memory_mb,
  gpu_devices=EXCLUDED.gpu_devices,
  updated_at=NOW()`,
		nodeID,
		reportTS,
		data.MonitorMetricsAvailable,
		monitorPercent(data.HostCPUPercent),
		finiteNonNegative(data.HostMemoryTotalMB),
		finiteNonNegative(data.HostMemoryUsedMB),
		finiteNonNegative(data.HostLoad1),
		finiteNonNegative(data.HostLoad5),
		finiteNonNegative(data.HostLoad15),
		int64(data.HostUptimeSeconds),
		finiteNonNegative(data.AgentMemoryMB),
		string(devicesJSON),
	)
	return err
}

func (s *Store) ListNodeMonitorStatuses(ctx context.Context, limit int) ([]NodeMonitorStatus, error) {
	if limit <= 0 || limit > 2000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT node_id, report_ts, metrics_available, host_cpu_percent,
       host_memory_total_mb, host_memory_used_mb,
       host_load_1, host_load_5, host_load_15, host_uptime_seconds,
       agent_memory_mb, COALESCE(gpu_devices, '[]'::jsonb)
FROM node_monitor_status
ORDER BY report_ts DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeMonitorStatus, 0)
	for rows.Next() {
		var item NodeMonitorStatus
		var uptime int64
		var devicesRaw []byte
		if err := rows.Scan(
			&item.NodeID,
			&item.ReportTS,
			&item.MonitorMetricsAvailable,
			&item.HostCPUPercent,
			&item.HostMemoryTotalMB,
			&item.HostMemoryUsedMB,
			&item.HostLoad1,
			&item.HostLoad5,
			&item.HostLoad15,
			&uptime,
			&item.AgentMemoryMB,
			&devicesRaw,
		); err != nil {
			return nil, err
		}
		if uptime > 0 {
			item.HostUptimeSeconds = uint64(uptime)
		}
		if err := json.Unmarshal(devicesRaw, &item.GPUDevices); err != nil {
			return nil, err
		}
		item.GPUDevices = normalizeMonitorGPUDevices(item.GPUDevices)
		item.ReportTS = asBeijingWallTime(item.ReportTS)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Server) handleAdminNodeMonitor(c *gin.Context) {
	limit := 200
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	nodes, err := s.store.ListNodes(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if getAuthRole(c) == "power_user" {
		username := strings.TrimSpace(c.GetString("auth_user"))
		visible, err := s.store.ListVisibleNodeIDsForPowerUser(c.Request.Context(), username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		visibleSet := make(map[string]struct{}, len(visible))
		for _, id := range visible {
			visibleSet[strings.TrimSpace(id)] = struct{}{}
		}
		filtered := make([]NodeStatus, 0, len(nodes))
		for _, node := range nodes {
			if _, ok := visibleSet[strings.TrimSpace(node.NodeID)]; ok {
				filtered = append(filtered, node)
			}
		}
		nodes = filtered
	}
	monitorRows, err := s.store.ListNodeMonitorStatuses(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	monitorByNode := make(map[string]NodeMonitorStatus, len(monitorRows))
	for _, item := range monitorRows {
		monitorByNode[item.NodeID] = item
	}
	views := make([]NodeMonitorView, 0, len(nodes))
	for _, node := range nodes {
		view := NodeMonitorView{NodeStatus: node, GPUDevices: []GPUDeviceStatus{}}
		if item, ok := monitorByNode[node.NodeID]; ok {
			ts := item.ReportTS
			view.MonitorReportTS = &ts
			view.MonitorMetricsAvailable = item.MonitorMetricsAvailable
			view.HostCPUPercent = item.HostCPUPercent
			view.HostMemoryTotalMB = item.HostMemoryTotalMB
			view.HostMemoryUsedMB = item.HostMemoryUsedMB
			view.HostLoad1 = item.HostLoad1
			view.HostLoad5 = item.HostLoad5
			view.HostLoad15 = item.HostLoad15
			view.HostUptimeSeconds = item.HostUptimeSeconds
			view.AgentMemoryMB = item.AgentMemoryMB
			view.GPUDevices = item.GPUDevices
		}
		views = append(views, view)
	}
	c.JSON(http.StatusOK, gin.H{"nodes": views, "generated_at": formatRFC3339InBeijing(nowInBeijing())})
}
