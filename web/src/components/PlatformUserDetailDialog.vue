<template>
  <el-dialog :model-value="modelValue" title="平台账号详情" width="760px" @update:model-value="emit('update:modelValue', $event)">
    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-skeleton v-else-if="loading" :rows="7" animated />
    <template v-else-if="user">
      <el-descriptions :column="2" border>
        <el-descriptions-item label="平台账号">{{ user.username }}</el-descriptions-item>
        <el-descriptions-item label="真实姓名">{{ user.real_name || "-" }}</el-descriptions-item>
        <el-descriptions-item label="学号">{{ user.student_id || "-" }}</el-descriptions-item>
        <el-descriptions-item label="邮箱">{{ user.email || "-" }}</el-descriptions-item>
        <el-descriptions-item label="导师">{{ user.advisor || "-" }}</el-descriptions-item>
        <el-descriptions-item label="电话">{{ user.phone || "-" }}</el-descriptions-item>
        <el-descriptions-item label="预计毕业">{{ fmtGrad(user.expected_graduation_year, user.expected_graduation_month) }}</el-descriptions-item>
        <el-descriptions-item label="通用积分">{{ Number((user.general_balance ?? user.balance ?? 0)).toFixed(2) }}</el-descriptions-item>
        <el-descriptions-item label="结转积分">{{ Number(user.carryover_balance || 0).toFixed(2) }}</el-descriptions-item>
        <el-descriptions-item label="节点专属积分">{{ Number(user.exclusive_balance || 0).toFixed(2) }}</el-descriptions-item>
        <el-descriptions-item label="总可用积分">{{ Number(user.total_balance ?? ((user.general_balance ?? user.balance ?? 0) + (user.carryover_balance || 0) + (user.exclusive_balance || 0))).toFixed(2) }}</el-descriptions-item>
        <el-descriptions-item label="本月已使用积分">{{ Number(user.month_used_points || 0).toFixed(2) }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ user.status || "-" }}</el-descriptions-item>
        <el-descriptions-item label="角色">{{ roleText(user.role || "") }}</el-descriptions-item>
      </el-descriptions>
      <div v-if="(user.exclusive_balances || []).length > 0" class="title">节点专属积分</div>
      <el-table v-if="(user.exclusive_balances || []).length > 0" :data="user.exclusive_balances || []" stripe max-height="220">
        <el-table-column prop="node_id" label="节点编号" width="140" />
        <el-table-column label="专属积分余额" width="160">
          <template #default="{ row }">{{ Number(row.balance || 0).toFixed(2) }}</template>
        </el-table-column>
        <el-table-column prop="updated_by" label="最后调整人" width="140" />
        <el-table-column prop="updated_at" label="更新时间" min-width="180" :formatter="tableTimeFormatter" />
      </el-table>
      <div class="title">节点账号映射</div>
      <el-table :data="user.node_accounts || []" stripe max-height="240" empty-text="暂无映射">
        <el-table-column prop="node_id" label="节点编号" width="140" />
        <el-table-column prop="local_username" label="节点账号" width="180" />
        <el-table-column label="状态" width="220">
          <template #default="{ row }">
            <div class="mapping-state-cell">
              <el-tag v-if="row.identity_aligned" type="success" effect="light">已就绪</el-tag>
              <el-tag v-else-if="row.identity_initializing" type="warning" effect="light">初始化中</el-tag>
              <el-tag v-else type="danger" effect="light">初始化失败</el-tag>
              <div v-if="mappingStateTip(row)" class="mapping-state-tip">{{ mappingStateTip(row) }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="updated_at" label="更新时间" min-width="180" :formatter="tableTimeFormatter" />
      </el-table>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import { ApiClient, type PlatformUserDetail, type UserNodeAccount } from "../lib/api";
import { settingsState } from "../lib/settingsStore";
import { authState } from "../lib/authStore";
import { formatServerDateTime } from "../lib/time";

const props = defineProps<{
  modelValue: boolean;
  username: string;
}>();
const emit = defineEmits<{
  (e: "update:modelValue", v: boolean): void;
}>();

const loading = ref(false);
const error = ref("");
const user = ref<PlatformUserDetail | null>(null);

function roleText(role: string): string {
  if (role === "admin") return "管理员";
  if (role === "power_user") return "高级用户";
  return "普通用户";
}

function fmtGrad(year: number, month: number): string {
  if (!year || !month) return "-";
  return `${year}-${String(month).padStart(2, "0")}`;
}

function tableTimeFormatter(_: unknown, __: unknown, cellValue: unknown): string {
  return formatServerDateTime(String(cellValue ?? ""));
}

function mappingStateTip(row: UserNodeAccount): string {
  if (row.identity_initializing) return "正在同步 UID/GID，完成前无法 SSH 登录";
  if (!row.identity_aligned) return "节点 UID/GID 尚未与平台 UID 对齐，请检查初始化状态或手动重同步";
  return "";
}

async function load() {
  const u = String(props.username || "").trim();
  if (!props.modelValue || !u) return;
  loading.value = true;
  error.value = "";
  user.value = null;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminPlatformUserDetail(u);
    user.value = r.user;
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

watch(() => [props.modelValue, props.username], () => {
  load();
}, { immediate: true });
</script>

<style scoped>
.mb {
  margin-bottom: 12px;
}
.title {
  margin: 12px 0 8px;
  font-weight: 700;
}
.mapping-state-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.mapping-state-tip {
  font-size: 12px;
  line-height: 1.4;
  color: #64748b;
}
</style>
