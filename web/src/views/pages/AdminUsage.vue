<template>
  <el-card>
    <template #header>
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><DataBoard /></el-icon></span>
          <div>
          <div class="title">使用记录</div>
          <div class="sub">需要管理员登录：GET /api/admin/usage，GET /api/admin/usage/export.csv</div>
          </div>
        </div>
        <div class="row">
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
          <el-button :loading="exporting" @click="exportCSV">导出 CSV</el-button>
        </div>
      </div>
    </template>

    <div class="content-stack">
      <el-alert v-if="error" :title="error" type="error" show-icon />

      <el-form inline>
        <el-form-item label="平台账号">
          <el-input v-model="billingUsername" placeholder="按平台账号查询" @keyup.enter="reload" />
        </el-form-item>
        <el-form-item label="节点账号">
          <el-input v-model="localUsername" placeholder="按节点账号查询" @keyup.enter="reload" />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="unregisteredOnly">仅看未注册偷跑</el-checkbox>
        </el-form-item>
        <el-form-item label="条数">
          <el-input-number v-model="limit" :min="1" :max="5000" />
        </el-form-item>
        <el-form-item label="导出From">
          <el-input v-model="from" placeholder="YYYY-MM-DD 或 RFC3339" />
        </el-form-item>
        <el-form-item label="导出To">
          <el-input v-model="to" placeholder="YYYY-MM-DD 或 RFC3339" />
        </el-form-item>
      </el-form>

      <el-table :data="records" stripe height="520" :row-class-name="rowClassName">
        <el-table-column prop="timestamp" label="时间" width="190" sortable />
        <el-table-column prop="node_id" label="节点" width="120" sortable />
        <el-table-column prop="local_username" label="节点账号" width="150" sortable />
        <el-table-column prop="billing_username" label="平台账号" width="170" sortable>
          <template #default="{ row }">
            <el-button
              v-if="row.registered !== false && (row.billing_username || row.username)"
              link
              type="primary"
              @click="openPlatformProfile(row.billing_username || row.username)"
            >
              {{ row.billing_username || row.username }}
            </el-button>
            <span v-else class="unregistered">未注册</span>
          </template>
        </el-table-column>
        <el-table-column label="CPU%" width="90" prop="cpu_percent" sortable>
          <template #default="{ row }">{{ fmt2(row.cpu_percent) }}</template>
        </el-table-column>
        <el-table-column label="内存MB" width="110" prop="memory_mb" sortable>
          <template #default="{ row }">{{ fmt2(row.memory_mb) }}</template>
        </el-table-column>
        <el-table-column label="积分消耗" width="100" prop="cost" sortable>
          <template #default="{ row }">{{ fmt2(row.cost) }}</template>
        </el-table-column>
        <el-table-column prop="gpu_usage" label="GPU明细(JSON)" />
      </el-table>

      <el-card v-if="unregisteredSummary.length > 0">
        <template #header>
          <div class="section-inline-title">
            <el-icon><WarningFilled /></el-icon>
            <span>未注册偷跑汇总</span>
          </div>
        </template>
        <el-table :data="unregisteredSummary" stripe max-height="260">
          <el-table-column prop="billing_username" label="平台账号" width="180" />
          <el-table-column prop="local_username" label="节点账号" width="160" />
          <el-table-column prop="nodes" label="涉及节点" min-width="260" />
          <el-table-column prop="count" label="记录数" width="100" sortable />
          <el-table-column label="操作" width="170">
            <template #default="{ row }">
              <el-button
                type="danger"
                size="small"
                :loading="blacklistingKey === `${row.local_username}::${row.nodes}`"
                @click="addUnregisteredToBlacklist(row)"
              >
                拉黑并断开SSH
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-dialog v-model="userActionsVisible" title="平台账号操作" width="480px">
        <div style="margin-bottom: 10px">当前账号：<b>{{ selectedPlatformUsername || "-" }}</b></div>
        <el-space>
          <el-button type="danger" @click="blockSelectedUser">拉黑</el-button>
          <el-button type="success" @click="unblockSelectedUser">拉白</el-button>
          <el-button type="warning" @click="deleteSelectedUser">删除</el-button>
        </el-space>
      </el-dialog>
      <PlatformUserDetailDialog v-model="profileVisible" :username="selectedProfileUsername" />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ApiClient, type UsageRecord } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { DataBoard, WarningFilled } from "@element-plus/icons-vue";

const loading = ref(false);
const exporting = ref(false);
const blacklistingKey = ref("");
const error = ref("");
const records = ref<UsageRecord[]>([]);
const userActionsVisible = ref(false);
const selectedPlatformUsername = ref("");
const profileVisible = ref(false);
const selectedProfileUsername = ref("");

const billingUsername = ref("");
const localUsername = ref("");
const unregisteredOnly = ref(false);
const limit = ref(200);
const from = ref("");
const to = ref("");

function fmt2(v: number): string {
  return Number(v ?? 0).toFixed(2);
}

async function reload() {
  loading.value = true;
  error.value = "";
  records.value = [];
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminUsage({
      billingUsername: billingUsername.value,
      localUsername: localUsername.value,
      unregisteredOnly: unregisteredOnly.value,
      limit: limit.value,
    });
    records.value = (r.records ?? []).map((x) => ({
      ...x,
      local_username: x.local_username || "-",
      billing_username: x.registered === false ? "" : (x.billing_username || x.username || ""),
    }));
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function exportCSV() {
  exporting.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const blob = await client.adminExportUsageCSV({
      username: billingUsername.value,
      from: from.value,
      to: to.value,
      limit: 20000,
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "usage_export.csv";
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    ElMessage.success("已开始下载 usage_export.csv");
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    exporting.value = false;
  }
}

const unregisteredSummary = computed(() => {
  const m = new Map<string, { billing_username: string; local_username: string; nodes: Set<string>; node_ids: string[]; count: number }>();
  for (const r of records.value) {
    if (r.registered !== false) continue;
    const b = "未注册";
    const l = r.local_username || "-";
    const key = `${r.username || "-"}::${l}`;
    const item = m.get(key) || { billing_username: b, local_username: l, nodes: new Set<string>(), node_ids: [], count: 0 };
    item.count += 1;
    if (r.node_id) item.nodes.add(r.node_id);
    m.set(key, item);
  }
  return Array.from(m.values()).map((x) => ({
    billing_username: x.billing_username,
    local_username: x.local_username,
    node_ids: Array.from(x.nodes).sort(),
    nodes: Array.from(x.nodes).sort().join(", "),
    count: x.count,
  }));
});

function rowClassName({ row }: { row: UsageRecord }) {
  return row.registered === false ? "unregistered-row" : "";
}

function openPlatformUserActions(username: string) {
  selectedPlatformUsername.value = String(username || "").trim();
  if (!selectedPlatformUsername.value) return;
  userActionsVisible.value = true;
}

function openPlatformProfile(username: string) {
  selectedProfileUsername.value = String(username || "").trim();
  if (!selectedProfileUsername.value) return;
  profileVisible.value = true;
}

async function blockSelectedUser() {
  const username = selectedPlatformUsername.value;
  if (!username) return;
  try {
    await ElMessageBox.confirm(`确认拉黑平台账号 ${username} 吗？`, "二次确认", { type: "warning", confirmButtonText: "确认拉黑", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminBlockUser(username);
    ElMessage.success(`已拉黑：${username}`);
    userActionsVisible.value = false;
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function unblockSelectedUser() {
  const username = selectedPlatformUsername.value;
  if (!username) return;
  try {
    await ElMessageBox.confirm(`确认拉白平台账号 ${username} 吗？`, "二次确认", { type: "warning", confirmButtonText: "确认拉白", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminUnblockUser(username);
    ElMessage.success(`已拉白：${username}`);
    userActionsVisible.value = false;
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function deleteSelectedUser() {
  const username = selectedPlatformUsername.value;
  if (!username) return;
  try {
    await ElMessageBox.confirm(`确认删除平台账号 ${username} 吗？`, "二次确认", { type: "warning", confirmButtonText: "确认删除", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminDeleteUser(username, "从使用记录页面删除");
    ElMessage.success(`已删除：${username}`);
    userActionsVisible.value = false;
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function addUnregisteredToBlacklist(row: { local_username: string; nodes: string; node_ids?: string[] }) {
  const local = String(row.local_username || "").trim();
  const nodeIDs = (row.node_ids ?? [])
    .map((x) => String(x || "").trim())
    .filter((x) => x && x !== "-");
  if (!local || !nodeIDs.length) {
    ElMessage.warning("缺少可用的节点账号或节点ID，无法加入黑名单");
    return;
  }

  try {
    await ElMessageBox.confirm(
      `确认将节点账号 ${local} 加入黑名单并立刻断开SSH吗？\n涉及节点：${nodeIDs.join(", ")}`,
      "二次确认",
      {
        type: "warning",
        confirmButtonText: "确认拉黑",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }

  blacklistingKey.value = `${local}::${row.nodes}`;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const results = await Promise.allSettled(nodeIDs.map((id) => client.adminUpsertBlacklist(id, [local], [])));
    const okCount = results.filter((x) => x.status === "fulfilled").length;
    const failCount = results.length - okCount;
    if (failCount > 0) {
      ElMessage.warning(`黑名单已部分生效：成功 ${okCount} 个节点，失败 ${failCount} 个节点`);
    } else {
      ElMessage.success(`已拉黑并下发断开SSH：${okCount} 个节点`);
    }
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    blacklistingKey.value = "";
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
.content-stack {
  width: 100%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.title {
  font-weight: 700;
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
  flex-shrink: 0;
}
.section-inline-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.sub {
  margin-top: 4px;
  font-size: 12px;
  color: #6b7280;
}
:deep(.unregistered-row > td) {
  background: #fee2e2 !important;
}
.unregistered {
  color: #b91c1c;
  font-weight: 600;
}
</style>
