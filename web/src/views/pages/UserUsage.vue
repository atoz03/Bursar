<template>
  <div class="user-fun-page">
    <div class="user-fun-bg">
      <div class="user-fun-flow a" />
      <div class="user-fun-flow b" />
      <div class="user-fun-blob a" />
      <div class="user-fun-blob b" />
      <div class="user-fun-spark a" />
      <div class="user-fun-spark b" />
      <div class="user-fun-sticker left">账单明细</div>
      <div class="user-fun-sticker right">按需刷新</div>
    </div>
    <el-card class="user-fun-card usage-card">
      <template #header>
        <div class="row">
          <div>
            <h2 class="user-fun-head-title">我的用量</h2>
            <p class="user-fun-head-sub">仅展示当前登录平台账号的积分消耗记录</p>
          </div>
          <el-button :loading="loading" type="primary" @click="query">刷新</el-button>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon />
      <el-alert
        title="提示：如果你发现某台节点没有记录，请先在“节点账号”里补全映射。"
        type="info"
        :closable="false"
        show-icon
        class="mb"
      />

      <el-form inline>
        <el-form-item label="条数">
          <el-input-number v-model="limit" :min="1" :max="5000" />
        </el-form-item>
      </el-form>

      <el-table :data="records" stripe height="520">
        <el-table-column prop="timestamp" label="时间" width="190" />
        <el-table-column prop="node_id" label="节点" width="120" />
        <el-table-column prop="local_username" label="节点账号" width="160" />
        <el-table-column prop="billing_username" label="平台账号" width="160" />
        <el-table-column label="CPU%" width="90">
          <template #default="{ row }">{{ fmt2(row.cpu_percent) }}</template>
        </el-table-column>
        <el-table-column label="内存MB" width="110">
          <template #default="{ row }">{{ fmt2(row.memory_mb) }}</template>
        </el-table-column>
        <el-table-column label="积分消耗" width="100">
          <template #default="{ row }">{{ fmt2(row.cost) }}</template>
        </el-table-column>
        <el-table-column label="GPU明细(JSON)" min-width="360">
          <template #default="{ row, $index }">
            <div class="gpu-cell">
              <span class="gpu-text">{{ gpuDisplayText(row, $index) }}</span>
              <el-button
                v-if="hasLongGpuUsage(row)"
                link
                type="primary"
                class="gpu-toggle"
                @click="toggleGpuExpand(row, $index)"
              >
                {{ isGpuExpanded(row, $index) ? "收起" : "展开" }}
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { ApiClient, type UsageRecord } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";

const loading = ref(false);
const error = ref("");
const records = ref<UsageRecord[]>([]);
const limit = ref(200);
const gpuExpandedKeys = ref<Set<string>>(new Set());
const gpuPreviewChars = 80;

function fmt2(v: number): string {
  return Number(v ?? 0).toFixed(2);
}

function gpuUsageText(row: UsageRecord): string {
  const text = String(row.gpu_usage ?? "").trim();
  return text || "-";
}

function gpuRowKey(row: UsageRecord, index: number): string {
  return [
    String(row.timestamp ?? ""),
    String(row.node_id ?? ""),
    String(row.local_username ?? ""),
    String(row.billing_username ?? ""),
    String(row.pid ?? ""),
    String(index),
  ].join("|");
}

function hasLongGpuUsage(row: UsageRecord): boolean {
  return gpuUsageText(row).length > gpuPreviewChars;
}

function isGpuExpanded(row: UsageRecord, index: number): boolean {
  return gpuExpandedKeys.value.has(gpuRowKey(row, index));
}

function gpuDisplayText(row: UsageRecord, index: number): string {
  const text = gpuUsageText(row);
  if (!hasLongGpuUsage(row) || isGpuExpanded(row, index)) return text;
  return `${text.slice(0, gpuPreviewChars)}...`;
}

function toggleGpuExpand(row: UsageRecord, index: number) {
  const key = gpuRowKey(row, index);
  const next = new Set(gpuExpandedKeys.value);
  if (next.has(key)) {
    next.delete(key);
  } else {
    next.add(key);
  }
  gpuExpandedKeys.value = next;
}

async function query() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl);
    const r = await client.userMyUsage(limit.value);
    records.value = (r.records ?? []).map((x) => ({
      ...x,
      local_username: x.local_username || "-",
      billing_username: x.billing_username || x.username,
    }));
    gpuExpandedKeys.value = new Set();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

query();
</script>

<style scoped>
.row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.usage-card { min-height: 560px; }
.mb { margin-bottom: 12px; }
.gpu-cell {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.gpu-text {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
  color: #334155;
  line-height: 1.35;
  white-space: pre-wrap;
  word-break: break-all;
}
.gpu-toggle {
  padding: 0;
  height: auto;
  line-height: 1.2;
}

@media (max-width: 900px) {
  .row { flex-wrap: wrap; }
}
</style>
