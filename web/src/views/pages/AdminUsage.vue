<template>
  <el-card>
    <template #header>
      <div class="row">
        <div>
          <div class="title">使用记录</div>
          <div class="sub">需要管理员登录：GET /api/admin/usage，GET /api/admin/usage/export.csv</div>
        </div>
        <div class="row">
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
          <el-button :loading="exporting" @click="exportCSV">导出 CSV</el-button>
        </div>
      </div>
    </template>

    <el-space direction="vertical" fill style="width: 100%">
      <el-alert v-if="error" :title="error" type="error" show-icon />

      <el-form inline>
        <el-form-item label="平台账号">
          <el-input v-model="billingUsername" placeholder="按系统账号查询" @keyup.enter="reload" />
        </el-form-item>
        <el-form-item label="机器本地账号">
          <el-input v-model="localUsername" placeholder="按机器账号查询" @keyup.enter="reload" />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="unregisteredOnly">仅看未注册偷跑</el-checkbox>
        </el-form-item>
        <el-form-item label="条数">
          <el-input-number v-model="limit" :min="1" :max="5000" />
        </el-form-item>
        <el-form-item label="导出From">
          <el-input v-model="from" placeholder="YYYY-MM-DD 或 RFC3339" />
        </el-form-item>
        <el-form-item label="导出To">
          <el-input v-model="to" placeholder="YYYY-MM-DD 或 RFC3339" />
        </el-form-item>
      </el-form>

      <el-table :data="records" stripe height="520" :row-class-name="rowClassName">
        <el-table-column prop="timestamp" label="时间" width="190" sortable />
        <el-table-column prop="node_id" label="节点" width="120" sortable />
        <el-table-column prop="local_username" label="机器本地账号" width="150" sortable />
        <el-table-column prop="billing_username" label="系统账号(平台)" width="170" sortable>
          <template #default="{ row }">
            <span :class="{ unregistered: row.registered === false }">
              {{ row.billing_username || row.username || "-" }}
              <span v-if="row.registered === false">（未注册）</span>
            </span>
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
        <el-table-column prop="gpu_usage" label="GPU明细(JSON)" />
      </el-table>

      <el-card v-if="unregisteredSummary.length > 0">
        <template #header><b>未注册偷跑汇总</b></template>
        <el-table :data="unregisteredSummary" stripe max-height="260">
          <el-table-column prop="billing_username" label="系统账号(平台)" width="180" />
          <el-table-column prop="local_username" label="机器本地账号" width="160" />
          <el-table-column prop="nodes" label="涉及节点" min-width="260" />
          <el-table-column prop="count" label="记录数" width="100" sortable />
        </el-table>
      </el-card>
    </el-space>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { ElMessage } from "element-plus";
import { ApiClient, type UsageRecord } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";

const loading = ref(false);
const exporting = ref(false);
const error = ref("");
const records = ref<UsageRecord[]>([]);

const billingUsername = ref("");
const localUsername = ref("");
const unregisteredOnly = ref(false);
const limit = ref(200);
const from = ref("");
const to = ref("");

function fmt2(v: number): string {
  return Number(v ?? 0).toFixed(2);
}

async function reload() {
  loading.value = true;
  error.value = "";
  records.value = [];
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminUsage({
      billingUsername: billingUsername.value,
      localUsername: localUsername.value,
      unregisteredOnly: unregisteredOnly.value,
      limit: limit.value,
    });
    records.value = (r.records ?? []).map((x) => ({
      ...x,
      local_username: x.local_username || "-",
      billing_username: x.billing_username || x.username,
    }));
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function exportCSV() {
  exporting.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const blob = await client.adminExportUsageCSV({
      username: billingUsername.value,
      from: from.value,
      to: to.value,
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

const unregisteredSummary = computed(() => {
  const m = new Map<string, { billing_username: string; local_username: string; nodes: Set<string>; count: number }>();
  for (const r of records.value) {
    if (r.registered !== false) continue;
    const b = r.billing_username || r.username || "-";
    const l = r.local_username || "-";
    const key = `${b}::${l}`;
    const item = m.get(key) || { billing_username: b, local_username: l, nodes: new Set<string>(), count: 0 };
    item.count += 1;
    if (r.node_id) item.nodes.add(r.node_id);
    m.set(key, item);
  }
  return Array.from(m.values()).map((x) => ({
    billing_username: x.billing_username,
    local_username: x.local_username,
    nodes: Array.from(x.nodes).sort().join(", "),
    count: x.count,
  }));
});

function rowClassName({ row }: { row: UsageRecord }) {
  return row.registered === false ? "unregistered-row" : "";
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
.title {
  font-weight: 700;
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
