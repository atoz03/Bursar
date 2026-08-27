package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultPowerRackHostReserveW = 500

var powerRackCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type PowerRack struct {
	RackCode  string    `json:"rack_code"`
	Name      string    `json:"name"`
	CapacityW int       `json:"capacity_w"`
	SlotCount int       `json:"slot_count"`
	Location  string    `json:"location"`
	Note      string    `json:"note"`
	SortOrder int       `json:"sort_order"`
	UpdatedBy string    `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PowerRackNode struct {
	NodeID          string    `json:"node_id"`
	RackCode        string    `json:"rack_code"`
	SlotNumber      int       `json:"slot_number"`
	AllocatedPowerW int       `json:"allocated_power_w"`
	DeviceLabel     string    `json:"device_label"`
	Note            string    `json:"note"`
	UpdatedBy       string    `json:"updated_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type PowerRackNodeView struct {
	PowerRackNode
	NodeKnown               bool       `json:"node_known"`
	Online                  bool       `json:"online"`
	PowerDataFresh          bool       `json:"power_data_fresh"`
	MonitorMetricsAvailable bool       `json:"monitor_metrics_available"`
	LastSeenAt              *time.Time `json:"last_seen_at,omitempty"`
	CPUModel                string     `json:"cpu_model"`
	CPUCount                int        `json:"cpu_count"`
	GPUModel                string     `json:"gpu_model"`
	GPUCount                int        `json:"gpu_count"`
	ReportedGPUCount        int        `json:"reported_gpu_count"`
	HostCPUPercent          float64    `json:"host_cpu_percent"`
	GPUPowerDrawW           float64    `json:"gpu_power_draw_w"`
	GPUPowerLimitW          float64    `json:"gpu_power_limit_w"`
	EstimatedPowerW         float64    `json:"estimated_power_w"`
	SuggestedPowerW         int        `json:"suggested_power_w"`
	Issues                  []string   `json:"issues"`
}

type PowerRackView struct {
	PowerRack
	AllocatedPowerW int                 `json:"allocated_power_w"`
	RemainingPowerW int                 `json:"remaining_power_w"`
	GPUPowerDrawW   float64             `json:"gpu_power_draw_w"`
	GPUPowerLimitW  float64             `json:"gpu_power_limit_w"`
	EstimatedPowerW float64             `json:"estimated_power_w"`
	UtilizationPct  float64             `json:"utilization_percent"`
	OnlineNodeCount int                 `json:"online_node_count"`
	NodeCount       int                 `json:"node_count"`
	IssueCount      int                 `json:"issue_count"`
	Status          string              `json:"status"`
	Nodes           []PowerRackNodeView `json:"nodes"`
}

type PowerRackNodeOption struct {
	NodeID          string  `json:"node_id"`
	Online          bool    `json:"online"`
	CPUModel        string  `json:"cpu_model"`
	GPUModel        string  `json:"gpu_model"`
	GPUCount        int     `json:"gpu_count"`
	GPUPowerLimitW  float64 `json:"gpu_power_limit_w"`
	SuggestedPowerW int     `json:"suggested_power_w"`
}

type PowerRackSummary struct {
	RackCount           int     `json:"rack_count"`
	CapacityW           int     `json:"capacity_w"`
	AllocatedPowerW     int     `json:"allocated_power_w"`
	GPUPowerDrawW       float64 `json:"gpu_power_draw_w"`
	GPUPowerLimitW      float64 `json:"gpu_power_limit_w"`
	EstimatedPowerW     float64 `json:"estimated_power_w"`
	OverflowRackCount   int     `json:"overflow_rack_count"`
	WarningRackCount    int     `json:"warning_rack_count"`
	ConfigIssueCount    int     `json:"config_issue_count"`
	OfflineNodeCount    int     `json:"offline_node_count"`
	UnreportedNodeCount int     `json:"unreported_node_count"`
}

type PowerRackOverview struct {
	Racks           []PowerRackView       `json:"racks"`
	UnassignedNodes []PowerRackNodeOption `json:"unassigned_nodes"`
	Summary         PowerRackSummary      `json:"summary"`
	GeneratedAt     string                `json:"generated_at"`
}

func normalizePowerRackCode(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("rack_code 不能为空")
	}
	if len(value) > 32 || !powerRackCodePattern.MatchString(value) {
		return "", errors.New("rack_code 仅支持 1-32 位字母、数字、下划线或连字符")
	}
	return value, nil
}

func validatePowerRackInput(rack PowerRack) (PowerRack, error) {
	code, err := normalizePowerRackCode(rack.RackCode)
	if err != nil {
		return PowerRack{}, err
	}
	rack.RackCode = code
	rack.Name = strings.TrimSpace(rack.Name)
	rack.Location = strings.TrimSpace(rack.Location)
	rack.Note = strings.TrimSpace(rack.Note)
	if rack.Name == "" {
		rack.Name = "机柜 " + code
	}
	if len(rack.Name) > 80 {
		return PowerRack{}, errors.New("name 最长 80 个字符")
	}
	if rack.CapacityW <= 0 || rack.CapacityW > 1_000_000 {
		return PowerRack{}, errors.New("capacity_w 应在 1-1000000 之间")
	}
	if rack.SlotCount <= 0 || rack.SlotCount > 60 {
		return PowerRack{}, errors.New("slot_count 应在 1-60 之间")
	}
	if len(rack.Location) > 160 {
		return PowerRack{}, errors.New("location 最长 160 个字符")
	}
	if len(rack.Note) > 2000 {
		return PowerRack{}, errors.New("note 最长 2000 个字符")
	}
	return rack, nil
}

func validatePowerRackNodeInput(item PowerRackNode) (PowerRackNode, error) {
	code, err := normalizePowerRackCode(item.RackCode)
	if err != nil {
		return PowerRackNode{}, err
	}
	item.RackCode = code
	item.NodeID = strings.TrimSpace(item.NodeID)
	item.DeviceLabel = strings.TrimSpace(item.DeviceLabel)
	item.Note = strings.TrimSpace(item.Note)
	if item.NodeID == "" || len(item.NodeID) > 50 {
		return PowerRackNode{}, errors.New("node_id 不能为空且最长 50 个字符")
	}
	if item.SlotNumber <= 0 || item.SlotNumber > 60 {
		return PowerRackNode{}, errors.New("slot_number 应在 1-60 之间")
	}
	if item.AllocatedPowerW < 0 || item.AllocatedPowerW > 1_000_000 {
		return PowerRackNode{}, errors.New("allocated_power_w 应在 0-1000000 之间")
	}
	if len(item.DeviceLabel) > 160 {
		return PowerRackNode{}, errors.New("device_label 最长 160 个字符")
	}
	if len(item.Note) > 2000 {
		return PowerRackNode{}, errors.New("note 最长 2000 个字符")
	}
	return item, nil
}

func normalizePowerRackTimes(rack *PowerRack) {
	if rack == nil {
		return
	}
	rack.CreatedAt = asBeijingWallTime(rack.CreatedAt)
	rack.UpdatedAt = asBeijingWallTime(rack.UpdatedAt)
}

func normalizePowerRackNodeTimes(item *PowerRackNode) {
	if item == nil {
		return
	}
	item.CreatedAt = asBeijingWallTime(item.CreatedAt)
	item.UpdatedAt = asBeijingWallTime(item.UpdatedAt)
}

func (s *Store) ListPowerRackConfig(ctx context.Context) ([]PowerRack, []PowerRackNode, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT rack_code, name, capacity_w, slot_count, location, note, sort_order,
       updated_by, created_at, updated_at
FROM power_racks
ORDER BY sort_order, rack_code`)
	if err != nil {
		return nil, nil, err
	}
	racks := make([]PowerRack, 0)
	for rows.Next() {
		var rack PowerRack
		if err := rows.Scan(
			&rack.RackCode, &rack.Name, &rack.CapacityW, &rack.SlotCount,
			&rack.Location, &rack.Note, &rack.SortOrder, &rack.UpdatedBy,
			&rack.CreatedAt, &rack.UpdatedAt,
		); err != nil {
			rows.Close()
			return nil, nil, err
		}
		normalizePowerRackTimes(&rack)
		racks = append(racks, rack)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	nodeRows, err := s.db.QueryContext(ctx, `
SELECT node_id, rack_code, slot_number, allocated_power_w, device_label, note,
       updated_by, created_at, updated_at
FROM power_rack_nodes
ORDER BY rack_code, slot_number, node_id`)
	if err != nil {
		return nil, nil, err
	}
	defer nodeRows.Close()
	nodes := make([]PowerRackNode, 0)
	for nodeRows.Next() {
		var item PowerRackNode
		if err := nodeRows.Scan(
			&item.NodeID, &item.RackCode, &item.SlotNumber, &item.AllocatedPowerW,
			&item.DeviceLabel, &item.Note, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, nil, err
		}
		normalizePowerRackNodeTimes(&item)
		nodes = append(nodes, item)
	}
	return racks, nodes, nodeRows.Err()
}

func (s *Store) UpsertPowerRack(ctx context.Context, rack PowerRack, updatedBy string) (PowerRack, error) {
	rack, err := validatePowerRackInput(rack)
	if err != nil {
		return PowerRack{}, err
	}
	updatedBy = strings.TrimSpace(updatedBy)
	if updatedBy == "" {
		updatedBy = "admin"
	}
	var out PowerRack
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		var maxSlot int
		if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(slot_number), 0)
FROM power_rack_nodes
WHERE rack_code=$1`, rack.RackCode).Scan(&maxSlot); err != nil {
			return err
		}
		if maxSlot > rack.SlotCount {
			return fmt.Errorf("机柜仍有设备位于槽位 %d，不能缩减为 %d 个槽位", maxSlot, rack.SlotCount)
		}
		return tx.QueryRowContext(ctx, `
INSERT INTO power_racks(rack_code, name, capacity_w, slot_count, location, note, sort_order, updated_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (rack_code) DO UPDATE SET
  name=EXCLUDED.name,
  capacity_w=EXCLUDED.capacity_w,
  slot_count=EXCLUDED.slot_count,
  location=EXCLUDED.location,
  note=EXCLUDED.note,
  sort_order=EXCLUDED.sort_order,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()
RETURNING rack_code, name, capacity_w, slot_count, location, note, sort_order,
          updated_by, created_at, updated_at`,
			rack.RackCode, rack.Name, rack.CapacityW, rack.SlotCount,
			rack.Location, rack.Note, rack.SortOrder, updatedBy,
		).Scan(
			&out.RackCode, &out.Name, &out.CapacityW, &out.SlotCount,
			&out.Location, &out.Note, &out.SortOrder, &out.UpdatedBy,
			&out.CreatedAt, &out.UpdatedAt,
		)
	})
	if err != nil {
		return PowerRack{}, err
	}
	normalizePowerRackTimes(&out)
	return out, nil
}

func (s *Store) DeletePowerRack(ctx context.Context, rackCode string) error {
	code, err := normalizePowerRackCode(rackCode)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM power_racks WHERE rack_code=$1`, code)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpsertPowerRackNode(ctx context.Context, item PowerRackNode, updatedBy string) (PowerRackNode, error) {
	item, err := validatePowerRackNodeInput(item)
	if err != nil {
		return PowerRackNode{}, err
	}
	updatedBy = strings.TrimSpace(updatedBy)
	if updatedBy == "" {
		updatedBy = "admin"
	}
	var out PowerRackNode
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		var slotCount int
		if err := tx.QueryRowContext(ctx, `SELECT slot_count FROM power_racks WHERE rack_code=$1`, item.RackCode).Scan(&slotCount); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("机柜不存在")
			}
			return err
		}
		if item.SlotNumber > slotCount {
			return fmt.Errorf("槽位 %d 超出机柜槽位数 %d", item.SlotNumber, slotCount)
		}
		var occupiedBy string
		err := tx.QueryRowContext(ctx, `
SELECT node_id FROM power_rack_nodes
WHERE rack_code=$1 AND slot_number=$2 AND node_id<>$3`,
			item.RackCode, item.SlotNumber, item.NodeID,
		).Scan(&occupiedBy)
		if err == nil {
			return fmt.Errorf("槽位 %d 已由节点 %s 占用", item.SlotNumber, occupiedBy)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return tx.QueryRowContext(ctx, `
INSERT INTO power_rack_nodes(node_id, rack_code, slot_number, allocated_power_w, device_label, note, updated_by)
VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (node_id) DO UPDATE SET
  rack_code=EXCLUDED.rack_code,
  slot_number=EXCLUDED.slot_number,
  allocated_power_w=EXCLUDED.allocated_power_w,
  device_label=EXCLUDED.device_label,
  note=EXCLUDED.note,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()
RETURNING node_id, rack_code, slot_number, allocated_power_w, device_label, note,
          updated_by, created_at, updated_at`,
			item.NodeID, item.RackCode, item.SlotNumber, item.AllocatedPowerW,
			item.DeviceLabel, item.Note, updatedBy,
		).Scan(
			&out.NodeID, &out.RackCode, &out.SlotNumber, &out.AllocatedPowerW,
			&out.DeviceLabel, &out.Note, &out.UpdatedBy, &out.CreatedAt, &out.UpdatedAt,
		)
	})
	if err != nil {
		return PowerRackNode{}, err
	}
	normalizePowerRackNodeTimes(&out)
	return out, nil
}

func (s *Store) DeletePowerRackNode(ctx context.Context, rackCode string, nodeID string) error {
	code, err := normalizePowerRackCode(rackCode)
	if err != nil {
		return err
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
DELETE FROM power_rack_nodes
WHERE rack_code=$1 AND node_id=$2`, code, nodeID)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func powerRackNodeOnline(node NodeStatus, now time.Time) bool {
	if node.LastSeenAt.IsZero() {
		return false
	}
	timeout := 5 * time.Minute
	if node.IntervalSeconds > 0 {
		intervalTimeout := time.Duration(node.IntervalSeconds*3) * time.Second
		if intervalTimeout > timeout {
			timeout = intervalTimeout
		}
	}
	age := now.Sub(node.LastSeenAt)
	return age >= -time.Minute && age <= timeout
}

func sumPowerRackGPU(devices []GPUDeviceStatus) (float64, float64) {
	var draw float64
	var limit float64
	for _, gpu := range devices {
		draw += finiteNonNegative(gpu.PowerDrawW)
		limit += finiteNonNegative(gpu.PowerLimitW)
	}
	return draw, limit
}

func suggestedNodePowerW(gpuLimitW float64) int {
	if gpuLimitW <= 0 {
		return 0
	}
	return int(math.Ceil((gpuLimitW+defaultPowerRackHostReserveW)/100.0) * 100)
}

// estimatedNodePowerW 用现有 Agent 指标估算整机实时功率：GPU 使用驱动实测值，
// 主机部分以“核算功率 - GPU 上限”为预算，并按 35% 空闲底座到 100% CPU 满载线性折算。
// CPU/存储节点没有 GPU 上限时，整台机器的核算功率作为主机预算。
func estimatedNodePowerW(item PowerRackNode, gpuDrawW float64, gpuLimitW float64, hostCPUPercent float64) float64 {
	cpu := monitorPercent(hostCPUPercent)
	hostBudget := float64(item.AllocatedPowerW) - gpuLimitW
	if gpuLimitW <= 0 {
		hostBudget = float64(item.AllocatedPowerW)
	}
	if hostBudget < 0 {
		hostBudget = 0
	}
	hostFactor := 0.35 + 0.65*(cpu/100.0)
	return finiteNonNegative(gpuDrawW) + hostBudget*hostFactor
}

func buildPowerRackNodeView(
	item PowerRackNode,
	node NodeStatus,
	nodeKnown bool,
	monitor NodeMonitorStatus,
	monitorKnown bool,
	now time.Time,
) PowerRackNodeView {
	view := PowerRackNodeView{
		PowerRackNode: item,
		NodeKnown:     nodeKnown,
		Issues:        []string{},
	}
	if !nodeKnown {
		view.Issues = append(view.Issues, "node_not_reported")
		return view
	}
	view.Online = powerRackNodeOnline(node, now)
	lastSeen := asBeijingWallTime(node.LastSeenAt)
	view.LastSeenAt = &lastSeen
	view.CPUModel = node.CPUModel
	view.CPUCount = node.CPUCount
	view.GPUModel = node.GPUModel
	view.GPUCount = node.GPUCount
	if !view.Online {
		view.Issues = append(view.Issues, "node_offline")
	}
	if monitorKnown {
		view.MonitorMetricsAvailable = monitor.MonitorMetricsAvailable
		view.HostCPUPercent = monitor.HostCPUPercent
		view.ReportedGPUCount = len(monitor.GPUDevices)
		view.GPUPowerDrawW, view.GPUPowerLimitW = sumPowerRackGPU(monitor.GPUDevices)
		view.SuggestedPowerW = suggestedNodePowerW(view.GPUPowerLimitW)
		view.PowerDataFresh = view.Online && monitor.MonitorMetricsAvailable && len(monitor.GPUDevices) > 0
		if view.Online && monitor.MonitorMetricsAvailable {
			view.EstimatedPowerW = estimatedNodePowerW(item, view.GPUPowerDrawW, view.GPUPowerLimitW, monitor.HostCPUPercent)
		}
	}
	if view.Online && node.GPUCount > 0 && (!monitorKnown || len(monitor.GPUDevices) == 0) {
		view.Issues = append(view.Issues, "gpu_power_missing")
	}
	if view.Online && node.GPUCount > 0 && monitorKnown && len(monitor.GPUDevices) > 0 && node.GPUCount != len(monitor.GPUDevices) {
		view.Issues = append(view.Issues, "gpu_count_mismatch")
	}
	if view.GPUPowerLimitW > 0 && float64(item.AllocatedPowerW) < view.GPUPowerLimitW {
		view.Issues = append(view.Issues, "allocated_below_gpu_limit")
	}
	return view
}

func isPowerRackConfigIssue(code string) bool {
	return code == "allocated_below_gpu_limit" || code == "gpu_count_mismatch" || code == "gpu_power_missing"
}

func buildPowerRackOverview(
	racks []PowerRack,
	assignments []PowerRackNode,
	nodes []NodeStatus,
	monitors []NodeMonitorStatus,
	now time.Time,
) PowerRackOverview {
	nodeByID := make(map[string]NodeStatus, len(nodes))
	for _, node := range nodes {
		nodeByID[node.NodeID] = node
	}
	monitorByID := make(map[string]NodeMonitorStatus, len(monitors))
	for _, monitor := range monitors {
		monitorByID[monitor.NodeID] = monitor
	}
	assigned := make(map[string]struct{}, len(assignments))
	assignmentsByRack := make(map[string][]PowerRackNode)
	for _, item := range assignments {
		assigned[item.NodeID] = struct{}{}
		assignmentsByRack[item.RackCode] = append(assignmentsByRack[item.RackCode], item)
	}

	overview := PowerRackOverview{
		Racks:           make([]PowerRackView, 0, len(racks)),
		UnassignedNodes: []PowerRackNodeOption{},
		GeneratedAt:     formatRFC3339InBeijing(inBeijing(now)),
	}
	for _, rack := range racks {
		view := PowerRackView{
			PowerRack: rack,
			Nodes:     []PowerRackNodeView{},
			Status:    "normal",
		}
		items := assignmentsByRack[rack.RackCode]
		sort.Slice(items, func(i, j int) bool {
			if items[i].SlotNumber != items[j].SlotNumber {
				return items[i].SlotNumber < items[j].SlotNumber
			}
			return items[i].NodeID < items[j].NodeID
		})
		for _, item := range items {
			node, nodeKnown := nodeByID[item.NodeID]
			monitor, monitorKnown := monitorByID[item.NodeID]
			nodeView := buildPowerRackNodeView(item, node, nodeKnown, monitor, monitorKnown, now)
			view.Nodes = append(view.Nodes, nodeView)
			view.NodeCount++
			view.AllocatedPowerW += item.AllocatedPowerW
			view.GPUPowerLimitW += nodeView.GPUPowerLimitW
			if nodeView.Online {
				view.OnlineNodeCount++
			} else {
				overview.Summary.OfflineNodeCount++
			}
			if !nodeView.NodeKnown {
				overview.Summary.UnreportedNodeCount++
			}
			if nodeView.PowerDataFresh {
				view.GPUPowerDrawW += nodeView.GPUPowerDrawW
			}
			if nodeView.Online && nodeView.MonitorMetricsAvailable {
				view.EstimatedPowerW += nodeView.EstimatedPowerW
			}
			nodeHasConfigIssue := false
			for _, issue := range nodeView.Issues {
				if isPowerRackConfigIssue(issue) {
					nodeHasConfigIssue = true
					break
				}
			}
			if nodeHasConfigIssue {
				view.IssueCount++
				overview.Summary.ConfigIssueCount++
			}
		}
		view.RemainingPowerW = rack.CapacityW - int(math.Ceil(view.EstimatedPowerW))
		if rack.CapacityW > 0 {
			view.UtilizationPct = view.EstimatedPowerW / float64(rack.CapacityW) * 100
		}
		switch {
		case view.EstimatedPowerW > float64(rack.CapacityW):
			view.Status = "overflow"
			overview.Summary.OverflowRackCount++
		case view.UtilizationPct >= 90:
			view.Status = "warning"
			overview.Summary.WarningRackCount++
		case view.IssueCount > 0:
			view.Status = "attention"
		}
		overview.Racks = append(overview.Racks, view)
		overview.Summary.CapacityW += rack.CapacityW
		overview.Summary.AllocatedPowerW += view.AllocatedPowerW
		overview.Summary.GPUPowerDrawW += view.GPUPowerDrawW
		overview.Summary.GPUPowerLimitW += view.GPUPowerLimitW
		overview.Summary.EstimatedPowerW += view.EstimatedPowerW
	}
	overview.Summary.RackCount = len(overview.Racks)

	for _, node := range nodes {
		if _, ok := assigned[node.NodeID]; ok {
			continue
		}
		monitor, monitorKnown := monitorByID[node.NodeID]
		var limit float64
		if monitorKnown {
			_, limit = sumPowerRackGPU(monitor.GPUDevices)
		}
		overview.UnassignedNodes = append(overview.UnassignedNodes, PowerRackNodeOption{
			NodeID:          node.NodeID,
			Online:          powerRackNodeOnline(node, now),
			CPUModel:        node.CPUModel,
			GPUModel:        node.GPUModel,
			GPUCount:        node.GPUCount,
			GPUPowerLimitW:  limit,
			SuggestedPowerW: suggestedNodePowerW(limit),
		})
	}
	sort.Slice(overview.UnassignedNodes, func(i, j int) bool {
		return overview.UnassignedNodes[i].NodeID < overview.UnassignedNodes[j].NodeID
	})
	return overview
}

func (s *Server) loadPowerRackOverview(ctx context.Context) (PowerRackOverview, error) {
	racks, assignments, err := s.store.ListPowerRackConfig(ctx)
	if err != nil {
		return PowerRackOverview{}, err
	}
	nodes, err := s.store.ListNodes(ctx, 2000)
	if err != nil {
		return PowerRackOverview{}, err
	}
	monitors, err := s.store.ListNodeMonitorStatuses(ctx, 2000)
	if err != nil {
		return PowerRackOverview{}, err
	}
	return buildPowerRackOverview(racks, assignments, nodes, monitors, nowInBeijing()), nil
}

type powerRackUpsertReq struct {
	RackCode  string `json:"rack_code"`
	Name      string `json:"name"`
	CapacityW int    `json:"capacity_w"`
	SlotCount int    `json:"slot_count"`
	Location  string `json:"location"`
	Note      string `json:"note"`
	SortOrder int    `json:"sort_order"`
}

type powerRackNodeUpsertReq struct {
	NodeID          string `json:"node_id"`
	SlotNumber      int    `json:"slot_number"`
	AllocatedPowerW int    `json:"allocated_power_w"`
	DeviceLabel     string `json:"device_label"`
	Note            string `json:"note"`
}

func (s *Server) handleAdminPowerRacksOverview(c *gin.Context) {
	overview, err := s.loadPowerRackOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, overview)
}

func (s *Server) handleAdminPowerRackUpsert(c *gin.Context) {
	var req powerRackUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rack, err := s.store.UpsertPowerRack(c.Request.Context(), PowerRack{
		RackCode:  req.RackCode,
		Name:      req.Name,
		CapacityW: req.CapacityW,
		SlotCount: req.SlotCount,
		Location:  req.Location,
		Note:      req.Note,
		SortOrder: req.SortOrder,
	}, s.currentOperator(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "rack": rack})
}

func (s *Server) handleAdminPowerRackDelete(c *gin.Context) {
	if err := s.store.DeletePowerRack(c.Request.Context(), c.Param("code")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "机柜不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminPowerRackNodeUpsert(c *gin.Context) {
	rackCode, err := normalizePowerRackCode(c.Param("code"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req powerRackNodeUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := s.store.UpsertPowerRackNode(c.Request.Context(), PowerRackNode{
		NodeID:          req.NodeID,
		RackCode:        rackCode,
		SlotNumber:      req.SlotNumber,
		AllocatedPowerW: req.AllocatedPowerW,
		DeviceLabel:     req.DeviceLabel,
		Note:            req.Note,
	}, s.currentOperator(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "node": item})
}

func (s *Server) handleAdminPowerRackNodeDelete(c *gin.Context) {
	if err := s.store.DeletePowerRackNode(c.Request.Context(), c.Param("code"), c.Param("node_id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "机柜设备不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
