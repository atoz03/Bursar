<template>
  <el-card>
    <template #header>
      <div class="head">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><UserFilled /></el-icon></span>
          <div>
          <div class="title">高级用户</div>
          <div class="sub">支持新增独立高级账号，也支持把现有平台用户提升为高级用户</div>
          </div>
        </div>
        <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />

    <el-card class="mb">
      <template #header>
        <div class="section-inline-title">
          <el-icon><Plus /></el-icon>
          <span>新增独立高级账号</span>
        </div>
      </template>
      <el-form inline>
        <el-form-item label="用户名 *"><el-input v-model="createForm.username" /></el-form-item>
        <el-form-item label="初始密码 *"><el-input v-model="createForm.password" type="password" show-password placeholder="请设置强密码" /></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_view_board">可看运营看板</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_view_nodes">可看节点状态</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_manage_nodes">可修改节点状态</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_points_users">可看积分用户/单调</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_points_batch_filtered">可做筛选批量积分</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_points_batch_all">可做全体积分调整</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_points_records">可看积分记录</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_points_monthly">可管月初规则/重置</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_points_special_rules">可管特殊月度积分</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_review_requests">可做注册审核</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="createForm.can_manage_platform_users">可管理平台用户</el-checkbox></el-form-item>
        <el-form-item><el-button type="primary" @click="create">新增高级用户</el-button></el-form-item>
      </el-form>
      <div class="tips">密码要求：{{ passwordRuleText }}</div>
      <div class="tips">要求：该用户名不能与现有平台账号/管理员/高级用户重名。</div>
    </el-card>

    <el-card class="mb">
      <template #header>
        <div class="section-inline-title">
          <el-icon><Connection /></el-icon>
          <span>提升现有平台用户</span>
        </div>
      </template>
      <el-form inline>
        <el-form-item label="平台用户名 *">
          <el-select
            v-model="promoteForm.username"
            filterable
            clearable
            remote
            :remote-method="onPlatformSearch"
            :loading="loading"
            placeholder="输入平台用户名匹配"
            style="width: 320px"
          >
            <el-option
              v-for="u in promotableUsers"
              :key="u.username"
              :label="`${u.username}（${u.real_name || '未填写姓名'}）`"
              :value="u.username"
            />
          </el-select>
        </el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_view_board">可看运营看板</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_view_nodes">可看节点状态</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_manage_nodes">可修改节点状态</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_points_users">可看积分用户/单调</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_points_batch_filtered">可做筛选批量积分</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_points_batch_all">可做全体积分调整</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_points_records">可看积分记录</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_points_monthly">可管月初规则/重置</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_points_special_rules">可管特殊月度积分</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_review_requests">可做注册审核</el-checkbox></el-form-item>
        <el-form-item><el-checkbox v-model="promoteForm.can_manage_platform_users">可管理平台用户</el-checkbox></el-form-item>
        <el-form-item><el-button type="primary" @click="promote">提升为高级用户</el-button></el-form-item>
      </el-form>
      <div class="tips">提升后，用户仍是同一平台账号，只是权限升级；可随时“取消高级”恢复普通用户。</div>
    </el-card>

    <el-table :data="rows" stripe>
      <el-table-column prop="username" label="用户名" width="170" />
      <el-table-column label="来源" width="140">
        <template #default="{ row }">{{ row.is_platform_user ? "平台用户提升" : "独立高级账号" }}</template>
      </el-table-column>
      <el-table-column label="运营看板" width="120">
        <template #default="{ row }"><el-switch v-model="row.can_view_board" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="节点状态" width="120">
        <template #default="{ row }"><el-switch v-model="row.can_view_nodes" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="节点修改" width="120">
        <template #default="{ row }"><el-switch v-model="row.can_manage_nodes" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="积分用户/单调" width="140">
        <template #default="{ row }"><el-switch v-model="row.can_points_users" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="筛选批量积分" width="140">
        <template #default="{ row }"><el-switch v-model="row.can_points_batch_filtered" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="全体积分调整" width="140">
        <template #default="{ row }"><el-switch v-model="row.can_points_batch_all" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="积分记录" width="120">
        <template #default="{ row }"><el-switch v-model="row.can_points_records" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="月初规则/重置" width="140">
        <template #default="{ row }"><el-switch v-model="row.can_points_monthly" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="特殊月度积分" width="140">
        <template #default="{ row }"><el-switch v-model="row.can_points_special_rules" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="注册审核" width="120">
        <template #default="{ row }"><el-switch v-model="row.can_review_requests" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="平台用户管理" width="140">
        <template #default="{ row }"><el-switch v-model="row.can_manage_platform_users" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column prop="updated_by" label="最近变更人" width="150" />
      <el-table-column prop="updated_at" label="最近变更时间" min-width="180" :formatter="tableTimeFormatter" />
      <el-table-column label="操作" width="120">
        <template #default="{ row }">
          <el-button
            v-if="row.is_platform_user"
            size="small"
            type="warning"
            @click="demote(row.username)"
          >取消高级</el-button>
          <el-button
            v-else
            size="small"
            type="danger"
            @click="remove(row.username)"
          >删除</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { ApiClient, type AdminUserDetail, type PowerUser } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { Connection, Plus, UserFilled } from "@element-plus/icons-vue";
import { STRONG_PASSWORD_RULE_TEXT, checkStrongPassword } from "../../lib/passwordPolicy";
import { formatServerDateTime } from "../../lib/time";

const loading = ref(false);
const error = ref("");
const success = ref("");
const rows = ref<PowerUser[]>([]);
const allPlatformUsers = ref<AdminUserDetail[]>([]);
const platformKeyword = ref("");
const passwordRuleText = STRONG_PASSWORD_RULE_TEXT;
const createForm = reactive({
  username: "",
  password: "",
  can_view_board: true,
  can_view_nodes: true,
  can_manage_nodes: false,
  can_points_users: false,
  can_points_batch_filtered: false,
  can_points_batch_all: false,
  can_points_records: false,
  can_points_monthly: false,
  can_points_special_rules: false,
  can_review_requests: false,
  can_manage_platform_users: false,
});
const promoteForm = reactive({
  username: "",
  can_view_board: true,
  can_view_nodes: true,
  can_manage_nodes: false,
  can_points_users: false,
  can_points_batch_filtered: false,
  can_points_batch_all: false,
  can_points_records: false,
  can_points_monthly: false,
  can_points_special_rules: false,
  can_review_requests: false,
  can_manage_platform_users: false,
});

function normalizePointsPermState<T extends {
  can_points_users: boolean;
  can_points_batch_filtered: boolean;
  can_points_batch_all: boolean;
  can_points_records: boolean;
  can_points_monthly: boolean;
  can_points_special_rules: boolean;
}>(state: T): T {
  if (state.can_points_batch_filtered) {
    state.can_points_users = true;
  }
  return state;
}

function pointsPermPayload(state: {
  can_points_users: boolean;
  can_points_batch_filtered: boolean;
  can_points_batch_all: boolean;
  can_points_records: boolean;
  can_points_monthly: boolean;
  can_points_special_rules: boolean;
}) {
  normalizePointsPermState(state);
  const canManagePoints =
    state.can_points_users ||
    state.can_points_batch_filtered ||
    state.can_points_batch_all ||
    state.can_points_records ||
    state.can_points_monthly ||
    state.can_points_special_rules;
  return {
    can_manage_points: canManagePoints,
    can_points_users: state.can_points_users,
    can_points_batch_filtered: state.can_points_batch_filtered,
    can_points_batch_all: state.can_points_batch_all,
    can_points_records: state.can_points_records,
    can_points_monthly: state.can_points_monthly,
    can_points_special_rules: state.can_points_special_rules,
  };
}

function tableTimeFormatter(_: unknown, __: unknown, cellValue: unknown): string {
  return formatServerDateTime(String(cellValue ?? ""));
}

const promotableUsers = computed(() => {
  const powerSet = new Set((rows.value ?? []).map((x) => x.username));
  const k = platformKeyword.value.trim().toLowerCase();
  return (allPlatformUsers.value ?? [])
    .filter((u) => u.role === "user")
    .filter((u) => !powerSet.has(u.username))
    .filter((u) => {
      if (!k) return true;
      return (
        String(u.username || "").toLowerCase().includes(k) ||
        String(u.real_name || "").toLowerCase().includes(k) ||
        String(u.email || "").toLowerCase().includes(k) ||
        String(u.student_id || "").toLowerCase().includes(k)
      );
    })
    .slice(0, 100);
});

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const [r, u] = await Promise.all([client.adminPowerUsers(2000), client.adminUsersDetails(5000)]);
    rows.value = r.users ?? [];
    allPlatformUsers.value = u.users ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

function onPlatformSearch(query: string) {
  platformKeyword.value = query || "";
}

async function create() {
  error.value = "";
  success.value = "";
  const pwdErr = checkStrongPassword(createForm.password);
  if (pwdErr) {
    error.value = pwdErr;
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const pointsPerms = pointsPermPayload(createForm);
    await client.adminCreatePowerUser({
      username: createForm.username.trim(),
      password: createForm.password,
      can_view_board: createForm.can_view_board,
      can_view_nodes: createForm.can_view_nodes,
      can_manage_nodes: createForm.can_manage_nodes,
      ...pointsPerms,
      can_review_requests: createForm.can_review_requests,
      can_manage_platform_users: createForm.can_manage_platform_users,
    });
    success.value = "高级用户创建成功";
    createForm.username = "";
    createForm.password = "";
    createForm.can_view_board = true;
    createForm.can_view_nodes = true;
    createForm.can_manage_nodes = false;
    createForm.can_points_users = false;
    createForm.can_points_batch_filtered = false;
    createForm.can_points_batch_all = false;
    createForm.can_points_records = false;
    createForm.can_points_monthly = false;
    createForm.can_points_special_rules = false;
    createForm.can_review_requests = false;
    createForm.can_manage_platform_users = false;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function promote() {
  error.value = "";
  success.value = "";
  if (!promoteForm.username.trim()) {
    error.value = "请先选择一个平台用户";
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const pointsPerms = pointsPermPayload(promoteForm);
    await client.adminPromotePowerUser({
      username: promoteForm.username.trim(),
      can_view_board: promoteForm.can_view_board,
      can_view_nodes: promoteForm.can_view_nodes,
      can_manage_nodes: promoteForm.can_manage_nodes,
      ...pointsPerms,
      can_review_requests: promoteForm.can_review_requests,
      can_manage_platform_users: promoteForm.can_manage_platform_users,
    });
    success.value = `已提升为高级用户：${promoteForm.username.trim()}`;
    promoteForm.username = "";
    promoteForm.can_view_board = true;
    promoteForm.can_view_nodes = true;
    promoteForm.can_manage_nodes = false;
    promoteForm.can_points_users = false;
    promoteForm.can_points_batch_filtered = false;
    promoteForm.can_points_batch_all = false;
    promoteForm.can_points_records = false;
    promoteForm.can_points_monthly = false;
    promoteForm.can_points_special_rules = false;
    promoteForm.can_review_requests = false;
    promoteForm.can_manage_platform_users = false;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function savePerm(row: PowerUser) {
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const pointsPerms = pointsPermPayload(row);
    await client.adminUpdatePowerUserPermissions(row.username, {
      can_view_board: row.can_view_board,
      can_view_nodes: row.can_view_nodes,
      can_manage_nodes: row.can_manage_nodes,
      ...pointsPerms,
      can_review_requests: row.can_review_requests,
      can_manage_platform_users: row.can_manage_platform_users,
    });
    success.value = `权限已更新：${row.username}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function remove(username: string) {
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminDeletePowerUser(username);
    success.value = `已删除高级用户：${username}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function demote(username: string) {
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminDemotePowerUser(username);
    success.value = `已取消高级用户：${username}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

reload();
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
  flex-shrink: 0;
}
.section-inline-title { display: inline-flex; align-items: center; gap: 8px; }
.title { font-weight: 700; }
.sub { margin-top: 4px; font-size: 12px; color: #64748b; }
.mb { margin-bottom: 12px; }
.tips { color: #64748b; font-size: 12px; }
</style>
