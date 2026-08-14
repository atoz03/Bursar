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
          <el-text type="info" size="small">自动刷新：{{ autoRefreshText }}</el-text>
          <el-button :loading="loading" type="primary" @click="reload(true)">刷新</el-button>
        </div>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />

    <el-alert
      v-if="!loading"
      :type="disasterReady ? 'success' : 'error'"
      :title="disasterReady ? '数据保护链路已就绪' : '容灾未就绪，请勿执行回切或接管'"
      :description="readinessDescription"
      show-icon
      :closable="false"
      class="readiness-alert"
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
        <div class="k">同步摘要</div>
        <div class="v">
          <el-tag :type="status.in_sync ? 'success' : 'warning'">{{ status.in_sync ? "已同步" : "存在差异" }}</el-tag>
        </div>
        <div class="s">检查时间：{{ fmtTime(status.checked) }}</div>
      </el-card>
      <el-card class="mini">
        <div class="k">版本一致</div>
        <div class="v">
          <el-tag :type="versionTagType">{{ versionText }}</el-tag>
        </div>
        <div class="s">本机 {{ shortHash(status.local.app_binary_sha256) }} / 对端 {{ shortHash(status.peer?.status?.app_binary_sha256) }}</div>
      </el-card>
    </div>

    <el-divider>备份与恢复验证</el-divider>
    <div class="top-cards backup-cards">
      <el-card class="mini">
        <div class="k">加密备份</div>
        <div class="v"><el-tag :type="backupTagType">{{ backupStateText }}</el-tag></div>
        <div class="s">{{ backupStatus?.backup.message || "尚未安装备份任务" }}</div>
      </el-card>
      <el-card class="mini">
        <div class="k">最近成功备份</div>
        <div class="v compact">{{ fmtTime(backupStatus?.backup.last_success_at || backupStatus?.backup.finished_at) }}</div>
        <div class="s">快照 {{ backupStatus?.backup.last_snapshot_id || backupStatus?.backup.snapshot_id || "-" }}</div>
      </el-card>
      <el-card class="mini">
        <div class="k">隔离恢复演练</div>
        <div class="v"><el-tag :type="verifyTagType">{{ verifyStateText }}</el-tag></div>
        <div class="s">{{ backupStatus?.verification.message || "尚无恢复验证记录" }}</div>
      </el-card>
      <el-card class="mini">
        <div class="k">保护范围</div>
        <div class="v compact">平台数据库与控制器配置</div>
        <div class="s">保留策略：7 日 / 4 周 / 12 月</div>
      </el-card>
    </div>

    <el-divider>同步配置</el-divider>
    <el-form :model="syncConfig" label-width="160px" class="cfg-form">
      <el-row :gutter="16">
        <el-col :xs="24" :md="8">
          <el-form-item label="启用自动同步">
            <el-switch v-model="syncConfig.enabled" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item label="每隔几天同步">
            <el-input-number v-model="syncConfig.interval_days" :min="1" :max="30" :step="1" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item label="开始小时">
            <el-select v-model="syncConfig.start_hour" style="width: 100%">
              <el-option v-for="h in hourOptions" :key="h" :label="`${h}:00`" :value="h" />
            </el-select>
          </el-form-item>
        </el-col>
      </el-row>

      <el-row :gutter="16">
        <el-col :xs="24" :md="8">
          <el-form-item label="容灾节点主机">
            <el-input v-model="syncConfig.dr_host" placeholder="例如 192.0.2.20" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item label="容灾 SSH 用户">
            <el-input v-model="syncConfig.dr_ssh_user" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item label="容灾 SSH 端口">
            <el-input-number v-model="syncConfig.dr_ssh_port" :min="1" :max="65535" :step="1" />
          </el-form-item>
        </el-col>
      </el-row>

      <el-row :gutter="16">
        <el-col :xs="24" :md="8">
          <el-form-item label="容灾节点服务端口">
            <el-input-number v-model="syncConfig.dr_controller_port" :min="1" :max="65535" :step="1" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item label="主节点地址">
            <el-input v-model="syncConfig.primary_host" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item label="主节点服务端口">
            <el-input-number v-model="syncConfig.primary_controller_port" :min="1" :max="65535" :step="1" />
          </el-form-item>
        </el-col>
      </el-row>

      <el-row :gutter="16">
        <el-col :xs="24" :md="8">
          <el-form-item label="同步脚本路径">
            <el-input v-model="syncConfig.script_path" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item label="容灾密钥路径">
            <el-input v-model="syncConfig.dr_key_file" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item label="容灾节点 ID">
            <el-input v-model="syncConfig.dr_node_id" />
          </el-form-item>
        </el-col>
      </el-row>

      <el-row :gutter="16">
        <el-col :xs="24" :md="8">
          <el-form-item label="同步前端页面">
            <el-switch v-model="syncConfig.sync_web_dist" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item label="同步数据库">
            <el-switch v-model="syncConfig.sync_database" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item label="自动故障接管">
            <el-switch v-model="syncConfig.auto_failover" />
          </el-form-item>
        </el-col>
      </el-row>

      <div class="ops">
        <el-button type="primary" :loading="saving" @click="saveConfig">保存配置</el-button>
        <el-button
          type="success"
          :disabled="!syncActionAvailable"
          :loading="syncingDirection === 'primary_to_standby'"
          @click="syncNow('primary_to_standby')"
        >主→容灾 立即同步</el-button>
        <el-button
          type="warning"
          :disabled="!syncActionAvailable"
          :loading="syncingDirection === 'standby_to_primary'"
          @click="syncNow('standby_to_primary')"
        >容灾→主 回切同步</el-button>
        <el-button type="danger" :disabled="!failoverActionAvailable" :loading="failoverLoading" @click="activateFailover">备机手动接管</el-button>
      </div>
      <div class="meta">
        <el-text type="info" size="small">任务运行中：{{ syncRunning ? "是" : "否" }}</el-text>
        <el-text type="info" size="small">下次计划同步：{{ fmtTime(nextRunAt) }}</el-text>
        <el-text type="info" size="small">上次同步：{{ fmtTime(lastRun?.started_at) }}</el-text>
      </div>
    </el-form>

    <el-divider>同步日志</el-divider>
    <el-table :data="syncRuns" border stripe>
      <el-table-column prop="run_id" label="ID" width="80" />
      <el-table-column prop="trigger_mode" label="触发方式" width="120" />
      <el-table-column prop="direction" label="方向" width="170">
        <template #default="{ row }">
          {{ formatDirection(row.direction) }}
        </template>
      </el-table-column>
      <el-table-column prop="status" label="结果" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'success' ? 'success' : row.status === 'running' ? 'warning' : 'danger'">
            {{ row.status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="started_by" label="操作人" width="120" />
      <el-table-column prop="started_at" label="开始时间" min-width="170">
        <template #default="{ row }">{{ fmtTime(row.started_at) }}</template>
      </el-table-column>
      <el-table-column prop="finished_at" label="结束时间" min-width="170">
        <template #default="{ row }">{{ fmtTime(row.finished_at) }}</template>
      </el-table-column>
      <el-table-column prop="summary" label="摘要" min-width="220" />
      <el-table-column label="步骤详情" min-width="320">
        <template #default="{ row }">
          <div v-if="row.detail?.length" class="step-wrap">
            <div v-for="(it, idx) in row.detail" :key="`${row.run_id}-${idx}`" class="step-item">
              <el-tag size="small" :type="it.success ? 'success' : 'danger'">{{ it.success ? '成功' : '失败' }}</el-tag>
              <span class="step-name">{{ it.name }}</span>
              <span class="step-msg">{{ it.message }}</span>
            </div>
          </div>
          <span v-else>-</span>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ApiClient, type BackupStatusResp, type HAStatusResp, type HASyncConfig, type HASyncRun } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { formatServerDateTime } from "../../lib/time";
import { Connection } from "@element-plus/icons-vue";

const loading = ref(false);
const saving = ref(false);
const failoverLoading = ref(false);
const syncingDirection = ref<"" | "primary_to_standby" | "standby_to_primary">("");
const error = ref("");
const status = ref<HAStatusResp | null>(null);
const backupStatus = ref<BackupStatusResp | null>(null);
const syncConfig = ref<HASyncConfig>({
  enabled: false,
  interval_days: 1,
  start_hour: 3,
  dr_node_id: "",
  dr_host: "",
  dr_ssh_port: 22,
  dr_ssh_user: "",
  dr_key_file: "",
  dr_controller_port: 60039,
  primary_host: "127.0.0.1",
  primary_controller_port: 60039,
  script_path: "/opt/gpu-ops/scripts/ha_sync_worker.sh",
  sync_web_dist: true,
  sync_database: true,
  auto_failover: false,
});
const syncRuns = ref<HASyncRun[]>([]);
const syncRunning = ref(false);
const nextRunAt = ref("");
const lastRun = ref<HASyncRun | null>(null);
const lastRefreshAt = ref<number | null>(null);
const AUTO_REFRESH_MS = 10 * 1000;
const autoRefreshRemainSec = ref(Math.floor(AUTO_REFRESH_MS / 1000));
let autoTimer: ReturnType<typeof setTimeout> | null = null;
let autoTickTimer: ReturnType<typeof setInterval> | null = null;

const hourOptions = computed(() => Array.from({ length: 24 }, (_, i) => i));
const peerLabel = computed(() => {
  if (!status.value?.peer) return "未配置";
  if (!status.value.peer.reachable) return "不可达";
  return "可达";
});
const lastRefreshText = computed(() => {
  if (!lastRefreshAt.value) return "尚未刷新";
  return formatServerDateTime(lastRefreshAt.value);
});
const autoRefreshText = computed(() => `${autoRefreshRemainSec.value}s`);
const versionText = computed(() => {
  if (!status.value?.peer) return "未配置";
  if (!status.value.peer.reachable) return "对端不可达";
  return status.value.version_match ? "一致" : "不一致";
});
const versionTagType = computed(() => {
  if (!status.value?.peer) return "info";
  if (!status.value.peer.reachable) return "warning";
  return status.value.version_match ? "success" : "danger";
});
const hasSuccessfulSync = computed(() => syncRuns.value.some((run) => run.status === "success" && run.direction === "primary_to_standby"));
const syncActionAvailable = computed(() => {
  const host = String(syncConfig.value.dr_host || "").trim();
  return status.value?.local?.role === "primary" && !!host && !/[<>]/.test(host) && host !== "127.0.0.1";
});
const failoverActionAvailable = computed(() => status.value?.local?.role === "standby" && syncConfig.value.auto_failover);
const disasterReady = computed(() =>
  !!status.value?.peer?.reachable
  && !!status.value?.version_match
  && hasSuccessfulSync.value
  && !!backupStatus.value?.ready,
);
const readinessDescription = computed(() => {
  const issues: string[] = [];
  if (!status.value?.peer?.reachable) issues.push("容灾对端不可达或未配置");
  if (status.value?.peer?.reachable && !status.value?.version_match) issues.push("主备版本不一致");
  if (!hasSuccessfulSync.value) issues.push("没有成功的主到备同步记录");
  if (!backupStatus.value) issues.push("备份状态接口尚未部署");
  for (const check of backupStatus.value?.checks || []) {
    if (!check.ok) issues.push(check.message);
  }
  return issues.length ? issues.join("；") : "最近备份、恢复演练和主备状态均通过检查。";
});
const backupStateText = computed(() => statusText(backupStatus.value?.backup.state));
const verifyStateText = computed(() => statusText(backupStatus.value?.verification.state));
const backupTagType = computed(() => jobTagType(backupStatus.value?.backup.state));
const verifyTagType = computed(() => jobTagType(backupStatus.value?.verification.state));

function statusText(state?: string): string {
  if (state === "success") return "成功";
  if (state === "running") return "运行中";
  if (state === "failed") return "失败";
  if (state === "not_configured") return "未配置";
  return state ? "异常" : "未配置";
}

function jobTagType(state?: string): "success" | "warning" | "danger" | "info" {
  if (state === "success") return "success";
  if (state === "running") return "warning";
  if (state === "failed" || state === "invalid" || state === "unreadable") return "danger";
  return "info";
}

function fmtTime(v?: string): string {
  if (!v) return "-";
  return formatServerDateTime(v || "");
}

function shortHash(v?: string): string {
  const s = String(v || "").trim();
  if (!s) return "-";
  return s.length <= 12 ? s : s.slice(0, 12);
}

function stopAutoTick() {
  if (autoTickTimer) {
    clearInterval(autoTickTimer);
    autoTickTimer = null;
  }
}

function startAutoTick() {
  stopAutoTick();
  autoRefreshRemainSec.value = Math.floor(AUTO_REFRESH_MS / 1000);
  autoTickTimer = setInterval(() => {
    if (autoRefreshRemainSec.value <= 0) {
      autoRefreshRemainSec.value = 0;
      return;
    }
    autoRefreshRemainSec.value -= 1;
  }, 1000);
}

function stopAutoRefresh() {
  if (autoTimer) {
    clearTimeout(autoTimer);
    autoTimer = null;
  }
  stopAutoTick();
}

function scheduleAutoRefresh() {
  stopAutoRefresh();
  startAutoTick();
  autoTimer = setTimeout(async () => {
    autoRefreshRemainSec.value = 0;
    if (!document.hidden) {
      await reload(false);
    }
    scheduleAutoRefresh();
  }, AUTO_REFRESH_MS);
}

function onVisibilityChange() {
  if (document.visibilityState !== "visible") return;
  void reload(true);
}

function formatDirection(v: string): string {
  const s = String(v || "").trim();
  if (s === "primary_to_standby") return "主 → 容灾";
  if (s === "standby_to_primary") return "容灾 → 主";
  return s || "-";
}

async function reload(resetTimer = true) {
  if (loading.value) return;
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const [statusResp, syncResp, backupResp] = await Promise.all([
      client.adminHAStatus(),
      client.adminHASyncConfig(30),
      client.adminBackupStatus().catch(() => null),
    ]);
    status.value = statusResp;
    if (backupResp) backupStatus.value = backupResp;
    syncConfig.value = { ...syncResp.config };
    syncRuns.value = syncResp.runs || [];
    syncRunning.value = !!syncResp.running;
    nextRunAt.value = String(syncResp.next_run_at || "");
    lastRun.value = syncResp.last_run || (syncRuns.value.length > 0 ? syncRuns.value[0] : null);
    lastRefreshAt.value = Date.now();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
    if (resetTimer) {
      scheduleAutoRefresh();
    }
  }
}

async function saveConfig() {
  if (saving.value) return;
  saving.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const resp = await client.adminSetHASyncConfig({ ...syncConfig.value });
    syncConfig.value = { ...resp.config };
    ElMessage.success("容灾同步配置已保存");
    await reload(false);
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    saving.value = false;
  }
}

async function syncNow(direction: "primary_to_standby" | "standby_to_primary") {
  if (syncingDirection.value) return;
  if (direction === "standby_to_primary") {
    try {
      await ElMessageBox.confirm("将执行“容灾 -> 主节点”回切同步，可能覆盖主节点当前数据，是否继续？", "确认回切同步", {
        type: "warning",
        confirmButtonText: "继续",
        cancelButtonText: "取消",
      });
    } catch {
      return;
    }
  }
  syncingDirection.value = direction;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminHASyncNow({
      direction,
      trigger_mode: direction === "standby_to_primary" ? "recovery" : "manual",
    });
    ElMessage.success(r.message || "HA 同步任务已启动");
    await reload(false);
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    syncingDirection.value = "";
  }
}

async function activateFailover() {
  if (failoverLoading.value) return;
  try {
    const prompt: any = await ElMessageBox.prompt("仅在确认主节点不可用、且备机数据有效时操作。请输入 ACTIVATE_STANDBY", "备机手动接管", {
      type: "warning",
      confirmButtonText: "确认接管",
      cancelButtonText: "取消",
      inputPattern: /^ACTIVATE_STANDBY$/,
      inputErrorMessage: "请输入 ACTIVATE_STANDBY",
    });
    if (prompt.value !== "ACTIVATE_STANDBY") return;
  } catch {
    return;
  }
  failoverLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminHAFailoverActivate("ACTIVATE_STANDBY");
    const fail = (r.steps || []).filter((x) => !x.success);
    if (fail.length > 0) {
      ElMessage.warning(`接管命令已执行，但有 ${fail.length} 个步骤失败，请查看日志`);
    } else {
      ElMessage.success(r.message || "容灾接管命令已执行");
    }
    await reload(false);
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    failoverLoading.value = false;
  }
}

onMounted(() => {
  document.addEventListener("visibilitychange", onVisibilityChange);
  reload(true);
});

onBeforeUnmount(() => {
  document.removeEventListener("visibilitychange", onVisibilityChange);
  stopAutoRefresh();
});
</script>

<style scoped>
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
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
}
.head-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.mb {
  margin-bottom: 12px;
}
.readiness-alert {
  margin-bottom: 18px;
  border-radius: 14px;
}
.top-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
  margin-bottom: 10px;
}
.mini .k {
  color: #64748b;
  font-size: 12px;
}
.mini .v {
  font-size: 18px;
  font-weight: 700;
  margin-top: 4px;
}
.mini .v.compact {
  font-size: 15px;
  line-height: 1.45;
}
.backup-cards .mini {
  min-height: 132px;
}
.mini .s {
  color: #64748b;
  margin-top: 6px;
  font-size: 12px;
}
.cfg-form {
  margin-bottom: 8px;
}
.ops {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.meta {
  margin-top: 10px;
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}
.step-wrap {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.step-item {
  display: flex;
  align-items: center;
  gap: 8px;
}
.step-name {
  font-weight: 600;
  color: #334155;
}
.step-msg {
  color: #64748b;
}
@media (max-width: 900px) {
  .top-cards {
    grid-template-columns: 1fr;
  }
}
</style>
