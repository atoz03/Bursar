<template>
  <el-card>
    <template #header>
      <div class="head">
        <div>
          <div class="title">高级用户</div>
          <div class="sub">可邀请受限管理账号（仅查看/审核权限）</div>
        </div>
        <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />

    <el-form inline>
      <el-form-item label="用户名 *"><el-input v-model="form.username" /></el-form-item>
      <el-form-item label="初始密码 *"><el-input v-model="form.password" type="password" show-password /></el-form-item>
      <el-form-item><el-checkbox v-model="form.can_view_board">可看运营看板</el-checkbox></el-form-item>
      <el-form-item><el-checkbox v-model="form.can_view_nodes">可看节点状态</el-checkbox></el-form-item>
      <el-form-item><el-checkbox v-model="form.can_review_requests">可做注册审核</el-checkbox></el-form-item>
      <el-form-item><el-button type="primary" @click="create">新增高级用户</el-button></el-form-item>
    </el-form>

    <el-table :data="rows" stripe>
      <el-table-column prop="username" label="用户名" width="170" />
      <el-table-column label="运营看板" width="120">
        <template #default="{ row }"><el-switch v-model="row.can_view_board" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="节点状态" width="120">
        <template #default="{ row }"><el-switch v-model="row.can_view_nodes" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column label="注册审核" width="120">
        <template #default="{ row }"><el-switch v-model="row.can_review_requests" @change="savePerm(row)" /></template>
      </el-table-column>
      <el-table-column prop="updated_by" label="最近变更人" width="150" />
      <el-table-column prop="updated_at" label="最近变更时间" min-width="180" />
      <el-table-column label="操作" width="120">
        <template #default="{ row }">
          <el-button size="small" type="danger" @click="remove(row.username)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { ApiClient, type PowerUser } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";

const loading = ref(false);
const error = ref("");
const success = ref("");
const rows = ref<PowerUser[]>([]);
const form = reactive({
  username: "",
  password: "",
  can_view_board: true,
  can_view_nodes: true,
  can_review_requests: false,
});

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminPowerUsers(2000);
    rows.value = r.users ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function create() {
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminCreatePowerUser({
      username: form.username.trim(),
      password: form.password,
      can_view_board: form.can_view_board,
      can_view_nodes: form.can_view_nodes,
      can_review_requests: form.can_review_requests,
    });
    success.value = "高级用户创建成功";
    form.username = "";
    form.password = "";
    form.can_view_board = true;
    form.can_view_nodes = true;
    form.can_review_requests = false;
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
    await client.adminUpdatePowerUserPermissions(row.username, {
      can_view_board: row.can_view_board,
      can_view_nodes: row.can_view_nodes,
      can_review_requests: row.can_review_requests,
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

reload();
</script>

<style scoped>
.head { display: flex; justify-content: space-between; align-items: center; }
.title { font-weight: 700; }
.sub { margin-top: 4px; font-size: 12px; color: #64748b; }
.mb { margin-bottom: 12px; }
</style>
