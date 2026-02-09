<template>
  <el-card>
    <template #header>
      <div class="row">
        <div>
          <div class="title">用户管理</div>
          <div class="sub">用户资料、积分状态、累计使用、节点账号映射</div>
        </div>
        <div class="row">
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
        </div>
      </div>
    </template>

    <el-space direction="vertical" fill style="width: 100%">
      <el-alert v-if="error" :title="error" type="error" show-icon />
      <el-form inline>
        <el-form-item label="用户名查询">
          <el-input v-model="keyword" placeholder="输入用户名过滤" clearable />
        </el-form-item>
      </el-form>

      <el-table :data="filteredRows" stripe height="560">
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="expand-wrap">
              <el-descriptions :column="3" border>
                <el-descriptions-item label="真实姓名">{{ row.real_name || "-" }}</el-descriptions-item>
                <el-descriptions-item label="导师">{{ row.advisor || "-" }}</el-descriptions-item>
                <el-descriptions-item label="预计毕业">{{ row.expected_graduation_year || "-" }}</el-descriptions-item>
                <el-descriptions-item label="手机号">{{ row.phone || "-" }}</el-descriptions-item>
                <el-descriptions-item label="累计记录">{{ row.usage_records }}</el-descriptions-item>
                <el-descriptions-item label="累计积分消耗">{{ fmt2(row.total_cost) }}</el-descriptions-item>
                <el-descriptions-item label="最后使用时间">{{ fmtTime(row.last_usage_at) }}</el-descriptions-item>
                <el-descriptions-item label="运营看板权限">{{ row.can_view_board ? "有" : "无" }}</el-descriptions-item>
                <el-descriptions-item label="节点状态权限">{{ row.can_view_nodes ? "有" : "无" }}</el-descriptions-item>
                <el-descriptions-item label="注册审核权限">{{ row.can_review_requests ? "有" : "无" }}</el-descriptions-item>
              </el-descriptions>

              <div class="node-title">该用户在各节点账号</div>
              <el-table :data="row.node_accounts ?? []" stripe size="small">
                <el-table-column prop="node_id" label="节点ID" width="140" />
                <el-table-column prop="local_username" label="本地用户名" width="180" />
                <el-table-column prop="billing_username" label="系统账号" width="180" />
                <el-table-column prop="updated_at" label="更新时间" min-width="180" />
              </el-table>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="username" label="用户名" width="160" />
        <el-table-column prop="role" label="角色" width="120">
          <template #default="{ row }">
            {{ row.role === "admin" ? "管理员" : (row.role === "power_user" ? "高级用户" : "普通用户") }}
          </template>
        </el-table-column>
        <el-table-column prop="student_id" label="学号" width="160" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column label="积分余额" width="120">
          <template #default="{ row }">{{ fmt2(row.balance) }}</template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="140" />
        <el-table-column prop="usage_records" label="使用记录数" width="120" />
        <el-table-column label="看板权限" width="96">
          <template #default="{ row }">{{ row.can_view_board ? "是" : "否" }}</template>
        </el-table-column>
        <el-table-column label="节点权限" width="96">
          <template #default="{ row }">{{ row.can_view_nodes ? "是" : "否" }}</template>
        </el-table-column>
        <el-table-column label="审核权限" width="96">
          <template #default="{ row }">{{ row.can_review_requests ? "是" : "否" }}</template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" :disabled="row.role !== 'user'" @click="openRecharge(row.username)">加分</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-space>

    <el-dialog v-model="rechargeVisible" title="积分调整" width="420px">
      <el-form label-width="90px">
        <el-form-item label="用户名">
          <el-input v-model="rechargeUser" disabled />
        </el-form-item>
        <el-form-item label="积分">
          <el-input-number v-model="rechargeAmount" :min="0.01" :max="100000" :step="10" />
        </el-form-item>
        <el-form-item label="方式">
          <el-input v-model="rechargeMethod" placeholder="admin/wechat/alipay" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="rechargeVisible = false">取消</el-button>
        <el-button :loading="rechargeLoading" type="primary" @click="doRecharge">确认</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { ApiClient, type AdminUserDetail } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";

const loading = ref(false);
const error = ref("");
const rows = ref<AdminUserDetail[]>([]);
const keyword = ref("");
const filteredRows = computed(() => {
  const k = keyword.value.trim().toLowerCase();
  if (!k) return rows.value;
  return rows.value.filter((x) => (x.username ?? "").toLowerCase().includes(k));
});

const rechargeVisible = ref(false);
const rechargeLoading = ref(false);
const rechargeUser = ref("");
const rechargeAmount = ref(100);
const rechargeMethod = ref("admin");

function fmt2(v: number): string {
  return Number(v ?? 0).toFixed(2);
}

function fmtTime(v: string): string {
  if (!v) return "-";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleString();
}

async function reload() {
  loading.value = true;
  error.value = "";
  rows.value = [];
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminUsersDetails(2000);
    rows.value = r.users ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

function openRecharge(username: string) {
  rechargeUser.value = username;
  rechargeAmount.value = 100;
  rechargeMethod.value = "admin";
  rechargeVisible.value = true;
}

async function doRecharge() {
  rechargeLoading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminRecharge(rechargeUser.value, rechargeAmount.value, rechargeMethod.value);
    rechargeVisible.value = false;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    rechargeLoading.value = false;
  }
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
.expand-wrap {
  padding: 8px 12px;
}
.node-title {
  margin: 12px 0 8px;
  font-weight: 600;
}
</style>
