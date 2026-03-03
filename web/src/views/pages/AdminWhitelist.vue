<template>
  <el-card>
    <template #header>
      <div class="head">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><List /></el-icon></span>
          <span>SSH 名单</span>
        </div>
        <el-button :loading="loading" type="primary" @click="reload(true)">刷新</el-button>
      </div>
    </template>
    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert
      title="规则说明：白名单/豁免名单与黑名单不能冲突。若发生冲突，以黑名单为最高优先级；系统会自动跳过或移除冲突项。"
      type="warning"
      show-icon
      class="mb"
    />
    <el-alert
      v-if="conflictHints.length > 0"
      :title="`检测到历史冲突 ${conflictHints.length} 项（黑名单优先）。建议清理冗余记录。`"
      type="error"
      show-icon
      class="mb"
      :description="conflictHints.slice(0, 10).join('；')"
    />
    <el-alert
      title="支持两种添加方式：按节点账号添加，或按平台账号添加（自动展开该平台账号在节点上的账号）。node_id=* 表示所有节点。"
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
      <el-form-item label="节点编号">
        <el-select v-model="nodeId" filterable style="width: 220px">
          <el-option label="所有节点 (*)" value="*" />
          <el-option v-for="id in nodeOptions" :key="id" :label="id" :value="id" />
        </el-select>
        <el-text type="info" size="small" class="hint-text">提示：选择 `*` 表示该名单类型下的全部节点用户。</el-text>
      </el-form-item>
      <el-form-item label="添加方式">
        <el-radio-group v-model="addMode">
          <el-radio-button label="local">按节点账号添加</el-radio-button>
          <el-radio-button label="platform">按平台账号添加</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item v-if="addMode === 'local'" label="节点账号">
        <el-select
          v-model="selectedLocalUsers"
          multiple
          filterable
          allow-create
          default-first-option
          clearable
          style="width: 320px"
          placeholder="输入或选择节点账号"
        >
          <el-option v-for="u in localUserOptions" :key="u" :label="u" :value="u" />
        </el-select>
        <el-text type="info" size="small" class="hint-text">提示：输入节点账号后请按 `Enter` 确认加入列表，否则不会提交。</el-text>
      </el-form-item>
      <el-form-item v-if="addMode === 'platform'" label="平台账号">
        <el-select
          v-model="selectedBillingUsers"
          multiple
          filterable
          allow-create
          default-first-option
          clearable
          style="width: 320px"
          placeholder="输入或选择平台账号"
        >
          <el-option v-for="u in billingUserOptions" :key="u" :label="u" :value="u" />
        </el-select>
        <el-text type="info" size="small" class="hint-text">提示：输入平台账号后请按 `Enter` 确认加入列表，否则不会提交。</el-text>
      </el-form-item>
      <el-form-item v-if="addMode === 'platform'" label="展开账号">
        <el-select
          v-model="selectedPlatformAccountKeys"
          multiple
          filterable
          clearable
          style="width: 480px"
          placeholder="默认 ALL（添加该平台账号的全部节点账号）"
        >
          <el-option label="ALL（该平台账号下全部节点账号）" :value="ALL_PLATFORM_KEY" />
          <el-option
            v-for="item in platformAccountRows"
            :key="item.key"
            :label="`${item.billing_username} / 节点 ${item.node_id} / 账号 ${item.local_username}`"
            :value="item.key"
          />
        </el-select>
        <el-text type="info" size="small" class="hint-text">
          选择 ALL 时会自动添加该平台账号全部节点账号；不选 ALL 时按你勾选的具体节点账号添加。
        </el-text>
      </el-form-item>
      <el-form-item label="理由">
        <el-input v-model="reason" placeholder="可选，默认空" style="width: 280px" />
      </el-form-item>
      <el-form-item>
        <el-button :type="mode === 'blacklist' ? 'danger' : 'primary'" @click="save">
          {{ saveButtonText }}
        </el-button>
      </el-form-item>
    </el-form>
    <el-alert
      :title="addMode === 'local' ? '当前为“按节点账号添加”：你填写的就是实际 Linux 节点账号。' : '当前为“按平台账号添加”：系统会自动展开该平台账号在节点上的账号并写入名单。'"
      type="warning"
      :closable="false"
      class="mb"
    />

    <el-form inline>
      <el-form-item label="按节点筛选">
        <el-input v-model="filterNode" placeholder="留空全部" />
      </el-form-item>
      <el-form-item><el-button @click="reload(true)">查询</el-button></el-form-item>
    </el-form>

    <el-table :data="currentRows" stripe>
      <el-table-column prop="node_id" label="节点编号" width="120" />
      <el-table-column prop="local_username" label="节点账号" width="180" />
      <el-table-column label="对应平台账号" min-width="180">
        <template #default="{ row }">
          <el-button v-if="row.billing_username" link type="primary" @click="openProfile(row.billing_username)">{{ row.billing_username }}</el-button>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column label="添加方式" width="120">
        <template #default="{ row }">
          <el-tag :type="row.source_type === 'platform' ? 'success' : 'info'" effect="plain">
            {{ row.source_type === "platform" ? "平台账号" : "节点账号" }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="reason" label="理由" min-width="220" />
      <el-table-column prop="created_by" label="创建人" width="160" />
      <el-table-column prop="updated_at" label="更新时间" min-width="180" />
      <el-table-column label="操作" width="120">
        <template #default="{ row }">
          <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <PlatformUserDetailDialog v-model="profileVisible" :username="selectedProfileUsername" />
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ApiClient, type SSHBlacklistEntry, type SSHExemptionEntry, type SSHWhitelistEntry } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { List } from "@element-plus/icons-vue";

const loading = ref(false);
const error = ref("");
const whitelistRows = ref<SSHWhitelistEntry[]>([]);
const blacklistRows = ref<SSHBlacklistEntry[]>([]);
const exemptionRows = ref<SSHExemptionEntry[]>([]);
const nodeOptions = ref<string[]>([]);
const mode = ref<"whitelist" | "blacklist" | "exemptions">("whitelist");

const nodeId = ref("*");
const addMode = ref<"local" | "platform">("local");
const selectedLocalUsers = ref<string[]>([]);
const selectedBillingUsers = ref<string[]>([]);
const reason = ref("");
const filterNode = ref("");
const localUserOptions = ref<string[]>([]);
const billingUserOptions = ref<string[]>([]);
const platformAccountRows = ref<Array<{ key: string; billing_username: string; node_id: string; local_username: string }>>([]);
const selectedPlatformAccountKeys = ref<string[]>([]);
const profileVisible = ref(false);
const selectedProfileUsername = ref("");
const ALL_PLATFORM_KEY = "__ALL__";
const SSH_LIST_MODE_KEY = "ssh_list_mode";

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
const conflictHints = computed(() => {
  const rows = [
    ...(whitelistRows.value || []).map((x) => ({ list: "白名单", node: x.node_id, user: x.local_username })),
    ...(blacklistRows.value || []).map((x) => ({ list: "黑名单", node: x.node_id, user: x.local_username })),
    ...(exemptionRows.value || []).map((x) => ({ list: "豁免名单", node: x.node_id, user: x.local_username })),
  ];
  const hints: string[] = [];
  const overlap = (a: string, b: string) => a === b || a === "*" || b === "*";
  for (let i = 0; i < rows.length; i++) {
    for (let j = i + 1; j < rows.length; j++) {
      if (rows[i].list === rows[j].list) continue;
      if (rows[i].user !== rows[j].user) continue;
      if (!overlap(rows[i].node, rows[j].node)) continue;
      hints.push(`${rows[i].user}（${rows[i].list}:${rows[i].node} vs ${rows[j].list}:${rows[j].node}）`);
    }
  }
  return uniqSorted(hints);
});

function setModeQuery(m: "whitelist" | "blacklist" | "exemptions") {
  const u = new URL(window.location.href);
  u.searchParams.set("mode", m);
  window.history.replaceState(null, "", `${u.pathname}${u.search}${u.hash}`);
}

function getInitMode(): "whitelist" | "blacklist" | "exemptions" {
  const u = new URL(window.location.href);
  const q = (u.searchParams.get("mode") || "").trim();
  if (q === "whitelist" || q === "blacklist" || q === "exemptions") return q;
  const saved = (localStorage.getItem(SSH_LIST_MODE_KEY) || "").trim();
  if (saved === "whitelist" || saved === "blacklist" || saved === "exemptions") return saved;
  return "whitelist";
}

async function reload(showToast = false) {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const [wl, bl, ex] = await Promise.all([
      client.adminWhitelist(filterNode.value.trim()),
      client.adminBlacklist(filterNode.value.trim()),
      client.adminExemptions(filterNode.value.trim()),
    ]);
    whitelistRows.value = wl.entries ?? [];
    blacklistRows.value = bl.entries ?? [];
    exemptionRows.value = ex.entries ?? [];
    if (showToast) {
      ElMessage.success(`刷新成功：白名单 ${whitelistRows.value.length}，黑名单 ${blacklistRows.value.length}，豁免名单 ${exemptionRows.value.length}`);
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

function uniqSorted(items: string[]): string[] {
  const s = new Set<string>();
  for (const x of items) {
    const v = (x || "").trim();
    if (!v) continue;
    s.add(v);
  }
  return Array.from(s).sort((a, b) => a.localeCompare(b));
}

async function loadSuggestions() {
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const [accounts, wl, bl, ex] = await Promise.all([
      client.adminAccounts(""),
      client.adminWhitelist(""),
      client.adminBlacklist(""),
      client.adminExemptions(""),
    ]);
    const locals = [
      ...(accounts.accounts ?? []).map((x) => x.local_username),
      ...(wl.entries ?? []).map((x) => x.local_username),
      ...(bl.entries ?? []).map((x) => x.local_username),
      ...(ex.entries ?? []).map((x) => x.local_username),
    ];
    const billing = [
      ...(accounts.accounts ?? []).map((x) => x.billing_username),
    ];
    localUserOptions.value = uniqSorted(locals);
    billingUserOptions.value = uniqSorted(billing);
  } catch {
    localUserOptions.value = [];
    billingUserOptions.value = [];
  }
}

async function loadPlatformAccountRows() {
  platformAccountRows.value = [];
  if (addMode.value !== "platform") return;
  const users = uniqSorted(selectedBillingUsers.value);
  if (users.length === 0) return;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const rows: Array<{ key: string; billing_username: string; node_id: string; local_username: string }> = [];
    for (const u of users) {
      const r = await client.adminAccounts(u);
      const matched = (r.accounts ?? []).filter((x) => String(x.billing_username || "").trim() === u);
      for (const it of matched) {
        const node = String(it.node_id || "").trim();
        const local = String(it.local_username || "").trim();
        if (!node || !local) continue;
        if (nodeId.value !== "*" && node !== nodeId.value) continue;
        rows.push({ key: `${u}|${node}|${local}`, billing_username: u, node_id: node, local_username: local });
      }
    }
    const dedup = new Map<string, { key: string; billing_username: string; node_id: string; local_username: string }>();
    for (const it of rows) dedup.set(it.key, it);
    platformAccountRows.value = Array.from(dedup.values()).sort((a, b) => a.key.localeCompare(b.key));
    selectedPlatformAccountKeys.value = [ALL_PLATFORM_KEY];
  } catch {
    platformAccountRows.value = [];
    selectedPlatformAccountKeys.value = [ALL_PLATFORM_KEY];
  }
}

async function save() {
  error.value = "";
  let names = addMode.value === "local" ? uniqSorted(selectedLocalUsers.value) : [];
  let billingNames = addMode.value === "platform" ? uniqSorted(selectedBillingUsers.value) : [];
  let platformAccounts: Array<{ billing_username: string; node_id: string; local_username: string }> = [];
  if (addMode.value === "platform") {
    const hasAll = selectedPlatformAccountKeys.value.includes(ALL_PLATFORM_KEY);
    if (!hasAll && selectedPlatformAccountKeys.value.length > 0) {
      const selectedSet = new Set(selectedPlatformAccountKeys.value);
      platformAccounts = platformAccountRows.value
        .filter((x) => selectedSet.has(x.key))
        .map((x) => ({ billing_username: x.billing_username, node_id: x.node_id, local_username: x.local_username }));
      billingNames = [];
    }
  }
  if (names.length === 0 && billingNames.length === 0 && platformAccounts.length === 0) {
    error.value = addMode.value === "local"
      ? "请至少输入一个节点账号（输入后请按 Enter 确认）"
      : "请至少输入一个平台账号（输入后请按 Enter 确认）";
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    if (mode.value === "whitelist") {
      const r = await client.adminUpsertWhitelist(nodeId.value.trim(), names, billingNames, reason.value.trim(), platformAccounts);
      if ((r.skipped_due_blacklist ?? []).length > 0) {
        ElMessage.warning(`白名单已保存 ${r.applied} 条，${(r.skipped_due_blacklist ?? []).length} 条因黑名单优先被跳过`);
      } else {
        ElMessage.success("白名单保存成功，节点将快速同步");
      }
    } else if (mode.value === "blacklist") {
      const r = await client.adminUpsertBlacklist(nodeId.value.trim(), names, billingNames, reason.value.trim(), platformAccounts);
      const rmW = Number(r.removed_from_whitelist || 0);
      const rmE = Number(r.removed_from_exemptions || 0);
      if (rmW > 0 || rmE > 0) {
        ElMessage.warning(`黑名单保存成功（黑名单优先），已移除冲突项：白名单 ${rmW}，豁免 ${rmE}`);
      } else {
        ElMessage.success("黑名单保存成功，已下发断开SSH会话指令");
      }
    } else {
      const r = await client.adminUpsertExemptions(nodeId.value.trim(), names, billingNames, reason.value.trim(), platformAccounts);
      if ((r.skipped_due_blacklist ?? []).length > 0) {
        ElMessage.warning(`豁免账号已保存 ${r.applied} 条，${(r.skipped_due_blacklist ?? []).length} 条因黑名单优先被跳过`);
      } else {
        ElMessage.success("豁免账号保存成功，节点将快速同步");
      }
    }
    selectedLocalUsers.value = [];
    selectedBillingUsers.value = [];
    selectedPlatformAccountKeys.value = [ALL_PLATFORM_KEY];
    platformAccountRows.value = [];
    reason.value = "";
    await loadSuggestions();
    await reload(true);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

function openProfile(username: string) {
  selectedProfileUsername.value = String(username || "").trim();
  if (!selectedProfileUsername.value) return;
  profileVisible.value = true;
}

async function remove(row: SSHWhitelistEntry | SSHBlacklistEntry | SSHExemptionEntry) {
  error.value = "";
  try {
    await ElMessageBox.confirm(
      mode.value === "whitelist"
        ? `确认删除白名单节点账号 ${row.local_username}（节点 ${row.node_id}）吗？系统将同时尝试断开其现有SSH会话。`
        : mode.value === "blacklist"
          ? `确认删除黑名单节点账号 ${row.local_username}（节点 ${row.node_id}）吗？`
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
    await loadSuggestions();
    await reload(true);
  } catch (e: any) {
    if (e === "cancel" || e === "close") return;
    error.value = e?.message ?? String(e);
  }
}

onMounted(() => {
  mode.value = getInitMode();
  setModeQuery(mode.value);
  reload();
  loadNodes();
  loadSuggestions();
});
watch(mode, (v) => {
  localStorage.setItem(SSH_LIST_MODE_KEY, v);
  setModeQuery(v);
  reload();
});
watch(addMode, (v) => {
  if (v === "local") selectedBillingUsers.value = [];
  if (v === "platform") {
    selectedLocalUsers.value = [];
    selectedPlatformAccountKeys.value = [ALL_PLATFORM_KEY];
    loadPlatformAccountRows();
  } else {
    platformAccountRows.value = [];
    selectedPlatformAccountKeys.value = [];
  }
});
watch([selectedBillingUsers, nodeId], () => {
  if (addMode.value === "platform") {
    loadPlatformAccountRows();
  }
});
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
}
.mb { margin-bottom: 12px; }
.hint-text { margin-left: 8px; }
</style>
