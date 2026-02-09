<template>
  <el-card>
    <template #header>
      <div class="head"><span>SSH 名单</span><el-button :loading="loading" type="primary" @click="reload">刷新</el-button></div>
    </template>
    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert
      title="支持两种添加方式：1) 按机器 + 本地系统账号 2) 按机器 + 平台系统账号（自动展开到该账号已绑定本地账号）。node_id=* 表示所有机器。"
      type="info"
      show-icon
      class="mb"
    />
    <el-tabs v-model="mode" class="mb">
      <el-tab-pane label="SSH 白名单" name="whitelist" />
      <el-tab-pane label="SSH 黑名单" name="blacklist" />
      <el-tab-pane label="SSH 豁免名单" name="exemptions" />
    </el-tabs>
    <el-alert
      v-if="mode === 'exemptions'"
      title="豁免账号权限：1) 登录校验最高优先级，忽略黑名单/白名单/注册映射限制；2) 不受“清除SSH状态”和黑名单加入时的强制断连影响；3) 控制器不可达时仍可通过本地豁免缓存登录。"
      type="warning"
      show-icon
      class="mb"
    />
    <el-form inline>
      <el-form-item label="机器编号">
        <el-select v-model="nodeId" filterable style="width: 220px">
          <el-option label="所有机器 (*)" value="*" />
          <el-option v-for="id in nodeOptions" :key="id" :label="id" :value="id" />
        </el-select>
      </el-form-item>
      <el-form-item label="本地系统账号">
        <el-input v-model="usernamesText" placeholder="alice,bob,charlie" style="width: 260px" />
      </el-form-item>
      <el-form-item label="平台系统账号">
        <el-input v-model="billingUsernamesText" placeholder="zhangsan,lisi" style="width: 260px" />
      </el-form-item>
      <el-form-item>
        <el-button :type="mode === 'blacklist' ? 'danger' : 'primary'" @click="save">
          {{ saveButtonText }}
        </el-button>
      </el-form-item>
    </el-form>

    <el-form inline>
      <el-form-item label="按机器筛选">
        <el-input v-model="filterNode" placeholder="留空全部" />
      </el-form-item>
      <el-form-item><el-button @click="reload">查询</el-button></el-form-item>
    </el-form>

    <el-table :data="currentRows" stripe>
      <el-table-column prop="node_id" label="机器编号" width="120" />
      <el-table-column prop="local_username" label="用户名" width="180" />
      <el-table-column prop="created_by" label="创建人" width="160" />
      <el-table-column prop="updated_at" label="更新时间" min-width="180" />
      <el-table-column label="操作" width="120">
        <template #default="{ row }">
          <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ApiClient, type SSHBlacklistEntry, type SSHExemptionEntry, type SSHWhitelistEntry } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";

const loading = ref(false);
const error = ref("");
const whitelistRows = ref<SSHWhitelistEntry[]>([]);
const blacklistRows = ref<SSHBlacklistEntry[]>([]);
const exemptionRows = ref<SSHExemptionEntry[]>([]);
const nodeOptions = ref<string[]>([]);
const mode = ref<"whitelist" | "blacklist" | "exemptions">("whitelist");

const nodeId = ref("*");
const usernamesText = ref("");
const billingUsernamesText = ref("");
const filterNode = ref("");

const currentRows = computed(() => {
  if (mode.value === "whitelist") return whitelistRows.value;
  if (mode.value === "blacklist") return blacklistRows.value;
  return exemptionRows.value;
});
const saveButtonText = computed(() => {
  if (mode.value === "whitelist") return "新增白名单";
  if (mode.value === "blacklist") return "新增黑名单并断开SSH";
  return "新增豁免账号";
});

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    if (mode.value === "whitelist") {
      const r = await client.adminWhitelist(filterNode.value.trim());
      whitelistRows.value = r.entries ?? [];
    } else if (mode.value === "blacklist") {
      const r = await client.adminBlacklist(filterNode.value.trim());
      blacklistRows.value = r.entries ?? [];
    } else {
      const r = await client.adminExemptions(filterNode.value.trim());
      exemptionRows.value = r.entries ?? [];
    }
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function loadNodes() {
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminNodes(2000);
    nodeOptions.value = (r.nodes ?? []).map((x) => x.node_id).filter(Boolean);
  } catch {
    nodeOptions.value = [];
  }
}

async function save() {
  error.value = "";
  const names = usernamesText.value.split(",").map((x) => x.trim()).filter(Boolean);
  const billingNames = billingUsernamesText.value.split(",").map((x) => x.trim()).filter(Boolean);
  if (names.length === 0 && billingNames.length === 0) {
    error.value = "请至少输入本地系统账号或平台系统账号";
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    if (mode.value === "whitelist") {
      await client.adminUpsertWhitelist(nodeId.value.trim(), names, billingNames);
      ElMessage.success("白名单保存成功，节点将快速同步");
    } else if (mode.value === "blacklist") {
      await client.adminUpsertBlacklist(nodeId.value.trim(), names, billingNames);
      ElMessage.success("黑名单保存成功，已下发断开SSH会话指令");
    } else {
      await client.adminUpsertExemptions(nodeId.value.trim(), names, billingNames);
      ElMessage.success("豁免账号保存成功，节点将快速同步");
    }
    usernamesText.value = "";
    billingUsernamesText.value = "";
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function remove(row: SSHWhitelistEntry | SSHBlacklistEntry | SSHExemptionEntry) {
  error.value = "";
  try {
    await ElMessageBox.confirm(
      mode.value === "whitelist"
        ? `确认删除白名单用户 ${row.local_username}（节点 ${row.node_id}）吗？系统将同时尝试断开其现有SSH会话。`
        : mode.value === "blacklist"
          ? `确认删除黑名单用户 ${row.local_username}（节点 ${row.node_id}）吗？`
          : `确认删除豁免账号 ${row.local_username}（节点 ${row.node_id}）吗？删除后将恢复正常SSH校验规则。`,
      "删除确认",
      { type: "warning", confirmButtonText: "确认删除", cancelButtonText: "取消" },
    );
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    if (mode.value === "whitelist") {
      await client.adminDeleteWhitelist(row.node_id, row.local_username);
      ElMessage.success("白名单删除成功，已下发断开SSH会话指令");
    } else if (mode.value === "blacklist") {
      await client.adminDeleteBlacklist(row.node_id, row.local_username);
      ElMessage.success("黑名单删除成功");
    } else {
      await client.adminDeleteExemptions(row.node_id, row.local_username);
      ElMessage.success("豁免账号删除成功");
    }
    await reload();
  } catch (e: any) {
    if (e === "cancel" || e === "close") return;
    error.value = e?.message ?? String(e);
  }
}

reload();
loadNodes();
watch(mode, () => {
  reload();
});
</script>

<style scoped>
.head { display: flex; justify-content: space-between; align-items: center; }
.mb { margin-bottom: 12px; }
</style>
