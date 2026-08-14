<template>
  <div class="user-fun-page">
    <el-card class="user-fun-card balance-card">
      <template #header>
        <div class="row">
          <div>
            <h2 class="user-fun-head-title">
              <el-badge :is-dot="pointsUnreadCount > 0" type="danger">
                <span>我的积分</span>
              </el-badge>
            </h2>
            <p class="user-fun-head-sub">登录态自动识别平台账号，积分与状态实时展示</p>
          </div>
          <el-space>
            <el-button :loading="loading" type="primary" @click="query">刷新</el-button>
          </el-space>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon />
      <el-alert
        v-if="authState.authenticated && !authState.twoFactorEnabled"
        title="建议开启双重验证，提升账号安全性。"
        type="warning"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      />
      <div v-if="authState.authenticated && !authState.twoFactorEnabled" style="margin-bottom: 12px">
        <el-button type="warning" plain @click="goProfile">前往个人资料开启 2FA</el-button>
      </div>
      <el-alert
        v-if="resp"
        :title="`当前状态：${statusLabel(resp.status)}${statusReason ? `；${statusReason}` : ''}`"
        :type="statusAlertType(resp.status)"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      />
      <el-alert
        v-if="resp && resp.status === 'warning'"
        :title="`积分已进入预警区：当前积分 ${fmt2(resp.general_balance ?? resp.balance)}，预警阈值 ${fmt2(resp.warning_threshold_points ?? 0)}。请尽快充值，避免触发限速。`"
        type="warning"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      />
      <el-alert
        v-if="resp && resp.status === 'limited'"
        :title="`已触发限速：当前积分 ${fmt2(resp.general_balance ?? resp.balance)}。当前仅可进行轻量操作。`"
        type="error"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      />
      <el-alert
        v-if="resp && resp.status === 'blocked'"
        :title="
          resp.manual_blocked
            ? '该平台账号已被管理员加入黑名单。当前不能使用平台资源，如有疑问请联系管理员。'
            : `已欠费：当前欠费 ${fmt2(resp.current_overdraft_points ?? 0)}，每月最大欠费上限 ${fmt2(resp.monthly_max_overdraft_limit ?? 0)}。${
                resp.overdraft_exceeded
                  ? '已超过欠费上限：GPU 已禁用，CPU 已限速，且首次越线会强制清理全部进程。'
                  : '未超过欠费上限：当前以限速为主，GPU 暂不禁用。'
              }`
        "
        type="error"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      />
      <el-alert
        v-if="resp && accountCount === 0"
        title="请尽快填写你已有的计算节点账号映射！当前未填写会导致系统无法准确识别你的节点使用。"
        type="error"
        show-icon
        :closable="false"
        style="margin-bottom: 12px; border: 2px solid #dc2626"
      />
      <el-card style="margin-bottom: 12px">
        <template #header>
          <div class="row">
            <div>
              <b>积分新增记录</b>
              <el-tag v-if="pointsUnreadCount > 0" type="danger" style="margin-left: 8px">
                新增 {{ pointsUnreadCount }} 条
              </el-tag>
              <el-tag v-if="pointsUnreadAmount > 0" type="success" style="margin-left: 8px">
                新增 {{ fmt2(pointsUnreadAmount) }} 分
              </el-tag>
            </div>
            <el-button size="small" :disabled="pointsUnreadCount <= 0" @click="markPointsSeen">标记已读</el-button>
          </div>
        </template>
        <div v-if="pointsRecords.length === 0" class="empty-tip">暂无积分新增记录</div>
        <el-table v-else :data="pointsRecords" stripe size="small" max-height="280">
          <el-table-column label="时间" width="180">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="类型" width="180">
            <template #default="{ row }">{{ pointsMethodLabel(row.method) }}</template>
          </el-table-column>
          <el-table-column label="积分变动" width="140">
            <template #default="{ row }">
              <span :class="Number(row.amount || 0) >= 0 ? 'delta-plus' : 'delta-minus'">
                {{ Number(row.amount || 0) >= 0 ? "+" : "" }}{{ fmt2(row.amount || 0) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="管理员备注" min-width="280">
            <template #default="{ row }">{{ row.reason || "-" }}</template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-descriptions v-if="resp" :column="2" border>
        <el-descriptions-item label="用户名">{{ resp.username }}</el-descriptions-item>
        <el-descriptions-item label="通用积分">{{ fmt2(resp.general_balance ?? resp.balance) }}</el-descriptions-item>
        <el-descriptions-item label="结转积分">{{ fmt2(resp.carryover_balance ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="节点专属积分">{{ fmt2(resp.exclusive_balance ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="总可用积分">{{ fmt2(resp.total_balance ?? ((resp.general_balance ?? resp.balance) + (resp.carryover_balance ?? 0) + (resp.exclusive_balance ?? 0))) }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="tagType(resp.status)">{{ statusLabel(resp.status) }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="预警阈值">{{ fmt2(resp.warning_threshold_points ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="每月最大欠费上限">{{ fmt2(resp.monthly_max_overdraft_limit ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="当前欠费">{{ fmt2(resp.current_overdraft_points ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="节点账号映射数">{{ accountCount }}</el-descriptions-item>
        <el-descriptions-item label="平台账号创建时间">{{ fmtTime(resp.account_created_at || "") }}</el-descriptions-item>
        <el-descriptions-item label="当月剩余积分">{{ fmt2(resp.month_remaining_points ?? resp.balance) }}</el-descriptions-item>
        <el-descriptions-item label="本月已使用积分">{{ fmt2(resp.month_used_points ?? 0) }}</el-descriptions-item>
        <el-descriptions-item :label="totalUsedPointsLabel">{{ fmt2(resp.total_used_points ?? 0) }}</el-descriptions-item>
      </el-descriptions>
      <el-card v-if="exclusiveRows.length > 0" style="margin-top: 12px">
        <template #header><b>节点专属积分明细</b></template>
        <el-table :data="exclusiveRows" stripe size="small" max-height="260">
          <el-table-column prop="node_id" label="节点编号" width="140" />
          <el-table-column label="专属积分余额" width="160">
            <template #default="{ row }">{{ fmt2(row.balance) }}</template>
          </el-table-column>
          <el-table-column prop="updated_by" label="最后调整人" width="140" />
          <el-table-column label="更新时间" min-width="180">
            <template #default="{ row }">{{ fmtTime(row.updated_at) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { ApiClient, type BalanceResp, type PointsOperationRecord } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { useRouter } from "vue-router";
import { authState } from "../../lib/authStore";
import { formatServerDate, formatServerDateTime, getServerTodayDateText } from "../../lib/time";

const loading = ref(false);
const error = ref("");
const resp = ref<BalanceResp | null>(null);
const accountCount = ref(0);
const pointsRecords = ref<PointsOperationRecord[]>([]);
const pointsUnreadCount = ref(0);
const pointsUnreadAmount = ref(0);
const pointsLatestRechargeID = ref(0);
const router = useRouter();

function fmt2(v: number): string {
  return Number(v || 0).toFixed(2);
}

function fmtTime(v: string): string {
  return formatServerDateTime(v);
}

function pointsMethodLabel(method: string): string {
  const m = String(method || "").trim();
  if (m === "monthly_reset") return "月初通用积分重置";
  if (m === "monthly_carryover_reset") return "月初结转积分重置";
  if (m === "points_adjust_plus") return "管理员单用户加分";
  if (m === "points_adjust_carry_plus") return "管理员单用户结转加分";
  if (m === "points_adjust_node_plus") return "管理员单用户节点专属加分";
  if (m === "points_batch_plus" || m === "points_batch_grant") return "管理员全体加分";
  if (m === "points_batch_carry_plus") return "管理员全体结转加分";
  if (m === "points_batch_node_plus") return "管理员全体节点专属加分";
  if (m === "points_batch_filtered_plus") return "管理员筛选批量加分";
  if (m === "points_batch_filtered_carry_plus") return "管理员筛选批量结转加分";
  if (m === "points_batch_filtered_node_plus") return "管理员筛选批量节点专属加分";
  if (m === "points_batch_filtered_set") return "管理员筛选批量设定通用积分";
  if (m === "points_batch_filtered_set_carry") return "管理员筛选批量设定结转积分";
  if (m === "points_batch_filtered_set_node") return "管理员筛选批量设定节点专属积分";
  return m || "-";
}

function pointsSeenKey(): string {
  const u = String(authState.username || "").trim() || "anonymous";
  return `gpuops_seen_points_recharge_id_${u}`;
}

function loadSeenRechargeID(): number {
  try {
    const raw = String(localStorage.getItem(pointsSeenKey()) || "").trim();
    const n = Number(raw);
    if (!Number.isFinite(n) || n <= 0) return 0;
    return Math.floor(n);
  } catch {
    return 0;
  }
}

function saveSeenRechargeID(v: number) {
  const n = Number(v || 0);
  if (!Number.isFinite(n) || n <= 0) return;
  try {
    localStorage.setItem(pointsSeenKey(), String(Math.floor(n)));
    window.dispatchEvent(new CustomEvent("gpuops-points-seen", { detail: { latest_recharge_id: Math.floor(n) } }));
  } catch {
    // ignore
  }
}

function markPointsSeen() {
  if (pointsLatestRechargeID.value <= 0) return;
  saveSeenRechargeID(pointsLatestRechargeID.value);
  pointsUnreadCount.value = 0;
  pointsUnreadAmount.value = 0;
}

function tagType(status: string) {
  if (status === "normal") return "success";
  if (status === "warning") return "warning";
  if (status === "limited") return "danger";
  if (status === "blocked") return "danger";
  return "info";
}

function statusLabel(status: string): string {
  if (status === "normal") return "正常";
  if (status === "warning") return "预警";
  if (status === "limited") return "受限";
  if (status === "blocked") return resp.value?.manual_blocked ? "已拉黑" : "欠费受限";
  return status || "未知";
}

function statusAlertType(status: string): "success" | "warning" | "error" | "info" {
  if (status === "normal") return "success";
  if (status === "warning") return "warning";
  if (status === "limited" || status === "blocked") return "error";
  return "info";
}

const statusReason = computed(() => {
  const s = String(resp.value?.status || "").trim();
  if (!s) return "";
  if (s === "normal") return "账号状态正常，可正常使用";
  if (s === "warning") {
    return "积分余额偏低，请及时充值";
  }
  if (s === "limited") return "已触发限速，仍可登录但性能受限";
  if (s === "blocked") {
    if (resp.value?.manual_blocked) {
      return "管理员已将该账号加入平台黑名单";
    }
    return resp.value?.overdraft_exceeded
      ? "已超过欠费上限：GPU 已禁用，CPU 已限速，并触发一次性清进程"
      : "已欠费但未超过欠费上限：当前主要执行限速"
  }
  return "";
});

const exclusiveRows = computed(() => resp.value?.exclusive_balances ?? []);

const totalUsedPointsLabel = computed(() => {
  const from = formatServerDate(resp.value?.account_created_at || "");
  const to = getServerTodayDateText();
  if (!resp.value || from === "-") return "累计使用积分";
  return `累计使用积分（${from} 至 ${to}）`;
});

async function query() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl);
    const seenID = loadSeenRechargeID();
    const [balanceResp, ac, pointResp] = await Promise.all([
      client.userMyBalance(),
      client.userAccounts(),
      client.userMyPointsIncrements({ sinceId: seenID, limit: 200 }),
    ]);
    resp.value = balanceResp;
    accountCount.value = (ac.accounts ?? []).length;
    pointsRecords.value = pointResp.records ?? [];
    pointsUnreadCount.value = Number(pointResp.unread_count || 0);
    pointsUnreadAmount.value = Number(pointResp.unread_amount || 0);
    pointsLatestRechargeID.value = Number(pointResp.latest_recharge_id || 0);
  } catch (e: any) {
    error.value = e?.body ? `${e.message}\n${e.body}` : (e?.message ?? String(e));
  } finally {
    loading.value = false;
  }
}

function goProfile() {
  router.push("/user/profile");
}

query();
</script>

<style scoped>
.balance-card {
  min-height: 420px;
}
.row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.empty-tip { color: #64748b; font-size: 13px; }
.delta-plus { color: #15803d; font-weight: 700; }
.delta-minus { color: #b91c1c; font-weight: 700; }
@media (max-width: 900px) {
  .row {
    flex-wrap: wrap;
  }
}
</style>
