<template>
  <div class="user-fun-page">
    <div class="user-fun-bg">
      <div class="user-fun-flow a" />
      <div class="user-fun-flow b" />
      <div class="user-fun-blob a" />
      <div class="user-fun-blob b" />
      <div class="user-fun-spark a" />
      <div class="user-fun-spark b" />
      <div class="user-fun-sticker left">立即生效</div>
      <div class="user-fun-sticker right">识别更准确</div>
    </div>
    <el-card class="user-fun-card accounts-card">
      <template #header>
        <div class="head">
          <div>
            <h2 class="user-fun-head-title">我的节点账号映射</h2>
            <p class="user-fun-head-sub">用于识别你在哪些节点有账号，无需管理员审核即可生效。</p>
          </div>
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
        </div>
      </template>
      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
      <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />
      <el-alert
        title="示例：如果你在 66666 端口上有一个叫 zhangsan 的账号，请填写：节点编号 66666，节点账号 zhangsan。"
        type="info"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-alert
        v-if="rows.length === 0"
        title="你还没有填写任何节点账号映射。请尽快补充，否则系统可能无法正确识别你的节点使用。"
        type="warning"
        :closable="false"
        show-icon
        class="mb strong-tip"
      />
      <el-card class="mb provision-msg-card">
        <template #header>
          <div class="head">
            <div>
              <strong>节点账号开通密钥通知（平台内）</strong>
              <div class="mini">密文与解密步骤在这里，提取码请查收注册邮箱。</div>
            </div>
          </div>
        </template>
        <el-alert
          title="解密步骤：1) 点击本行“去解密” 2) 自动带入本条密文 3) 输入邮箱中的提取码 4) 下载私钥并按 SSH 命令连接。"
          type="info"
          :closable="false"
          show-icon
          class="mb"
        />
        <el-alert
          title="注意：首次点击“去解密”后开始计时，24 小时后该条密文会自动销毁（记录保留，但操作会置灰）。"
          type="warning"
          :closable="false"
          show-icon
          class="mb"
        />
        <el-table :data="provisionMessages" stripe size="small" max-height="260">
          <el-table-column prop="created_at" label="开通时间" min-width="170" />
          <el-table-column prop="node_id" label="节点编号" width="120" />
          <el-table-column prop="local_username" label="节点账号" width="140" />
          <el-table-column prop="ssh_host" label="SSH 主机" min-width="160" />
          <el-table-column prop="ssh_port" label="端口" width="80" />
          <el-table-column label="密文状态" width="180">
            <template #default="{ row }">
              <div class="msg-status-cell">
                <el-tag v-if="isMessageDestroyed(row)" type="info" effect="plain">已销毁</el-tag>
                <el-tag v-else-if="row.first_decrypted_at" type="warning" effect="plain">倒计时中</el-tag>
                <el-tag v-else type="success" effect="plain">未开始</el-tag>
                <div v-if="row.destroy_after_at" class="msg-status-deadline">销毁时间：{{ formatDestroyAfter(row) }}</div>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="download_filename" label="建议文件名" min-width="160" />
          <el-table-column label="操作" width="260">
            <template #default="{ row }">
              <el-button
                size="small"
                type="primary"
                :disabled="isMessageDestroyed(row)"
                @click="openDecryptorWithPayload(row)"
              >
                去解密
              </el-button>
              <el-button size="small" :disabled="isMessageDestroyed(row)" @click="copyPayload(row)">
                复制密文
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
      <div class="section-inline-title">
        <span class="title">① 先添加你已有的节点账号映射（推荐优先完成）</span>
      </div>
      <el-alert
        title="请优先添加你已经拥有的节点账号映射，这一步无需审核且立即生效。下面“申请账号”仅用于没有任何节点账号的新生。"
        type="warning"
        :closable="false"
        show-icon
        class="mb strong-tip"
      />
      <el-alert
        title="若该节点账号已被他人绑定，系统会拒绝提交并提示具体账号。冒充绑定他人节点账号将被追责。"
        type="error"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-form inline class="editor">
        <el-form-item label="节点编号">
          <el-input v-model="nodeId" style="width: 220px" placeholder="例如 66666" />
        </el-form-item>
        <el-form-item label="节点账号">
          <el-input v-model="localUsername" style="width: 260px" placeholder="例如 zhangsan" />
        </el-form-item>
        <el-form-item><el-button type="primary" @click="add">{{ isEditing ? "保存修改" : "新增/覆盖" }}</el-button></el-form-item>
      </el-form>
      <el-table :data="rows" stripe class="table">
        <el-table-column prop="node_id" label="节点编号" width="160" />
        <el-table-column prop="local_username" label="节点账号" width="200" />
        <el-table-column prop="billing_username" label="平台账号" width="180" />
        <el-table-column prop="updated_at" label="更新时间" min-width="190" />
        <el-table-column label="操作" width="220">
          <template #default="{ row }">
            <el-button size="small" @click="prefill(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-divider v-if="rows.length === 0" />
      <el-card v-if="rows.length === 0" class="mb open-request-card">
        <template #header>
          <div class="head">
            <div>
              <strong>② 新生节点账号申请（无已有节点账号时使用）</strong>
              <div class="mini">此申请用于“还没有任何节点账号”的新生；同一时刻最多 1 条待审核申请。</div>
            </div>
          </div>
        </template>
        <el-alert
          title="本申请不需要你填写节点编号和节点账号，管理员会按你的说明分配。开通理由必须原文包含“研究方向”四个字，并写清课题内容和资源用途。"
          type="warning"
          :closable="false"
          show-icon
          class="mb"
        />
        <el-alert
          v-if="pendingOpenRequest"
          :title="`你已有待审核申请（ID ${pendingOpenRequest.request_id}，提交时间 ${pendingOpenRequest.created_at}），请勿重复提交。`"
          type="info"
          :closable="false"
          show-icon
          class="mb"
        />
        <el-form label-position="top">
          <el-form-item label="开通理由（必填，至少 20 字，且必须包含“研究方向”这四个字）">
            <el-input
              v-model="openReason"
              type="textarea"
              :rows="5"
              maxlength="800"
              show-word-limit
              :placeholder="openReasonPlaceholder"
            />
          </el-form-item>
          <el-button type="primary" :loading="openRequesting" :disabled="!!pendingOpenRequest" @click="submitOpenRequest">
            提交节点开通申请
          </el-button>
        </el-form>
        <el-divider />
        <div class="mini mb">我的节点开通申请记录</div>
        <el-table :data="userOpenRequests" stripe size="small" max-height="220">
          <el-table-column prop="request_id" label="ID" width="80" />
          <el-table-column label="分配节点" width="120">
            <template #default="{ row }">{{ (row.node_id || "").trim() || "-" }}</template>
          </el-table-column>
          <el-table-column label="分配账号" width="140">
            <template #default="{ row }">{{ (row.local_username || "").trim() || "-" }}</template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="110" />
          <el-table-column prop="message" label="开通理由" min-width="260" />
          <el-table-column prop="created_at" label="提交时间" min-width="170" />
        </el-table>
      </el-card>
    </el-card>
    <el-dialog v-model="decryptVisible" title="节点密钥解密" width="880px" @close="closeDecryptor">
      <p class="decrypt-note">输入邮件中的“加密密钥串”和“提取码”，解密后可下载 `ssh -i` 直接使用的私钥文件。</p>
      <KeyDecryptorPanel :initial-payload="decryptPayload" :initial-code="decryptCode" />
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { useRoute, useRouter } from "vue-router";
import { ApiClient, type UserNodeAccount, type UserProvisionMessage, type UserRequest } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import KeyDecryptorPanel from "../../components/KeyDecryptorPanel.vue";

const router = useRouter();
const route = useRoute();

const loading = ref(false);
const error = ref("");
const success = ref("");
const rows = ref<UserNodeAccount[]>([]);
const provisionMessages = ref<UserProvisionMessage[]>([]);
const userOpenRequests = ref<UserRequest[]>([]);
const nodeId = ref("");
const localUsername = ref("");
const editOldNode = ref("");
const editOldUser = ref("");
const isEditing = computed(() => !!(editOldNode.value && editOldUser.value));
const decryptVisible = ref(false);
const selectedPayload = ref("");
const selectedCode = ref("");
const openRequesting = ref(false);
const openReason = ref("");
const openReasonPlaceholder = [
  "请详细填写（至少 20 字）：",
  "1) 研究方向：请直接写出“研究方向”这四个字，再写具体方向",
  "2) 当前课题/项目名称",
  "3) 预计使用时长与频率",
  "4) 主要使用场景（训练/推理/数据处理等）",
].join("\n");

const pendingOpenRequest = computed(() =>
  (userOpenRequests.value || []).find((x) => x.request_type === "open" && x.status === "pending"),
);

const decryptPayload = computed(() => {
  const fromQuery = String(route.query.payload || "");
  if (fromQuery.trim()) return fromQuery;
  return selectedPayload.value;
});
const decryptCode = computed(() => {
  const fromQuery = String(route.query.code || "");
  if (fromQuery.trim()) return fromQuery;
  return selectedCode.value;
});

onMounted(() => {
  if (String(route.query.tool || "").trim() === "key-decryptor") {
    decryptVisible.value = true;
  }
});

watch(
  () => route.query.tool,
  (v) => {
    if (String(v || "").trim() === "key-decryptor") {
      decryptVisible.value = true;
    }
  },
);

function isMessageDestroyed(row: UserProvisionMessage): boolean {
  if (!row) return true;
  if (String(row.destroyed_at || "").trim()) return true;
  if (!String(row.encrypted_payload || "").trim()) return true;
  const destroyAfterText = String(row.destroy_after_at || "").trim();
  if (!destroyAfterText) return false;
  const destroyAfter = new Date(destroyAfterText);
  if (Number.isNaN(destroyAfter.getTime())) return false;
  return Date.now() >= destroyAfter.getTime();
}

function formatDestroyAfter(row: UserProvisionMessage): string {
  const t = String(row?.destroy_after_at || "").trim();
  if (!t) return "";
  const d = new Date(t);
  if (Number.isNaN(d.getTime())) return t;
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  const hh = String(d.getHours()).padStart(2, "0");
  const mi = String(d.getMinutes()).padStart(2, "0");
  const ss = String(d.getSeconds()).padStart(2, "0");
  return `${yyyy}-${mm}-${dd} ${hh}:${mi}:${ss}`;
}

async function openDecryptorWithPayload(row: UserProvisionMessage) {
  if (isMessageDestroyed(row)) {
    await ElMessageBox.alert("该密钥通知已销毁，无法继续解密。请联系管理员重新下发。", "已销毁", {
      type: "warning",
      confirmButtonText: "我知道了",
    });
    return;
  }
  const isFirstStart = !String(row.first_decrypted_at || "").trim();
  if (isFirstStart) {
    try {
      await ElMessageBox.confirm(
        "首次点击“去解密”后将开始 24 小时倒计时，到期后该条密文会自动销毁且不可恢复。是否继续？",
        "一次性提醒",
        { type: "warning", confirmButtonText: "继续去解密", cancelButtonText: "取消" },
      );
    } catch {
      return;
    }
  }
  try {
    const r = await client().userProvisionMessageDecryptStart(Number(row.message_id || 0));
    const msg = r.message;
    const destroyAt = formatDestroyAfter(msg);
    if (destroyAt) {
      success.value = `该密文将在 ${destroyAt} 自动销毁，请尽快完成下载与保存`;
    } else {
      success.value = "已进入解密流程，请尽快完成下载与保存";
    }
    selectedPayload.value = String(msg.encrypted_payload || "").trim();
    if (!selectedPayload.value) {
      await reload();
      await ElMessageBox.alert("该密钥通知已销毁，无法继续解密。请联系管理员重新下发。", "已销毁", {
        type: "warning",
        confirmButtonText: "我知道了",
      });
      return;
    }
    selectedCode.value = "";
    decryptVisible.value = true;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
    await ElMessageBox.alert(error.value, "无法解密", { type: "error", confirmButtonText: "我知道了" });
  }
}

async function copyPayload(row: UserProvisionMessage) {
  if (isMessageDestroyed(row)) {
    ElMessage.warning("该密文已销毁，无法复制");
    return;
  }
  const text = String(row.encrypted_payload || "").trim();
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
    ElMessage.success("密文已复制");
  } catch {
    ElMessage.error("复制失败，请手动复制");
  }
}

async function closeDecryptor() {
  selectedPayload.value = "";
  selectedCode.value = "";
  const tool = String(route.query.tool || "").trim();
  if (tool !== "key-decryptor") return;
  const nextQuery: Record<string, any> = { ...route.query };
  delete nextQuery.tool;
  delete nextQuery.payload;
  delete nextQuery.code;
  await router.replace({ path: "/user/accounts", query: nextQuery });
}

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const r = await client().userAccounts();
    rows.value = r.accounts ?? [];
    const m = await client().userProvisionMessages(120);
    provisionMessages.value = m.messages ?? [];
    const billing = String(authState.username || "").trim();
    if (billing) {
      const reqs = await client().userRequests(billing, 200);
      userOpenRequests.value = (reqs.requests ?? []).filter((x) => String(x.request_type || "") === "open");
    } else {
      userOpenRequests.value = [];
    }
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function submitOpenRequest() {
  error.value = "";
  success.value = "";
  const billing = String(authState.username || "").trim();
  if (!billing) {
    error.value = "当前登录状态异常，请重新登录后再提交申请";
    await ElMessageBox.alert(error.value, "提交失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  if (rows.value.length > 0) {
    error.value = "你已有节点账号映射，不能提交节点开通申请";
    await ElMessageBox.alert(error.value, "提交失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  if (pendingOpenRequest.value) {
    error.value = "你已有待审核的节点开通申请，请勿重复提交";
    await ElMessageBox.alert(error.value, "提交失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  const reason = openReason.value.trim();
  if (reason.length < 20) {
    error.value = "请详细填写开通理由（至少 20 个字，且必须包含“研究方向”四个字）";
    await ElMessageBox.alert(error.value, "提交失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  if (!reason.includes("研究方向")) {
    error.value = "开通理由必须原文包含“研究方向”这四个字";
    await ElMessageBox.alert(error.value, "提交失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  openRequesting.value = true;
  try {
    await client().createOpenRequest(billing, reason);
    success.value = "节点开通申请已提交，等待管理员审核";
    openReason.value = "";
    await reload();
    await ElMessageBox.alert(success.value, "提交成功", { type: "success", confirmButtonText: "我知道了" });
  } catch (e: any) {
    error.value = e?.message ?? String(e);
    await ElMessageBox.alert(error.value, "提交失败", { type: "error", confirmButtonText: "我知道了" });
  } finally {
    openRequesting.value = false;
  }
}

function prefill(row: UserNodeAccount) {
  editOldNode.value = row.node_id;
  editOldUser.value = row.local_username;
  nodeId.value = row.node_id;
  localUsername.value = row.local_username;
}

function parseMappingOwnershipConflict(e: any): { matched: boolean; node: string; local: string; mappedBilling: string } {
  if (Number(e?.status || 0) !== 409) return { matched: false, node: "", local: "", mappedBilling: "" };
  try {
    const body = JSON.parse(String(e?.body || "{}"));
    if (String(body?.reason || "").trim() !== "mapping_exists_other_user") {
      return { matched: false, node: "", local: "", mappedBilling: "" };
    }
    return {
      matched: true,
      node: String(body?.node_id || "").trim(),
      local: String(body?.local_username || "").trim(),
      mappedBilling: String(body?.mapped_billing_username || "").trim(),
    };
  } catch {
    return { matched: false, node: "", local: "", mappedBilling: "" };
  }
}

async function add() {
  error.value = "";
  success.value = "";
  try {
    const node = nodeId.value.trim();
    const local = localUsername.value.trim();
    if (!node || !local) {
      error.value = "请先填写节点编号和节点账号，再保存映射";
      return;
    }
    if (editOldNode.value && editOldUser.value) {
      try {
        await ElMessageBox.confirm(
          `确认修改该映射吗？\n原映射：节点 ${editOldNode.value} / 账号 ${editOldUser.value}\n新映射：节点 ${node} / 账号 ${local}\n\n若填写错误，在已开启 SSH 拦截的节点上可能暂时无法登录该节点账号。`,
          "二次确认",
          { type: "warning", confirmButtonText: "确认修改", cancelButtonText: "取消" },
        );
      } catch {
        return;
      }
      await client().userUpdateAccount({
        old_node_id: editOldNode.value,
        old_local_username: editOldUser.value,
        new_node_id: node,
        new_local_username: local,
      });
      editOldNode.value = "";
      editOldUser.value = "";
    } else {
      await client().userUpsertAccount(node, local);
    }
    success.value = "保存成功（此映射无需管理员审核，立即生效）";
    nodeId.value = "";
    localUsername.value = "";
    await reload();
  } catch (e: any) {
    const conflict = parseMappingOwnershipConflict(e);
    if (conflict.matched) {
      const node = conflict.node || nodeId.value.trim();
      const local = conflict.local || localUsername.value.trim();
      const mapped = conflict.mappedBilling || "未知平台账号";
      error.value = `节点 ${node} 的账号 ${local} 已绑定到平台账号 ${mapped}，严禁冒充绑定，冒充行为将追责`;
      await ElMessageBox.alert(
        `节点 ${node} 的账号 ${local} 已绑定到平台账号 ${mapped}。\n\n严禁冒充绑定：冒充行为会被审计记录并追责，请联系管理员核验处理。`,
        "绑定失败",
        { type: "error", confirmButtonText: "我知道了" },
      );
      return;
    }
    error.value = e?.message ?? String(e);
  }
}

async function remove(row: UserNodeAccount) {
  error.value = "";
  success.value = "";
  try {
    await ElMessageBox.confirm(
      `确认删除该映射吗？\n节点 ${row.node_id} / 账号 ${row.local_username}\n\n删除后，在已开启 SSH 拦截的节点上该账号将无法通过平台规则登录。`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认删除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    await client().userDeleteAccount(row.node_id, row.local_username);
    success.value = "删除成功";
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
    if (Number(e?.status || 0) === 409) {
      await ElMessageBox.alert(error.value, "绑定冲突警告", { type: "warning", confirmButtonText: "我知道了" });
    }
  }
}

reload();
</script>

<style scoped>
.accounts-card {
  min-height: 500px;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}
.section-inline-title {
  margin: 8px 0 10px;
  padding: 8px 10px;
  border-left: 4px solid #2563eb;
  background: #eff6ff;
  border-radius: 8px;
}
.section-inline-title .title {
  font-weight: 800;
  color: #0f172a;
}
.editor {
  margin-top: 4px;
}
.mb { margin-bottom: 12px; }
.strong-tip {
  border: 2px dashed #f59e0b;
}
.table {
  margin-top: 8px;
}
.mini {
  margin-top: 4px;
  font-size: 12px;
  color: #64748b;
}
.msg-status-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.msg-status-deadline {
  font-size: 11px;
  color: #64748b;
  line-height: 1.3;
}
.decrypt-note {
  margin: 0 0 10px;
  color: #334155;
}

@media (max-width: 900px) {
  .head {
    flex-wrap: wrap;
  }
}
</style>
