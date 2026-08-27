<template>
  <div class="rack-page">
    <section class="ops-page-hero">
      <div class="ops-hero-copy">
        <span class="ops-eyebrow">RACK POWER PLAN</span>
        <div class="ops-title-row">
          <span class="ops-hero-icon"><el-icon><Grid /></el-icon></span>
          <div>
            <h1>机柜功率</h1>
            <p>实时功率、核算功率与机柜极限统一对照。</p>
          </div>
        </div>
      </div>
      <div class="ops-hero-actions">
        <span class="live-badge"><i />实时</span>
        <span class="ops-sync-meta">15 秒自动刷新 · {{ lastRefreshText }}</span>
        <el-button type="primary" :loading="loading" @click="loadOverview">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </section>

    <el-alert
      v-if="summary.overflow_rack_count || summary.warning_rack_count || summary.config_issue_count"
      class="rack-alert"
      type="warning"
      show-icon
      :closable="false"
      :title="alertTitle"
    />
    <el-alert
      v-else-if="summary.offline_node_count || summary.unreported_node_count"
      class="rack-alert"
      type="info"
      show-icon
      :closable="false"
      :title="`当前有 ${summary.offline_node_count} 台离线设备，${summary.unreported_node_count} 台设备未在节点监控中找到；配置仍会保留。`"
    />
    <el-alert v-if="error" class="rack-alert" type="error" show-icon :closable="false" :title="error" />

    <section class="ops-metric-grid rack-summary-grid">
      <article class="ops-metric-card compact-metric ops-tone-green">
        <div class="compact-metric-copy">
          <span>实时 / 极限</span>
          <strong>{{ powerText(summary.estimated_power_w) }} <i>/ {{ powerText(summary.capacity_w) }}</i></strong>
          <small>当前容量 {{ percentText(summary.estimated_power_w, summary.capacity_w) }}</small>
        </div>
        <div class="mini-donut" :style="donutStyle(summary.estimated_power_w, summary.capacity_w, '#10b981')"><span>{{ percentText(summary.estimated_power_w, summary.capacity_w) }}</span></div>
      </article>
      <article class="ops-metric-card compact-metric ops-tone-blue">
        <div class="compact-metric-copy">
          <span>核算 / 极限</span>
          <strong>{{ powerText(summary.allocated_power_w) }} <i>/ {{ powerText(summary.capacity_w) }}</i></strong>
          <small>规划占用 {{ percentText(summary.allocated_power_w, summary.capacity_w) }}</small>
        </div>
        <div class="mini-donut" :style="donutStyle(summary.allocated_power_w, summary.capacity_w, '#3b82f6')"><span>{{ percentText(summary.allocated_power_w, summary.capacity_w) }}</span></div>
      </article>
      <article class="ops-metric-card compact-metric ops-tone-violet">
        <div class="compact-metric-copy">
          <span>实时覆盖</span>
          <strong>{{ realtimeNodeCount }} <i>/ {{ configuredNodeCount }} 台</i></strong>
          <small>{{ missingRealtimeNodeCount }} 台无实时值</small>
        </div>
        <div class="mini-donut" :style="donutStyle(realtimeNodeCount, configuredNodeCount, '#8b5cf6')"><span>{{ percentText(realtimeNodeCount, configuredNodeCount) }}</span></div>
      </article>
      <article class="ops-metric-card compact-metric ops-tone-amber">
        <div class="compact-metric-copy">
          <span>实时告警</span>
          <strong>{{ alarmRackCount }} <i>/ {{ summary.rack_count }} 柜</i></strong>
          <small>超限 {{ summary.overflow_rack_count }} · 临界 {{ summary.warning_rack_count }}</small>
        </div>
        <div class="mini-donut" :style="donutStyle(alarmRackCount, summary.rack_count, '#f59e0b')"><span>{{ alarmRackCount }}</span></div>
      </article>
    </section>

    <section class="rack-toolbar">
      <div class="filter-tabs" role="tablist" aria-label="机柜状态筛选">
        <button v-for="item in filterOptions" :key="item.value" type="button" :class="{ active: activeFilter === item.value }" @click="activeFilter = item.value">
          {{ item.label }} <span>{{ item.count }}</span>
        </button>
      </div>
      <div class="rack-actions">
        <el-button plain @click="openNewRack"><el-icon><Plus /></el-icon>新增机柜</el-button>
      </div>
    </section>

    <section v-if="filteredRacks.length" class="rack-grid" v-loading="loading && !overview">
      <article v-for="rack in filteredRacks" :key="rack.rack_code" class="rack-card" :class="`rack-state-${rack.status}`">
        <header class="rack-head">
          <div>
            <div class="rack-title-line">
              <span class="rack-status-dot" />
              <h2>{{ rack.name }}</h2>
              <el-tag size="small" :type="rackTagType(rack.status)">{{ rackStatusText(rack.status) }}</el-tag>
            </div>
            <p><span v-if="rack.location">{{ rack.location }} · </span>{{ rack.node_count }}/{{ rack.slot_count }} 槽 · {{ rack.online_node_count }}/{{ rack.node_count }} 在线</p>
          </div>
          <el-button text circle title="编辑机柜" @click="openEditRack(rack)"><el-icon><EditPen /></el-icon></el-button>
        </header>

        <div class="rack-power-overview">
          <div class="rack-live-line">
            <span><small>实时 / 极限</small><strong>{{ powerText(rack.estimated_power_w) }}</strong><i>/ {{ powerText(rack.capacity_w) }}</i></span>
            <b>{{ percentText(rack.estimated_power_w, rack.capacity_w) }}</b>
          </div>
          <div class="rack-progress power-scale">
            <i class="planned-range" :style="{ width: `${powerPercent(rack.allocated_power_w, rack.capacity_w)}%` }" />
            <b class="live-range" :style="{ width: `${powerPercent(rack.estimated_power_w, rack.capacity_w)}%` }" />
            <em class="planned-marker" :style="{ left: `${powerPercent(rack.allocated_power_w, rack.capacity_w)}%` }" />
          </div>
          <div class="rack-power-foot">
            <span><i class="legend-plan" />核算 {{ powerText(rack.allocated_power_w) }}</span>
            <span>{{ rack.remaining_power_w >= 0 ? `实时余量 ${powerText(rack.remaining_power_w)}` : `实时超出 ${powerText(Math.abs(rack.remaining_power_w))}` }}</span>
          </div>
        </div>

        <div class="rack-slots">
          <div v-for="slot in rackSlots(rack)" :key="`${rack.rack_code}-${slot.number}`" class="rack-slot" :class="{ empty: !slot.node }">
            <template v-if="slot.node">
              <div class="slot-main">
                <div class="slot-node-head">
                  <span class="slot-index">{{ slot.number }}</span>
                  <strong>{{ slot.node.node_id }}</strong>
                  <span class="slot-online" :class="{ offline: !slot.node.online }"><i />{{ slot.node.online ? "在线" : (slot.node.node_known ? "离线" : "未上报") }}</span>
                  <span v-if="visibleNodeIssue(slot.node)" class="node-issue">{{ issueLabel(visibleNodeIssue(slot.node)) }}</span>
                  <span class="slot-actions">
                    <el-button text size="small" title="编辑设备" @click="openEditNode(rack, slot.node)"><el-icon><EditPen /></el-icon></el-button>
                    <el-button text size="small" type="danger" title="移除设备" @click="deleteNode(rack, slot.node)"><el-icon><Delete /></el-icon></el-button>
                  </span>
                </div>
                <span class="slot-label">{{ slot.node.device_label || slot.node.gpu_model || "设备标签待填写" }}</span>
                <div class="node-power-labels">
                  <span class="node-live"><small>实时</small><b>{{ nodeHasRealtime(slot.node) ? powerText(slot.node.estimated_power_w) : "--" }}</b></span>
                  <span class="node-plan"><small>核算</small><b>{{ powerText(slot.node.allocated_power_w) }}</b></span>
                </div>
                <div class="node-power-scale" :class="{ exceeded: nodeExceedsPlan(slot.node) }" :title="nodePowerTitle(slot.node)">
                  <i
                    class="node-live-range"
                    :style="{ width: `${nodePlanPercent(slot.node)}%` }"
                  />
                </div>
              </div>
            </template>
            <button v-else type="button" class="empty-slot-button" @click="openNewNode(rack, slot.number)">
              <span class="slot-index">{{ slot.number }}</span><el-icon><Plus /></el-icon><span>添加设备</span>
            </button>
          </div>
        </div>
      </article>
    </section>
    <el-empty v-else description="没有符合筛选条件的机柜" />

    <section v-if="overview?.unassigned_nodes.length" class="unassigned-card">
      <div class="section-heading">
        <div><span class="section-eyebrow">UNASSIGNED NODES</span><h2>未分配节点</h2></div>
        <span>这些节点已上报，但还没有放入机柜槽位</span>
      </div>
      <div class="unassigned-list">
        <button v-for="node in overview.unassigned_nodes" :key="node.node_id" type="button" class="unassigned-node" @click="openNewNodeForUnassigned(node)">
          <strong>{{ node.node_id }}</strong>
          <span>{{ node.gpu_model || node.cpu_model || "硬件待上报" }}</span>
          <small>{{ node.online ? "在线" : "离线" }} · 建议 {{ powerText(node.suggested_power_w) || "手动填写" }}</small>
        </button>
      </div>
    </section>

    <el-dialog v-model="rackDialogVisible" :title="editingRack ? '编辑机柜' : '新增机柜'" width="560px" destroy-on-close>
      <el-form label-position="top" @submit.prevent>
        <div class="form-grid">
          <el-form-item label="机柜编号" required>
            <el-input v-model="rackForm.rack_code" :disabled="!!editingRack" placeholder="例如 07" />
          </el-form-item>
          <el-form-item label="显示名称" required>
            <el-input v-model="rackForm.name" placeholder="机柜 07" />
          </el-form-item>
          <el-form-item label="容量（W）" required>
            <el-input-number v-model="rackForm.capacity_w" :min="1" :max="1000000" :step="100" controls-position="right" />
          </el-form-item>
          <el-form-item label="槽位数" required>
            <el-input-number v-model="rackForm.slot_count" :min="1" :max="60" :step="1" controls-position="right" />
          </el-form-item>
        </div>
        <el-form-item label="位置"><el-input v-model="rackForm.location" placeholder="例如 机房 A 区 / 第 2 排" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="rackForm.note" type="textarea" :rows="3" placeholder="功率上限来源、PDU 信息等" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button v-if="editingRack" type="danger" plain @click="deleteRack">删除机柜</el-button>
        <span class="dialog-spacer" />
        <el-button @click="rackDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveRack">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="nodeDialogVisible" :title="editingNode ? '编辑槽位设备' : '添加槽位设备'" width="600px" destroy-on-close>
      <el-form label-position="top" @submit.prevent>
        <div class="form-grid">
          <el-form-item label="节点编号" required>
            <el-select v-model="nodeForm.node_id" filterable allow-create default-first-option :disabled="!!editingNode" placeholder="选择或输入节点编号" @change="onNodeSelectionChanged">
              <el-option v-for="node in nodeChoices" :key="node.node_id" :label="`${node.node_id} · ${node.gpu_model || node.cpu_model || '硬件待上报'}`" :value="node.node_id" />
            </el-select>
          </el-form-item>
          <el-form-item label="所在机柜" required>
            <el-select v-model="nodeForm.rack_code" placeholder="选择机柜">
              <el-option v-for="rack in racks" :key="rack.rack_code" :label="rack.name" :value="rack.rack_code" />
            </el-select>
          </el-form-item>
          <el-form-item label="槽位" required>
            <el-input-number v-model="nodeForm.slot_number" :min="1" :max="60" :step="1" controls-position="right" />
          </el-form-item>
          <el-form-item label="核算功率（W）" required>
            <el-input-number v-model="nodeForm.allocated_power_w" :min="0" :max="1000000" :step="100" controls-position="right" />
          </el-form-item>
        </div>
        <el-alert v-if="selectedNodeOption?.suggested_power_w" type="info" :closable="false" show-icon :title="`按 GPU 功率上限 + 500W 主机余量，建议核算 ${powerText(selectedNodeOption.suggested_power_w)}。当前值仍可手工调整。`" />
        <el-form-item label="设备标签"><el-input v-model="nodeForm.device_label" placeholder="例如 4× GPU / 存储" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="nodeForm.note" type="textarea" :rows="3" placeholder="安装位置、功率来源或其他说明" /></el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-spacer" />
        <el-button @click="nodeDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveNode">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Delete, EditPen, Grid, Plus, Refresh } from "@element-plus/icons-vue";
import { ApiClient, type PowerRack, type PowerRackNode, type PowerRackNodeOption, type PowerRackOverview } from "../../lib/api";
import { authState } from "../../lib/authStore";
import { settingsState } from "../../lib/settingsStore";

type RackFilter = "all" | "normal" | "warning" | "attention" | "overflow";
type RackForm = { rack_code: string; name: string; capacity_w: number; slot_count: number; location: string; note: string; sort_order: number };
type NodeForm = { node_id: string; rack_code: string; slot_number: number; allocated_power_w: number; device_label: string; note: string };
type RackSlot = { number: number; node?: PowerRackNode };

const AUTO_REFRESH_SECONDS = 15;
const overview = ref<PowerRackOverview | null>(null);
const loading = ref(false);
const saving = ref(false);
const error = ref("");
const lastRefreshAt = ref(0);
const activeFilter = ref<RackFilter>("all");
const rackDialogVisible = ref(false);
const nodeDialogVisible = ref(false);
const editingRack = ref<PowerRack | null>(null);
const editingNode = ref<PowerRackNode | null>(null);
const nodeOldRackCode = ref("");
const rackForm = ref<RackForm>(emptyRackForm());
const nodeForm = ref<NodeForm>(emptyNodeForm());
let refreshTimer: ReturnType<typeof setInterval> | null = null;

const racks = computed(() => overview.value?.racks ?? []);
const summary = computed(() => overview.value?.summary ?? {
  rack_count: 0, capacity_w: 0, allocated_power_w: 0, gpu_power_draw_w: 0, gpu_power_limit_w: 0, estimated_power_w: 0,
  overflow_rack_count: 0, warning_rack_count: 0, config_issue_count: 0, offline_node_count: 0, unreported_node_count: 0,
});
const configuredNodeCount = computed(() => racks.value.reduce((sum, rack) => sum + Number(rack.node_count || 0), 0));
const realtimeNodeCount = computed(() => racks.value.reduce(
  (sum, rack) => sum + rack.nodes.filter((node) => nodeHasRealtime(node)).length,
  0,
));
const missingRealtimeNodeCount = computed(() => Math.max(0, configuredNodeCount.value - realtimeNodeCount.value));
const alarmRackCount = computed(() => summary.value.overflow_rack_count + summary.value.warning_rack_count);
const alertTitle = computed(() => {
  const parts: string[] = [];
  if (summary.value.overflow_rack_count) parts.push(`${summary.value.overflow_rack_count} 个机柜实时功率超过配置极限`);
  if (summary.value.warning_rack_count) parts.push(`${summary.value.warning_rack_count} 个机柜实时功率接近配置极限`);
  if (!parts.length && summary.value.config_issue_count) parts.push(`${summary.value.config_issue_count} 个节点配置需要核对，但不计入实时功率报警`);
  return `${parts.join("；")}。`;
});
const lastRefreshText = computed(() => lastRefreshAt.value ? new Date(lastRefreshAt.value).toLocaleTimeString("zh-CN", { hour12: false }) : "尚未刷新");
const filterOptions = computed(() => [
  { value: "all" as RackFilter, label: "全部", count: racks.value.length },
  { value: "normal" as RackFilter, label: "正常", count: racks.value.filter((rack) => rack.status === "normal").length },
  { value: "warning" as RackFilter, label: "接近上限", count: racks.value.filter((rack) => rack.status === "warning").length },
  { value: "attention" as RackFilter, label: "需检查", count: racks.value.filter((rack) => rack.status === "attention").length },
  { value: "overflow" as RackFilter, label: "实时超限", count: racks.value.filter((rack) => rack.status === "overflow").length },
]);
const filteredRacks = computed(() => racks.value.filter((rack) => activeFilter.value === "all" || rack.status === activeFilter.value));
const nodeChoices = computed<PowerRackNodeOption[]>(() => {
  const all = new Map<string, PowerRackNodeOption>();
  for (const node of overview.value?.unassigned_nodes ?? []) all.set(node.node_id, node);
  for (const rack of racks.value) {
    for (const node of rack.nodes) {
      if (!all.has(node.node_id)) {
        all.set(node.node_id, {
          node_id: node.node_id,
          online: node.online,
          cpu_model: node.cpu_model,
          gpu_model: node.gpu_model,
          gpu_count: node.gpu_count,
          gpu_power_limit_w: node.gpu_power_limit_w,
          suggested_power_w: node.suggested_power_w,
        });
      }
    }
  }
  return Array.from(all.values()).sort((a, b) => a.node_id.localeCompare(b.node_id, undefined, { numeric: true }));
});
const selectedNodeOption = computed(() => nodeChoices.value.find((node) => node.node_id === nodeForm.value.node_id));

function emptyRackForm(): RackForm { return { rack_code: "", name: "", capacity_w: 8000, slot_count: 5, location: "", note: "", sort_order: 0 }; }
function emptyNodeForm(): NodeForm { return { node_id: "", rack_code: "", slot_number: 1, allocated_power_w: 0, device_label: "", note: "" }; }

function client(): ApiClient { return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken }); }

async function loadOverview(): Promise<void> {
  if (loading.value) return;
  loading.value = true;
  error.value = "";
  try {
    overview.value = await client().adminPowerRacks();
    lastRefreshAt.value = Date.now();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

function startRefresh(): void {
  refreshTimer = setInterval(() => {
    if (!document.hidden && !rackDialogVisible.value && !nodeDialogVisible.value) void loadOverview();
  }, AUTO_REFRESH_SECONDS * 1000);
}

function powerText(value: number): string {
  const watts = Math.max(0, Number(value || 0));
  if (!watts) return "0W";
  if (watts >= 1000) return `${(watts / 1000).toFixed(1)}kW`;
  return `${Math.round(watts)}W`;
}

function powerPercent(value: number, capacity: number): number {
  if (!capacity) return 0;
  return Math.min(100, Math.max(0, Number(value || 0) / Number(capacity) * 100));
}

function percentText(value: number, maximum: number): string {
  if (!maximum) return "0%";
  return `${Math.round(Math.min(999, Math.max(0, Number(value || 0) / Number(maximum) * 100)))}%`;
}

function donutStyle(value: number, maximum: number, color: string): Record<string, string> {
  const percent = maximum ? Math.min(100, Math.max(0, Number(value || 0) / Number(maximum) * 100)) : 0;
  return { background: `conic-gradient(${color} ${percent}%, #e6edf5 ${percent}% 100%)` };
}

function nodeHasRealtime(node: PowerRackNode): boolean {
  return !!node.online && !!node.monitor_metrics_available;
}

function nodePlanPercent(node: PowerRackNode): number {
  if (!nodeHasRealtime(node) || !node.allocated_power_w) return 0;
  return Math.min(100, Math.max(0, Number(node.estimated_power_w || 0) / Number(node.allocated_power_w) * 100));
}

function nodeExceedsPlan(node: PowerRackNode): boolean {
  return nodeHasRealtime(node) && Number(node.estimated_power_w || 0) > Number(node.allocated_power_w || 0);
}

function nodePowerTitle(node: PowerRackNode): string {
  if (!nodeHasRealtime(node)) return "当前无实时功率数据";
  const parts = [`整机实时测算 ${powerText(node.estimated_power_w)}`, `核算功率 ${powerText(node.allocated_power_w)}`];
  if (node.gpu_power_draw_w > 0) parts.push(`其中 GPU 实测 ${powerText(node.gpu_power_draw_w)}`);
  return parts.join(" · ");
}

function visibleNodeIssue(node: PowerRackNode): string {
  return node.issues.find((issue) => issue !== "node_offline" && issue !== "node_not_reported") || "";
}

function rackSlots(rack: PowerRack): RackSlot[] {
  const bySlot = new Map(rack.nodes.map((node) => [node.slot_number, node]));
  return Array.from({ length: rack.slot_count }, (_, index) => ({ number: index + 1, node: bySlot.get(index + 1) }));
}

function rackStatusText(status: string): string {
  if (status === "overflow") return "实时超限";
  if (status === "warning") return "接近上限";
  if (status === "attention") return "需检查";
  return "正常";
}

function rackTagType(status: string): "success" | "warning" | "danger" | "info" {
  if (status === "overflow") return "danger";
  if (status === "warning" || status === "attention") return "warning";
  return "success";
}

function issueLabel(code: string): string {
  const labels: Record<string, string> = {
    allocated_below_gpu_limit: "核算偏低",
    gpu_count_mismatch: "GPU数不一致",
    gpu_power_missing: "功率缺失",
    node_offline: "节点离线",
    node_not_reported: "未上报",
  };
  return labels[code] || "需检查";
}

function openNewRack(): void {
  const next = racks.value.reduce((max, rack) => Math.max(max, Number(rack.rack_code) || 0), 0) + 1;
  editingRack.value = null;
  rackForm.value = { ...emptyRackForm(), rack_code: String(next).padStart(2, "0"), name: `机柜 ${String(next).padStart(2, "0")}`, sort_order: next };
  rackDialogVisible.value = true;
}

function openEditRack(rack: PowerRack): void {
  editingRack.value = rack;
  rackForm.value = {
    rack_code: rack.rack_code,
    name: rack.name,
    capacity_w: rack.capacity_w,
    slot_count: rack.slot_count,
    location: rack.location || "",
    note: rack.note || "",
    sort_order: rack.sort_order,
  };
  rackDialogVisible.value = true;
}

async function saveRack(): Promise<void> {
  if (!rackForm.value.rack_code.trim() || !rackForm.value.name.trim()) {
    ElMessage.warning("请填写机柜编号和名称");
    return;
  }
  saving.value = true;
  try {
    await client().adminPowerRackUpsert(rackForm.value);
    rackDialogVisible.value = false;
    await loadOverview();
    ElMessage.success("机柜配置已保存");
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    saving.value = false;
  }
}

async function deleteRack(): Promise<void> {
  if (!editingRack.value) return;
  try {
    await ElMessageBox.confirm(`删除 ${editingRack.value.name} 后，其槽位设备配置也会删除，确认继续吗？`, "删除机柜", { type: "warning", confirmButtonText: "确认删除", cancelButtonText: "取消" });
    saving.value = true;
    await client().adminPowerRackDelete(editingRack.value.rack_code);
    rackDialogVisible.value = false;
    await loadOverview();
    ElMessage.success("机柜已删除");
  } catch (e: any) {
    if (String(e?.message || "") !== "cancel") ElMessage.error(e?.message ?? String(e));
  } finally {
    saving.value = false;
  }
}

function openNewNode(rack: PowerRack, slotNumber: number): void {
  editingNode.value = null;
  nodeOldRackCode.value = "";
  nodeForm.value = { ...emptyNodeForm(), rack_code: rack.rack_code, slot_number: slotNumber };
  nodeDialogVisible.value = true;
}

function openNewNodeForUnassigned(node: PowerRackNodeOption): void {
  const rack = racks.value[0];
  if (!rack) return;
  openNewNode(rack, firstEmptySlot(rack));
  nodeForm.value.node_id = node.node_id;
  onNodeSelectionChanged();
}

function firstEmptySlot(rack: PowerRack): number {
  const used = new Set(rack.nodes.map((node) => node.slot_number));
  return Array.from({ length: rack.slot_count }, (_, i) => i + 1).find((slot) => !used.has(slot)) || rack.slot_count;
}

function openEditNode(rack: PowerRack, node: PowerRackNode): void {
  editingNode.value = node;
  nodeOldRackCode.value = rack.rack_code;
  nodeForm.value = {
    node_id: node.node_id,
    rack_code: rack.rack_code,
    slot_number: node.slot_number,
    allocated_power_w: node.allocated_power_w,
    device_label: node.device_label || "",
    note: node.note || "",
  };
  nodeDialogVisible.value = true;
}

function onNodeSelectionChanged(): void {
  const option = selectedNodeOption.value;
  if (!option) return;
  if (!nodeForm.value.allocated_power_w && option.suggested_power_w) nodeForm.value.allocated_power_w = option.suggested_power_w;
  if (!nodeForm.value.device_label) nodeForm.value.device_label = option.gpu_model || option.cpu_model || "";
}

async function saveNode(): Promise<void> {
  if (!nodeForm.value.node_id.trim() || !nodeForm.value.rack_code || nodeForm.value.slot_number <= 0) {
    ElMessage.warning("请填写节点、机柜和槽位");
    return;
  }
  saving.value = true;
  try {
    await client().adminPowerRackNodeUpsert(nodeForm.value.rack_code, {
      node_id: nodeForm.value.node_id,
      slot_number: nodeForm.value.slot_number,
      allocated_power_w: nodeForm.value.allocated_power_w,
      device_label: nodeForm.value.device_label,
      note: nodeForm.value.note,
    });
    nodeDialogVisible.value = false;
    await loadOverview();
    ElMessage.success("槽位设备已保存");
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    saving.value = false;
  }
}

async function deleteNode(rack: PowerRack, node: PowerRackNode): Promise<void> {
  try {
    await ElMessageBox.confirm(`移除 ${node.node_id} 的机柜配置？节点本身不会被删除。`, "移除槽位设备", { type: "warning", confirmButtonText: "移除配置", cancelButtonText: "取消" });
    await client().adminPowerRackNodeDelete(rack.rack_code, node.node_id);
    await loadOverview();
    ElMessage.success("槽位配置已移除");
  } catch (e: any) {
    if (String(e?.message || "") !== "cancel") ElMessage.error(e?.message ?? String(e));
  }
}

onMounted(() => {
  void loadOverview();
  startRefresh();
});

onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer);
});
</script>

<style scoped>
.rack-page { min-height: calc(100vh - 96px); padding: 2px; color: #102033; }
.rack-alert { margin-top: 16px; }
.rack-summary-grid { margin-top: 13px; gap: 10px; }
.compact-metric { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-height: 84px; padding: 11px 13px; }
.compact-metric::after { width: 68px; height: 68px; }
.compact-metric-copy { min-width: 0; }
.compact-metric-copy > span { display: block; color: #64748b; font-size: 11px; font-weight: 700; }
.compact-metric-copy > strong { display: flex; align-items: baseline; gap: 4px; margin-top: 6px; color: #0f172a; font-size: 20px; line-height: 1; letter-spacing: -.035em; white-space: nowrap; }
.compact-metric-copy > strong i { color: #718096; font-size: 11px; font-style: normal; font-weight: 650; letter-spacing: 0; }
.compact-metric-copy > small { display: block; margin-top: 5px; color: #94a3b8; font-size: 10px; white-space: nowrap; }
.mini-donut { position: relative; z-index: 1; display: grid; place-items: center; width: 48px; height: 48px; flex: 0 0 48px; border-radius: 50%; }
.mini-donut::after { content: ""; position: absolute; inset: 7px; border-radius: 50%; background: rgba(255,255,255,.96); }
.mini-donut span { position: relative; z-index: 1; color: #475569; font-size: 9px; font-weight: 800; }
.rack-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin: 18px 0; padding: 10px; border: 1px solid rgba(148,163,184,.25); border-radius: 16px; background: rgba(255,255,255,.7); }
.filter-tabs { display: flex; flex-wrap: wrap; gap: 6px; }
.filter-tabs button { appearance: none; padding: 9px 13px; border: 0; border-radius: 11px; color: #526277; background: transparent; cursor: pointer; font: inherit; font-size: 13px; font-weight: 650; transition: .2s ease; }
.filter-tabs button span { display: inline-grid; place-items: center; min-width: 21px; height: 21px; margin-left: 4px; padding: 0 6px; border-radius: 999px; background: rgba(100,116,139,.1); font-size: 11px; }
.filter-tabs button:hover { color: #0369a1; background: rgba(14,165,233,.08); }
.filter-tabs button.active { color: #047857; background: rgba(16,185,129,.12); }
.rack-actions { display: flex; gap: 8px; }
.rack-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(350px, 1fr)); align-items: stretch; gap: 14px; }
.rack-card { --rack-state: #10b981; display: flex; min-width: 0; height: 100%; box-sizing: border-box; overflow: hidden; flex-direction: column; padding: 13px; border: 1px solid rgba(148,163,184,.25); border-top: 3px solid var(--rack-state); border-radius: 16px; background: linear-gradient(155deg, rgba(255,255,255,.98), rgba(244,248,252,.92)); box-shadow: 0 10px 24px rgba(40,61,84,.08); transition: transform .2s ease, box-shadow .2s ease; }
.rack-card:hover { transform: translateY(-2px); box-shadow: 0 20px 42px rgba(35,58,82,.14); }
.rack-state-warning { --rack-state: #f59e0b; }
.rack-state-attention { --rack-state: #f97316; }
.rack-state-overflow { --rack-state: #ef4444; }
.rack-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
.rack-title-line { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.rack-status-dot { width: 10px; height: 10px; border-radius: 50%; background: var(--rack-state); box-shadow: 0 0 0 5px color-mix(in srgb, var(--rack-state) 13%, transparent); }
.rack-head h2 { margin: 0; color: #17283c; font-size: 17px; letter-spacing: -.02em; }
.rack-head p { margin: 5px 0 0 18px; color: #8a98a8; font-size: 11px; }
.rack-power-overview { margin: 10px 0 8px; padding: 9px 10px; border: 1px solid rgba(148,163,184,.12); border-radius: 12px; background: linear-gradient(135deg, rgba(236,242,248,.82), rgba(247,250,252,.9)); }
.rack-live-line, .rack-power-foot { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.rack-live-line > span { display: flex; align-items: baseline; gap: 5px; min-width: 0; }
.rack-live-line small { color: #64748b; font-size: 10px; font-weight: 700; }
.rack-live-line strong { color: #0f766e; font-size: 18px; line-height: 1; letter-spacing: -.03em; }
.rack-live-line i { color: #718096; font-size: 11px; font-style: normal; font-weight: 650; }
.rack-live-line > b { color: #0f766e; font-size: 12px; }
.rack-progress { position: relative; height: 10px; margin: 8px 0 7px; overflow: hidden; border: 1px solid rgba(148,163,184,.24); border-radius: 999px; background: rgba(255,255,255,.75); }
.rack-progress .planned-range { position: absolute; inset: 0 auto 0 0; display: block; border-radius: inherit; background: rgba(59,130,246,.18); transition: width .45s ease; }
.rack-progress .live-range { position: absolute; inset: 1px auto 1px 1px; display: block; border-radius: inherit; background: linear-gradient(90deg, #10b981, #34d399); transition: width .45s ease; }
.rack-progress .planned-marker { position: absolute; z-index: 2; top: -2px; bottom: -2px; width: 2px; border-radius: 2px; background: #2563eb; transform: translateX(-1px); }
.rack-state-warning .rack-progress .live-range { background: linear-gradient(90deg, #10b981, #f59e0b); }
.rack-state-overflow .rack-progress .planned-marker { background: #ef4444; }
.rack-power-foot { color: #7b8999; font-size: 10px; }
.rack-power-foot span { display: flex; align-items: center; gap: 4px; white-space: nowrap; }
.legend-plan { display: inline-block; width: 7px; height: 7px; border-radius: 2px; background: #3b82f6; }
.rack-slots { display: grid; gap: 6px; }
.rack-slot { display: flex; align-items: center; width: 100%; height: 76px; min-height: 76px; box-sizing: border-box; padding: 7px 9px; border: 1px solid rgba(148,163,184,.18); border-radius: 11px; background: rgba(255,255,255,.72); transition: border-color .2s ease, background .2s ease; }
.rack-slot:hover { border-color: rgba(59,130,246,.25); background: rgba(255,255,255,.94); }
.rack-slot.empty { border-style: dashed; background: rgba(241,245,249,.46); }
.slot-main { min-width: 0; width: 100%; }
.slot-node-head { display: flex; align-items: center; min-width: 0; gap: 7px; }
.slot-node-head strong { color: #1e3a5f; font-size: 13px; letter-spacing: .015em; }
.slot-index { display: inline-grid; place-items: center; width: 20px; height: 20px; flex: 0 0 20px; border-radius: 6px; color: #64748b; background: #eef2f7; font-size: 9px; font-weight: 800; }
.slot-online { display: inline-flex; align-items: center; gap: 4px; color: #059669; font-size: 9px; font-weight: 700; }
.slot-online i { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
.slot-online.offline { color: #f97316; }
.node-issue { overflow: hidden; color: #d97706; font-size: 9px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }
.slot-actions { display: flex; gap: 0; margin-left: auto; opacity: .58; transition: opacity .2s ease; }
.rack-slot:hover .slot-actions { opacity: 1; }
.slot-actions .el-button { width: 22px; height: 22px; margin-left: 0; padding: 2px; }
.slot-label { display: block; max-width: calc(100% - 4px); margin: 2px 0 0 27px; overflow: hidden; color: #7b8999; font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.node-power-labels { display: flex; align-items: baseline; justify-content: space-between; gap: 14px; margin-top: 4px; color: #64748b; white-space: nowrap; }
.node-power-labels span { display: flex; align-items: baseline; gap: 4px; }
.node-power-labels small { color: #94a3b8; font-size: 9px; }
.node-power-labels b { color: #334155; font-size: 11px; }
.node-power-labels .node-live b { color: #047857; }
.node-power-scale { position: relative; height: 6px; margin-top: 4px; overflow: hidden; border-radius: 999px; background: #e7edf3; box-shadow: inset 0 0 0 1px rgba(148,163,184,.14); }
.node-power-scale .node-live-range { position: absolute; inset: 0 auto 0 0; display: block; border-radius: inherit; background: linear-gradient(90deg, #10b981, #34d399); transition: width .4s ease; }
.node-power-scale.exceeded .node-live-range { background: linear-gradient(90deg, #f59e0b, #ef4444); }
.empty-slot-button { display: flex; align-items: center; justify-content: center; gap: 7px; width: 100%; min-height: 60px; border: 0; color: #8a98a8; background: transparent; cursor: pointer; font: inherit; font-size: 11px; }
.empty-slot-button:hover { color: #2563eb; }
.unassigned-card { margin-top: 18px; padding: 18px; border: 1px solid rgba(148,163,184,.25); border-radius: 18px; background: rgba(255,255,255,.68); box-shadow: 0 13px 34px rgba(45,71,112,.08); }
.section-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 12px; }
.section-heading h2 { margin: 3px 0 0; color: #17283c; font-size: 18px; }
.section-heading > span { color: #94a3b8; font-size: 12px; }
.section-eyebrow { color: #2563eb; font-size: 10px; font-weight: 800; letter-spacing: .13em; }
.unassigned-list { display: flex; flex-wrap: wrap; gap: 9px; margin-top: 14px; }
.unassigned-node { display: flex; flex-direction: column; align-items: flex-start; min-width: 180px; padding: 11px 13px; border: 1px solid rgba(148,163,184,.24); border-radius: 12px; color: inherit; background: rgba(248,250,252,.8); cursor: pointer; text-align: left; }
.unassigned-node:hover { border-color: rgba(37,99,235,.4); background: rgba(239,246,255,.9); }
.unassigned-node strong { color: #1e3a5f; }
.unassigned-node span { max-width: 220px; margin-top: 4px; overflow: hidden; color: #64748b; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.unassigned-node small { margin-top: 5px; color: #94a3b8; font-size: 10px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0 14px; }
.form-grid .el-input-number, .form-grid .el-select { width: 100%; }
.dialog-spacer { flex: 1; }
:deep(.el-dialog__footer) { display: flex; align-items: center; }

@media (max-width: 760px) {
  .rack-toolbar { align-items: stretch; flex-direction: column; }
  .rack-actions { justify-content: flex-end; }
  .rack-grid { grid-template-columns: minmax(0, 1fr); }
  .section-heading { align-items: flex-start; flex-direction: column; }
}
@media (max-width: 520px) {
  .rack-summary-grid { grid-template-columns: 1fr; }
  .form-grid { grid-template-columns: 1fr; }
  .rack-card { padding: 14px; }
}
</style>
