<template>
  <el-card>
    <template #header>
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><DataBoard /></el-icon></span>
          <div>
          <div class="title">进程审计</div>
          <div class="sub">查询、审计和导出节点进程记录</div>
          </div>
        </div>
        <div class="row">
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
          <el-button :loading="exporting" :disabled="!canRunRangeAction" @click="exportCSV">导出区间 CSV</el-button>
        </div>
      </div>
    </template>

    <div class="content-stack">
      <el-alert v-if="error" :title="error" type="error" show-icon />

      <el-form inline>
        <el-form-item label="平台账号">
          <el-input v-model="billingUsername" placeholder="按平台账号查询" @keyup.enter="reload" />
        </el-form-item>
        <el-form-item label="节点账号">
          <el-input v-model="localUsername" placeholder="按节点账号查询" @keyup.enter="reload" />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="unregisteredOnly">仅看未注册偷跑</el-checkbox>
        </el-form-item>
        <el-form-item label="条数">
          <el-input-number v-model="limit" :min="1" :max="5000" />
        </el-form-item>
        <el-form-item label="日期区间">
          <el-date-picker
            v-model="rangeDays"
            type="daterange"
            value-format="YYYY-MM-DD"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            :disabled-date="usageRangeDisabledDate"
            @change="onRangeChange"
          />
        </el-form-item>
        <el-form-item>
          <el-button :loading="estimatingRange" :disabled="!canRunRangeAction" @click="estimateRange">估算大小</el-button>
        </el-form-item>
        <el-form-item>
          <el-button :loading="deletingRange" type="danger" plain :disabled="!canRunRangeAction" @click="deleteRangeRecords">删除区间记录</el-button>
        </el-form-item>
      </el-form>
      <el-alert
        v-if="availableDaysLoaded"
        type="info"
        :closable="false"
        show-icon
        :title="`已加载 ${availableDays.length} 个有记录日期；无记录日期已灰显不可选`"
      />
      <el-alert
        v-if="rangeEstimate"
        type="warning"
        :closable="false"
        show-icon
        :title="`区间估算：${rangeEstimate.records} 条记录，CSV约 ${bytesText(rangeEstimate.estimated_csv_bytes)}，数据库约 ${bytesText(rangeEstimate.estimated_db_bytes)}`"
      />

      <el-card>
        <template #header>
          <div class="section-inline-title">进程使用记录</div>
        </template>
        <el-table :data="records" stripe max-height="520" :row-class-name="rowClassName">
          <el-table-column prop="timestamp" label="时间" width="190" sortable :formatter="tableTimeFormatter" />
          <el-table-column prop="node_id" label="节点" width="120" sortable />
          <el-table-column prop="local_username" label="节点账号" width="150" sortable />
          <el-table-column prop="billing_username" label="平台账号" width="170" sortable>
            <template #default="{ row }">
              <el-button
                v-if="row.registered !== false && (row.billing_username || row.username)"
                link
                type="primary"
                @click="openPlatformProfile(row.billing_username || row.username)"
              >
                {{ row.billing_username || row.username }}
              </el-button>
              <span v-else class="unregistered">未注册</span>
            </template>
          </el-table-column>
          <el-table-column label="CPU%" width="90" prop="cpu_percent" sortable>
            <template #default="{ row }">{{ fmt2(row.cpu_percent) }}</template>
          </el-table-column>
          <el-table-column label="内存MB" width="110" prop="memory_mb" sortable>
            <template #default="{ row }">{{ fmt2(row.memory_mb) }}</template>
          </el-table-column>
          <el-table-column label="积分消耗" width="100" prop="cost" sortable>
            <template #default="{ row }">{{ fmt2(row.cost) }}</template>
          </el-table-column>
          <el-table-column prop="gpu_usage" label="GPU 明细" />
        </el-table>
      </el-card>

      <el-card>
        <template #header>
          <div class="section-inline-title">强制终止记录</div>
        </template>
        <el-table :data="killRecords" stripe max-height="420">
          <el-table-column prop="timestamp" label="时间" width="190" sortable :formatter="tableTimeFormatter" />
          <el-table-column prop="node_id" label="节点" width="120" sortable />
          <el-table-column prop="local_username" label="节点账号" width="150" sortable />
          <el-table-column prop="billing_username" label="平台账号" width="170" sortable>
            <template #default="{ row }">
              <el-button
                v-if="row.registered !== false && row.billing_username"
                link
                type="primary"
                @click="openPlatformProfile(row.billing_username)"
              >
                {{ row.billing_username }}
              </el-button>
              <span v-else class="unregistered">{{ row.billing_username || "未注册/未知" }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="action_type" label="动作" width="140" sortable>
            <template #default="{ row }">{{ killActionLabel(row.action_type) }}</template>
          </el-table-column>
          <el-table-column prop="reason" label="原因" min-width="320" />
          <el-table-column prop="pids" label="PID" min-width="160">
            <template #default="{ row }">{{ killPidsText(row.pids) }}</template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-card v-if="unregisteredSummary.length > 0">
        <template #header>
          <div class="section-inline-title">
            <el-icon><WarningFilled /></el-icon>
            <span>未注册偷跑汇总</span>
          </div>
        </template>
        <el-table :data="unregisteredSummary" stripe max-height="260">
          <el-table-column prop="billing_username" label="平台账号" width="180" />
          <el-table-column prop="local_username" label="节点账号" width="160" />
          <el-table-column prop="nodes" label="涉及节点" min-width="260" />
          <el-table-column prop="count" label="记录数" width="100" sortable />
          <el-table-column label="操作" width="170">
            <template #default="{ row }">
              <el-button
                type="danger"
                size="small"
                :loading="blacklistingKey === `${row.local_username}::${row.nodes}`"
                @click="addUnregisteredToBlacklist(row)"
              >
                拉黑并断开SSH
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <PlatformUserDetailDialog v-model="profileVisible" :username="selectedProfileUsername" />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ApiClient, type ProcessKillRecord, type UsageRecord } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { DataBoard, WarningFilled } from "@element-plus/icons-vue";
import { formatServerDate, formatServerDateTime, getServerTodayDateText } from "../../lib/time";

const loading = ref(false);
const exporting = ref(false);
const blacklistingKey = ref("");
const error = ref("");
const records = ref<UsageRecord[]>([]);
const killRecords = ref<ProcessKillRecord[]>([]);
const availableDays = ref<Array<{ date: string; record_count: number; estimated_csv_bytes: number }>>([]);
const availableDaysLoaded = ref(false);
const estimatingRange = ref(false);
const deletingRange = ref(false);
const rangeEstimate = ref<{ records: number; estimated_csv_bytes: number; estimated_db_bytes: number } | null>(null);
const profileVisible = ref(false);
const selectedProfileUsername = ref("");

const billingUsername = ref("");
const localUsername = ref("");
const unregisteredOnly = ref(false);
const limit = ref(200);
const rangeDays = ref<string[]>([]);

function fmt2(v: number): string {
  return Number(v ?? 0).toFixed(2);
}

function tableTimeFormatter(_: unknown, __: unknown, cellValue: unknown): string {
  return formatServerDateTime(String(cellValue ?? ""));
}

function bytesText(n: number): string {
  const x = Number(n || 0);
  if (x < 1024) return `${x} B`;
  if (x < 1024 * 1024) return `${(x / 1024).toFixed(2)} KB`;
  if (x < 1024 * 1024 * 1024) return `${(x / 1024 / 1024).toFixed(2)} MB`;
  return `${(x / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function dateKey(date: Date): string {
  return formatServerDate(date);
}

const availableDaySet = computed(() => {
  const s = new Set<string>();
  for (const d of availableDays.value) {
    const k = String(d.date || "").trim();
    if (k) s.add(k);
  }
  return s;
});

const canRunRangeAction = computed(() => Array.isArray(rangeDays.value) && rangeDays.value.length === 2 && !!rangeDays.value[0] && !!rangeDays.value[1]);

function usageRangeDisabledDate(d: Date): boolean {
  if (dateKey(d) > getServerTodayDateText()) return true;
  if (!availableDaysLoaded.value) return false;
  return !availableDaySet.value.has(dateKey(d));
}

function onRangeChange() {
  rangeEstimate.value = null;
}

async function reload() {
  loading.value = true;
  error.value = "";
  records.value = [];
  killRecords.value = [];
  availableDaysLoaded.value = false;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const [r, d] = await Promise.all([
      client.adminUsage({
        billingUsername: billingUsername.value,
        localUsername: localUsername.value,
        unregisteredOnly: unregisteredOnly.value,
        limit: limit.value,
      }),
      client.adminUsageDays({
        billingUsername: billingUsername.value,
        localUsername: localUsername.value,
        unregisteredOnly: unregisteredOnly.value,
      }),
    ]);
    records.value = (r.records ?? []).map((x) => ({
      ...x,
      local_username: x.local_username || "-",
      billing_username: x.registered === false ? "" : (x.billing_username || x.username || ""),
    }));
    killRecords.value = (r.kill_records ?? []).map((x) => ({
      ...x,
      local_username: x.local_username || "-",
      billing_username: x.billing_username || "",
      pids: Array.isArray(x.pids) ? x.pids : [],
    }));
    availableDays.value = d.days ?? [];
    availableDaysLoaded.value = true;
    if (rangeDays.value.length === 2) {
      const a = String(rangeDays.value[0] || "");
      const b = String(rangeDays.value[1] || "");
      if (!availableDaySet.value.has(a) || !availableDaySet.value.has(b)) {
        rangeDays.value = [];
        rangeEstimate.value = null;
      }
    }
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

function killActionLabel(actionType: string): string {
  const t = String(actionType || "").trim();
  if (t === "kill_all_processes") return "终止用户全部进程";
  if (t === "kill_all_user_processes") return "终止全部用户进程";
  if (t === "kill_process") return "终止指定进程";
  return t || "-";
}

function killPidsText(pids: number[] | undefined): string {
  if (!Array.isArray(pids) || pids.length === 0) return "-";
  return pids.join(", ");
}

async function exportCSV() {
  if (!canRunRangeAction.value) {
    ElMessage.warning("请先选择有效日期区间");
    return;
  }
  exporting.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const blob = await client.adminExportUsageCSV({
      billingUsername: billingUsername.value,
      localUsername: localUsername.value,
      unregisteredOnly: unregisteredOnly.value,
      from: rangeDays.value[0],
      to: rangeDays.value[1],
      limit: 20000,
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "usage_export.csv";
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    ElMessage.success("已开始下载 usage_export.csv");
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    exporting.value = false;
  }
}

async function estimateRange() {
  if (!canRunRangeAction.value) {
    ElMessage.warning("请先选择有效日期区间");
    return;
  }
  estimatingRange.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminUsageRangeEstimate({
      from: rangeDays.value[0],
      to: rangeDays.value[1],
      billingUsername: billingUsername.value,
      localUsername: localUsername.value,
      unregisteredOnly: unregisteredOnly.value,
    });
    rangeEstimate.value = {
      records: Number(r.records ?? 0),
      estimated_csv_bytes: Number(r.estimated_csv_bytes ?? 0),
      estimated_db_bytes: Number(r.estimated_db_bytes ?? 0),
    };
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    estimatingRange.value = false;
  }
}

async function deleteRangeRecords() {
  if (!canRunRangeAction.value) {
    ElMessage.warning("请先选择有效日期区间");
    return;
  }
  if (!rangeEstimate.value) {
    await estimateRange();
    if (!rangeEstimate.value) return;
  }
  try {
    await ElMessageBox.confirm(
      `确认删除 ${rangeDays.value[0]} 至 ${rangeDays.value[1]} 的使用记录吗？\n预计 ${rangeEstimate.value.records} 条，CSV约 ${bytesText(rangeEstimate.value.estimated_csv_bytes)}。`,
      "二次确认",
      {
        type: "warning",
        confirmButtonText: "确认删除",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  deletingRange.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const res = await client.adminUsageDeleteRange({
      from: rangeDays.value[0],
      to: rangeDays.value[1],
      billing_username: billingUsername.value,
      local_username: localUsername.value,
      unregistered_only: unregisteredOnly.value,
      confirm: true,
    });
    ElMessage.success(`删除完成：${Number(res.deleted_records ?? 0)} 条`);
    rangeEstimate.value = null;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    deletingRange.value = false;
  }
}

const unregisteredSummary = computed(() => {
  const m = new Map<string, { billing_username: string; local_username: string; nodes: Set<string>; node_ids: string[]; count: number }>();
  for (const r of records.value) {
    if (r.registered !== false) continue;
    const b = "未注册";
    const l = r.local_username || "-";
    const key = `${r.username || "-"}::${l}`;
    const item = m.get(key) || { billing_username: b, local_username: l, nodes: new Set<string>(), node_ids: [], count: 0 };
    item.count += 1;
    if (r.node_id) item.nodes.add(r.node_id);
    m.set(key, item);
  }
  return Array.from(m.values()).map((x) => ({
    billing_username: x.billing_username,
    local_username: x.local_username,
    node_ids: Array.from(x.nodes).sort(),
    nodes: Array.from(x.nodes).sort().join(", "),
    count: x.count,
  }));
});

function rowClassName({ row }: { row: UsageRecord }) {
  return row.registered === false ? "unregistered-row" : "";
}

function openPlatformProfile(username: string) {
  selectedProfileUsername.value = String(username || "").trim();
  if (!selectedProfileUsername.value) return;
  profileVisible.value = true;
}

async function addUnregisteredToBlacklist(row: { local_username: string; nodes: string; node_ids?: string[] }) {
  const local = String(row.local_username || "").trim();
  const nodeIDs = (row.node_ids ?? [])
    .map((x) => String(x || "").trim())
    .filter((x) => x && x !== "-");
  if (!local || !nodeIDs.length) {
    ElMessage.warning("缺少可用的节点账号或节点ID，无法加入黑名单");
    return;
  }

  try {
    await ElMessageBox.confirm(
      `确认将节点账号 ${local} 加入黑名单并立刻断开SSH吗？\n涉及节点：${nodeIDs.join(", ")}`,
      "二次确认",
      {
        type: "warning",
        confirmButtonText: "确认拉黑",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }

  blacklistingKey.value = `${local}::${row.nodes}`;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const results = await Promise.allSettled(nodeIDs.map((id) => client.adminUpsertBlacklist(id, [local], [])));
    const okCount = results.filter((x) => x.status === "fulfilled").length;
    const failCount = results.length - okCount;
    if (failCount > 0) {
      ElMessage.warning(`黑名单已部分生效：成功 ${okCount} 个节点，失败 ${failCount} 个节点`);
    } else {
      ElMessage.success(`已拉黑并下发断开SSH：${okCount} 个节点`);
    }
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    blacklistingKey.value = "";
  }
}

reload();
</script>

<style scoped>
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.content-stack {
  width: 100%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.title {
  font-weight: 700;
}
.section-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.section-icon {
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #dbeafe;
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
  flex-shrink: 0;
}
.section-inline-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.sub {
  margin-top: 4px;
  font-size: 12px;
  color: #6b7280;
}
:deep(.unregistered-row > td) {
  background: #fee2e2 !important;
}
.unregistered {
  color: #b91c1c;
  font-weight: 600;
}
</style>
