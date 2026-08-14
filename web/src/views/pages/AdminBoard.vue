<template>
  <div class="board-wrap">
    <section class="board-hero">
      <div class="hero-copy">
        <span class="eyebrow">OPERATIONS OVERVIEW</span>
        <div class="hero-title-row">
          <span class="hero-icon"><el-icon><DataBoard /></el-icon></span>
          <div>
            <h1>运营看板</h1>
            <p>聚合资源使用与积分变化，详情数据按需加载。</p>
          </div>
        </div>
      </div>
      <div class="head-actions">
        <span v-if="lastSyncedAt" class="sync-time">更新于 {{ lastSyncedAt }}</span>
        <el-button v-if="authState.role === 'admin'" plain @click="toggleRetention">
          <el-icon><Delete /></el-icon>{{ retentionExpanded ? "收起数据工具" : "数据工具" }}
        </el-button>
        <el-button v-if="authState.role === 'admin'" plain :loading="exporting" @click="exportRangeCSV">导出 CSV</el-button>
        <el-button type="primary" :loading="loading" @click="loadAll">同步数据</el-button>
      </div>
    </section>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />

    <el-card class="range-card" shadow="never">
      <div class="range-toolbar">
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
        <div class="quick-ranges">
          <span>快捷区间</span>
          <el-button v-for="item in quickRangeOptions" :key="item.days" text @click="applyQuickRange(item.days)">{{ item.label }}</el-button>
          <el-button v-if="rangeDirty" type="primary" plain @click="loadAll">应用区间</el-button>
        </div>
      </div>
      <div class="range-meta">当前数据：{{ appliedFromDate }} 至 {{ appliedToDate }}</div>
    </el-card>

    <div class="metric-grid" v-loading="loading">
      <article class="metric-card tone-blue">
        <span class="metric-label">平台用户</span>
        <strong>{{ displayUserRows.length }}</strong>
        <small>{{ activeUserCount }} 位在区间内有记录</small>
      </article>
      <article class="metric-card tone-violet">
        <span class="metric-label">GPU 使用时间</span>
        <strong>{{ fmtCompactDuration(totalGPUSeconds) }}</strong>
        <small>按节点采样周期累计</small>
      </article>
      <article class="metric-card tone-cyan">
        <span class="metric-label">CPU 使用时间</span>
        <strong>{{ fmtCompactDuration(totalCPUSeconds) }}</strong>
        <small>有效 CPU 活跃时间</small>
      </article>
      <article class="metric-card tone-amber">
        <span class="metric-label">积分消耗</span>
        <strong>{{ fmt2(totalCost) }}</strong>
        <small>区间内累计消耗</small>
      </article>
    </div>

    <el-card v-if="authState.role === 'admin' && retentionExpanded" class="section-card tool-card" v-loading="retentionLoading">
      <template #header>
        <div class="section-title-wrap">
          <span class="section-icon tone-retention"><el-icon><Delete /></el-icon></span>
          <span>数据留存与删除</span>
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
          <span>用户使用概览</span>
        </div>
      </template>
      <div class="table-wrap">
        <el-table :data="displayUserRows" stripe table-layout="auto" v-loading="loading" empty-text="当前区间暂无用户数据">
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

    <el-card v-if="activeUsername" class="board-card section-card">
      <template #header>
        <div class="section-title-wrap">
          <span class="section-icon tone-nodes"><el-icon><Monitor /></el-icon></span>
          <span>节点使用详情</span>
        </div>
      </template>
      <div style="margin-bottom: 8px; color: #64748b">
        当前平台账号：{{ activeUsername || "未选择" }}
      </div>
      <div class="table-wrap">
        <el-table :data="nodeRows" stripe table-layout="auto" v-loading="nodeLoading">
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
          placeholder="筛选平台账号"
          @select="onMonthlyUserSelect"
        />
        <el-button link type="primary" @click="monthlyUserKeyword = ''; monthlyTablePage = 1">清空筛选</el-button>
      </div>
      <div class="monthly-load-more">
        <el-button v-if="!monthlyRequested" type="primary" plain :loading="monthlyLoading" @click="requestMonthlyRows">
          加载月度统计
        </el-button>
        <el-button v-else type="primary" plain :loading="monthlyLoading" :disabled="!monthlyHasMore" @click="loadMoreMonthlyRows">
          {{ monthlyHasMore ? "加载更多月度记录" : "月度记录已全部加载" }}
        </el-button>
        <el-text type="info" size="small">
          {{ monthlyRequested ? `已加载 ${monthlyRows.length} 条月度汇总` : "月度聚合按需加载，不影响看板首屏" }}
        </el-text>
      </div>
      <div v-if="monthlyRequested" class="table-wrap">
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
      <div v-if="monthlyRequested" class="table-pagination">
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
const fromDate = ref(shiftServerDateText(beijingToday, -30));
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
let loadAllSeq = 0;
const appliedFromDate = ref(fromDate.value);
const appliedToDate = ref(toDate.value);
const lastSyncedAt = ref("");
const retentionExpanded = ref(false);
const retentionLoading = ref(false);
const monthlyRequested = ref(false);
const nodeLoading = ref(false);
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
const quickRangeOptions = [
  { label: "7 天", days: 7 },
  { label: "30 天", days: 30 },
  { label: "90 天", days: 90 },
  { label: "1 年", days: 365 },
];

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

const activeUserCount = computed(() => displayUserRows.value.filter((row) => Number(row.usage_records || 0) > 0).length);
const totalCPUSeconds = computed(() => displayUserRows.value.reduce((sum, row) => sum + Number(row.cpu_usage_seconds || 0), 0));
const totalGPUSeconds = computed(() => displayUserRows.value.reduce((sum, row) => sum + Number(row.gpu_usage_seconds || 0), 0));
const totalCost = computed(() => displayUserRows.value.reduce((sum, row) => sum + Number(row.total_cost || 0), 0));
const rangeDirty = computed(() => fromDate.value !== appliedFromDate.value || toDate.value !== appliedToDate.value);

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

const pagedMonthlyRows = computed(() => {
  const page = Number(monthlyTablePage.value || 1);
  const size = Number(monthlyTablePageSize.value || 50);
  const start = (page - 1) * size;
  return filteredMonthlyRows.value.slice(start, start + size);
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

function fmtCompactDuration(seconds: number): string {
  const value = Math.max(0, Number(seconds || 0));
  if (value < 3600) return `${Math.round(value / 60)} 分钟`;
  const hours = value / 3600;
  if (hours < 1000) return `${hours.toFixed(hours >= 100 ? 0 : 1)} 小时`;
  return `${(hours / 1000).toFixed(1)}k 小时`;
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
}

function applyQuickRange(days: number) {
  const today = getBeijingTodayText();
  fromDate.value = shiftServerDateText(today, -Math.max(1, days));
  toDate.value = today;
  void loadAll();
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
  if (monthlyLoading.value) return null;
  monthlyLoading.value = true;
  try {
    const offset = reset ? 0 : monthlyOffset.value;
    const resp = await client.adminStatsMonthly({ from, to, limit: MONTHLY_FETCH_BATCH_SIZE, offset });
    const nextRows = resp.rows ?? [];
    if (reset) {
      monthlyRows.value = nextRows;
      monthlyOffset.value = nextRows.length;
      monthlyTablePage.value = 1;
    } else {
      monthlyRows.value = [...monthlyRows.value, ...nextRows];
      monthlyOffset.value += nextRows.length;
    }
    monthlyHasMore.value = !!resp.has_more;
    monthlyLoadedFrom.value = from;
    monthlyLoadedTo.value = to;
    return resp;
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

async function requestMonthlyRows() {
  monthlyRequested.value = true;
  const [from, to] = getRangeSafe();
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  try {
    await loadMonthlyRows(client, from, to, true);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
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

async function toggleRetention() {
  retentionExpanded.value = !retentionExpanded.value;
  if (!retentionExpanded.value || authState.role !== "admin") return;
  retentionLoading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await Promise.all([refreshRetentionStatus(client), refreshUsageDayStats(client)]);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    retentionLoading.value = false;
  }
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
    appliedFromDate.value = datePrefix(u.from || r.from || from, from);
    appliedToDate.value = datePrefix(u.to || r.to || to, to);
    userRows.value = u.rows ?? [];
    rechargeRows.value = r.rows ?? [];
    allPlatformUsers.value = userRows.value
      .map((row) => String(row.platform_username || "").trim())
      .filter(Boolean)
      .sort((a, b) => a.localeCompare(b));
    activeUsername.value = "";
    nodeRows.value = [];
    lastSyncedAt.value = new Date().toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
    if (monthlyRequested.value) {
      monthlyRows.value = [];
      monthlyOffset.value = 0;
      void loadMonthlyRows(client, from, to, true).catch((e: any) => {
        error.value = e?.message ?? String(e);
      });
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
  nodeLoading.value = true;
  try {
    const [from, to] = getRangeSafe();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminStatsPlatformUserNodes(username, { from, to, limit: 2000 });
    nodeRows.value = r.rows ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    nodeLoading.value = false;
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
.board-wrap { width: 100%; min-width: 0; display: flex; flex-direction: column; gap: 16px; }
.board-hero {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 24px 26px;
  overflow: hidden;
  border: 1px solid #dce6f2;
  border-radius: 20px;
  background:
    radial-gradient(500px 180px at 92% -20%, rgba(37, 99, 235, .14), transparent 70%),
    linear-gradient(135deg, #fff 0%, #f7faff 100%);
  box-shadow: 0 12px 30px rgba(15, 23, 42, .07);
}
.hero-copy { min-width: 0; }
.eyebrow { display: block; margin-bottom: 9px; color: #2563eb; font-size: 11px; font-weight: 800; letter-spacing: .13em; }
.hero-title-row { display: flex; align-items: center; gap: 14px; }
.hero-icon { width: 46px; height: 46px; display: grid; place-items: center; flex: 0 0 auto; border-radius: 14px; color: #fff; font-size: 23px; background: linear-gradient(135deg, #2563eb, #4f46e5); box-shadow: 0 10px 22px rgba(37, 99, 235, .24); }
.hero-title-row h1 { margin: 0; font-size: clamp(24px, 2vw, 31px); line-height: 1.2; letter-spacing: -.035em; }
.hero-title-row p { margin: 6px 0 0; color: #64748b; font-size: 14px; }
.head-actions { display: flex; align-items: center; justify-content: flex-end; gap: 9px; flex-wrap: wrap; }
.sync-time { color: #94a3b8; font-size: 12px; white-space: nowrap; }
.mb { margin-bottom: 0; }
.range-card { border-radius: 16px !important; }
.range-card :deep(.el-card__body) { padding: 14px 16px; }
.range-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.range-toolbar :deep(.el-form-item) { margin: 0; }
.quick-ranges { display: flex; align-items: center; justify-content: flex-end; gap: 3px; flex-wrap: wrap; color: #94a3b8; font-size: 12px; }
.range-meta { margin-top: 8px; color: #94a3b8; font-size: 12px; }
.metric-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; min-height: 126px; }
.metric-card { position: relative; min-width: 0; padding: 20px; overflow: hidden; border: 1px solid #e2e8f0; border-radius: 18px; background: #fff; box-shadow: 0 8px 22px rgba(15, 23, 42, .055); }
.metric-card::after { content: ""; position: absolute; right: -24px; top: -30px; width: 100px; height: 100px; border-radius: 999px; opacity: .12; background: currentColor; }
.metric-label { display: block; color: #64748b; font-size: 13px; font-weight: 650; }
.metric-card strong { display: block; margin-top: 13px; color: #0f172a; font-size: clamp(23px, 2vw, 30px); line-height: 1; letter-spacing: -.04em; white-space: nowrap; }
.metric-card small { display: block; margin-top: 10px; color: #94a3b8; font-size: 12px; }
.metric-card.tone-blue { color: #2563eb; border-top: 3px solid #3b82f6; }
.metric-card.tone-violet { color: #7c3aed; border-top: 3px solid #8b5cf6; }
.metric-card.tone-cyan { color: #0891b2; border-top: 3px solid #06b6d4; }
.metric-card.tone-amber { color: #d97706; border-top: 3px solid #f59e0b; }
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
.board-card { min-width: 0; overflow: hidden; }
.section-card {
  border: 1px solid #e1e8f1;
  box-shadow: 0 8px 24px rgba(15, 23, 42, .055);
}
.section-card :deep(.el-card__header) {
  padding: 15px 18px;
  background: #fff;
  border-bottom: 1px solid #edf1f6;
}
.section-card :deep(.el-card__body) { padding: 16px 18px 18px; }
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
.table-wrap :deep(.el-table) { min-width: 980px; }
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
  color: #2563eb;
  font-weight: 650;
}
@media (max-width: 1280px) { .metric-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 1200px) {
  .table-wrap :deep(.el-table) {
    min-width: 880px;
  }
}
@media (max-width: 900px) {
  .board-hero { align-items: flex-start; flex-direction: column; padding: 20px; }
  .head-actions { justify-content: flex-start; }
  .range-toolbar { align-items: flex-start; flex-direction: column; }
  .quick-ranges { justify-content: flex-start; }
}
@media (max-width: 620px) {
  .metric-grid { grid-template-columns: 1fr; }
  .hero-title-row { align-items: flex-start; }
  .head-actions { width: 100%; }
  .head-actions :deep(.el-button) { margin-left: 0; }
  .range-toolbar :deep(.el-date-editor) { width: 132px; }
  .monthly-filter-row, .monthly-load-more { align-items: flex-start; flex-direction: column; }
  .monthly-user-autocomplete { width: 100%; }
}
</style>
