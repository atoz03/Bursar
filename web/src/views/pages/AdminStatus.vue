<template>
  <div class="monitor-page">
    <section class="ops-page-hero">
      <div class="ops-hero-copy">
        <span class="ops-eyebrow">CLUSTER OVERVIEW</span>
        <div class="ops-title-row">
          <span class="ops-hero-icon"><el-icon><Monitor /></el-icon></span>
          <div>
            <h1>集群总览</h1>
            <p>优先展示 CPU 与逐卡 GPU 状态，异常节点自动排在前面。</p>
          </div>
        </div>
      </div>
      <div class="ops-hero-actions">
        <span class="live-badge"><i />实时</span>
        <span class="ops-sync-meta">{{ AUTO_REFRESH_SECONDS }} 秒自动刷新 · {{ lastRefreshText }}</span>
        <el-button :loading="loading" type="primary" @click="loadMonitor">
          <el-icon><Refresh /></el-icon>
          立即刷新
        </el-button>
      </div>
    </section>

    <el-alert v-if="error" class="monitor-error" type="error" show-icon :closable="false" :title="error" />

    <section class="ops-metric-grid monitor-summary-grid">
      <article class="ops-metric-card ops-tone-green">
        <span>在线节点</span>
        <strong>{{ onlineCount }}<small>/ {{ nodes.length }}</small></strong>
        <small>当前可正常上报的节点</small>
      </article>
      <article class="ops-metric-card ops-tone-violet">
        <span>活跃 GPU</span>
        <strong>{{ busyGPUCount }}<small>/ {{ totalGPUCount }}</small></strong>
        <small>逐卡统计当前活跃设备</small>
      </article>
      <article class="ops-metric-card ops-tone-blue">
        <span>平均 CPU</span>
        <strong>{{ averageCPUText }}</strong>
        <small>在线节点平均占用率</small>
      </article>
      <article class="ops-metric-card ops-tone-amber">
        <span>需关注</span>
        <strong>{{ warningCount }}<small> 台</small></strong>
        <small>资源或服务状态异常</small>
      </article>
    </section>

    <section class="monitor-toolbar">
      <div class="filter-tabs" role="tablist" aria-label="节点状态筛选">
        <button
          v-for="item in filterOptions"
          :key="item.value"
          type="button"
          :class="{ active: activeFilter === item.value }"
          @click="activeFilter = item.value"
        >
          {{ item.label }} <span>{{ item.count }}</span>
        </button>
      </div>
      <el-input v-model="keyword" class="node-search" clearable placeholder="搜索节点、CPU 或 GPU 型号">
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
    </section>

    <div v-if="loading && nodes.length === 0" class="skeleton-grid">
      <el-skeleton v-for="i in 6" :key="i" animated class="node-skeleton" :rows="8" />
    </div>

    <section v-else-if="filteredNodes.length" class="node-grid">
      <article
        v-for="node in filteredNodes"
        :key="node.node_id"
        class="node-card"
        :class="`state-${nodeState(node)}`"
      >
        <header class="node-head">
          <div class="node-identity">
            <span class="status-dot" />
            <div>
              <h2>{{ node.node_id }}</h2>
              <span>{{ compactModel(node.gpu_model || node.cpu_model || "硬件信息待上报") }}</span>
            </div>
          </div>
          <div class="node-head-tags">
            <span class="state-chip">{{ nodeStateText(node) }}</span>
            <span class="heartbeat-chip">{{ heartbeatText(node) }}</span>
          </div>
        </header>

        <div class="primary-metrics">
          <div class="metric-panel cpu-panel">
            <div class="metric-title"><span>CPU</span><strong>{{ metricPercent(node.host_cpu_percent, node.monitor_metrics_available) }}</strong></div>
            <div class="progress-track"><span :class="barTone(node.host_cpu_percent)" :style="barWidth(node.host_cpu_percent)" /></div>
            <div class="metric-foot">
              <span>{{ node.cpu_count || "-" }} 核</span>
              <span>负载 {{ loadText(node) }}</span>
            </div>
          </div>
          <div class="metric-panel memory-panel">
            <div class="metric-title"><span>内存</span><strong>{{ metricPercent(memoryPercent(node), node.monitor_metrics_available) }}</strong></div>
            <div class="progress-track"><span :class="barTone(memoryPercent(node))" :style="barWidth(memoryPercent(node))" /></div>
            <div class="metric-foot">
              <span>{{ memoryUsageText(node) }}</span>
            </div>
          </div>
        </div>

        <div class="gpu-section">
          <div class="section-label">
            <span>GPU <em>{{ compactModel(node.gpu_model || "") }}</em></span>
            <small><b>{{ gpuBusyOnNode(node) }}</b>/{{ node.gpu_count || displayGPUs(node).length }} 活跃</small>
          </div>
          <div v-if="displayGPUs(node).length" class="gpu-grid">
            <div
              v-for="gpu in displayGPUs(node)"
              :key="`${node.node_id}-${gpu.index}`"
              class="gpu-tile"
              :class="gpuState(gpu)"
            >
              <div class="gpu-head">
                <strong :title="gpu.name || node.gpu_model">GPU {{ gpu.index }}</strong>
                <span>{{ gpu.pending ? "待上报" : gpuStateText(gpu) }}</span>
              </div>
              <div class="gpu-values">
                <span><i>核心</i><b>{{ gpu.pending ? "--" : `${round(gpu.utilization_percent)}%` }}</b></span>
                <span><i>显存</i><b>{{ gpu.pending ? "--" : `${round(gpuMemoryPercent(gpu))}%` }}</b></span>
              </div>
              <div class="gpu-bars">
                <div class="mini-track"><i :class="barTone(gpu.utilization_percent)" :style="barWidth(gpu.utilization_percent)" /></div>
                <div class="mini-track"><i :class="barTone(gpuMemoryPercent(gpu))" :style="barWidth(gpuMemoryPercent(gpu))" /></div>
              </div>
              <div class="gpu-meta">
                <span>{{ gpu.pending ? "等待新 Agent" : `${formatMemory(gpu.memory_used_mb)} / ${formatMemory(gpu.memory_total_mb)}` }}</span>
                <span v-if="!gpu.pending">{{ temperatureText(gpu) }} · {{ gpu.compute_processes || 0 }} 进程</span>
              </div>
            </div>
          </div>
          <div v-else class="no-gpu">GPU 数据不可用或未配置</div>
        </div>

        <footer class="node-footer">
          <span><i>硬盘</i>{{ diskText(node) }}</span>
          <span><i>SSH</i>{{ node.ssh_active_count || 0 }}</span>
          <span><i>运行</i>{{ uptimeText(node.host_uptime_seconds).replace("运行 ", "") }}</span>
          <span class="heartbeat"><i>负载</i>{{ Number(node.host_load_1 || 0).toFixed(2) }}</span>
        </footer>
      </article>
    </section>

    <el-empty v-else description="没有符合筛选条件的节点" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Monitor, Refresh, Search } from "@element-plus/icons-vue";
import { ApiClient, type GPUDeviceStatus, type NodeMonitorStatus } from "../../lib/api";
import { authState } from "../../lib/authStore";
import { settingsState } from "../../lib/settingsStore";

type FilterValue = "all" | "online" | "busy" | "warning" | "offline";
type DisplayGPU = GPUDeviceStatus & { pending?: boolean };

const AUTO_REFRESH_SECONDS = 15;
const nodes = ref<NodeMonitorStatus[]>([]);
const loading = ref(false);
const error = ref("");
const keyword = ref("");
const activeFilter = ref<FilterValue>("all");
const lastRefreshAt = ref(0);
let refreshTimer: ReturnType<typeof setInterval> | null = null;

function heartbeatTimeoutMs(node: NodeMonitorStatus): number {
  return Math.max(5 * 60_000, Number(node.interval_seconds || 0) * 3_000);
}

function isOnline(node: NodeMonitorStatus): boolean {
  const ts = Date.parse(String(node.last_seen_at || node.last_report_ts || ""));
  return Number.isFinite(ts) && Date.now() - ts <= heartbeatTimeoutMs(node);
}

function memoryPercent(node: NodeMonitorStatus): number {
  const total = Number(node.host_memory_total_mb || 0);
  return total > 0 ? Number(node.host_memory_used_mb || 0) / total * 100 : 0;
}

function diskPercent(node: NodeMonitorStatus): number {
  const total = Number(node.disk_total_gb || 0);
  return total > 0 ? Number(node.disk_used_gb || 0) / total * 100 : 0;
}

function gpuMemoryPercent(gpu: DisplayGPU): number {
  const total = Number(gpu.memory_total_mb || 0);
  return total > 0 ? Number(gpu.memory_used_mb || 0) / total * 100 : 0;
}

function isGPUActive(gpu: DisplayGPU): boolean {
  return !gpu.pending && (Number(gpu.utilization_percent || 0) >= 5 || Number(gpu.compute_processes || 0) > 0);
}

function hasWarning(node: NodeMonitorStatus): boolean {
  if (!isOnline(node)) return false;
  const unhealthyService = (node.system_services || []).some((item) => item.deployed && !item.healthy);
  const hotGPU = (node.gpu_devices || []).some((gpu) => Number(gpu.temperature_c || 0) >= 85);
  return unhealthyService || Number(node.host_cpu_percent || 0) >= 90 || memoryPercent(node) >= 90 || diskPercent(node) >= 90 || hotGPU;
}

function nodeState(node: NodeMonitorStatus): "online" | "warning" | "offline" | "pending" {
  if (!isOnline(node)) return "offline";
  if (hasWarning(node)) return "warning";
  if (!node.monitor_metrics_available) return "pending";
  return "online";
}

function nodeStateText(node: NodeMonitorStatus): string {
  const state = nodeState(node);
  if (state === "offline") return "离线";
  if (state === "warning") return "需关注";
  if (state === "pending") return "等待指标";
  return "运行正常";
}

const onlineCount = computed(() => nodes.value.filter(isOnline).length);
const warningCount = computed(() => nodes.value.filter(hasWarning).length);
const totalGPUCount = computed(() => nodes.value.reduce((sum, node) => sum + Number(node.gpu_count || node.gpu_devices?.length || 0), 0));
const busyGPUCount = computed(() => nodes.value.reduce((sum, node) => sum + (node.gpu_devices || []).filter(isGPUActive).length, 0));
const averageCPUText = computed(() => {
  const ready = nodes.value.filter((node) => isOnline(node) && node.monitor_metrics_available);
  if (!ready.length) return "--";
  const avg = ready.reduce((sum, node) => sum + Number(node.host_cpu_percent || 0), 0) / ready.length;
  return `${avg.toFixed(1)}%`;
});

const filterOptions = computed(() => [
  { value: "all" as FilterValue, label: "全部节点", count: nodes.value.length },
  { value: "online" as FilterValue, label: "在线", count: onlineCount.value },
  { value: "busy" as FilterValue, label: "GPU 活跃", count: nodes.value.filter((node) => (node.gpu_devices || []).some(isGPUActive)).length },
  { value: "warning" as FilterValue, label: "需关注", count: warningCount.value },
  { value: "offline" as FilterValue, label: "离线", count: nodes.value.length - onlineCount.value },
]);

const filteredNodes = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  return [...nodes.value]
    .filter((node) => {
      if (activeFilter.value === "online" && !isOnline(node)) return false;
      if (activeFilter.value === "offline" && isOnline(node)) return false;
      if (activeFilter.value === "warning" && !hasWarning(node)) return false;
      if (activeFilter.value === "busy" && !(node.gpu_devices || []).some(isGPUActive)) return false;
      if (!q) return true;
      const haystack = [node.node_id, node.cpu_model, node.gpu_model, node.node_ip, ...(node.gpu_devices || []).map((gpu) => gpu.name)].join(" ").toLowerCase();
      return haystack.includes(q);
    })
    .sort((a, b) => {
      const rank = (node: NodeMonitorStatus) => nodeState(node) === "warning" ? 0 : nodeState(node) === "online" ? 1 : nodeState(node) === "pending" ? 2 : 3;
      return rank(a) - rank(b) || String(a.node_id).localeCompare(String(b.node_id), undefined, { numeric: true });
    });
});

const lastRefreshText = computed(() => {
  if (!lastRefreshAt.value) return "尚未刷新";
  return new Date(lastRefreshAt.value).toLocaleTimeString("zh-CN", { hour12: false });
});

function displayGPUs(node: NodeMonitorStatus): DisplayGPU[] {
  if (node.gpu_devices?.length) return node.gpu_devices;
  return Array.from({ length: Number(node.gpu_count || 0) }, (_, index) => ({
    index,
    name: node.gpu_model,
    utilization_percent: 0,
    memory_used_mb: 0,
    memory_total_mb: 0,
    compute_processes: 0,
    pending: true,
  }));
}

function gpuBusyOnNode(node: NodeMonitorStatus): number {
  return (node.gpu_devices || []).filter(isGPUActive).length;
}

function gpuState(gpu: DisplayGPU): string {
  if (gpu.pending) return "gpu-pending";
  if (Number(gpu.temperature_c || 0) >= 85) return "gpu-hot";
  if (isGPUActive(gpu)) return "gpu-busy";
  return "gpu-idle";
}

function gpuStateText(gpu: DisplayGPU): string {
  if (Number(gpu.temperature_c || 0) >= 85) return "温度偏高";
  return isGPUActive(gpu) ? "运行中" : "空闲";
}

function barTone(value: number): string {
  if (Number(value || 0) >= 90) return "bar-danger";
  if (Number(value || 0) >= 70) return "bar-warning";
  return "bar-normal";
}

function barWidth(value: number): Record<string, string> {
  return { width: `${Math.max(0, Math.min(100, Number(value || 0)))}%` };
}

function round(value: number): number {
  return Math.round(Number(value || 0));
}

function metricPercent(value: number, available: boolean): string {
  return available ? `${Number(value || 0).toFixed(1)}%` : "--";
}

function formatMemory(mb: number): string {
  const value = Number(mb || 0);
  if (!value) return "--";
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} TiB`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} GiB`;
  return `${value.toFixed(value >= 100 ? 0 : 1)} MiB`;
}

function memoryUsageText(node: NodeMonitorStatus): string {
  if (!node.monitor_metrics_available || !node.host_memory_total_mb) return "等待上报";
  return `${formatMemory(node.host_memory_used_mb)} / ${formatMemory(node.host_memory_total_mb)}`;
}

function loadText(node: NodeMonitorStatus): string {
  if (!node.monitor_metrics_available) return "--";
  return `${Number(node.host_load_1 || 0).toFixed(2)} · ${Number(node.host_load_5 || 0).toFixed(2)} · ${Number(node.host_load_15 || 0).toFixed(2)}`;
}

function diskText(node: NodeMonitorStatus): string {
  return node.disk_total_gb ? `${round(diskPercent(node))}%` : "--";
}

function uptimeText(seconds: number): string {
  const value = Number(seconds || 0);
  if (!value) return "运行时间 --";
  const days = Math.floor(value / 86400);
  if (days > 0) return `运行 ${days} 天`;
  return `运行 ${Math.max(1, Math.floor(value / 3600))} 小时`;
}

function heartbeatText(node: NodeMonitorStatus): string {
  const ts = Date.parse(String(node.last_seen_at || ""));
  if (!Number.isFinite(ts)) return "--";
  const seconds = Math.max(0, Math.floor((Date.now() - ts) / 1000));
  if (seconds < 60) return `${seconds} 秒前`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分前`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前`;
  return `${Math.floor(seconds / 86400)} 天前`;
}

function temperatureText(gpu: DisplayGPU): string {
  const temp = Number(gpu.temperature_c || 0);
  return temp > 0 ? `${round(temp)}℃` : "温度 --";
}

function compactModel(value: string): string {
  const text = String(value || "").trim();
  return text.length > 30 ? `${text.slice(0, 28)}…` : text;
}

async function loadMonitor(): Promise<void> {
  if (loading.value) return;
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const result = await client.adminNodeMonitor(200);
    nodes.value = result.nodes || [];
    lastRefreshAt.value = Date.now();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

function startRefresh(): void {
  refreshTimer = setInterval(() => {
    if (!document.hidden) void loadMonitor();
  }, AUTO_REFRESH_SECONDS * 1000);
}

onMounted(() => {
  void loadMonitor();
  startRefresh();
});

onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer);
});
</script>

<style scoped>
.monitor-page {
  min-height: calc(100vh - 96px);
  padding: 2px;
  color: #102033;
}

.monitor-error { margin-top: 16px; }

.monitor-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin: 18px 0; padding: 10px; border: 1px solid rgba(148,163,184,.25); border-radius: 16px; background: rgba(255,255,255,.7); }
.filter-tabs { display: flex; flex-wrap: wrap; gap: 6px; }
.filter-tabs button { appearance: none; padding: 9px 13px; border: 0; border-radius: 11px; color: #526277; background: transparent; cursor: pointer; font: inherit; font-size: 13px; font-weight: 650; transition: .2s ease; }
.filter-tabs button span { display: inline-grid; place-items: center; min-width: 21px; height: 21px; margin-left: 4px; padding: 0 6px; border-radius: 999px; background: rgba(100,116,139,.1); font-size: 11px; }
.filter-tabs button:hover { background: rgba(14,165,233,.08); color: #0369a1; }
.filter-tabs button.active { color: #047857; background: rgba(16,185,129,.12); }
.node-search { width: min(330px, 100%); }

.node-grid, .skeleton-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(390px, 1fr)); align-items: start; gap: 18px; }
.node-skeleton { min-height: 460px; padding: 24px; border-radius: 20px; background: #fff; }
.node-card { --state: #10b981; position: relative; overflow: hidden; padding: 20px; border: 1px solid rgba(148,163,184,.28); border-top: 3px solid var(--state); border-radius: 21px; background: linear-gradient(155deg, rgba(255,255,255,.96), rgba(244,248,252,.92)); box-shadow: 0 15px 35px rgba(40,61,84,.09); transition: transform .2s ease, box-shadow .2s ease; }
.node-card:hover { transform: translateY(-3px); box-shadow: 0 20px 42px rgba(35,58,82,.14); }
.node-card.state-warning { --state: #f59e0b; } .node-card.state-offline { --state: #94a3b8; } .node-card.state-pending { --state: #3b82f6; }
.node-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.node-identity { display: flex; align-items: flex-start; gap: 11px; min-width: 0; }
.status-dot { width: 11px; height: 11px; margin-top: 7px; flex: 0 0 auto; border-radius: 50%; background: var(--state); box-shadow: 0 0 0 5px color-mix(in srgb, var(--state) 13%, transparent); }
.node-identity h2 { margin: 0; font-size: 19px; letter-spacing: -.02em; }
.node-identity div > span { display: block; max-width: 250px; margin-top: 4px; overflow: hidden; color: #7a8899; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.node-head-tags { display: flex; align-items: flex-end; gap: 5px; flex-direction: column; }
.state-chip, .uptime-chip { padding: 5px 9px; border-radius: 999px; color: color-mix(in srgb, var(--state) 80%, #1e293b); background: color-mix(in srgb, var(--state) 10%, white); font-size: 11px; font-weight: 700; white-space: nowrap; }
.uptime-chip { color: #64748b; background: #eef2f6; font-weight: 500; }

.primary-metrics { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin: 19px 0 17px; padding: 16px; border-radius: 16px; background: rgba(232,239,245,.7); }
.metric-panel { min-width: 0; }
.metric-title { display: flex; align-items: center; justify-content: space-between; color: #46566a; font-size: 13px; }
.metric-title strong { color: #17283c; font-size: 18px; }
.progress-track, .mini-track { overflow: hidden; height: 6px; margin: 8px 0; border-radius: 999px; background: rgba(148,163,184,.2); }
.progress-track span, .mini-track i { display: block; height: 100%; border-radius: inherit; transition: width .45s ease; }
.bar-normal { background: linear-gradient(90deg, #10b981, #34d399); } .bar-warning { background: linear-gradient(90deg, #f59e0b, #fbbf24); } .bar-danger { background: linear-gradient(90deg, #ef4444, #fb7185); }
.metric-foot { display: flex; align-items: center; justify-content: space-between; gap: 8px; color: #718095; font-size: 10px; white-space: nowrap; }

.gpu-section { padding-top: 2px; }
.section-label { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.section-label > span { display: flex; align-items: center; gap: 7px; color: #33465c; font-size: 13px; font-weight: 800; }
.section-label small { color: #7d8b9d; }
.gpu-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; max-height: 335px; padding-right: 3px; overflow-y: auto; scrollbar-width: thin; }
.gpu-tile { --gpu-tone: #10b981; min-width: 0; padding: 12px; border: 1px solid color-mix(in srgb, var(--gpu-tone) 20%, #dbe4ec); border-radius: 14px; background: color-mix(in srgb, var(--gpu-tone) 4%, white); }
.gpu-tile.gpu-busy { --gpu-tone: #8b5cf6; } .gpu-tile.gpu-hot { --gpu-tone: #ef4444; } .gpu-tile.gpu-pending { --gpu-tone: #94a3b8; }
.gpu-head { display: flex; align-items: center; justify-content: space-between; gap: 6px; }
.gpu-head strong { color: #25384e; font-size: 13px; }
.gpu-head span { color: var(--gpu-tone); font-size: 10px; font-weight: 750; }
.gpu-model { margin: 3px 0 10px; overflow: hidden; color: #8793a3; font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.gpu-row { display: grid; grid-template-columns: 30px 34px 1fr; align-items: center; gap: 5px; color: #6a7a8e; font-size: 10px; }
.gpu-row b { color: #304359; font-size: 10px; text-align: right; }
.mini-track { height: 4px; margin: 4px 0; }
.gpu-meta { display: flex; align-items: center; justify-content: space-between; gap: 5px; margin-top: 6px; color: #6d7d90; font-size: 9px; }
.no-gpu { display: grid; place-items: center; min-height: 84px; border: 1px dashed #cbd5e1; border-radius: 14px; color: #94a3b8; font-size: 12px; }

.node-footer { display: grid; grid-template-columns: repeat(4, auto); align-items: center; gap: 10px; margin-top: 16px; padding-top: 14px; border-top: 1px solid #e2e8f0; color: #34475d; font-size: 11px; }
.node-footer span { display: flex; gap: 4px; white-space: nowrap; }
.node-footer i { color: #8a98a9; font-style: normal; }
.node-footer .heartbeat { justify-self: end; }

@media (max-width: 760px) {
  .monitor-toolbar { align-items: stretch; flex-direction: column; }
  .node-grid, .skeleton-grid { grid-template-columns: minmax(0, 1fr); }
  .node-search { width: 100%; }
}

@media (max-width: 500px) {
  .primary-metrics, .gpu-grid { grid-template-columns: 1fr; }
  .node-card { padding: 16px; }
  .node-footer { grid-template-columns: repeat(2, 1fr); }
  .node-footer .heartbeat { justify-self: start; }
}
/* 轻量监控主题：减少嵌套容器和装饰，让节点与 GPU 数据成为视觉主体。 */
.monitor-page {
  min-height: calc(100vh - 96px);
  padding: 4px 2px 28px;
  color: #172235;
}
.monitor-summary-grid { margin: 14px 0; }
.live-badge { display: inline-flex; align-items: center; gap: 6px; padding: 4px 9px; border-radius: 999px; color: #07885f; background: #e9f9f2; font-size: 11px; font-weight: 700; }
.live-badge i { width: 6px; height: 6px; border-radius: 50%; background: #10b981; box-shadow: 0 0 0 4px rgba(16,185,129,.12); }
@keyframes statusLivePulse {
  0%, 100% { opacity: .72; transform: scale(.88); }
  50% { opacity: 1; transform: scale(1.12); }
}

.monitor-toolbar { margin: 0 0 14px; padding: 4px 2px; border: 0; border-radius: 0; background: transparent; }
.filter-tabs { gap: 4px; padding: 3px; border-radius: 11px; background: #eef2f5; }
.filter-tabs button { padding: 7px 11px; border-radius: 8px; color: #667587; font-size: 12px; font-weight: 600; }
.filter-tabs button span { min-width: 18px; height: 18px; margin-left: 3px; padding: 0 5px; background: rgba(100,116,139,.09); font-size: 10px; }
.filter-tabs button:hover { color: #17694f; background: rgba(255,255,255,.62); }
.filter-tabs button.active { color: #087854; background: #fff; box-shadow: 0 1px 4px rgba(38,55,72,.1); }
.node-search { width: min(290px, 100%); }
.node-search :deep(.el-input__wrapper) { border-radius: 10px; box-shadow: 0 0 0 1px #dde4ea inset; background: #fff; }

.node-grid, .skeleton-grid { grid-template-columns: repeat(auto-fill, minmax(350px, 1fr)); gap: 14px; }
.node-grid { align-items: stretch; grid-auto-rows: 445px; }
.node-skeleton { min-height: 390px; padding: 20px; border: 1px solid #e3e8ed; border-radius: 16px; box-shadow: none; }
.node-card {
  --state: #10b981;
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
  padding: 17px;
  border: 1px solid rgba(255,255,255,.76);
  border-top: 1px solid rgba(255,255,255,.76);
  border-radius: 16px;
  background: linear-gradient(145deg, rgba(255,255,255,.74), rgba(247,250,255,.46));
  box-shadow: 0 12px 30px rgba(36,53,72,.09), inset 0 1px 0 rgba(255,255,255,.92);
}
.node-card::before { content: ""; position: absolute; top: 0; bottom: 0; left: 0; width: 3px; background: var(--state); }
.node-card:hover { transform: translateY(-2px); border-color: #cfd9e1; box-shadow: 0 12px 28px rgba(36,53,72,.1); }
.node-card.state-warning { --state: #f59e0b; }
.node-card.state-offline { --state: #a8b2bf; background: linear-gradient(145deg, rgba(250,251,252,.72), rgba(241,245,249,.42)); }
.node-card.state-pending { --state: #3b82f6; }

.node-head { flex: 0 0 auto; align-items: flex-start; }
.node-identity { gap: 10px; }
.status-dot { width: 8px; height: 8px; margin-top: 6px; box-shadow: 0 0 0 4px color-mix(in srgb, var(--state) 11%, transparent); }
.node-identity h2 { font-size: 17px; }
.node-identity div > span { max-width: 225px; margin-top: 3px; color: #8793a1; font-size: 11px; }
.node-head-tags { gap: 4px; }
.state-chip { padding: 4px 8px; font-size: 10px; }
.heartbeat-chip { color: #929dab; font-size: 10px; white-space: nowrap; }

.primary-metrics { flex: 0 0 auto; gap: 18px; margin: 16px 0; padding: 0; border-radius: 0; background: transparent; }
.metric-panel { padding: 12px; border: 1px solid rgba(255,255,255,.74); border-radius: 12px; background: rgba(255,255,255,.42); box-shadow: inset 0 1px 0 rgba(255,255,255,.88); }
.metric-title { color: #667587; font-size: 11px; }
.metric-title strong { color: #17283a; font-size: 17px; }
.progress-track { height: 5px; margin: 7px 0; background: #e7ecf0; }
.metric-foot { justify-content: flex-start; overflow: hidden; color: #8995a3; font-size: 9px; }
.metric-foot span { min-width: 0; overflow: hidden; text-overflow: ellipsis; }

.gpu-section { display: flex; flex: 1 1 auto; min-height: 0; padding-top: 1px; flex-direction: column; }
.section-label { margin-bottom: 8px; }
.section-label > span { min-width: 0; gap: 7px; color: #314255; font-size: 12px; }
.section-label > span em { overflow: hidden; color: #94a0ad; font-size: 10px; font-style: normal; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.section-label small { flex: 0 0 auto; color: #8c98a6; font-size: 10px; }
.section-label small b { color: #5c6e82; font-weight: 750; }
.gpu-grid { flex: 1 1 auto; align-content: start; min-height: 0; max-height: none; gap: 7px; padding-right: 5px; overflow-y: scroll; scrollbar-width: thin; scrollbar-color: #b9c4ce #edf1f4; scrollbar-gutter: stable; }
.gpu-grid::-webkit-scrollbar { width: 7px; }
.gpu-grid::-webkit-scrollbar-track { border-radius: 999px; background: #edf1f4; }
.gpu-grid::-webkit-scrollbar-thumb { border: 2px solid #edf1f4; border-radius: 999px; background: #aebbc6; }
.gpu-tile { padding: 10px; border-color: rgba(255,255,255,.74); border-radius: 11px; background: rgba(255,255,255,.44); box-shadow: inset 0 1px 0 rgba(255,255,255,.82); }
.gpu-tile.gpu-busy { border-color: rgba(196,181,253,.58); background: rgba(250,245,255,.58); }
.gpu-tile.gpu-hot { border-color: rgba(254,202,202,.72); background: rgba(255,248,248,.62); }
.gpu-tile.gpu-pending { border-style: dashed; background: rgba(248,250,252,.44); }
.gpu-head strong { font-size: 11px; }
.gpu-head span { font-size: 9px; }
.gpu-values { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 8px; }
.gpu-values span { display: flex; align-items: baseline; justify-content: space-between; gap: 4px; }
.gpu-values i { color: #8d99a6; font-size: 9px; font-style: normal; }
.gpu-values b { color: #304155; font-size: 11px; }
.gpu-bars { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.mini-track { height: 3px; margin: 4px 0 5px; background: #e6ebef; }
.gpu-meta { margin-top: 3px; color: #8b97a4; font-size: 8px; }
.no-gpu { flex: 1 1 auto; min-height: 64px; border-color: #dce3e9; border-radius: 11px; color: #9aa5b1; font-size: 11px; background: #fafbfc; }

.node-footer { flex: 0 0 auto; grid-template-columns: repeat(4, 1fr); gap: 7px; margin-top: 13px; padding-top: 11px; color: #536579; font-size: 9px; }
.node-footer span { display: grid; gap: 2px; white-space: nowrap; }
.node-footer i { color: #9ba6b2; }
.node-footer .heartbeat { justify-self: stretch; }

@media (max-width: 950px) {
  .monitor-summary-grid { grid-template-columns: repeat(2, 1fr); }
}

@media (max-width: 760px) {
  .monitor-toolbar { align-items: stretch; flex-direction: column; }
  .node-grid, .skeleton-grid { grid-template-columns: minmax(0, 1fr); }
  .node-grid { grid-auto-rows: 485px; }
  .node-search { width: 100%; }
}

@media (max-width: 500px) {
  .monitor-summary-grid { grid-template-columns: 1fr; }
  .primary-metrics, .gpu-grid { grid-template-columns: 1fr; }
  .node-grid { grid-auto-rows: 535px; }
  .gpu-grid { max-height: none; }
  .node-card { padding: 15px; }
}
</style>
