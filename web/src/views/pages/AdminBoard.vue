<template>
  <div class="board-wrap">
    <section class="ops-page-hero board-hero">
      <div class="ops-hero-copy">
        <span class="ops-eyebrow">OPERATIONS OVERVIEW</span>
        <div class="ops-title-row">
          <span class="ops-hero-icon"><el-icon><DataBoard /></el-icon></span>
          <div>
            <h1>运营看板</h1>
            <p>统一查看活跃用户、进程采样时长、积分消耗与积分变动。</p>
          </div>
        </div>
      </div>
      <div class="ops-hero-actions">
        <span v-if="lastSyncedAt" class="ops-sync-meta">更新于 {{ lastSyncedAt }}</span>
        <el-button v-if="authState.role === 'admin'" plain @click="openRetentionTools">
          <el-icon><Delete /></el-icon>数据工具
        </el-button>
        <el-button v-if="authState.role === 'admin'" plain :loading="exporting" :disabled="rangeDirty" @click="exportRangeCSV">
          <el-icon><Download /></el-icon>导出当前区间
        </el-button>
        <el-button type="primary" :loading="loading || monthlyLoading" @click="loadDashboard(true)">
          <el-icon><Refresh /></el-icon>刷新
        </el-button>
      </div>
    </section>

    <el-alert v-if="error" :title="error" type="error" show-icon class="board-alert" />

    <section class="range-panel">
      <div class="range-picker-wrap">
        <span class="range-label">统计区间</span>
        <el-date-picker
          v-model="draftRange"
          type="daterange"
          value-format="YYYY-MM-DD"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          :clearable="false"
          :editable="false"
          :disabled-date="disableFutureDate"
          @change="normalizeDraftRange"
        />
        <el-button type="primary" :disabled="!rangeDirty" :loading="loading" @click="applyDraftRange">应用区间</el-button>
      </div>
      <div class="quick-ranges">
        <el-button v-for="item in quickRangeOptions" :key="item.days" text @click="applyQuickRange(item.days)">{{ item.label }}</el-button>
      </div>
      <div class="applied-range">
        <span>当前数据</span><b>{{ appliedRange[0] }} 至 {{ appliedRange[1] }}</b>
        <el-tag v-if="rangeDirty" size="small" type="warning" effect="plain">待应用</el-tag>
      </div>
    </section>

    <section class="ops-metric-grid board-metrics" v-loading="loading && !userRows.length">
      <article class="ops-metric-card metric-compact ops-tone-green">
        <span>活跃用户</span>
        <strong>{{ activeUserCount }}<small> / {{ userRows.length }} 人</small></strong>
        <small>区间内存在进程采样</small>
      </article>
      <article class="ops-metric-card metric-compact ops-tone-amber">
        <span>积分消耗</span>
        <strong>{{ fmt2(totalCost) }}</strong>
        <small>区间累计计算资源扣费</small>
      </article>
      <article class="ops-metric-card metric-compact ops-tone-violet">
        <span>GPU 进程时长</span>
        <strong>{{ fmtCompactDuration(totalGPUSeconds) }}</strong>
        <small>按 GPU 进程采样周期累计</small>
      </article>
      <article class="ops-metric-card metric-compact ops-tone-blue">
        <span>CPU 进程时长</span>
        <strong>{{ fmtCompactDuration(totalCPUSeconds) }}</strong>
        <small>按 CPU 进程采样周期累计</small>
      </article>
    </section>

    <section class="overview-grid">
      <article class="panel trend-panel">
        <header class="panel-head">
          <div>
            <span class="section-eyebrow">DAILY TREND</span>
            <h2>逐日趋势</h2>
          </div>
          <el-radio-group v-model="trendMetric" size="small">
            <el-radio-button value="cost">积分消耗</el-radio-button>
            <el-radio-button value="active">活跃用户</el-radio-button>
          </el-radio-group>
        </header>
        <div class="trend-summary">
          <span>合计 <b>{{ trendTotalText }}</b></span>
          <span>日均 <b>{{ trendAverageText }}</b></span>
          <span>峰值 <b>{{ trendPeakText }}</b></span>
        </div>
        <div class="trend-scroll" v-loading="loading && !dailyRows.length">
          <div class="trend-chart" :style="{ minWidth: `${Math.max(620, dailyRows.length * 22)}px` }">
            <div v-for="(row, index) in dailyRows" :key="row.date" class="trend-column" :title="trendTooltip(row)">
              <div class="trend-bar-track"><i :style="{ height: `${trendBarHeight(row)}%` }" /></div>
              <span>{{ trendLabelVisible(index) ? row.date.slice(5) : '' }}</span>
            </div>
          </div>
        </div>
      </article>

      <article class="panel points-overview">
        <header class="panel-head">
          <div>
            <span class="section-eyebrow">POINTS FLOW</span>
            <h2>积分变动</h2>
          </div>
          <el-icon class="panel-icon"><Coin /></el-icon>
        </header>
        <div class="flow-item increase">
          <span>发放</span><strong>+{{ fmt2(totalIncrease) }}</strong><small>{{ totalIncreaseCount }} 次</small>
        </div>
        <div class="flow-item decrease">
          <span>扣减</span><strong>-{{ fmt2(totalDecrease) }}</strong><small>{{ totalDecreaseCount }} 次</small>
        </div>
        <div class="flow-net">
          <span>净变化</span><b :class="{ negative: totalNetChange < 0 }">{{ signedPoints(totalNetChange) }}</b>
        </div>
        <div class="activity-ring-row">
          <div class="activity-ring" :style="donutStyle(activeUserCount, userRows.length)"><span>{{ percentText(activeUserCount, userRows.length) }}</span></div>
          <div><b>{{ activeUserCount }} 位活跃用户</b><small>{{ Math.max(0, userRows.length - activeUserCount) }} 位区间内无采样</small></div>
        </div>
      </article>
    </section>

    <section class="panel users-panel">
      <header class="panel-head users-head">
        <div>
          <span class="section-eyebrow">USER RANKING</span>
          <h2>用户使用排行</h2>
        </div>
        <div class="table-tools">
          <el-input v-model="userKeyword" clearable placeholder="搜索平台账号" class="keyword-input" />
          <el-select v-model="userActivityFilter" class="activity-select">
            <el-option label="全部用户" value="all" />
            <el-option label="仅活跃" value="active" />
            <el-option label="仅未活跃" value="inactive" />
          </el-select>
        </div>
      </header>
      <div class="table-wrap">
        <el-table :data="pagedUserRows" stripe table-layout="auto" v-loading="loading" empty-text="当前条件下暂无用户">
          <el-table-column label="平台账号" min-width="150">
            <template #default="{ row }">
              <el-button class="user-link" link @click="openUserDrawer(row.platform_username)">{{ row.platform_username }}</el-button>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="92">
            <template #default="{ row }"><el-tag size="small" :type="row.usage_records > 0 ? 'success' : 'info'">{{ row.usage_records > 0 ? '活跃' : '未活跃' }}</el-tag></template>
          </el-table-column>
          <el-table-column label="GPU进程时长" min-width="130"><template #default="{ row }">{{ fmtDuration(row.gpu_process_seconds ?? row.gpu_usage_seconds) }}</template></el-table-column>
          <el-table-column label="CPU进程时长" min-width="130"><template #default="{ row }">{{ fmtDuration(row.cpu_process_seconds ?? row.cpu_usage_seconds) }}</template></el-table-column>
          <el-table-column label="积分消耗" min-width="108"><template #default="{ row }">{{ fmt2(row.total_cost) }}</template></el-table-column>
          <el-table-column label="总可用积分" min-width="118"><template #default="{ row }">{{ fmt2(row.total_balance ?? row.general_balance) }}</template></el-table-column>
          <el-table-column label="最后使用" min-width="168"><template #default="{ row }">{{ row.last_usage_at ? formatServerDateTime(row.last_usage_at) : '-' }}</template></el-table-column>
          <el-table-column label="操作" width="86" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="openUserDrawer(row.platform_username)">查看</el-button></template></el-table-column>
        </el-table>
      </div>
      <div class="table-pagination">
        <el-pagination v-model:current-page="userPage" v-model:page-size="userPageSize" background layout="total, sizes, prev, pager, next" :page-sizes="[20, 50, 100]" :total="filteredUserRows.length" />
      </div>
    </section>

    <section class="panel changes-panel">
      <header class="panel-head users-head">
        <div><span class="section-eyebrow">POINTS OPERATIONS</span><h2>用户积分变动</h2></div>
        <el-input v-model="pointsKeyword" clearable placeholder="搜索平台账号" class="keyword-input" />
      </header>
      <div class="table-wrap">
        <el-table :data="filteredRechargeRows" stripe table-layout="auto" empty-text="当前区间暂无积分变动">
          <el-table-column label="平台账号" min-width="150"><template #default="{ row }"><el-button class="user-link" link @click="openUserDrawer(row.username)">{{ row.username }}</el-button></template></el-table-column>
          <el-table-column label="发放" min-width="126"><template #default="{ row }"><span class="points-positive">+{{ fmt2(row.increase_total) }}</span><small class="cell-note">{{ row.increase_count }} 次</small></template></el-table-column>
          <el-table-column label="扣减" min-width="126"><template #default="{ row }"><span class="points-negative">-{{ fmt2(row.decrease_total) }}</span><small class="cell-note">{{ row.decrease_count }} 次</small></template></el-table-column>
          <el-table-column label="净变化" min-width="110"><template #default="{ row }"><b :class="row.net_change < 0 ? 'points-negative' : 'points-positive'">{{ signedPoints(row.net_change) }}</b></template></el-table-column>
          <el-table-column prop="last_recharge" label="最后操作" min-width="180" :formatter="tableTimeFormatter" />
        </el-table>
      </div>
    </section>

    <section class="panel monthly-panel">
      <header class="panel-head users-head">
        <div>
          <span class="section-eyebrow">MONTHLY REPORT</span>
          <h2>完整自然月报</h2>
        </div>
        <div class="table-tools">
          <el-date-picker v-model="monthlyMonth" type="month" value-format="YYYY-MM" :clearable="false" :editable="false" :disabled-date="disableIncompleteMonth" @change="loadMonthlyReport" />
          <el-input v-model="monthlyKeyword" clearable placeholder="搜索平台账号" class="keyword-input" />
          <el-select v-model="monthlyUsageFilter" class="activity-select">
            <el-option label="全部用户" value="all" />
            <el-option label="有使用" value="active" />
            <el-option label="无使用" value="inactive" />
          </el-select>
        </div>
      </header>
      <div class="monthly-summary">
        <span><b>{{ monthlyRows.length }}</b> 位授权用户</span>
        <span><b>{{ monthlyActiveCount }}</b> 位有使用</span>
        <span>消耗 <b>{{ fmt2(monthlyTotalCost) }}</b> 积分</span>
      </div>
      <div class="table-wrap">
        <el-table :data="pagedMonthlyRows" stripe table-layout="auto" v-loading="monthlyLoading" empty-text="该自然月暂无用户数据">
          <el-table-column prop="month" label="月份" width="100" />
          <el-table-column label="平台账号" min-width="140"><template #default="{ row }"><el-button class="user-link" link @click="openUserDrawer(row.username)">{{ row.username }}</el-button></template></el-table-column>
          <el-table-column label="状态" width="92"><template #default="{ row }"><el-tag size="small" :type="row.usage_records > 0 ? 'success' : 'info'">{{ row.usage_records > 0 ? '有使用' : '无使用' }}</el-tag></template></el-table-column>
          <el-table-column label="GPU进程时长" min-width="130"><template #default="{ row }">{{ fmtDuration(row.gpu_process_seconds) }}</template></el-table-column>
          <el-table-column label="CPU进程时长" min-width="130"><template #default="{ row }">{{ fmtDuration(row.cpu_process_seconds) }}</template></el-table-column>
          <el-table-column label="积分消耗" min-width="108"><template #default="{ row }">{{ fmt2(row.total_cost) }}</template></el-table-column>
        </el-table>
      </div>
      <div class="table-pagination">
        <el-pagination v-model:current-page="monthlyPage" v-model:page-size="monthlyPageSize" background layout="total, sizes, prev, pager, next" :page-sizes="[20, 50, 100]" :total="filteredMonthlyRows.length" />
      </div>
    </section>

    <el-drawer v-model="userDrawerVisible" :title="`${activeUsername || '用户'} · 区间节点明细`" size="min(920px, 94vw)" destroy-on-close @closed="closeUserDrawer">
      <div class="drawer-range">统计区间：{{ drawerRange[0] }} 至 {{ drawerRange[1] }}</div>
      <div class="drawer-metrics">
        <div><span>节点数</span><b>{{ nodeRows.length }}</b></div>
        <div><span>CPU积分</span><b>{{ fmt2(nodeCPUCost) }}</b></div>
        <div><span>GPU积分</span><b>{{ fmt2(nodeGPUCost) }}</b></div>
        <div><span>总消耗</span><b>{{ fmt2(nodeTotalCost) }}</b></div>
      </div>
      <div class="drawer-actions" v-if="canOpenProfile"><el-button plain @click="openSelectedProfile">查看账号资料</el-button></div>
      <div class="table-wrap">
        <el-table :data="nodeRows" stripe table-layout="auto" v-loading="nodeLoading" empty-text="该区间无节点使用记录">
          <el-table-column prop="node_id" label="节点" min-width="90" />
          <el-table-column prop="gpu_model" label="GPU" min-width="160" />
          <el-table-column prop="gpu_count" label="卡数" width="68" />
          <el-table-column label="CPU积分" min-width="96"><template #default="{ row }">{{ fmt2(row.cpu_cost) }}</template></el-table-column>
          <el-table-column label="GPU积分" min-width="96"><template #default="{ row }">{{ fmt2(row.gpu_cost) }}</template></el-table-column>
          <el-table-column label="总消耗" min-width="96"><template #default="{ row }">{{ fmt2(row.total_cost) }}</template></el-table-column>
          <el-table-column prop="last_usage_at" label="最后使用" min-width="176" :formatter="tableTimeFormatter" />
        </el-table>
      </div>
    </el-drawer>

    <el-drawer v-if="authState.role === 'admin'" v-model="retentionVisible" title="数据留存与删除" size="760px" destroy-on-close class="glass-drawer">
      <div v-loading="retentionLoading" class="retention-drawer-content">
        <el-form inline>
          <el-form-item label="自动删除保留天数"><el-input-number v-model="retentionDaysDraft" :min="0" :max="3650" :step="1" :precision="0" /></el-form-item>
          <el-form-item><el-button type="primary" :loading="retentionSaving" @click="saveRetentionDays">保存设置</el-button></el-form-item>
          <el-form-item><span class="retention-tip">0 表示关闭自动删除。</span></el-form-item>
        </el-form>
        <div class="retention-meta">
          <span>上次删除：{{ retentionStatus?.last_deleted_at ? tableTimeFormatter(null, null, retentionStatus.last_deleted_at) : '暂无' }}</span>
          <span>模式：{{ retentionModeText(retentionStatus?.last_deleted_mode) }}</span>
          <span>删除 {{ Number(retentionStatus?.last_deleted_records || 0) }} 条</span>
        </div>
        <el-form inline class="delete-form">
          <el-form-item label="立即删除区间">
            <el-date-picker v-model="deleteRangeDays" type="daterange" value-format="YYYY-MM-DD" range-separator="至" start-placeholder="开始日期" end-placeholder="结束日期" :disabled-date="usageDeleteDisabledDate" @change="onDeleteRangeChange" />
          </el-form-item>
          <el-form-item><el-button :loading="deleteRangeEstimating" :disabled="!canDeleteRangeAction" @click="estimateDeleteRange">估算大小</el-button></el-form-item>
          <el-form-item><el-button type="danger" plain :loading="deleteRangeDeleting" :disabled="!canDeleteRangeAction" @click="deleteRangeNow">立即删除</el-button></el-form-item>
        </el-form>
        <el-alert v-if="availableUsageDaysLoaded" type="info" :closable="false" show-icon :title="`有记录日期 ${availableUsageDays.length} 天；无记录日期不可选`" />
        <el-alert v-if="deleteRangeEstimate" class="estimate-alert" type="warning" :closable="false" show-icon :title="`删除估算：${deleteRangeEstimate.records} 条，CSV约 ${bytesText(deleteRangeEstimate.estimated_csv_bytes)}，数据库约 ${bytesText(deleteRangeEstimate.estimated_db_bytes)}`" />
      </div>
    </el-drawer>

    <PlatformUserDetailDialog v-model="profileVisible" :username="selectedProfileUsername" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Coin, DataBoard, Delete, Download, Refresh } from "@element-plus/icons-vue";
import type {
  PlatformUsageNodeDetail,
  PlatformUsageUserSummary,
  RechargeSummary,
  UsageDailyOverview,
  UsageDayStat,
  UsageMonthlySummary,
  UsageRetentionStatus,
} from "../../lib/api";
import { ApiClient } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { formatServerDate, formatServerDateTime, getServerTodayDateText, normalizeServerDateInput, shiftServerDateText } from "../../lib/time";

type TrendMetric = "cost" | "active";
type ActivityFilter = "all" | "active" | "inactive";

const today = getServerTodayDateText();
const initialRange: [string, string] = [shiftServerDateText(today, -29), today];
const draftRange = ref<[string, string]>([...initialRange]);
const appliedRange = ref<[string, string]>([...initialRange]);
const loading = ref(false);
const exporting = ref(false);
const error = ref("");
const lastSyncedAt = ref("");
let rangeLoadSeq = 0;

const userRows = ref<PlatformUsageUserSummary[]>([]);
const dailyRows = ref<UsageDailyOverview[]>([]);
const rechargeRows = ref<RechargeSummary[]>([]);
const trendMetric = ref<TrendMetric>("cost");
const userKeyword = ref("");
const userActivityFilter = ref<ActivityFilter>("all");
const userPage = ref(1);
const userPageSize = ref(20);
const pointsKeyword = ref("");

const monthlyMonth = ref(previousCompleteMonth(today));
const monthlyRows = ref<UsageMonthlySummary[]>([]);
const monthlyLoading = ref(false);
const monthlyKeyword = ref("");
const monthlyUsageFilter = ref<ActivityFilter>("all");
const monthlyPage = ref(1);
const monthlyPageSize = ref(20);
let monthlyLoadSeq = 0;

const userDrawerVisible = ref(false);
const activeUsername = ref("");
const drawerRange = ref<[string, string]>([...initialRange]);
const nodeRows = ref<PlatformUsageNodeDetail[]>([]);
const nodeLoading = ref(false);
let nodeLoadSeq = 0;
const profileVisible = ref(false);
const selectedProfileUsername = ref("");

const retentionVisible = ref(false);
const retentionLoading = ref(false);
const retentionDaysDraft = ref(0);
const retentionStatus = ref<UsageRetentionStatus | null>(null);
const retentionSaving = ref(false);
const deleteRangeDays = ref<string[]>([]);
const deleteRangeEstimating = ref(false);
const deleteRangeDeleting = ref(false);
const deleteRangeEstimate = ref<{ records: number; estimated_csv_bytes: number; estimated_db_bytes: number } | null>(null);
const availableUsageDays = ref<UsageDayStat[]>([]);
const availableUsageDaysLoaded = ref(false);

const quickRangeOptions = [
  { label: "7 天", days: 7 },
  { label: "30 天", days: 30 },
  { label: "90 天", days: 90 },
  { label: "1 年", days: 365 },
];

const rangeDirty = computed(() => draftRange.value[0] !== appliedRange.value[0] || draftRange.value[1] !== appliedRange.value[1]);
const activeUserCount = computed(() => userRows.value.filter((row) => Number(row.usage_records || 0) > 0).length);
const totalCPUSeconds = computed(() => userRows.value.reduce((sum, row) => sum + Number(row.cpu_process_seconds ?? row.cpu_usage_seconds ?? 0), 0));
const totalGPUSeconds = computed(() => userRows.value.reduce((sum, row) => sum + Number(row.gpu_process_seconds ?? row.gpu_usage_seconds ?? 0), 0));
const totalCost = computed(() => userRows.value.reduce((sum, row) => sum + Number(row.total_cost || 0), 0));
const totalIncrease = computed(() => rechargeRows.value.reduce((sum, row) => sum + Number(row.increase_total || 0), 0));
const totalDecrease = computed(() => rechargeRows.value.reduce((sum, row) => sum + Number(row.decrease_total || 0), 0));
const totalNetChange = computed(() => rechargeRows.value.reduce((sum, row) => sum + Number(row.net_change || 0), 0));
const totalIncreaseCount = computed(() => rechargeRows.value.reduce((sum, row) => sum + Number(row.increase_count || 0), 0));
const totalDecreaseCount = computed(() => rechargeRows.value.reduce((sum, row) => sum + Number(row.decrease_count || 0), 0));
const canOpenProfile = computed(() => authState.role === "admin" || authState.canManagePlatformUsers || authState.canPointsUsers);

const filteredUserRows = computed(() => {
  const keyword = userKeyword.value.trim().toLowerCase();
  return userRows.value.filter((row) => {
    const active = Number(row.usage_records || 0) > 0;
    if (userActivityFilter.value === "active" && !active) return false;
    if (userActivityFilter.value === "inactive" && active) return false;
    return !keyword || row.platform_username.toLowerCase().includes(keyword);
  });
});
const pagedUserRows = computed(() => paginate(filteredUserRows.value, userPage.value, userPageSize.value));
const filteredRechargeRows = computed(() => {
  const keyword = pointsKeyword.value.trim().toLowerCase();
  return rechargeRows.value.filter((row) => !keyword || row.username.toLowerCase().includes(keyword));
});
const filteredMonthlyRows = computed(() => {
  const keyword = monthlyKeyword.value.trim().toLowerCase();
  return monthlyRows.value.filter((row) => {
    const active = Number(row.usage_records || 0) > 0;
    if (monthlyUsageFilter.value === "active" && !active) return false;
    if (monthlyUsageFilter.value === "inactive" && active) return false;
    return !keyword || row.username.toLowerCase().includes(keyword);
  });
});
const pagedMonthlyRows = computed(() => paginate(filteredMonthlyRows.value, monthlyPage.value, monthlyPageSize.value));
const monthlyActiveCount = computed(() => monthlyRows.value.filter((row) => Number(row.usage_records || 0) > 0).length);
const monthlyTotalCost = computed(() => monthlyRows.value.reduce((sum, row) => sum + Number(row.total_cost || 0), 0));
const nodeCPUCost = computed(() => nodeRows.value.reduce((sum, row) => sum + Number(row.cpu_cost || 0), 0));
const nodeGPUCost = computed(() => nodeRows.value.reduce((sum, row) => sum + Number(row.gpu_cost || 0), 0));
const nodeTotalCost = computed(() => nodeRows.value.reduce((sum, row) => sum + Number(row.total_cost || 0), 0));

const trendValues = computed(() => dailyRows.value.map((row) => trendMetric.value === "cost" ? Number(row.total_cost || 0) : Number(row.active_users || 0)));
const trendMax = computed(() => Math.max(0, ...trendValues.value));
const trendTotal = computed(() => trendValues.value.reduce((sum, value) => sum + value, 0));
const trendAverage = computed(() => dailyRows.value.length ? trendTotal.value / dailyRows.value.length : 0);
const trendTotalText = computed(() => trendMetric.value === "cost" ? fmt2(trendTotal.value) : `${Math.round(trendTotal.value)} 人次`);
const trendAverageText = computed(() => trendMetric.value === "cost" ? fmt2(trendAverage.value) : `${trendAverage.value.toFixed(1)} 人`);
const trendPeakText = computed(() => trendMetric.value === "cost" ? fmt2(trendMax.value) : `${Math.round(trendMax.value)} 人`);

const availableUsageDaySet = computed(() => new Set(availableUsageDays.value.map((row) => String(row.date || "").trim()).filter(Boolean)));
const canDeleteRangeAction = computed(() => authState.role === "admin" && deleteRangeDays.value.length === 2 && !!deleteRangeDays.value[0] && !!deleteRangeDays.value[1]);

function apiClient(): ApiClient { return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken }); }
function fmt2(value: number): string { return Number(value || 0).toFixed(2); }
function fmtDuration(seconds: number): string {
  const value = Math.max(0, Math.round(Number(seconds || 0)));
  if (value < 60) return `${value}秒`;
  const hours = Math.floor(value / 3600);
  const minutes = Math.floor((value % 3600) / 60);
  return hours ? `${hours}小时${minutes}分钟` : `${minutes}分钟`;
}
function fmtCompactDuration(seconds: number): string {
  const hours = Math.max(0, Number(seconds || 0)) / 3600;
  if (hours < 1) return `${Math.round(hours * 60)} 分钟`;
  if (hours < 1000) return `${hours.toFixed(hours >= 100 ? 0 : 1)} 小时`;
  return `${(hours / 1000).toFixed(1)}k 小时`;
}
function signedPoints(value: number): string { const n = Number(value || 0); return `${n > 0 ? "+" : ""}${fmt2(n)}`; }
function percentText(value: number, total: number): string { return total ? `${Math.round(Number(value || 0) / total * 100)}%` : "0%"; }
function donutStyle(value: number, total: number): Record<string, string> {
  const percent = total ? Math.min(100, Math.max(0, Number(value || 0) / total * 100)) : 0;
  return { background: `conic-gradient(#10b981 ${percent}%, #e6edf5 ${percent}% 100%)` };
}
function paginate<T>(rows: T[], page: number, size: number): T[] { const start = (Math.max(1, page) - 1) * size; return rows.slice(start, start + size); }
function tableTimeFormatter(_: unknown, __: unknown, value: unknown): string { return formatServerDateTime(String(value ?? "")); }
function disableFutureDate(date: Date): boolean { return formatServerDate(date) > getServerTodayDateText(); }
function normalizeDraftRange(): void {
  const safe = normalizeRange(draftRange.value);
  draftRange.value = safe;
}
function normalizeRange(value: unknown): [string, string] {
  const raw = Array.isArray(value) ? value : [];
  let from = normalizeServerDateInput(raw[0], appliedRange.value[0]);
  let to = normalizeServerDateInput(raw[1], appliedRange.value[1]);
  const currentToday = getServerTodayDateText();
  if (from > currentToday) from = currentToday;
  if (to > currentToday) to = currentToday;
  if (from > to) [from, to] = [to, from];
  return [from, to];
}
async function applyDraftRange(): Promise<void> {
  const safe = normalizeRange(draftRange.value);
  draftRange.value = [...safe];
  appliedRange.value = [...safe];
  if (userDrawerVisible.value) userDrawerVisible.value = false;
  await loadRangeData();
}
async function applyQuickRange(days: number): Promise<void> {
  const currentToday = getServerTodayDateText();
  const next: [string, string] = [shiftServerDateText(currentToday, -(Math.max(1, days) - 1)), currentToday];
  draftRange.value = [...next];
  appliedRange.value = [...next];
  if (userDrawerVisible.value) userDrawerVisible.value = false;
  await loadRangeData();
}
async function loadRangeData(): Promise<boolean> {
  const seq = ++rangeLoadSeq;
  const [from, to] = appliedRange.value;
  loading.value = true;
  error.value = "";
  try {
    const client = apiClient();
    const [users, daily, changes] = await Promise.all([
      client.adminStatsPlatformUsers({ from, to, limit: 10000 }),
      client.adminStatsDaily({ from, to }),
      client.adminStatsRecharges({ from, to, limit: 10000 }),
    ]);
    if (seq !== rangeLoadSeq) return false;
    userRows.value = users.rows ?? [];
    dailyRows.value = daily.rows ?? [];
    rechargeRows.value = changes.rows ?? [];
    userPage.value = 1;
    return true;
  } catch (e: any) {
    if (seq === rangeLoadSeq) error.value = e?.message ?? String(e);
    return false;
  } finally {
    if (seq === rangeLoadSeq) loading.value = false;
  }
}
async function loadDashboard(includeMonthly: boolean): Promise<void> {
  const drawerUser = userDrawerVisible.value ? activeUsername.value : "";
  const [rangeOK, monthlyOK] = await Promise.all([loadRangeData(), includeMonthly ? loadMonthlyReport() : Promise.resolve(true)]);
  if (rangeOK && monthlyOK) {
    lastSyncedAt.value = new Date().toLocaleTimeString("zh-CN", { hour12: false });
    if (drawerUser && userDrawerVisible.value) await openUserDrawer(drawerUser);
  }
}

function previousCompleteMonth(dateText: string): string {
  const [year, month] = dateText.split("-").map(Number);
  const value = new Date(Date.UTC(year, month - 2, 1));
  return `${value.getUTCFullYear()}-${String(value.getUTCMonth() + 1).padStart(2, "0")}`;
}
function monthBounds(monthText: string): [string, string] {
  const [year, month] = monthText.split("-").map(Number);
  const lastDay = new Date(Date.UTC(year, month, 0)).getUTCDate();
  return [`${monthText}-01`, `${monthText}-${String(lastDay).padStart(2, "0")}`];
}
function disableIncompleteMonth(date: Date): boolean { return formatServerDate(date).slice(0, 7) >= getServerTodayDateText().slice(0, 7); }
async function loadMonthlyReport(): Promise<boolean> {
  const month = String(monthlyMonth.value || previousCompleteMonth(getServerTodayDateText()));
  if (month >= getServerTodayDateText().slice(0, 7)) monthlyMonth.value = previousCompleteMonth(getServerTodayDateText());
  const [from, to] = monthBounds(monthlyMonth.value);
  const seq = ++monthlyLoadSeq;
  monthlyLoading.value = true;
  error.value = "";
  try {
    const response = await apiClient().adminStatsMonthly({ from, to, limit: 20000, offset: 0 });
    if (seq !== monthlyLoadSeq) return false;
    monthlyRows.value = response.rows ?? [];
    monthlyPage.value = 1;
    return true;
  } catch (e: any) {
    if (seq === monthlyLoadSeq) error.value = e?.message ?? String(e);
    return false;
  } finally {
    if (seq === monthlyLoadSeq) monthlyLoading.value = false;
  }
}

function trendBarHeight(row: UsageDailyOverview): number {
  const value = trendMetric.value === "cost" ? Number(row.total_cost || 0) : Number(row.active_users || 0);
  if (!value || !trendMax.value) return 0;
  return Math.max(3, value / trendMax.value * 100);
}
function trendLabelVisible(index: number): boolean { return index === 0 || index === dailyRows.value.length - 1 || index % Math.max(1, Math.ceil(dailyRows.value.length / 7)) === 0; }
function trendTooltip(row: UsageDailyOverview): string { return `${row.date} · 活跃 ${row.active_users} 人 · 消耗 ${fmt2(row.total_cost)} 积分`; }

async function openUserDrawer(username: string): Promise<void> {
  const value = String(username || "").trim();
  if (!value) return;
  const seq = ++nodeLoadSeq;
  activeUsername.value = value;
  drawerRange.value = [...appliedRange.value];
  nodeRows.value = [];
  userDrawerVisible.value = true;
  nodeLoading.value = true;
  try {
    const [from, to] = drawerRange.value;
    const response = await apiClient().adminStatsPlatformUserNodes(value, { from, to, limit: 2000 });
    if (seq !== nodeLoadSeq || activeUsername.value !== value) return;
    nodeRows.value = response.rows ?? [];
  } catch (e: any) {
    if (seq === nodeLoadSeq) error.value = e?.message ?? String(e);
  } finally {
    if (seq === nodeLoadSeq) nodeLoading.value = false;
  }
}
function closeUserDrawer(): void { nodeLoadSeq += 1; activeUsername.value = ""; nodeRows.value = []; nodeLoading.value = false; }
function openSelectedProfile(): void { if (!activeUsername.value || !canOpenProfile.value) return; selectedProfileUsername.value = activeUsername.value; profileVisible.value = true; }

async function exportRangeCSV(): Promise<void> {
  if (rangeDirty.value) { ElMessage.warning("请先应用日期区间"); return; }
  const [from, to] = appliedRange.value;
  exporting.value = true;
  error.value = "";
  try {
    const blob = await apiClient().adminExportUsageCSV({ from, to, limit: 200000 });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `board_usage_${from}_${to}.csv`;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(url);
    ElMessage.success(`已开始下载 board_usage_${from}_${to}.csv`);
  } catch (e: any) { error.value = e?.message ?? String(e); }
  finally { exporting.value = false; }
}

function bytesText(value: number): string {
  const size = Number(value || 0);
  if (size < 1024) return `${size} B`;
  if (size < 1024 ** 2) return `${(size / 1024).toFixed(2)} KB`;
  if (size < 1024 ** 3) return `${(size / 1024 ** 2).toFixed(2)} MB`;
  return `${(size / 1024 ** 3).toFixed(2)} GB`;
}
function retentionModeText(value?: string): string { return value === "auto" ? "自动" : value === "manual" ? "手动" : "-"; }
function applyRetentionStatus(response: UsageRetentionStatus): void {
  retentionStatus.value = response;
  retentionDaysDraft.value = Math.min(3650, Math.max(0, Math.round(Number(response?.retention_days || 0))));
}
async function refreshRetentionStatus(client = apiClient()): Promise<void> { if (authState.role === "admin") applyRetentionStatus(await client.adminUsageRetentionGet()); }
async function refreshUsageDayStats(client = apiClient(), force = false): Promise<void> {
  if (authState.role !== "admin" || (availableUsageDaysLoaded.value && !force)) return;
  const response = await client.adminUsageDays({});
  availableUsageDays.value = response.days ?? [];
  availableUsageDaysLoaded.value = true;
}
async function openRetentionTools(): Promise<void> {
  if (authState.role !== "admin") return;
  retentionVisible.value = true;
  retentionLoading.value = true;
  try { const client = apiClient(); await Promise.all([refreshRetentionStatus(client), refreshUsageDayStats(client)]); }
  catch (e: any) { error.value = e?.message ?? String(e); }
  finally { retentionLoading.value = false; }
}
async function saveRetentionDays(): Promise<void> {
  retentionSaving.value = true;
  try {
    const days = Math.min(3650, Math.max(0, Math.round(Number(retentionDaysDraft.value || 0))));
    applyRetentionStatus(await apiClient().adminUsageRetentionSet({ retention_days: days }));
    ElMessage.success(days ? `已设置保留 ${days} 天` : "已关闭自动删除");
  } catch (e: any) { error.value = e?.message ?? String(e); }
  finally { retentionSaving.value = false; }
}
function getDeleteRangeSafe(): [string, string] {
  if (deleteRangeDays.value.length !== 2) throw new Error("请先选择完整删除区间");
  const safe = normalizeRange(deleteRangeDays.value);
  deleteRangeDays.value = [...safe];
  return safe;
}
function usageDeleteDisabledDate(date: Date): boolean { return disableFutureDate(date) || (availableUsageDaysLoaded.value && !availableUsageDaySet.value.has(formatServerDate(date))); }
function onDeleteRangeChange(): void { deleteRangeEstimate.value = null; }
async function estimateDeleteRange(): Promise<void> {
  if (!canDeleteRangeAction.value) return;
  deleteRangeEstimating.value = true;
  try {
    const [from, to] = getDeleteRangeSafe();
    const response = await apiClient().adminUsageRangeEstimate({ from, to });
    deleteRangeEstimate.value = { records: Number(response.records || 0), estimated_csv_bytes: Number(response.estimated_csv_bytes || 0), estimated_db_bytes: Number(response.estimated_db_bytes || 0) };
  } catch (e: any) { error.value = e?.message ?? String(e); }
  finally { deleteRangeEstimating.value = false; }
}
async function deleteRangeNow(): Promise<void> {
  if (!canDeleteRangeAction.value) return;
  deleteRangeDeleting.value = true;
  try {
    const [from, to] = getDeleteRangeSafe();
    await ElMessageBox.confirm(`将删除 ${from} 至 ${to} 的全部使用记录，此操作不可恢复。`, "确认删除", { type: "warning", confirmButtonText: "确认删除", cancelButtonText: "取消" });
    const client = apiClient();
    const response = await client.adminUsageDeleteRange({ from, to, confirm: true });
    ElMessage.success(`删除完成：${response.deleted_records} 条记录`);
    await Promise.all([refreshRetentionStatus(client), refreshUsageDayStats(client, true), loadDashboard(true)]);
  } catch (e: any) { if (String(e?.message || "") !== "cancel") error.value = e?.message ?? String(e); }
  finally { deleteRangeDeleting.value = false; }
}

watch([userKeyword, userActivityFilter, userPageSize], () => { userPage.value = 1; });
watch([monthlyKeyword, monthlyUsageFilter, monthlyPageSize], () => { monthlyPage.value = 1; });
onMounted(() => { void loadDashboard(true); });
</script>

<style scoped>
.board-wrap { width: 100%; min-width: 0; display: flex; flex-direction: column; gap: 14px; color: #102033; }
.board-alert { margin: 0; }
.range-panel { display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: 12px 18px; padding: 12px 14px; border: 1px solid rgba(148,163,184,.24); border-radius: 16px; background: rgba(255,255,255,.7); box-shadow: 0 9px 24px rgba(45,71,112,.06); }
.range-picker-wrap, .quick-ranges, .applied-range { display: flex; align-items: center; gap: 8px; }
.range-label, .applied-range span { color: #64748b; font-size: 12px; font-weight: 700; white-space: nowrap; }
.quick-ranges { justify-content: flex-start; }
.applied-range { justify-content: flex-end; color: #475569; font-size: 11px; }
.applied-range b { white-space: nowrap; }
.board-metrics { gap: 10px; }
.metric-compact { min-height: 94px; padding: 14px 16px; }
.metric-compact::after { width: 76px; height: 76px; }
.metric-compact > span { font-size: 11px; }
.metric-compact > strong { margin-top: 8px; font-size: 23px; }
.metric-compact > small { margin-top: 7px; font-size: 10px; }
.overview-grid { display: grid; grid-template-columns: minmax(0, 2fr) minmax(260px, .72fr); gap: 14px; }
.panel { min-width: 0; overflow: hidden; padding: 16px; border: 1px solid rgba(148,163,184,.24); border-radius: 18px; background: linear-gradient(145deg, rgba(255,255,255,.82), rgba(247,250,252,.62)); box-shadow: 0 12px 30px rgba(45,71,112,.07); }
.panel-head { display: flex; align-items: center; justify-content: space-between; gap: 14px; margin-bottom: 14px; }
.panel-head h2 { margin: 3px 0 0; color: #17283c; font-size: 17px; }
.section-eyebrow { color: #3b82f6; font-size: 9px; font-weight: 800; letter-spacing: .13em; }
.panel-icon { color: #f59e0b; font-size: 24px; }
.trend-summary { display: flex; gap: 22px; margin-bottom: 10px; color: #7b8999; font-size: 10px; }
.trend-summary b { color: #334155; font-size: 12px; }
.trend-scroll { min-height: 176px; overflow-x: auto; }
.trend-chart { display: flex; align-items: flex-end; height: 176px; gap: 3px; padding: 4px 2px 0; }
.trend-column { display: flex; min-width: 14px; height: 100%; flex: 1 1 0; flex-direction: column; align-items: center; justify-content: flex-end; }
.trend-bar-track { position: relative; width: 70%; min-width: 7px; height: 145px; overflow: hidden; border-radius: 5px 5px 2px 2px; background: rgba(226,232,240,.56); }
.trend-bar-track i { position: absolute; inset: auto 0 0; border-radius: 5px 5px 2px 2px; background: linear-gradient(180deg, #60a5fa, #2563eb); transition: height .25s ease; }
.trend-column > span { height: 18px; margin-top: 5px; color: #94a3b8; font-size: 8px; white-space: nowrap; }
.points-overview { display: flex; flex-direction: column; }
.flow-item { display: grid; grid-template-columns: 1fr auto; align-items: baseline; padding: 10px 0; border-bottom: 1px solid rgba(148,163,184,.14); }
.flow-item > span { color: #64748b; font-size: 11px; font-weight: 700; }
.flow-item strong { font-size: 19px; }
.flow-item small { grid-column: 1 / -1; margin-top: 2px; color: #94a3b8; font-size: 9px; }
.flow-item.increase strong, .points-positive { color: #059669; }
.flow-item.decrease strong, .points-negative { color: #dc2626; }
.flow-net { display: flex; align-items: center; justify-content: space-between; padding: 11px 0; color: #64748b; font-size: 11px; }
.flow-net b { color: #0f766e; font-size: 16px; }
.flow-net b.negative { color: #dc2626; }
.activity-ring-row { display: flex; align-items: center; gap: 12px; margin-top: auto; padding-top: 10px; }
.activity-ring { position: relative; display: grid; place-items: center; width: 54px; height: 54px; flex: 0 0 54px; border-radius: 50%; }
.activity-ring::after { content: ""; position: absolute; inset: 7px; border-radius: 50%; background: #fff; }
.activity-ring span { position: relative; z-index: 1; color: #475569; font-size: 9px; font-weight: 800; }
.activity-ring-row b, .activity-ring-row small { display: block; }
.activity-ring-row b { color: #334155; font-size: 11px; }
.activity-ring-row small { margin-top: 3px; color: #94a3b8; font-size: 9px; }
.users-panel, .changes-panel, .monthly-panel { padding: 0; }
.users-panel > .panel-head, .changes-panel > .panel-head, .monthly-panel > .panel-head { padding: 15px 16px 0; }
.users-head { align-items: flex-end; }
.table-tools { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
.keyword-input { width: 190px; }
.activity-select { width: 116px; }
.table-wrap { width: 100%; min-width: 0; overflow-x: auto; }
.table-pagination { display: flex; justify-content: flex-end; padding: 10px 14px 14px; }
.user-link { font-weight: 700; }
.cell-note { display: block; margin-top: 2px; color: #94a3b8; font-size: 9px; }
.monthly-summary { display: flex; gap: 20px; padding: 0 16px 12px; color: #7b8999; font-size: 10px; }
.monthly-summary b { color: #334155; font-size: 12px; }
.drawer-range { margin-bottom: 12px; color: #64748b; font-size: 12px; }
.drawer-metrics { display: grid; grid-template-columns: repeat(4, minmax(0,1fr)); gap: 8px; margin-bottom: 12px; }
.drawer-metrics div { padding: 10px 12px; border-radius: 11px; background: #f4f7fb; }
.drawer-metrics span, .drawer-metrics b { display: block; }
.drawer-metrics span { color: #94a3b8; font-size: 9px; }
.drawer-metrics b { margin-top: 4px; color: #334155; font-size: 16px; }
.drawer-actions { display: flex; justify-content: flex-end; margin-bottom: 8px; }
.retention-tip { color: #64748b; font-size: 12px; }
.retention-meta { display: flex; flex-wrap: wrap; gap: 10px 18px; margin-bottom: 10px; color: #64748b; font-size: 12px; }
.delete-form { margin-top: 12px; }
.estimate-alert { margin-top: 10px; }

@media (max-width: 1100px) {
  .range-panel { grid-template-columns: 1fr; }
  .applied-range { justify-content: flex-start; }
  .overview-grid { grid-template-columns: 1fr; }
}
@media (max-width: 760px) {
  .range-picker-wrap { align-items: stretch; flex-direction: column; }
  .range-picker-wrap :deep(.el-date-editor) { width: 100%; }
  .users-head { align-items: flex-start; flex-direction: column; }
  .table-tools { justify-content: flex-start; width: 100%; }
  .keyword-input { width: min(100%, 240px); }
  .drawer-metrics { grid-template-columns: repeat(2, minmax(0,1fr)); }
}
</style>
