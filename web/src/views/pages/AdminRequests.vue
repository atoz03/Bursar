<template>
  <el-card>
    <template #header>
      <div class="row">
        <div>
          <div class="title">注册 / 开号申请审核</div>
          <div class="sub">需要管理员登录：GET /api/admin/requests，POST /api/admin/requests/:id/approve|reject</div>
        </div>
        <div class="row">
          <el-select v-model="status" style="width: 160px" @change="reload">
            <el-option label="待审核" value="pending" />
            <el-option label="已通过" value="approved" />
            <el-option label="已拒绝" value="rejected" />
            <el-option label="全部" value="" />
          </el-select>
          <el-button :disabled="selectedIds.length===0" :loading="batchLoading" type="success" @click="batchApprove">批量通过</el-button>
          <el-button :disabled="selectedIds.length===0" :loading="batchLoading" type="danger" @click="batchReject">批量拒绝</el-button>
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
        </div>
      </div>
    </template>

    <el-space direction="vertical" fill style="width: 100%">
      <el-alert v-if="error" :title="error" type="error" show-icon />

      <el-table :data="rows" stripe height="560" @selection-change="onSelectionChange" :row-class-name="rowClassName">
        <el-table-column type="selection" width="45" />
        <el-table-column prop="request_id" label="ID" width="80" />
        <el-table-column prop="request_type" label="类型" width="100" />
        <el-table-column prop="billing_username" label="系统账号" width="160" />
        <el-table-column prop="node_id" label="端口" width="110" />
        <el-table-column prop="local_username" label="机器用户名" width="160" />
        <el-table-column prop="status" label="状态" width="120" />
        <el-table-column prop="apply_count_by_billing" label="申请次数" width="100" />
        <el-table-column prop="duplicate_reason" label="风险提示" width="220" />
        <el-table-column prop="created_at" label="提交时间" min-width="180" />
        <el-table-column prop="reviewed_by" label="审核人" width="130" />
        <el-table-column prop="reviewed_at" label="审核时间" width="170" />
        <el-table-column prop="message" label="备注" min-width="220" />
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-space>
              <el-button
                size="small"
                type="success"
                :disabled="row.status !== 'pending'"
                :loading="actionLoadingId === row.request_id"
                @click="approve(row.request_id)"
              >
                通过
              </el-button>
              <el-button
                size="small"
                type="danger"
                :disabled="row.status !== 'pending'"
                :loading="actionLoadingId === row.request_id"
                @click="reject(row.request_id)"
              >
                拒绝
              </el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <el-divider />
      <div class="title">用户资料关键信息变更审核</div>
      <el-form inline>
        <el-form-item label="用户名筛选">
          <el-input v-model="profileUsername" placeholder="按平台用户名筛选" @keyup.enter="reloadProfileChanges" />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="profileStatus" style="width: 140px" @change="reloadProfileChanges">
            <el-option label="待审核" value="pending" />
            <el-option label="已通过" value="approved" />
            <el-option label="已拒绝" value="rejected" />
            <el-option label="全部" value="" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button :loading="profileLoading" type="primary" @click="reloadProfileChanges">刷新</el-button>
        </el-form-item>
      </el-form>
      <el-table :data="profileRows" stripe height="360">
        <el-table-column prop="request_id" label="ID" width="80" />
        <el-table-column prop="billing_username" label="申请用户" width="140" />
        <el-table-column prop="status" label="状态" width="100" />
        <el-table-column prop="old_username" label="原用户名" width="120" />
        <el-table-column prop="new_username" label="新用户名" width="120" />
        <el-table-column prop="old_email" label="原邮箱" min-width="170" />
        <el-table-column prop="new_email" label="新邮箱" min-width="170" />
        <el-table-column prop="old_student_id" label="原学号" width="120" />
        <el-table-column prop="new_student_id" label="新学号" width="120" />
        <el-table-column prop="reason" label="变更备注" min-width="180" />
        <el-table-column prop="reviewed_by" label="审核人" width="120" />
        <el-table-column prop="reviewed_at" label="审核时间" width="170" />
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-space>
              <el-button
                size="small"
                type="success"
                :disabled="row.status !== 'pending'"
                :loading="profileActionId === row.request_id"
                @click="approveProfileChange(row.request_id)"
              >
                通过
              </el-button>
              <el-button
                size="small"
                type="danger"
                :disabled="row.status !== 'pending'"
                :loading="profileActionId === row.request_id"
                @click="rejectProfileChange(row.request_id)"
              >
                拒绝
              </el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>
    </el-space>
  </el-card>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { ApiClient, type ProfileChangeRequest, type UserRequest } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";

const loading = ref(false);
const error = ref("");
const status = ref("pending");
const rows = ref<UserRequest[]>([]);
const actionLoadingId = ref<number | null>(null);
const batchLoading = ref(false);
const selectedIds = ref<number[]>([]);
const profileLoading = ref(false);
const profileActionId = ref<number | null>(null);
const profileRows = ref<ProfileChangeRequest[]>([]);
const profileStatus = ref("pending");
const profileUsername = ref("");

async function reload() {
  loading.value = true;
  error.value = "";
  rows.value = [];
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminRequests({ status: status.value, limit: 500 });
    rows.value = r.requests ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function approve(id: number) {
  actionLoadingId.value = id;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminApproveRequest(id);
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    actionLoadingId.value = null;
  }
}

async function reject(id: number) {
  actionLoadingId.value = id;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminRejectRequest(id);
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    actionLoadingId.value = null;
  }
}

function onSelectionChange(v: UserRequest[]) {
  selectedIds.value = (v ?? []).map((x) => x.request_id);
}

function rowClassName({ row }: { row: UserRequest }) {
  return row.duplicate_flag ? "dup-row" : "";
}

async function batchApprove() {
  await batchReview("approved");
}
async function batchReject() {
  await batchReview("rejected");
}
async function batchReview(newStatus: "approved" | "rejected") {
  if (selectedIds.value.length === 0) return;
  batchLoading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminBatchReview(selectedIds.value, newStatus);
    if (r.fail_count > 0) {
      error.value = `批量处理完成：成功 ${r.ok_count}，失败 ${r.fail_count}`;
    }
    selectedIds.value = [];
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    batchLoading.value = false;
  }
}

async function reloadProfileChanges() {
  profileLoading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminProfileChangeRequests({
      status: profileStatus.value,
      username: profileUsername.value,
      limit: 500,
    });
    profileRows.value = r.requests ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    profileLoading.value = false;
  }
}

async function approveProfileChange(id: number) {
  profileActionId.value = id;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminApproveProfileChange(id);
    await reloadProfileChanges();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    profileActionId.value = null;
  }
}

async function rejectProfileChange(id: number) {
  profileActionId.value = id;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminRejectProfileChange(id);
    await reloadProfileChanges();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    profileActionId.value = null;
  }
}

reload();
reloadProfileChanges();
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
:deep(.dup-row > td) {
  background: #fee2e2 !important;
}
</style>
