<template>
  <div class="board-wrap">
    <el-card class="section-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-overview"><el-icon><DataBoard /></el-icon></span>
            <span class="title">运营看板</span>
          </div>
          <div class="head-actions">
            <el-button type="primary" :loading="loading" @click="loadAll">刷新</el-button>
            <el-button v-if="authState.role === 'admin'" :loading="exporting" @click="exportRangeCSV">导出区间 CSV</el-button>
          </div>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />

      <el-form inline>
        <el-form-item label="统计区间">
          <el-date-picker
            v-model="fromDate"
            type="date"
            placeholder="开始日期"
            value-format="YYYY-MM-DD"
            :clearable="false"
            :editable="false"
            :disabled-date="disableFutureDate"
            @change="onStatsRangeChanged"
          />
          <span class="range-sep">至</span>
          <el-date-picker
            v-model="toDate"
            type="date"
            placeholder="结束日期"
            value-format="YYYY-MM-DD"
            :clearable="false"
            :editable="false"
            :disabled-date="disableFutureDate"
            @change="onStatsRangeChanged"
          />
        </el-form-item>
      </el-form>
      <div class="range-meta">
        生效区间：{{ appliedFromDate }} 至 {{ appliedToDate }}
      </div>
    </el-card>

    <el-card v-if="authState.role === 'admin'" class="section-card">
      <template #header>
        <div class="section-title-wrap">
          <span class="section-icon tone-retention"><el-icon><Delete /></el-icon></span>
          <span>数据留存与删除（仅管理员）</span>
        </div>
      </template>
      <el-form inline>
        <el-form-item label="自动删除保留天数">
          <el-input-number v-model="retentionDaysDraft" :min="0" :max="3650" :step="1" :precision="0" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="retentionSaving" @click="saveRetentionDays">保存自动删除设置</el-button>
        </el-form-item>
        <el-form-item>
          <span class="retention-tip">`0` 表示关闭自动删除；大于 `0` 时，系统每小时巡检、每天最多自动清理一次。</span>
        </el-form-item>
      </el-form>
      <div class="retention-meta">
        <span>上次删除时间：{{ retentionStatus?.last_deleted_at ? tableTimeFormatter(null, null, retentionStatus.last_deleted_at) : "暂无" }}</span>
        <span>上次删除日期：{{ retentionStatus?.last_deleted_day || "-" }}</span>
        <span>上次删除模式：{{ retentionModeText(retentionStatus?.last_deleted_mode) }}</span>
        <span>删除条数：{{ Number(retentionStatus?.last_deleted_records || 0) }}</span>
      </div>
      <div class="retention-meta">
        <span>上次删除范围：{{ retentionStatus?.last_deleted_from ? datePrefix(retentionStatus.last_deleted_from, "-") : "最早记录" }} 至 {{ retentionStatus?.last_deleted_to ? datePrefix(retentionStatus.last_deleted_to, "-") : "-" }}</span>
      </div>

      <el-form inline style="margin-top: 10px">
        <el-form-item label="立即删除区间">
          <el-date-picker
            v-model="deleteRangeDays"
            type="daterange"
            value-format="YYYY-MM-DD"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            :disabled-date="usageDeleteDisabledDate"
            @change="onDeleteRangeChange"
          />
        </el-form-item>
        <el-form-item>
          <el-button :loading="deleteRangeEstimating" :disabled="!canDeleteRangeAction" @click="estimateDeleteRange">估算删除大小</el-button>
        </el-form-item>
        <el-form-item>
          <el-button type="danger" plain :loading="deleteRangeDeleting" :disabled="!canDeleteRangeAction" @click="deleteRangeNow">立即删除该区间</el-button>
        </el-form-item>
      </el-form>
      <el-alert
        v-if="availableUsageDaysLoaded"
        type="info"
        :closable="false"
        show-icon
        :title="`有记录日期共 ${availableUsageDays.length} 天；无记录日期灰显不可选`"
      />
      <el-alert
        v-if="deleteRangeEstimate"
        type="warning"
        :closable="false"
        show-icon
        :title="`删除估算：${deleteRangeEstimate.records} 条，CSV约 ${bytesText(deleteRangeEstimate.estimated_csv_bytes)}，数据库约 ${bytesText(deleteRangeEstimate.estimated_db_bytes)}`"
      />
    </el-card>

    <el-card class="board-card section-card">
      <template #header>
        <div class="section-title-wrap">
          <span class="section-icon tone-users"><el-icon><UserFilled /></el-icon></span>
          <span>平台用户使用情况（区间汇总）</span>
        </div>
      </template>
      <div class="table-wrap">
        <el-table :data="displayUserRows" stripe table-layout="auto">
          <el-table-column label="平台用户" min-width="160">
            <template #default="{ row }">
              <el-button class="user-link" link @click="openUser(row.platform_username)">
                {{ row.platform_username }}
              </el-button>
            </template>
          </el-table-column>
          <el-table-column label="CPU使用时间" min-width="130">
            <template #default="{ row }">{{ fmtDuration(row.cpu_usage_seconds) }}</template>
          </el-table-column>
          <el-table-column label="GPU使用时间" min-width="130">
            <template #default="{ row }">{{ fmtDuration(row.gpu_usage_seconds) }}</template>
          </el-table-column>
          <el-table-column label="CPU占用率%" min-width="116">
            <template #default="{ row }">{{ fmt2(row.cpu_util_percent) }}</template>
          </el-table-column>
          <el-table-column label="GPU占用率%" min-width="116">
            <template #default="{ row }">{{ fmt2(row.gpu_util_percent) }}</template>
          </el-table-column>
          <el-table-column label="积分消耗" min-width="110">
            <template #default="{ row }">{{ fmt2(row.total_cost) }}</template>
          </el-table-column>
          <el-table-column label="剩余通用积分" min-width="130">
            <template #default="{ row }">{{ fmt2(row.general_balance) }}</template>
          </el-table-column>
        </el-table>
      </div>
    </el-card>

    <el-card class="board-card section-card">
      <template #header>
        <div class="section-title-wrap">
          <span class="section-icon tone-nodes"><el-icon><Monitor /></el-icon></span>
          <span>平台账号在各节点使用详情（点击上表平台账号查看）</span>
        </div>
      </template>
      <div style="margin-bottom: 8px; color: #64748b">
        当前平台账号：{{ activeUsername || "未选择" }}
      </div>
      <div class="table-wrap">
        <el-table :data="nodeRows" stripe table-layout="auto">
          <el-table-column prop="node_id" label="节点端口" min-width="100" />
          <el-table-column prop="cpu_model" label="CPU型号" min-width="170" />
          <el-table-column prop="cpu_count" label="CPU数" min-width="74" />
          <el-table-column prop="gpu_model" label="GPU型号" min-width="170" />
          <el-table-column prop="gpu_count" label="GPU数" min-width="74" />
          <el-table-column prop="usage_records" label="记录数" min-width="74" />
          <el-table-column label="CPU使用积分" min-width="110">
            <template #default="{ row }">{{ fmt2(row.cpu_cost) }}</template>
          </el-table-column>
          <el-table-column label="GPU使用积分" min-width="110">
            <template #default="{ row }">{{ fmt2(row.gpu_cost) }}</template>
          </el-table-column>
          <el-table-column label="积分消耗" min-width="96">
            <template #default="{ row }">{{ fmt2(row.total_cost) }}</template>
          </el-table-column>
          <el-table-column prop="last_seen_at" label="节点最后心跳" min-width="170" :formatter="tableTimeFormatter" />
          <el-table-column prop="last_usage_at" label="最后使用时间" min-width="170" :formatter="tableTimeFormatter" />
        </el-table>
      </div>
    </el-card>

    <el-card class="board-card section-card">
      <template #header>
        <div class="section-title-wrap">
          <span class="section-icon tone-monthly"><el-icon><Clock /></el-icon></span>
          <span>每月所有平台用户使用情况</span>
        </div>
      </template>
      <div class="monthly-filter-row">
        <el-autocomplete
          v-model="monthlyUserKeyword"
          class="monthly-user-autocomplete"
          :fetch-suggestions="queryMonthlyUsers"
          clearable
          placeholder="输入平台账号筛选（支持联想）"
          @select="onMonthlyUserSelect"
        />
        <el-button link type="primary" @click="monthlyUserKeyword = ''; monthlyTablePage = 1">清空筛选</el-button>
      </div>
      <div class="monthly-load-more">
        <el-button type="primary" plain :loading="monthlyLoading" :disabled="!monthlyHasMore" @click="loadMoreMonthlyRows">
          {{ monthlyHasMore ? "加载更多月度记录" : "月度记录已全部加载" }}
        </el-button>
        <el-text type="info" size="small">已加载 {{ monthlyRows.length }} 条月度汇总</el-text>
      </div>
      <div class="table-wrap">
        <el-table :data="pagedMonthlyRows" stripe table-layout="auto">
          <el-table-column prop="month" label="月份" min-width="90" />
          <el-table-column label="平台账号" min-width="130">
            <template #default="{ row }">
              <el-button class="user-link" link @click="openUser(row.username)">{{ row.username }}</el-button>
            </template>
          </el-table-column>
          <el-table-column prop="usage_records" label="记录数" min-width="90" />
          <el-table-column prop="gpu_process_records" label="GPU记录" min-width="96" />
          <el-table-column prop="cpu_process_records" label="CPU记录" min-width="96" />
          <el-table-column label="积分消耗" min-width="106">
            <template #default="{ row }">{{ fmt2(row.total_cost) }}</template>
          </el-table-column>
          <el-table-column label="CPU总占用%" min-width="120">
            <template #default="{ row }">{{ fmt2(row.total_cpu_percent) }}</template>
          </el-table-column>
        </el-table>
      </div>
      <div class="table-pagination">
        <el-pagination
          v-model:current-page="monthlyTablePage"
          v-model:page-size="monthlyTablePageSize"
          background
          layout="total, sizes, prev, pager, next"
          :page-sizes="[20, 50, 100, 200]"
          :total="filteredMonthlyRows.length"
        />
      </div>
    </el-card>

    <el-card class="board-card section-card">
      <template #header>
        <div class="section-title-wrap">
          <span class="section-icon tone-points"><el-icon><Coin /></el-icon></span>
          <span>积分增加统计</span>
        </div>
      </template>
      <div class="table-wrap">
        <el-table :data="rechargeRows" stripe table-layout="auto">
          <el-table-column label="平台账号" min-width="160">
            <template #default="{ row }">
              <el-button class="user-link" link @click="openUser(row.username)">{{ row.username }}</el-button>
            </template>
          </el-table-column>
          <el-table-column prop="recharge_count" label="加分次数" min-width="100" />
          <el-table-column label="加分总额" min-width="120">
            <template #default="{ row }">{{ fmt2(row.recharge_total) }}</template>
          </el-table-column>
          <el-table-column prop="last_recharge" label="最后加分时间" min-width="180" :formatter="tableTimeFormatter" />
        </el-table>
      </div>
    </el-card>
    <PlatformUserDetailDialog v-model="profileVisible" :username="selectedProfileUsername" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import type { PlatformUsageNodeDetail, PlatformUsageUserSummary, RechargeSummary, UsageDayStat, UsageMonthlySummary, UsageRetentionStatus } from "../../lib/api";
import { ApiClient } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { Clock, Coin, DataBoard, Delete, Monitor, UserFilled } from "@element-plus/icons-vue";
import { formatServerDate, formatServerDateTime, getServerTodayDateText, normalizeServerDateInput, shiftServerDateText } from "../../lib/time";

const loading = ref(false);
const exporting = ref(false);
const error = ref("");

const beijingToday = getBeijingTodayText();
const fromDate = ref(shiftServerDateText(beijingToday, -365));
const toDate = ref(beijingToday);

const userRows = ref<PlatformUsageUserSummary[]>([]);
const monthlyRows = ref<UsageMonthlySummary[]>([]);
const rechargeRows = ref<RechargeSummary[]>([]);
const monthlyUserKeyword = ref("");
const monthlyLoading = ref(false);
const monthlyHasMore = ref(false);
const monthlyOffset = ref(0);
const monthlyLoadedFrom = ref("");
const monthlyLoadedTo = ref("");
const monthlyTablePage = ref(1);
const monthlyTablePageSize = ref(50);
const allPlatformUsers = ref<string[]>([]);
const nodeRows = ref<PlatformUsageNodeDetail[]>([]);
const activeUsername = ref("");
const profileVisible = ref(false);
const selectedProfileUsername = ref("");
let rangeAutoReloadTimer: ReturnType<typeof setTimeout> | null = null;
let loadAllSeq = 0;
const appliedFromDate = ref(fromDate.value);
const appliedToDate = ref(toDate.value);
const retentionDaysDraft = ref(0);
const retentionStatus = ref<UsageRetentionStatus | null>(null);
const retentionSaving = ref(false);
const deleteRangeDays = ref<string[]>([]);
const deleteRangeEstimating = ref(false);
const deleteRangeDeleting = ref(false);
const deleteRangeEstimate = ref<{ records: number; estimated_csv_bytes: number; estimated_db_bytes: number } | null>(null);
const availableUsageDays = ref<UsageDayStat[]>([]);
const availableUsageDaysLoaded = ref(false);
const MONTHLY_FETCH_BATCH_SIZE = 1000;

function tableTimeFormatter(_: unknown, __: unknown, cellValue: unknown): string {
  return formatServerDateTime(String(cellValue ?? ""));
}

const displayUserRows = computed<PlatformUsageUserSummary[]>(() => {
  const byUser = new Map<string, PlatformUsageUserSummary>();
  for (const row of userRows.value) {
    const u = String(row.platform_username || "").trim();
    if (u) byUser.set(u, row);
  }
  const usernames = allPlatformUsers.value
    .map((u) => String(u || "").trim())
    .filter(Boolean)
    .sort((a, b) => a.localeCompare(b));
  if (!usernames.length) return userRows.value;
  return usernames.map((u) => {
    const hit = byUser.get(u);
    if (hit) return hit;
    return {
      platform_username: u,
      usage_records: 0,
      cpu_usage_seconds: 0,
      gpu_usage_seconds: 0,
      cpu_util_percent: 0,
      gpu_util_percent: 0,
      total_cost: 0,
      general_balance: 0,
    };
  });
});

const monthlyUserCandidates = computed(() => {
  const s = new Set<string>();
  for (const u of allPlatformUsers.value) {
    const v = String(u || "").trim();
    if (v) s.add(v);
  }
  for (const row of monthlyRows.value) {
    const u = String(row.username || "").trim();
    if (u) s.add(u);
  }
  for (const row of userRows.value) {
    const u = String(row.platform_username || "").trim();
    if (u) s.add(u);
  }
  return Array.from(s).sort((a, b) => a.localeCompare(b));
});

const zeroUsageMonthlyRows = computed<UsageMonthlySummary[]>(() => {
  const used = new Set(
    monthlyRows.value
      .map((row) => String(row.username || "").trim())
      .filter(Boolean),
  );
  return allPlatformUsers.value
    .map((u) => String(u || "").trim())
    .filter((u) => u && !used.has(u))
    .sort((a, b) => a.localeCompare(b))
    .map((username) => ({
      month: "无使用记录",
      username,
      usage_records: 0,
      gpu_process_records: 0,
      cpu_process_records: 0,
      total_cpu_percent: 0,
      total_memory_mb: 0,
      total_cost: 0,
    }));
});

const filteredMonthlyRows = computed(() => {
  const kw = String(monthlyUserKeyword.value || "").trim().toLowerCase();
  const full = [...monthlyRows.value, ...zeroUsageMonthlyRows.value];
  if (!kw) return full;
  return full.filter((row) => String(row.username || "").toLowerCase().includes(kw));
});

const availableUsageDaySet = computed(() => {
  const s = new Set<string>();
  for (const row of availableUsageDays.value) {
    const k = String(row.date || "").trim();
    if (k) s.add(k);
  }
  return s;
});

const canDeleteRangeAction = computed(() => {
  if (authState.role !== "admin") return false;
  return Array.isArray(deleteRangeDays.value) && deleteRangeDays.value.length === 2 && !!deleteRangeDays.value[0] && !!deleteRangeDays.value[1];
});

function getBeijingTodayText(): string {
  return getServerTodayDateText();
}

function normalizeDateInput(v: unknown, fallback: string): string {
  return normalizeServerDateInput(v, fallback);
}

const pagedMonthlyRows = computed(() => {
  const page = Number(monthlyTablePage.value || 1);
  const size = Number(monthlyTablePageSize.value || 50);
  const start = (page - 1) * size;
  return filteredMonthlyRows.value.slice(start, start + size);
});
}

function fmt2(v: number): string {
  return Number(v ?? 0).toFixed(2);
}

function fmtDuration(seconds: number): string {
  const s = Math.max(0, Math.round(Number(seconds || 0)));
  if (s < 60) return `${s}秒`;
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h > 0) return `${h}小时${m}分钟`;
  return `${m}分钟`;
}

function disableFutureDate(d: Date): boolean {
  return formatServerDate(d) > getBeijingTodayText();
}

function getRangeSafe(): [string, string] {
  const todayText = getBeijingTodayText();
  let from = normalizeDateInput(fromDate.value, shiftServerDateText(todayText, -365));
  let to = normalizeDateInput(toDate.value, todayText);
  if (from > todayText) from = todayText;
  if (to > todayText) to = todayText;
  if (from > to) {
    const t = from;
    from = to;
    to = t;
  }
  fromDate.value = from;
  toDate.value = to;
  return [from, to];
}

function onStatsRangeChanged() {
  getRangeSafe();
  scheduleAutoReloadByRangeChange();
}

function datePrefix(v: unknown, fallback: string): string {
  const s = String(v || "").trim();
  if (!s) return fallback;
  const m = s.match(/^(\d{4}-\d{2}-\d{2})/);
  if (m) return m[1];
  return normalizeDateInput(s, fallback);
}

function queryMonthlyUsers(queryString: string, cb: (items: Array<{ value: string }>) => void) {
  const q = String(queryString || "").trim().toLowerCase();
  const list = monthlyUserCandidates.value
    .filter((u) => (q ? u.toLowerCase().includes(q) : true))
    .slice(0, 30)
    .map((u) => ({ value: u }));
  cb(list);
}

function onMonthlyUserSelect(item: { value?: string }) {
  monthlyUserKeyword.value = String(item?.value || "").trim();
  monthlyTablePage.value = 1;
}

async function loadMonthlyRows(client: ApiClient, from: string, to: string, reset: boolean) {
  if (monthlyLoading.value) return;
  monthlyLoading.value = true;
  try {
    const offset = reset ? 0 : monthlyOffset.value;
    const m = await client.adminStatsMonthly({ from, to, limit: MONTHLY_FETCH_BATCH_SIZE, offset });
    const nextRows = m.rows ?? [];
    if (reset) {
      monthlyRows.value = nextRows;
      monthlyOffset.value = nextRows.length;
      monthlyTablePage.value = 1;
    } else {
      monthlyRows.value = [...monthlyRows.value, ...nextRows];
      monthlyOffset.value += nextRows.length;
    }
    monthlyHasMore.value = !!m.has_more;
    monthlyLoadedFrom.value = from;
    monthlyLoadedTo.value = to;
  } finally {
    monthlyLoading.value = false;
  }
}

async function loadMoreMonthlyRows() {
  if (!monthlyHasMore.value || monthlyLoading.value) return;
  const from = monthlyLoadedFrom.value || String(fromDate.value || "").trim();
  const to = monthlyLoadedTo.value || String(toDate.value || "").trim();
  if (!from || !to) return;
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  await loadMonthlyRows(client, from, to, false);
}

function bytesText(n: number): string {
  const x = Number(n || 0);
  if (x < 1024) return `${x} B`;
  if (x < 1024 * 1024) return `${(x / 1024).toFixed(2)} KB`;
  if (x < 1024 * 1024 * 1024) return `${(x / 1024 / 1024).toFixed(2)} MB`;
  return `${(x / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function retentionModeText(v?: string): string {
  const m = String(v || "").trim().toLowerCase();
  if (m === "auto") return "自动";
  if (m === "manual") return "手动";
  return "-";
}

function getDeleteRangeSafe(): [string, string] {
  const todayText = getBeijingTodayText();
  let from = normalizeDateInput(deleteRangeDays.value?.[0], "");
  let to = normalizeDateInput(deleteRangeDays.value?.[1], "");
  if (!from || !to) {
    throw new Error("请先选择完整删除区间");
  }
  if (from > todayText) from = todayText;
  if (to > todayText) to = todayText;
  if (from > to) {
    const t = from;
    from = to;
    to = t;
  }
  deleteRangeDays.value = [from, to];
  return [from, to];
}

function onDeleteRangeChange() {
  deleteRangeEstimate.value = null;
}

function usageDeleteDisabledDate(d: Date): boolean {
  if (disableFutureDate(d)) return true;
  if (!availableUsageDaysLoaded.value) return false;
  return !availableUsageDaySet.value.has(formatServerDate(d));
}

function applyRetentionStatus(resp: UsageRetentionStatus) {
  retentionStatus.value = resp;
  const days = Number(resp?.retention_days ?? 0);
  if (Number.isFinite(days) && days >= 0) {
    retentionDaysDraft.value = Math.min(3650, Math.round(days));
  } else {
    retentionDaysDraft.value = 0;
  }
}

async function refreshRetentionStatus(client?: ApiClient) {
  if (authState.role !== "admin") return;
  const c = client ?? new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  const resp = await c.adminUsageRetentionGet();
  applyRetentionStatus(resp);
}

async function refreshUsageDayStats(client?: ApiClient, force = false) {
  if (authState.role !== "admin") return;
  if (availableUsageDaysLoaded.value && !force) return;
  const c = client ?? new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  const resp = await c.adminUsageDays({});
  availableUsageDays.value = resp.days ?? [];
  availableUsageDaysLoaded.value = true;
}

async function loadAll() {
  const seq = ++loadAllSeq;
  loading.value = true;
  error.value = "";
  try {
    const [from, to] = getRangeSafe();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const [u, r] = await Promise.all([
      client.adminStatsPlatformUsers({ from, to, limit: 1000 }),
      client.adminStatsRecharges({ from, to, limit: 1000 }),
    ]);
    if (seq !== loadAllSeq) return;
    appliedFromDate.value = datePrefix(u.from || m.from || r.from || from, from);
    appliedToDate.value = datePrefix(u.to || m.to || r.to || to, to);
    userRows.value = u.rows ?? [];
    rechargeRows.value = r.rows ?? [];
    await loadMonthlyRows(client, from, to, true);
    if (authState.role === "admin") {
      const [usersResp, retentionResp] = await Promise.all([
        client.adminUsersDetails(5000),
        client.adminUsageRetentionGet(),
      ]);
      if (seq !== loadAllSeq) return;
      allPlatformUsers.value = (usersResp.users ?? [])
        .map((u: any) => String(u?.username || "").trim())
        .filter(Boolean);
      applyRetentionStatus(retentionResp);
      if (!availableUsageDaysLoaded.value) {
        const daysResp = await client.adminUsageDays({});
        if (seq !== loadAllSeq) return;
        availableUsageDays.value = daysResp.days ?? [];
        availableUsageDaysLoaded.value = true;
      }
    } else {
      // 高级用户不请求超管专属接口，避免“已授权看板却报无权限”。
      const s = new Set<string>();
      for (const row of userRows.value) {
        const u = String(row.platform_username || "").trim();
        if (u) s.add(u);
      }
      for (const row of monthlyRows.value) {
        const u = String(row.username || "").trim();
        if (u) s.add(u);
      }
      allPlatformUsers.value = Array.from(s).sort((a, b) => a.localeCompare(b));
      retentionStatus.value = null;
      retentionDaysDraft.value = 0;
      availableUsageDays.value = [];
      availableUsageDaysLoaded.value = false;
    }
    if (displayUserRows.value.length > 0) {
      await loadUserNodes(displayUserRows.value[0].platform_username);
    } else {
      activeUsername.value = "";
      nodeRows.value = [];
    }
  } catch (e: any) {
    if (seq !== loadAllSeq) return;
    error.value = e?.message ?? String(e);
  } finally {
    if (seq === loadAllSeq) {
      loading.value = false;
    }
  }
}

async function loadUserNodes(username: string) {
  if (!username) return;
  activeUsername.value = username;
  try {
    const [from, to] = getRangeSafe();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminStatsPlatformUserNodes(username, { from, to, limit: 2000 });
    nodeRows.value = r.rows ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function exportRangeCSV() {
  exporting.value = true;
  error.value = "";
  try {
    const [from, to] = getRangeSafe();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const blob = await client.adminExportUsageCSV({ from, to, limit: 200000 });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `board_usage_${from}_${to}.csv`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    ElMessage.success(`已开始下载 board_usage_${from}_${to}.csv`);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    exporting.value = false;
  }
}

async function saveRetentionDays() {
  if (authState.role !== "admin") return;
  retentionSaving.value = true;
  error.value = "";
  try {
    const retentionDays = Math.min(3650, Math.max(0, Math.round(Number(retentionDaysDraft.value || 0))));
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const resp = await client.adminUsageRetentionSet({ retention_days: retentionDays });
    applyRetentionStatus(resp);
    ElMessage.success(retentionDays > 0 ? `已设置自动删除保留 ${retentionDays} 天` : "已关闭自动删除");
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    retentionSaving.value = false;
  }
}

async function estimateDeleteRange() {
  if (!canDeleteRangeAction.value) return;
  deleteRangeEstimating.value = true;
  error.value = "";
  try {
    const [from, to] = getDeleteRangeSafe();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const resp = await client.adminUsageRangeEstimate({ from, to });
    deleteRangeEstimate.value = {
      records: Number(resp.records || 0),
      estimated_csv_bytes: Number(resp.estimated_csv_bytes || 0),
      estimated_db_bytes: Number(resp.estimated_db_bytes || 0),
    };
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    deleteRangeEstimating.value = false;
  }
}

async function deleteRangeNow() {
  if (!canDeleteRangeAction.value) return;
  deleteRangeDeleting.value = true;
  error.value = "";
  try {
    const [from, to] = getDeleteRangeSafe();
    await ElMessageBox.confirm(
      `将删除 ${from} 至 ${to} 的全部使用记录。此操作不可恢复，确认继续吗？`,
      "确认删除区间记录",
      { type: "warning", confirmButtonText: "确认删除", cancelButtonText: "取消" },
    );
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const resp = await client.adminUsageDeleteRange({ from, to, confirm: true });
    deleteRangeEstimate.value = {
      records: Number(resp.records_before || 0),
      estimated_csv_bytes: Number(resp.estimated_csv_bytes || 0),
      estimated_db_bytes: Number(resp.estimated_db_bytes || 0),
    };
    ElMessage.success(`删除完成：${resp.deleted_records} 条记录`);
    await Promise.all([
      refreshRetentionStatus(client),
      refreshUsageDayStats(client, true),
      loadAll(),
    ]);
  } catch (e: any) {
    if (String(e?.message || "") !== "cancel") {
      error.value = e?.message ?? String(e);
    }
  } finally {
    deleteRangeDeleting.value = false;
  }
}

function openUser(username: string) {
  const u = String(username || "").trim();
  if (!u) return;
  selectedProfileUsername.value = u;
  profileVisible.value = true;
  loadUserNodes(u);
}

function scheduleAutoReloadByRangeChange() {
  if (rangeAutoReloadTimer) {
    clearTimeout(rangeAutoReloadTimer);
  }
  rangeAutoReloadTimer = setTimeout(() => {
    rangeAutoReloadTimer = null;
    void loadAll();
  }, 250);
}

loadAll();

watch(
  () => filteredMonthlyRows.value.length,
  () => {
    const total = filteredMonthlyRows.value.length;
    const size = Number(monthlyTablePageSize.value || 50);
    const maxPage = Math.max(1, Math.ceil(total / size));
    if (monthlyTablePage.value > maxPage) {
      monthlyTablePage.value = maxPage;
    }
  },
);
</script>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.head-actions { display: inline-flex; align-items: center; gap: 8px; }
.title { font-weight: 700; font-size: 16px; }
.mb { margin-bottom: 12px; }
.range-meta { margin-top: 8px; color: #475569; font-size: 13px; }
.retention-meta {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  color: #475569;
  font-size: 13px;
}
.retention-tip {
  color: #64748b;
  font-size: 13px;
}
.board-wrap {
  width: 100%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.board-card {
  min-width: 0;
  overflow: hidden;
}
.section-card {
  border: 1px solid var(--border-color);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.06);
}
.section-card :deep(.el-card__header) {
  padding: 12px 16px;
  background: #f7fbff;
  border-bottom: 1px solid var(--border-color);
}
.section-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-weight: 700;
  color: var(--text-primary);
}
.section-icon {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.35);
}
.section-icon :deep(svg) {
  width: 16px;
  height: 16px;
}
.tone-overview {
  background: linear-gradient(135deg, #2563eb, #1d4ed8);
  color: #dbeafe;
}
.tone-users {
  background: linear-gradient(135deg, #0f766e, #0d9488);
  color: #ccfbf1;
}
.tone-nodes {
  background: linear-gradient(135deg, #7c3aed, #6d28d9);
  color: #ede9fe;
}
.tone-monthly {
  background: linear-gradient(135deg, #0369a1, #0284c7);
  color: #e0f2fe;
}
.tone-points {
  background: linear-gradient(135deg, #be123c, #e11d48);
  color: #ffe4e6;
}
.tone-retention {
  background: linear-gradient(135deg, #9a3412, #ea580c);
  color: #ffedd5;
}
.table-wrap {
  width: 100%;
  overflow-x: auto;
}
.table-wrap :deep(.el-table) {
  min-width: 980px;
}
.monthly-filter-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.monthly-load-more {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.monthly-user-autocomplete {
  width: 320px;
}
.table-pagination {
  margin-top: 10px;
  display: flex;
  justify-content: flex-end;
}
.range-sep {
  color: #64748b;
  margin: 0 8px;
}
.user-link {
  color: #0f766e;
  font-weight: 600;
}
@media (max-width: 1200px) {
  .table-wrap :deep(.el-table) {
    min-width: 880px;
  }
}
@media (max-width: 900px) {
  .head {
    flex-wrap: wrap;
  }
}
</style>
