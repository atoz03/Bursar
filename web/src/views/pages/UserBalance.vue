<template>
  <div class="user-fun-page">
    <div class="user-fun-bg">
      <div class="user-fun-flow a" />
      <div class="user-fun-flow b" />
      <div class="user-fun-blob a" />
      <div class="user-fun-blob b" />
      <div class="user-fun-spark a" />
      <div class="user-fun-spark b" />
      <div class="user-fun-sticker left">积分中心</div>
      <div class="user-fun-sticker right">节点账号别忘填</div>
    </div>
    <el-card class="user-fun-card balance-card">
      <template #header>
        <div class="row">
          <div>
            <h2 class="user-fun-head-title">我的积分</h2>
            <p class="user-fun-head-sub">登录态自动识别平台账号，积分与状态实时展示</p>
          </div>
          <el-space>
            <el-button @click="goProfile">个人信息修改</el-button>
            <el-button :loading="loading" type="primary" @click="query">刷新</el-button>
          </el-space>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon />
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
        :title="`已触发限速：当前积分 ${fmt2(resp.general_balance ?? resp.balance)}，限速阈值 ${fmt2(resp.limited_threshold_points ?? 0)}。当前仅可进行轻量操作。`"
        type="error"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      />
      <el-alert
        v-if="resp && resp.status === 'blocked'"
        :title="`已欠费：当前欠费 ${fmt2(resp.current_overdraft_points ?? 0)}，每月最大欠费上限 ${fmt2(resp.monthly_max_overdraft_limit ?? 0)}。${
          resp.overdraft_exceeded
            ? '已超过欠费上限：GPU 已禁用，CPU 已限速，且首次越线会强制清理全部进程。'
            : '未超过欠费上限：当前以限速为主，GPU 暂不禁用。'
        }`"
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
      <el-card v-if="announcements.length > 0" style="margin-bottom: 12px">
        <template #header><b>公告</b></template>
        <div v-for="a in announcements" :key="a.announcement_id" style="padding: 6px 0; border-bottom: 1px solid #eef2f7">
          <div style="font-weight: 600">{{ a.pinned ? "📌 " : "" }}{{ a.title }}</div>
          <div class="md-body" v-html="renderMarkdown(a.content)" />
        </div>
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
        <el-descriptions-item label="限速阈值">{{ fmt2(resp.limited_threshold_points ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="每月最大欠费上限">{{ fmt2(resp.monthly_max_overdraft_limit ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="当前欠费">{{ fmt2(resp.current_overdraft_points ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="节点账号映射数">{{ accountCount }}</el-descriptions-item>
        <el-descriptions-item label="平台账号创建时间">{{ fmtTime(resp.account_created_at || "") }}</el-descriptions-item>
        <el-descriptions-item label="当月剩余积分">{{ fmt2(resp.month_remaining_points ?? resp.balance) }}</el-descriptions-item>
        <el-descriptions-item label="本月已使用积分">{{ fmt2(resp.month_used_points ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="累计使用积分">{{ fmt2(resp.total_used_points ?? 0) }}</el-descriptions-item>
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
import { ApiClient, type Announcement, type BalanceResp } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { useRouter } from "vue-router";
import { renderMarkdown } from "../../lib/markdown";
import { formatServerHMS } from "../../lib/time";

const loading = ref(false);
const error = ref("");
const resp = ref<BalanceResp | null>(null);
const announcements = ref<Announcement[]>([]);
const accountCount = ref(0);
const router = useRouter();

function fmt2(v: number): string {
  return Number(v || 0).toFixed(2);
}

function fmtTime(v: string): string {
  return formatServerHMS(v);
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
  if (status === "blocked") return "欠费受限";
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
    return "积分余额接近限速阈值，请及时充值";
  }
  if (s === "limited") return "已触发限速，仍可登录但性能受限";
  if (s === "blocked") {
    return resp.value?.overdraft_exceeded
      ? "已超过欠费上限：GPU 已禁用，CPU 已限速，并触发一次性清进程"
      : "已欠费但未超过欠费上限：当前主要执行限速"
  }
  return "";
});

const exclusiveRows = computed(() => resp.value?.exclusive_balances ?? []);

async function query() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl);
    resp.value = await client.userMyBalance();
    const ac = await client.userAccounts();
    accountCount.value = (ac.accounts ?? []).length;
    const ar = await client.announcements(10);
    announcements.value = ar.announcements ?? [];
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
.md-body :deep(p) { margin: 6px 0; color:#475569; }
.md-body :deep(h1), .md-body :deep(h2), .md-body :deep(h3), .md-body :deep(h4) { margin: 8px 0; color: #0f172a; }
.md-body :deep(ul) { padding-left: 18px; margin: 6px 0; color:#475569; }
.md-body :deep(code) { background: #f1f5f9; padding: 1px 4px; border-radius: 4px; }
.md-body :deep(blockquote) { margin: 6px 0; padding-left: 10px; color: #475569; border-left: 3px solid #cbd5e1; }

@media (max-width: 900px) {
  .row {
    flex-wrap: wrap;
  }
}
</style>
