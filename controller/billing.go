package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type PriceIndex struct {
	// models 按长度倒序，避免 "RTX 30" 抢先匹配 "RTX 3090"
	models []string
	price  map[string]float64
}

func NewPriceIndex(rows []PriceRow) PriceIndex {
	price := make(map[string]float64, len(rows))
	models := make([]string, 0, len(rows))
	for _, r := range rows {
		m := strings.TrimSpace(r.Model)
		if m == "" {
			continue
		}
		price[m] = r.Price
		models = append(models, m)
	}
	sort.Slice(models, func(i, j int) bool {
		if len(models[i]) == len(models[j]) {
			return models[i] > models[j]
		}
		return len(models[i]) > len(models[j])
	})
	return PriceIndex{models: models, price: price}
}

func (pi PriceIndex) MatchPrice(gpuModel string) (float64, bool) {
	for _, m := range pi.models {
		if strings.Contains(gpuModel, m) {
			return pi.price[m], true
		}
	}
	return 0, false
}

// CalculateProcessCost 计算单个进程在一个采样周期（默认 1 分钟）内的费用。
func CalculateProcessCost(proc UserProcess, prices PriceIndex, defaultPricePerMinute float64) float64 {
	cost := 0.0
	for _, g := range proc.GPUUsage {
		if p, ok := prices.MatchPrice(g.GPUModel); ok {
			cost += p
		} else {
			cost += defaultPricePerMinute
		}
	}
	// 金额保留 4 位小数，便于后续聚合与对账
	return math.Round(cost*10000) / 10000
}

// CalculateProcessCostWithPolicy 计算“节点策略 + 全局价格”下的 GPU 费用。
// 优先级：节点按卡型单价 > 节点统一单价 > 全局 GPU 型号单价 > 默认单价。
func CalculateProcessCostWithPolicy(
	proc UserProcess,
	nodeModelPrices PriceIndex,
	nodePricePerMinute *float64,
	globalPrices PriceIndex,
	defaultPricePerMinute float64,
) float64 {
	return CalculateGPUUsageCostWithPolicy(proc.GPUUsage, nodeModelPrices, nodePricePerMinute, globalPrices, defaultPricePerMinute)
}

func CalculateGPUUsageCostWithPolicy(
	gpuUsage []GPUUsage,
	nodeModelPrices PriceIndex,
	nodePricePerMinute *float64,
	globalPrices PriceIndex,
	defaultPricePerMinute float64,
) float64 {
	cost := 0.0
	for _, g := range gpuUsage {
		if p, ok := nodeModelPrices.MatchPrice(g.GPUModel); ok {
			cost += p
			continue
		}
		if nodePricePerMinute != nil {
			cost += *nodePricePerMinute
			continue
		}
		if p, ok := globalPrices.MatchPrice(g.GPUModel); ok {
			cost += p
			continue
		}
		cost += defaultPricePerMinute
	}
	return math.Round(cost*10000) / 10000
}

func GPUUsageBillingKey(g GPUUsage) string {
	if busID := strings.TrimSpace(g.GPUBusID); busID != "" {
		return "bus:" + strings.ToLower(busID)
	}
	if g.GPUID >= 0 {
		return fmt.Sprintf("id:%d", g.GPUID)
	}
	return "model:" + strings.TrimSpace(g.GPUModel)
}

func ChargeableGPUUsageForBilling(gpuUsage []GPUUsage, seen map[string]struct{}) []GPUUsage {
	if len(gpuUsage) == 0 {
		return nil
	}
	out := make([]GPUUsage, 0, len(gpuUsage))
	for _, g := range gpuUsage {
		key := GPUUsageBillingKey(g)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, g)
	}
	return out
}

func StatusForBalance(balance, warningThreshold, limitedThreshold float64) string {
	if balance < 0 {
		return "blocked"
	}
	if balance < limitedThreshold {
		return "limited"
	}
	if balance < warningThreshold {
		return "warning"
	}
	return "normal"
}

// EffectiveStatusForBalance 用于接口展示：
// 1) 默认按“实时可用积分”计算状态；
// 2) 仅当账号确实处于管理员手动拉黑时，才强制保持 blocked。
func EffectiveStatusForBalance(storedStatus string, manualBlocked bool, effectiveBalance, warningThreshold, limitedThreshold float64) string {
	derived := StatusForBalance(effectiveBalance, warningThreshold, limitedThreshold)
	if manualBlocked && strings.EqualFold(strings.TrimSpace(storedStatus), "blocked") && derived != "blocked" {
		return "blocked"
	}
	return derived
}

func normalizeMonthlyMaxOverdraftLimit(limit float64) float64 {
	if math.IsNaN(limit) || math.IsInf(limit, 0) || limit < 0 {
		return 0
	}
	return limit
}

// DecideActions 根据余额状态决定提示动作（仅 notify）。
// 具体的 GPU 限制组/CPU 限速/强制中断，由调用方按余额策略单独下发，
// 这样可避免策略耦合导致重复动作或误触发。
func DecideActions(prevStatus string, user User, warningThreshold, limitedThreshold, maxOverdraftLimit float64) []Action {
	newStatus := user.Status
	if newStatus == "" {
		newStatus = StatusForBalance(user.Balance, warningThreshold, limitedThreshold)
	}

	var actions []Action

	switch newStatus {
	case "warning":
		if prevStatus != "warning" {
			actions = append(actions, Action{
				Type:     "notify",
				Username: user.Username,
				Message:  formatBalanceMessage("余额预警", user.Balance),
			})
		}
	case "limited":
		if prevStatus != "limited" {
			actions = append(actions, Action{
				Type:     "notify",
				Username: user.Username,
				Message:  formatBalanceMessage("余额不足，已触发限速", user.Balance),
			})
		}
	case "blocked":
		if prevStatus != "blocked" {
			msg := formatBalanceMessage("已欠费，已触发限速", user.Balance)
			if isOverdraftExceeded(user.Balance, maxOverdraftLimit) {
				msg = formatBalanceMessage(
					fmt.Sprintf("已超过欠费上限 %.2f，GPU 将禁用并触发一次性清进程", normalizeMonthlyMaxOverdraftLimit(maxOverdraftLimit)),
					user.Balance,
				)
			}
			actions = append(actions, Action{
				Type:     "notify",
				Username: user.Username,
				Message:  msg,
			})
		}
	}

	return actions
}

func formatBalanceMessage(prefix string, balance float64) string {
	return strings.TrimSpace(prefix) + "（当前积分：" + formatMoney(balance) + "）"
}

func formatMoney(v float64) string {
	// 统一输出两位小数，便于脚本解析与前端展示
	return fmt.Sprintf("%.2f", v)
}
