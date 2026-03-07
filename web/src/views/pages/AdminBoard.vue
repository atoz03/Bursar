<template>
  <div class="board-wrap">
    <el-card class="section-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-overview"><el-icon><DataBoard /></el-icon></span>
            <span class="title">运营看板</span>
          </div>
          <el-button type="primary" :loading="loading" @click="loadAll">刷新</el-button>
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
            :disabled-date="disableFutureDate"
          />
          <span class="range-sep">至</span>
          <el-date-picker
            v-model="toDate"
            type="date"
            placeholder="结束日期"
            value-format="YYYY-MM-DD"
            :disabled-date="disableFutureDate"
          />
        </el-form-item>
      </el-form>
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
          <el-table-column prop="usage_records" label="记录数" min-width="96" />
          <el-table-column prop="gpu_process_records" label="GPU记录" min-width="96" />
          <el-table-column prop="cpu_process_records" label="CPU记录" min-width="96" />
          <el-table-column label="CPU总占用%" min-width="116">
            <template #default="{ row }">{{ fmt2(row.total_cpu_percent) }}</template>
          </el-table-column>
          <el-table-column label="内存MB累计" min-width="116">
            <template #default="{ row }">{{ fmt2(row.total_memory_mb) }}</template>
          </el-table-column>
          <el-table-column label="积分消耗" min-width="110">
            <template #default="{ row }">{{ fmt2(row.total_cost) }}</template>
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
          <el-table-column label="CPU累计%" min-width="96">
            <template #default="{ row }">{{ fmt2(row.total_cpu_percent) }}</template>
          </el-table-column>
          <el-table-column label="内存累计MB" min-width="110">
            <template #default="{ row }">{{ fmt2(row.total_memory_mb) }}</template>
          </el-table-column>
          <el-table-column label="积分消耗" min-width="96">
            <template #default="{ row }">{{ fmt2(row.total_cost) }}</template>
          </el-table-column>
          <el-table-column prop="last_seen_at" label="节点最后心跳" min-width="170" />
          <el-table-column prop="last_usage_at" label="最后使用时间" min-width="170" />
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
          <el-table-column prop="last_recharge" label="最后加分时间" min-width="180" />
        </el-table>
      </div>
    </el-card>
    <PlatformUserDetailDialog v-model="profileVisible" :username="selectedProfileUsername" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { PlatformUsageNodeDetail, PlatformUsageUserSummary, RechargeSummary, UsageMonthlySummary } from "../../lib/api";
import { ApiClient } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { Clock, Coin, DataBoard, Monitor, UserFilled } from "@element-plus/icons-vue";

const loading = ref(false);
const error = ref("");

const today = new Date();
const yearAgo = new Date();
yearAgo.setDate(today.getDate() - 365);
const fromDate = ref(fmtDate(yearAgo));
const toDate = ref(fmtDate(today));

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
const MONTHLY_FETCH_BATCH_SIZE = 1000;

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
      gpu_process_records: 0,
      cpu_process_records: 0,
      total_cpu_percent: 0,
      total_memory_mb: 0,
      total_cost: 0,
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

const pagedMonthlyRows = computed(() => {
  const page = Number(monthlyTablePage.value || 1);
  const size = Number(monthlyTablePageSize.value || 50);
  const start = (page - 1) * size;
  return filteredMonthlyRows.value.slice(start, start + size);
});

function fmtDate(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function fmt2(v: number): string {
  return Number(v ?? 0).toFixed(2);
}

function disableFutureDate(d: Date): boolean {
  const endOfToday = new Date();
  endOfToday.setHours(23, 59, 59, 999);
  return d.getTime() > endOfToday.getTime();
}

function getRangeSafe(): [string, string] {
  const todayText = fmtDate(new Date());
  let from = String(fromDate.value || "").trim() || fmtDate(yearAgo);
  let to = String(toDate.value || "").trim() || todayText;
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

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [from, to] = getRangeSafe();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const [u, r] = await Promise.all([
      client.adminStatsPlatformUsers({ from, to, limit: 1000 }),
      client.adminStatsRecharges({ from, to, limit: 1000 }),
    ]);
    userRows.value = u.rows ?? [];
    rechargeRows.value = r.rows ?? [];
    await loadMonthlyRows(client, from, to, true);
    if (authState.role === "admin") {
      const usersResp = await client.adminUsersDetails(5000);
      allPlatformUsers.value = (usersResp.users ?? [])
        .map((u: any) => String(u?.username || "").trim())
        .filter(Boolean);
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
    }
    if (displayUserRows.value.length > 0) {
      await loadUserNodes(displayUserRows.value[0].platform_username);
    } else {
      activeUsername.value = "";
      nodeRows.value = [];
    }
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
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
.head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.title { font-weight: 700; font-size: 16px; }
.mb { margin-bottom: 12px; }
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
