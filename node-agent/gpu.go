package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var errNoNvidiaSMI = errors.New("未检测到 nvidia-smi")

func (a *NodeAgent) getGPUUsageMap(ctx context.Context) (map[int32][]GPUUsage, error) {
	out := make(map[int32][]GPUUsage)

	lines, err := a.runNvidiaSMI(ctx,
		"--query-compute-apps=pid,gpu_name,gpu_bus_id,used_memory",
		"--format=csv,noheader,nounits",
	)
	if err != nil {
		// 无 GPU/无驱动时允许降级
		if errors.Is(err, errNoNvidiaSMI) {
			return out, nil
		}
		return nil, err
	}

	busIDToIndex, _ := a.getBusIDToIndexMap(ctx)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := splitCSVLine(line)
		if len(parts) < 4 {
			continue
		}

		pid, err := parseInt32(parts[0])
		if err != nil || pid <= 0 {
			continue
		}

		gpuName := strings.TrimSpace(parts[1])
		busID := strings.TrimSpace(parts[2])
		memMB, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)

		gpuID := int32(-1)
		if idx, ok := busIDToIndex[normalizeBusID(busID)]; ok {
			gpuID = idx
		}

		out[pid] = append(out[pid], GPUUsage{
			GPUID:    gpuID,
			GPUModel: gpuName,
			GPUBusID: busID,
			MemoryMB: memMB,
		})
	}

	return out, nil
}

func (a *NodeAgent) getGPUDeviceStatuses(ctx context.Context) ([]GPUDeviceStatus, error) {
	lines, err := a.runNvidiaSMI(ctx,
		"--query-gpu=index,uuid,name,pci.bus_id,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,power.limit",
		"--format=csv,noheader,nounits",
	)
	if err != nil {
		if errors.Is(err, errNoNvidiaSMI) {
			return []GPUDeviceStatus{}, nil
		}
		return nil, err
	}
	out := make([]GPUDeviceStatus, 0, len(lines))
	for _, line := range lines {
		if item, ok := parseGPUDeviceStatusLine(line); ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func parseGPUDeviceStatusLine(line string) (GPUDeviceStatus, bool) {
	parts := splitCSVLine(strings.TrimSpace(line))
	if len(parts) < 10 {
		return GPUDeviceStatus{}, false
	}
	idx, err := parseInt32(parts[0])
	if err != nil || idx < 0 {
		return GPUDeviceStatus{}, false
	}
	return GPUDeviceStatus{
		Index:          idx,
		UUID:           strings.TrimSpace(parts[1]),
		Name:           strings.TrimSpace(parts[2]),
		BusID:          strings.TrimSpace(parts[3]),
		UtilizationPct: parseNvidiaFloat(parts[4]),
		MemoryUsedMB:   parseNvidiaFloat(parts[5]),
		MemoryTotalMB:  parseNvidiaFloat(parts[6]),
		TemperatureC:   parseNvidiaFloat(parts[7]),
		PowerDrawW:     parseNvidiaFloat(parts[8]),
		PowerLimitW:    parseNvidiaFloat(parts[9]),
	}, true
}

func parseNvidiaFloat(raw string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func applyGPUProcessCounts(devices []GPUDeviceStatus, usage map[int32][]GPUUsage) []GPUDeviceStatus {
	if len(devices) == 0 {
		return devices
	}
	byIndex := make(map[int32]int, len(devices))
	for i := range devices {
		byIndex[devices[i].Index] = i
	}
	for _, processGPUs := range usage {
		seen := make(map[int32]struct{}, len(processGPUs))
		for _, item := range processGPUs {
			if _, duplicate := seen[item.GPUID]; duplicate {
				continue
			}
			seen[item.GPUID] = struct{}{}
			if i, ok := byIndex[item.GPUID]; ok {
				devices[i].ComputeProcesses++
			}
		}
	}
	return devices
}

func (a *NodeAgent) getBusIDToIndexMap(ctx context.Context) (map[string]int32, error) {
	now := time.Now()
	a.cacheMu.Lock()
	if len(a.gpuBusMapCache) > 0 && !a.gpuBusMapCachedAt.IsZero() && now.Sub(a.gpuBusMapCachedAt) < a.gpuBusMapCacheTTL {
		cached := make(map[string]int32, len(a.gpuBusMapCache))
		for k, v := range a.gpuBusMapCache {
			cached[k] = v
		}
		a.cacheMu.Unlock()
		return cached, nil
	}
	a.cacheMu.Unlock()

	lines, err := a.runNvidiaSMI(ctx,
		"--query-gpu=index,pci.bus_id",
		"--format=csv,noheader",
	)
	if err != nil {
		if errors.Is(err, errNoNvidiaSMI) {
			return map[string]int32{}, nil
		}
		return nil, err
	}

	out := make(map[string]int32)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := splitCSVLine(line)
		if len(parts) < 2 {
			continue
		}
		idx64, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 32)
		if err != nil {
			continue
		}
		busID := normalizeBusID(parts[1])
		out[busID] = int32(idx64)
	}
	a.cacheMu.Lock()
	a.gpuBusMapCache = make(map[string]int32, len(out))
	for k, v := range out {
		a.gpuBusMapCache[k] = v
	}
	a.gpuBusMapCachedAt = now
	a.cacheMu.Unlock()
	return out, nil
}

func (a *NodeAgent) getGPUInventory(ctx context.Context) (string, int, error) {
	now := time.Now()
	a.cacheMu.Lock()
	if !a.gpuInventoryCachedAt.IsZero() && now.Sub(a.gpuInventoryCachedAt) < a.gpuInventoryCacheTTL {
		model := a.gpuInventoryModelCache
		count := a.gpuInventoryCountCache
		a.cacheMu.Unlock()
		return model, count, nil
	}
	a.cacheMu.Unlock()

	lines, err := a.runNvidiaSMI(ctx,
		"--query-gpu=name",
		"--format=csv,noheader",
	)
	if err != nil {
		if errors.Is(err, errNoNvidiaSMI) {
			return "", 0, nil
		}
		return "", 0, err
	}
	count := 0
	model := ""
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		count++
		if model == "" {
			model = name
		}
	}
	a.cacheMu.Lock()
	a.gpuInventoryModelCache = model
	a.gpuInventoryCountCache = count
	a.gpuInventoryCachedAt = now
	a.cacheMu.Unlock()
	return model, count, nil
}

func (a *NodeAgent) runNvidiaSMI(ctx context.Context, args ...string) ([]string, error) {
	smiCtx := ctx
	var cancel context.CancelFunc
	if a.gpuCommandTimeout > 0 {
		smiCtx, cancel = context.WithTimeout(ctx, a.gpuCommandTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(smiCtx, "nvidia-smi", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		// Linux 上找不到命令一般是 *exec.Error
		var ee *exec.Error
		if errors.As(err, &ee) {
			return nil, errNoNvidiaSMI
		}
		return nil, fmt.Errorf("nvidia-smi 执行失败：%w（stderr=%s）", err, strings.TrimSpace(stderr.String()))
	}

	text := strings.TrimSpace(string(b))
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	return lines, nil
}

func splitCSVLine(line string) []string {
	// nvidia-smi 的 csv 输出本身不包含复杂转义，这里做最小实现即可
	parts := strings.Split(line, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func parseInt32(s string) (int32, error) {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 32)
	return int32(v), err
}

func normalizeBusID(busID string) string {
	// 统一为大写，去掉多余空格
	return strings.ToUpper(strings.TrimSpace(busID))
}
