<template>
  <el-card class="ha-page">
    <template #header>
      <div class="head">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><Connection /></el-icon></span>
          <span>容灾同步</span>
        </div>
        <div class="head-actions">
          <el-text type="info" size="small">上次刷新：{{ lastRefreshText }}</el-text>
          <el-button :loading="loading" type="primary" @click="reload(true)">刷新</el-button>
        </div>
      </div>
    </template>
    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert
      v-if="status?.note"
      :title="status.note"
      type="info"
      show-icon
      class="mb"
    />

    <div v-if="status" class="top-cards">
      <el-card class="mini">
        <div class="k">本机</div>
        <div class="v">{{ status.local.node || "未命名" }} / {{ status.local.role || "-" }}</div>
        <div class="s">{{ status.local.listen_addr }}</div>
      </el-card>
      <el-card class="mini">
        <div class="k">对端</div>
        <div class="v">{{ peerLabel }}</div>
        <div class="s">{{ status.peer_url || "-" }}</div>
      </el-card>
      <el-card class="mini">
        <div class="k">同步状态</div>
        <div class="v">
          <el-tag :type="status.in_sync ? 'success' : 'warning'">{{ status.in_sync ? "已同步" : "存在差异" }}</el-tag>
        </div>
        <div class="s">检查时间：{{ status.checked || "-" }}</div>
      </el-card>
    </div>

    <el-divider>本机摘要</el-divider>
    <el-descriptions v-if="status" :column="3" border>
      <el-descriptions-item label="平台用户">{{ status.local.summary.user_accounts_count }}</el-descriptions-item>
      <el-descriptions-item label="账号映射">{{ status.local.summary.node_accounts_count }}</el-descriptions-item>
      <el-descriptions-item label="待审核注册">{{ status.local.summary.pending_requests_count }}</el-descriptions-item>
      <el-descriptions-item label="白名单">{{ status.local.summary.whitelist_count }}</el-descriptions-item>
      <el-descriptions-item label="黑名单">{{ status.local.summary.blacklist_count }}</el-descriptions-item>
      <el-descriptions-item label="豁免名单">{{ status.local.summary.exemptions_count }}</el-descriptions-item>
      <el-descriptions-item label="使用记录总数">{{ status.local.summary.usage_records_count }}</el-descriptions-item>
      <el-descriptions-item label="最近使用记录">{{ status.local.summary.latest_usage_at || "-" }}</el-descriptions-item>
      <el-descriptions-item label="最近节点上报">{{ status.local.summary.latest_node_seen_at || "-" }}</el-descriptions-item>
      <el-descriptions-item label="摘要哈希" :span="3">
        <code>{{ status.local.summary.digest }}</code>
      </el-descriptions-item>
    </el-descriptions>

    <template v-if="status?.peer?.reachable && status.peer.status">
      <el-divider>对端摘要</el-divider>
      <el-descriptions :column="3" border>
        <el-descriptions-item label="节点">{{ status.peer.status.node || "-" }}</el-descriptions-item>
        <el-descriptions-item label="角色">{{ status.peer.status.role || "-" }}</el-descriptions-item>
        <el-descriptions-item label="地址">{{ status.peer.status.listen_addr || "-" }}</el-descriptions-item>
        <el-descriptions-item label="平台用户">{{ status.peer.status.summary.user_accounts_count }}</el-descriptions-item>
        <el-descriptions-item label="账号映射">{{ status.peer.status.summary.node_accounts_count }}</el-descriptions-item>
        <el-descriptions-item label="待审核注册">{{ status.peer.status.summary.pending_requests_count }}</el-descriptions-item>
        <el-descriptions-item label="白名单">{{ status.peer.status.summary.whitelist_count }}</el-descriptions-item>
        <el-descriptions-item label="黑名单">{{ status.peer.status.summary.blacklist_count }}</el-descriptions-item>
        <el-descriptions-item label="豁免名单">{{ status.peer.status.summary.exemptions_count }}</el-descriptions-item>
        <el-descriptions-item label="摘要哈希" :span="3">
          <code>{{ status.peer.status.summary.digest }}</code>
        </el-descriptions-item>
      </el-descriptions>
    </template>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { ApiClient, type HAStatusResp } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { Connection } from "@element-plus/icons-vue";

const loading = ref(false);
const error = ref("");
const status = ref<HAStatusResp | null>(null);
const lastRefreshAt = ref("");
const AUTO_REFRESH_MS = 5 * 60 * 1000;
let autoTimer: ReturnType<typeof setTimeout> | null = null;

const peerLabel = computed(() => {
  if (!status.value?.peer) return "未配置";
  if (!status.value.peer.reachable) return "不可达";
  return "可达";
});

const lastRefreshText = computed(() => {
  const v = String(lastRefreshAt.value || "").trim();
  if (!v) return "尚未刷新";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleString();
});

function stopAutoRefresh() {
  if (autoTimer) {
    clearTimeout(autoTimer);
    autoTimer = null;
  }
}

function scheduleAutoRefresh() {
  stopAutoRefresh();
  autoTimer = setTimeout(async () => {
    await reload(false);
    scheduleAutoRefresh();
  }, AUTO_REFRESH_MS);
}

async function reload(resetTimer = true) {
  if (loading.value) return;
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    status.value = await client.adminHAStatus();
    lastRefreshAt.value = new Date().toISOString();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
    if (resetTimer) {
      scheduleAutoRefresh();
    }
  }
}

onMounted(() => {
  reload(true);
});

onBeforeUnmount(() => {
  stopAutoRefresh();
});
</script>

<style scoped>
.head { display: flex; justify-content: space-between; align-items: center; }
.section-title-wrap { display: inline-flex; align-items: center; gap: 10px; }
.section-icon {
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #dbeafe;
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
}
.head-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.mb { margin-bottom: 12px; }
.top-cards {
  display: grid;
  grid-template-columns: repeat(3, minmax(220px, 1fr));
  gap: 12px;
  margin-bottom: 10px;
}
.mini .k { color: #64748b; font-size: 12px; }
.mini .v { font-size: 18px; font-weight: 700; margin-top: 4px; }
.mini .s { color: #64748b; margin-top: 6px; font-size: 12px; }
@media (max-width: 900px) {
  .top-cards { grid-template-columns: 1fr; }
}
</style>
