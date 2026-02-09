<template>
  <div class="board-wrap">
    <el-card>
      <template #header>
        <div class="head">
          <span class="title">运营看板</span>
          <el-button type="primary" :loading="loading" @click="loadAll">刷新</el-button>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />

      <el-form inline>
        <el-form-item label="统计区间">
          <el-date-picker
            v-model="range"
            type="daterange"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            value-format="YYYY-MM-DD"
          />
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="board-card">
      <template #header><b>平台用户使用情况（区间汇总）</b></template>
      <el-table :data="userRows" stripe>
        <el-table-column label="平台用户" min-width="160">
          <template #default="{ row }">
            <el-button class="user-link" link @click="loadUserNodes(row.platform_username)">
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
    </el-card>

    <el-card class="board-card">
      <template #header><b>用户在各节点使用详情（点击上表用户名查看）</b></template>
      <div style="margin-bottom: 8px; color: #64748b">
        当前用户：{{ activeUsername || "未选择" }}
      </div>
      <el-table :data="nodeRows" stripe>
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
    </el-card>

    <el-card class="board-card">
      <template #header><b>每月所有用户使用情况</b></template>
      <el-table :data="monthlyRows" stripe>
        <el-table-column prop="month" label="月份" min-width="90" />
        <el-table-column prop="username" label="用户" min-width="130" />
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
    </el-card>

    <el-card class="board-card">
      <template #header><b>积分增加统计</b></template>
      <el-table :data="rechargeRows" stripe>
        <el-table-column prop="username" label="用户" min-width="160" />
        <el-table-column prop="recharge_count" label="加分次数" min-width="100" />
        <el-table-column label="加分总额" min-width="120">
          <template #default="{ row }">{{ fmt2(row.recharge_total) }}</template>
        </el-table-column>
        <el-table-column prop="last_recharge" label="最后加分时间" min-width="180" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import type { PlatformUsageNodeDetail, PlatformUsageUserSummary, RechargeSummary, UsageMonthlySummary } from "../../lib/api";
import { ApiClient } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";

const loading = ref(false);
const error = ref("");

const today = new Date();
const yearAgo = new Date();
yearAgo.setDate(today.getDate() - 365);
const range = ref<[string, string]>([fmtDate(yearAgo), fmtDate(today)]);

const userRows = ref<PlatformUsageUserSummary[]>([]);
const monthlyRows = ref<UsageMonthlySummary[]>([]);
const rechargeRows = ref<RechargeSummary[]>([]);
const nodeRows = ref<PlatformUsageNodeDetail[]>([]);
const activeUsername = ref("");

function fmtDate(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function fmt2(v: number): string {
  return Number(v ?? 0).toFixed(2);
}

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [from, to] = range.value;
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const [u, m, r] = await Promise.all([
      client.adminStatsPlatformUsers({ from, to, limit: 1000 }),
      client.adminStatsMonthly({ from, to, limit: 50000 }),
      client.adminStatsRecharges({ from, to, limit: 1000 }),
    ]);
    userRows.value = u.rows ?? [];
    monthlyRows.value = m.rows ?? [];
    rechargeRows.value = r.rows ?? [];
    if (userRows.value.length > 0) {
      await loadUserNodes(userRows.value[0].platform_username);
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
    const [from, to] = range.value;
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminStatsPlatformUserNodes(username, { from, to, limit: 2000 });
    nodeRows.value = r.rows ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

loadAll();
</script>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; }
.title { font-weight: 700; font-size: 16px; }
.mb { margin-bottom: 12px; }
.board-wrap {
  width: 100%;
  display: grid;
  gap: 12px;
}
.board-card {
  overflow: hidden;
}
.user-link {
  color: #0f766e;
  font-weight: 600;
}
</style>
