<template>
  <div class="user-fun-page">
    <div class="user-fun-bg">
      <div class="user-fun-flow a" />
      <div class="user-fun-flow b" />
      <div class="user-fun-blob a" />
      <div class="user-fun-blob b" />
      <div class="user-fun-spark a" />
      <div class="user-fun-spark b" />
      <div class="user-fun-sticker left">5分钟校验</div>
      <div class="user-fun-sticker right">防冒充绑定</div>
    </div>
    <el-card class="user-fun-card accounts-card">
      <template #header>
        <div class="head">
          <div>
            <h2 class="user-fun-head-title">
              <span>我的节点账号映射</span>
              <span v-if="headerNeedAttention" class="menu-red-dot" aria-label="账号已开通提醒" />
            </h2>
            <p class="user-fun-head-sub">按步骤完成映射即可开始正常使用节点。</p>
          </div>
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
        </div>
      </template>
      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
      <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />
      <div v-if="showFirstGuideRedDots" class="first-guide-banner mb">
        <div class="first-guide-title">
          <span class="menu-red-dot inline-dot" />
          <span>首次登录请先看这里</span>
        </div>
        <div class="first-guide-body">
          已经有节点账号时，用“绑定已有账号”；从未分配过节点账号时，直接用“申请新账号”，不需要 challenge。
        </div>
        <el-button size="small" type="success" plain class="first-guide-dismiss" @click="dismissFirstGuide">✓ 已了解</el-button>
      </div>
      <el-card v-if="hasProvisionMessages" class="mb provision-msg-card">
        <template #header>
          <div class="head">
            <div>
              <strong>
                节点账号开通密钥通知（平台内）
                <span v-if="hasNewProvisionMessage" class="menu-red-dot inline-dot" aria-label="账号已开通提醒" />
              </strong>
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
          <el-table-column prop="created_at" label="开通时间" min-width="170" :formatter="tableTimeFormatter" />
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
                <div v-if="row.destroy_after_at" class="msg-status-deadline">销毁时间：{{ formatServerDateTime(row.destroy_after_at) }}</div>
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
      <el-tabs v-model="accountMode" class="account-tabs">
        <el-tab-pane label="绑定已有节点账号" name="existing">
          <div class="path-grid mb">
            <div class="path-panel">
              <div class="panel-head">
                <strong>发起 challenge</strong>
                <span class="mini">仅用于你已经拥有、且当前能 SSH 登录的节点账号。</span>
              </div>
              <el-alert
                v-if="bindCooldownUntil"
                :title="`当前账号在绑定冷却中，冷却结束时间：${bindCooldownUntil}`"
                type="warning"
                :closable="false"
                show-icon
                class="mb"
              />
              <el-form label-position="top" class="compact-form">
                <div class="form-row">
                  <el-form-item label="节点编号">
                    <el-input v-model="nodeId" placeholder="例如 66666" />
                  </el-form-item>
                  <el-form-item label="节点账号">
                    <el-input v-model="localUsername" placeholder="例如 zhangsan" />
                  </el-form-item>
                </div>
                <el-button type="primary" @click="add">发起 challenge</el-button>
              </el-form>
            </div>

            <div class="path-panel challenge-panel">
              <div class="panel-head">
                <strong>当前 challenge</strong>
                <span class="mini">创建后 5 分钟内登录节点执行命令。</span>
              </div>
              <div v-if="activeChallenge" class="challenge-command-box">
                <div class="challenge-meta">
                  <el-tag type="primary" effect="plain">{{ activeChallenge.node_id }} / {{ activeChallenge.local_username }}</el-tag>
                  <span :class="['challenge-countdown-value', `is-${activeChallengeCountdownLevel}`]">
                    {{ activeChallengeCountdownText }}
                  </span>
                </div>
                <div class="command-line">
                  <code>{{ activeChallenge.claim_command }}</code>
                </div>
                <div class="payload-actions">
                  <el-button size="small" type="primary" @click="copyText(activeChallenge.claim_command)">复制命令</el-button>
                  <el-button size="small" @click="copyText(activeChallenge.challenge_token)">复制 token</el-button>
                </div>
              </div>
              <div v-else class="empty-tip">暂无进行中的 challenge。</div>
            </div>
          </div>

          <div class="path-panel mb">
            <div class="panel-head">
              <strong>已生效映射</strong>
              <span class="mini">状态为“已就绪”后表示节点侧身份已经对齐。</span>
            </div>
            <el-alert
              v-if="hasPendingAccounts"
              title="检测到账号仍在等待节点确认 UID/GID，系统会自动刷新状态。"
              type="warning"
              :closable="false"
              show-icon
              class="mb"
            />
            <el-table :data="rows" stripe class="table" empty-text="暂无已绑定节点账号">
              <el-table-column prop="node_id" label="节点编号" width="160" />
              <el-table-column prop="local_username" label="节点账号" width="200" />
              <el-table-column prop="billing_username" label="平台账号" width="180" />
              <el-table-column label="状态" width="220">
                <template #default="{ row }">
                  <div class="mapping-state-cell">
                    <el-tag v-if="row.identity_aligned" type="success" effect="light">已就绪</el-tag>
                    <el-tag v-else-if="row.identity_initializing" type="warning" effect="light">初始化中</el-tag>
                    <el-tag v-else type="info" effect="light">待同步</el-tag>
                    <div v-if="row.identity_initializing" class="mini mapping-state-tip">正在同步 UID/GID，完成前无法 SSH 登录</div>
                    <div v-else-if="!row.identity_aligned" class="mini mapping-state-tip">节点尚未回传最新 UID/GID 快照，请稍后自动刷新</div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column prop="created_at" label="生效时间" min-width="190" :formatter="tableTimeFormatter" />
              <el-table-column label="操作" min-width="260">
                <template #default="{ row }">
                  <el-button size="small" type="danger" :disabled="isUnbindSubmitBlocked(row)" @click="remove(row)">申请解绑</el-button>
                  <div v-if="isUnbindSubmitBlocked(row)" class="mini unbind-block-tip">{{ unbindSubmitBlockedMessage(row) }}</div>
                </template>
              </el-table-column>
            </el-table>
          </div>

          <el-card v-if="mappedNodeInfos.length > 0" class="mb provision-msg-card">
          <template #header>
            <div class="head">
              <div>
                <strong>已映射节点价格与存储配额</strong>
                <div class="mini">仅展示你已成功映射的节点账号。</div>
              </div>
            </div>
          </template>
          <el-alert
            title="兼容性提醒：如果少数应用在绑定或账号调整后无法正常启动，可先尝试删除 /home/{用户名}/.cache、/home/{用户名}/.config、/home/{用户名}/.local、/home/{用户名}/.vscode-server 等缓存目录后再重试，以此类推。请将 {用户名} 替换为你的节点账号。原理是删除旧缓存文件，让应用按新的系统身份重新生成并重新适配。"
            type="warning"
            :closable="false"
            show-icon
            class="mb"
          />
          <div class="quota-tip-box mb">
            <div class="quota-tip-head">存储使用提醒</div>
            <p class="quota-tip-line">
              请重点关注 <span class="blue-keyword">CPU 单价</span>、<span class="blue-keyword">GPU 单价</span> 和
              <span class="blue-keyword">配额分区</span>。超过 <span class="blue-keyword">配额分区</span> 的硬限制后会
              <span class="blue-keyword">禁止写入</span>。
            </p>
            <p class="quota-tip-line">
              建议将大文件优先放到 <span class="blue-keyword">/mnt/{你的磁盘目录}/{用户名}</span> 或
              <span class="blue-keyword">/shared/node/{用户名}</span>、
              <span class="blue-keyword">/shared/cluster/{用户名}</span>，再通过
              <span class="blue-keyword">软链接</span> 使用。
            </p>
            <p class="quota-tip-line quota-tip-subline">
              <span class="blue-keyword">/mnt</span> 下通常是节点本地硬盘，速度更快，且无需经过网络传输；能放本地大文件时，优先使用这里。
            </p>
            <p class="quota-tip-line quota-tip-subline">
              <span class="blue-keyword">/shared/cluster</span> 是所有节点共享的 NFS，
              <span class="blue-keyword">/shared/node</span> 是每个节点各自独有的目录。
            </p>
            <p class="quota-tip-line quota-tip-subline">关键数据请及时备份到本地，避免误删或覆盖。</p>
          </div>
          <el-table :data="mappedNodeInfos" stripe size="small">
            <el-table-column prop="node_id" label="节点编号" width="120" />
            <el-table-column prop="local_username" label="节点账号" width="150" />
            <el-table-column label="配额分区" width="110">
              <template #default="{ row }">{{ quotaMountLabel(row) }}</template>
            </el-table-column>
            <el-table-column label="GPU 单价(积分/卡分钟)" min-width="210">
              <template #default="{ row }">
                <div>{{ formatPrice(row.effective_gpu_price_per_minute) }}</div>
                <div class="mini">来源：{{ formatPriceSource(row.gpu_price_source, "gpu") }}</div>
              </template>
            </el-table-column>
            <el-table-column label="CPU 单价(积分/核分钟)" min-width="210">
              <template #default="{ row }">
                <div>{{ formatPrice(row.effective_cpu_price_per_core_minute) }}</div>
                <div class="mini">来源：{{ formatPriceSource(row.cpu_price_source, "cpu") }}</div>
              </template>
            </el-table-column>
            <el-table-column label="配额已用" min-width="130">
              <template #default="{ row }">{{ formatMBToGB(row.home_quota_used_mb) }}</template>
            </el-table-column>
            <el-table-column label="软配额" min-width="130">
              <template #default="{ row }">{{ formatMBToGB(row.home_quota_soft_mb) }}</template>
            </el-table-column>
            <el-table-column label="硬配额" min-width="130">
              <template #default="{ row }">{{ formatMBToGB(row.home_quota_hard_mb) }}</template>
            </el-table-column>
            <el-table-column label="写入状态" width="120">
              <template #default="{ row }">
                <el-tag :type="homeQuotaTagType(row)" effect="plain">{{ homeQuotaTagText(row) }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
        </el-tab-pane>

        <el-tab-pane label="申请新节点账号" name="open">
          <div class="path-panel mb">
            <div class="panel-head">
              <strong>新生节点账号申请</strong>
              <span class="mini">从未分配过节点账号时使用；此流程不需要 challenge。</span>
            </div>
            <el-alert
              v-if="rows.length > 0 || activeChallenge"
              title="你已有映射或正在进行 challenge，当前通常不需要提交新账号开通申请。"
              type="info"
              :closable="false"
              show-icon
              class="mb"
            />
            <el-alert
              v-if="pendingOpenRequest"
              :title="`你已有待审核申请（ID ${pendingOpenRequest.request_id}，提交时间 ${formatServerDateTime(pendingOpenRequest.created_at)}），请勿重复提交。`"
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
              <el-button type="primary" :loading="openRequesting" :disabled="!!pendingOpenRequest || rows.length > 0 || !!activeChallenge" @click="submitOpenRequest">
                提交节点开通申请
              </el-button>
            </el-form>
          </div>

          <div class="path-panel">
            <div class="panel-head">
              <strong>我的开通申请记录</strong>
              <span class="mini">管理员处理结果会显示在这里。</span>
            </div>
            <el-table :data="userOpenRequests" stripe size="small" max-height="260" empty-text="暂无开通申请">
            <el-table-column prop="request_id" label="ID" width="80" />
            <el-table-column label="分配节点" width="120">
              <template #default="{ row }">{{ (row.node_id || "").trim() || "-" }}</template>
            </el-table-column>
            <el-table-column label="分配账号" width="140">
              <template #default="{ row }">{{ (row.local_username || "").trim() || "-" }}</template>
            </el-table-column>
            <el-table-column prop="status" label="状态" width="110" />
            <el-table-column prop="message" label="开通理由" min-width="260" />
            <el-table-column label="拒绝理由" min-width="240">
              <template #default="{ row }">{{ (row.reject_reason || "").trim() || "-" }}</template>
            </el-table-column>
            <el-table-column prop="created_at" label="提交时间" min-width="170" :formatter="tableTimeFormatter" />
          </el-table>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>
    <el-dialog v-model="decryptVisible" title="节点密钥解密" width="880px" @close="closeDecryptor">
      <p class="decrypt-note">输入邮件中的“加密密钥串”和“提取码”，解密后可下载 `ssh -i` 直接使用的私钥文件。</p>
      <KeyDecryptorPanel :initial-payload="decryptPayload" :initial-code="decryptCode" />
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { useRoute, useRouter } from "vue-router";
import {
  ApiClient,
  type UserNodeAccount,
  type UserNodeBindChallengeInfo,
  type UserNodeBindCooldown,
  type UserMappedNodeInfo,
  type UserProvisionMessage,
  type UserRequest,
} from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import KeyDecryptorPanel from "../../components/KeyDecryptorPanel.vue";
import { formatServerDateTime, toServerEpochMs } from "../../lib/time";
import { writeClipboardText } from "../../lib/clipboard";

const router = useRouter();
const route = useRoute();

const loading = ref(false);
const error = ref("");
const success = ref("");
const rows = ref<UserNodeAccount[]>([]);
const mappedNodeInfos = ref<UserMappedNodeInfo[]>([]);
const activeChallenge = ref<UserNodeBindChallengeInfo | null>(null);
const bindCooldown = ref<UserNodeBindCooldown | null>(null);
const provisionMessages = ref<UserProvisionMessage[]>([]);
const userRequests = ref<UserRequest[]>([]);
const userOpenRequests = ref<UserRequest[]>([]);
const nodeId = ref("");
const localUsername = ref("");
const nowTick = ref(Date.now());
const decryptVisible = ref(false);
const selectedPayload = ref("");
const selectedCode = ref("");
const openRequesting = ref(false);
const openReason = ref("");
const firstGuideRead = ref(false);
const accountMode = ref<"existing" | "open">("open");
let challengeCountdownTimer: ReturnType<typeof setInterval> | null = null;
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null;
const USER_ACCOUNTS_GUIDE_KEY_PREFIX = "gpuops.user_accounts.guide_seen";
const openReasonPlaceholder = [
  "请详细填写（至少 20 字）：",
  "1) 研究方向：请直接写出“研究方向”这四个字，再写具体方向",
  "2) 当前课题/项目名称",
  "3) 预计使用时长与频率",
  "4) 主要使用场景（训练/推理/数据处理等）",
].join("\n");

function userAccountsGuideStorageKey(): string {
  const username = String(authState.username || "").trim().toLowerCase();
  return `${USER_ACCOUNTS_GUIDE_KEY_PREFIX}:${username || "anonymous"}`;
}

function loadFirstGuideReadState(): boolean {
  try {
    return String(localStorage.getItem(userAccountsGuideStorageKey()) || "").trim() === "1";
  } catch {
    return false;
  }
}

function dismissFirstGuide() {
  firstGuideRead.value = true;
  try {
    localStorage.setItem(userAccountsGuideStorageKey(), "1");
  } catch {
    // ignore localStorage errors
  }
}

function tableTimeFormatter(_: unknown, __: unknown, cellValue: unknown): string {
  return formatServerDateTime(String(cellValue ?? ""));
}

function formatPrice(value: unknown): string {
  const v = Number(value);
  if (!Number.isFinite(v)) return "-";
  return v.toFixed(4);
}

function formatMBToGB(value: unknown): string {
  const v = Number(value);
  if (!Number.isFinite(v)) return "-";
  return `${(v / 1024).toFixed(2)} GB`;
}

function formatPriceSource(source: string, kind: "gpu" | "cpu"): string {
  const s = String(source || "").trim();
  if (!s) return "默认";
  if (s === "node_model_override") return "节点型号价";
  if (s === "node_price_policy") return "节点GPU单价";
  if (s === "resource_prices_gpu_model") return "全局型号价";
  if (s === "node_cpu_price_policy") return "节点CPU单价";
  if (s === "resource_prices_cpu_core") return "全局CPU_CORE";
  if (s === "config_default_cpu_price") return "默认CPU单价";
  if (s === "config_default_gpu_price") return "默认GPU单价";
  return kind === "gpu" ? "GPU默认" : "CPU默认";
}

function quotaMountLabel(row: UserMappedNodeInfo): string {
  const mountpoint = String(row.home_quota_mountpoint || "").trim();
  return mountpoint || "-";
}

function homeQuotaTagType(row: UserMappedNodeInfo): "success" | "warning" | "danger" | "info" {
  const used = Number(row.home_quota_used_mb);
  const soft = Number(row.home_quota_soft_mb);
  const hard = Number(row.home_quota_hard_mb);
  if (!Number.isFinite(used) || !Number.isFinite(soft) || !Number.isFinite(hard) || (!row.home_quota_enforced && soft <= 0 && hard <= 0)) {
    return "info";
  }
  if (hard > 0 && used >= hard) return "danger";
  if (soft > 0 && used >= soft) return "warning";
  return "success";
}

function homeQuotaTagText(row: UserMappedNodeInfo): string {
  const used = Number(row.home_quota_used_mb);
  const soft = Number(row.home_quota_soft_mb);
  const hard = Number(row.home_quota_hard_mb);
  if (!Number.isFinite(used) || !Number.isFinite(soft) || !Number.isFinite(hard) || (!row.home_quota_enforced && soft <= 0 && hard <= 0)) {
    return "未上报";
  }
  if (hard > 0 && used >= hard) return "已超硬限制";
  if (soft > 0 && used >= soft) return "已超软限制";
  return "正常";
}

const pendingOpenRequest = computed(() =>
  (userOpenRequests.value || []).find((x) => x.request_type === "open" && x.status === "pending"),
);
const unbindBlockedRequestByKey = computed(() => {
  const map = new Map<string, UserRequest>();
  for (const req of userRequests.value || []) {
    if (String(req.request_type || "").trim() !== "unbind") continue;
    const status = String(req.status || "").trim();
    if (status !== "pending") continue;
    const key = mappingKey(req.node_id, req.local_username);
    if (!key) continue;
    const prev = map.get(key);
    if (!prev) {
      map.set(key, req);
      continue;
    }
    const prevTs = toServerEpochMs(String(prev.created_at || ""));
    const curTs = toServerEpochMs(String(req.created_at || ""));
    if (curTs >= prevTs) {
      map.set(key, req);
    }
  }
  return map;
});
const bindCooldownUntil = computed(() => {
  const t = String(bindCooldown.value?.cooldown_until || "").trim();
  if (!t) return "";
  return formatServerDateTime(t);
});
const activeChallengeRemainingSeconds = computed(() => {
  const expiresAt = String(activeChallenge.value?.expires_at || "").trim();
  if (!expiresAt) return null;
  const expiresMs = toServerEpochMs(expiresAt);
  if (!Number.isFinite(expiresMs)) return null;
  return Math.floor((expiresMs - nowTick.value) / 1000);
});
const activeChallengeCountdownLevel = computed(() => {
  const remain = activeChallengeRemainingSeconds.value;
  if (remain === null) return "safe";
  if (remain <= 0) return "expired";
  if (remain <= 60) return "danger";
  if (remain <= 180) return "warn";
  return "safe";
});
const activeChallengeCountdownText = computed(() => {
  const remain = activeChallengeRemainingSeconds.value;
  if (remain === null) return "-";
  if (remain <= 0) return "已超时";
  const minutes = Math.floor(remain / 60);
  const seconds = remain % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
});
const hasProvisionMessages = computed(() => (provisionMessages.value || []).length > 0);
const hasNewProvisionMessage = computed(() =>
  (provisionMessages.value || []).some((x) => !isMessageDestroyed(x) && !String(x.first_decrypted_at || "").trim()),
);
const showFirstGuideRedDots = computed(() => !firstGuideRead.value);
const headerNeedAttention = computed(() => hasNewProvisionMessage.value || showFirstGuideRedDots.value);
const hasPendingAccounts = computed(() => (rows.value || []).some((x) => !x.identity_aligned));

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
  nowTick.value = Date.now();
  challengeCountdownTimer = setInterval(() => {
    nowTick.value = Date.now();
  }, 1000);
  firstGuideRead.value = loadFirstGuideReadState();
  if (String(route.query.tool || "").trim() === "key-decryptor") {
    decryptVisible.value = true;
  }
});

onUnmounted(() => {
  if (challengeCountdownTimer) {
    clearInterval(challengeCountdownTimer);
    challengeCountdownTimer = null;
  }
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer);
    autoRefreshTimer = null;
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
  const destroyAfterMs = toServerEpochMs(destroyAfterText);
  if (Number.isNaN(destroyAfterMs)) return false;
  return Date.now() >= destroyAfterMs;
}

function mappingKey(nodeID: string, localUsername: string): string {
  return `${String(nodeID || "").trim()}::${String(localUsername || "").trim()}`;
}

function findBlockedUnbindRequest(row: UserNodeAccount): UserRequest | null {
  return unbindBlockedRequestByKey.value.get(mappingKey(row.node_id, row.local_username)) || null;
}

function isUnbindSubmitBlocked(row: UserNodeAccount): boolean {
  return !!findBlockedUnbindRequest(row);
}

function unbindSubmitBlockedMessage(row: UserNodeAccount): string {
  const req = findBlockedUnbindRequest(row);
  if (!req) return "";
  return `已申请解绑（ID ${req.request_id}）待审核，不能重复提交`;
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
    const destroyAt = formatServerDateTime(msg.destroy_after_at || "");
    if (destroyAt && destroyAt !== "-") {
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
    await writeClipboardText(text);
    ElMessage.success("密文已复制");
  } catch {
    ElMessage.error("复制失败，请手动复制");
  }
}

async function copyText(text: string) {
  const v = String(text || "").trim();
  if (!v) return;
  try {
    await writeClipboardText(v);
    ElMessage.success("已复制");
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

function syncAutoRefreshTimer() {
  const shouldPoll = !!activeChallenge.value || hasPendingAccounts.value;
  if (shouldPoll && !autoRefreshTimer) {
    autoRefreshTimer = setInterval(() => {
      void reload(true);
    }, 5000);
    return;
  }
  if (!shouldPoll && autoRefreshTimer) {
    clearInterval(autoRefreshTimer);
    autoRefreshTimer = null;
  }
}

async function reload(silent = false) {
  if (!silent) {
    loading.value = true;
    error.value = "";
  }
  try {
    const r = await client().userAccounts();
    rows.value = r.accounts ?? [];
    mappedNodeInfos.value = r.mapped_node_infos ?? [];
    activeChallenge.value = (r.active_challenge as UserNodeBindChallengeInfo) || null;
    bindCooldown.value = (r.bind_cooldown as UserNodeBindCooldown) || null;
    if (activeChallenge.value || (rows.value || []).length > 0) {
      accountMode.value = "existing";
    }
    const m = await client().userProvisionMessages(120);
    provisionMessages.value = m.messages ?? [];
    const reqs = await client().userRequests(200);
    userRequests.value = reqs.requests ?? [];
    userOpenRequests.value = userRequests.value.filter((x) => String(x.request_type || "") === "open");
    syncAutoRefreshTimer();
  } catch (e: any) {
    if (!silent) {
      error.value = e?.message ?? String(e);
    }
  } finally {
    if (!silent) {
      loading.value = false;
    }
  }
}

async function submitOpenRequest() {
  error.value = "";
  success.value = "";
  const billing = String(authState.username || "").trim();
  if (!billing) {
    error.value = "当前登录状态异常，请重新登录后再提交申请";
    return;
  }
  if (rows.value.length > 0) {
    error.value = "你已有节点账号映射，不能提交节点开通申请";
    return;
  }
  if (pendingOpenRequest.value) {
    error.value = "你已有待审核的节点开通申请，请勿重复提交";
    return;
  }
  const reason = openReason.value.trim();
  if (reason.length < 20) {
    error.value = "请详细填写开通理由（至少 20 个字，且必须包含“研究方向”四个字）";
    return;
  }
  if (!reason.includes("研究方向")) {
    error.value = "开通理由必须原文包含“研究方向”这四个字";
    return;
  }
  openRequesting.value = true;
  try {
    await client().createOpenRequest(reason);
    success.value = "节点开通申请已提交，等待管理员审核";
    openReason.value = "";
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    openRequesting.value = false;
  }
}

function parseMappingOwnershipConflict(e: any): { matched: boolean; node: string; local: string } {
  if (Number(e?.status || 0) !== 409) return { matched: false, node: "", local: "" };
  try {
    const body = JSON.parse(String(e?.body || "{}"));
    if (String(body?.reason || "").trim() !== "mapping_exists_other_user") {
      return { matched: false, node: "", local: "" };
    }
    return {
      matched: true,
      node: String(body?.node_id || "").trim(),
      local: String(body?.local_username || "").trim(),
    };
  } catch {
    return { matched: false, node: "", local: "" };
  }
}

function parseActiveChallengeConflict(e: any): { matched: boolean; node: string; local: string; expiresAt: string } {
  if (Number(e?.status || 0) !== 409) return { matched: false, node: "", local: "", expiresAt: "" };
  try {
    const body = JSON.parse(String(e?.body || "{}"));
    if (String(body?.reason || "").trim() !== "bind_active_challenge_exists") {
      return { matched: false, node: "", local: "", expiresAt: "" };
    }
    return {
      matched: true,
      node: String(body?.active_challenge_node || "").trim(),
      local: String(body?.active_challenge_local || "").trim(),
      expiresAt: String(body?.active_expires_at || "").trim(),
    };
  } catch {
    return { matched: false, node: "", local: "", expiresAt: "" };
  }
}

async function add() {
  error.value = "";
  success.value = "";
  try {
    const node = nodeId.value.trim();
    const local = localUsername.value.trim();
    if (!node || !local) {
      error.value = "请先填写节点编号和节点账号，再发起 challenge";
      return;
    }
    const r = await client().userUpsertAccount(node, local, "用户页面发起 challenge 绑定");
    activeChallenge.value = r.challenge || null;
    accountMode.value = "existing";
    success.value = `challenge 已创建：请在 5 分钟内登录节点 ${node} 的账号 ${local} 并执行页面中的 gpuops-claim 命令`;
    nodeId.value = "";
    localUsername.value = "";
    await reload();
  } catch (e: any) {
    const activeConflict = parseActiveChallengeConflict(e);
    if (activeConflict.matched) {
      error.value = "当前已有进行中的 challenge，同一时间只能存在一个绑定挑战";
      success.value = `进行中的 challenge：${activeConflict.node || "-"} / ${activeConflict.local || "-"}，到期时间 ${formatServerDateTime(activeConflict.expiresAt || "")}`;
      return;
    }
    const conflict = parseMappingOwnershipConflict(e);
    if (conflict.matched) {
      const node = conflict.node || nodeId.value.trim();
      const local = conflict.local || localUsername.value.trim();
      error.value = `节点 ${node} 的账号 ${local} 已被其他平台账号绑定，禁止换绑`;
      return;
    }
    error.value = e?.message ?? String(e);
  }
}

async function remove(row: UserNodeAccount) {
  error.value = "";
  success.value = "";
  const blockedMsg = unbindSubmitBlockedMessage(row);
  if (blockedMsg) {
    error.value = blockedMsg;
    await ElMessageBox.alert(blockedMsg, "不能重复提交解绑申请", { type: "warning", confirmButtonText: "我知道了" });
    return;
  }
  let reason = "";
  try {
    const input: any = await ElMessageBox.prompt(
      `请填写解绑理由（至少 10 个字）：\n节点 ${row.node_id} / 账号 ${row.local_username}`,
      "提交解绑申请",
      { type: "warning", confirmButtonText: "提交申请", cancelButtonText: "取消", inputPlaceholder: "例如：毕业离组、账号停用、误绑更正" },
    );
    reason = String(input?.value || "").trim();
    if (reason.length < 10) {
      ElMessage.warning("解绑理由至少 10 个字");
      return;
    }
  } catch {
    return;
  }
  try {
    await client().userDeleteAccount(row.node_id, row.local_username, reason);
    success.value = "解绑申请已提交，等待管理员审核";
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
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
.user-fun-head-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.menu-red-dot {
  display: inline-block;
  width: 9px;
  height: 9px;
  border-radius: 999px;
  background: #ef4444;
  box-shadow: 0 0 0 2px rgba(239, 68, 68, 0.2);
  vertical-align: middle;
}
.inline-dot {
  margin-left: 6px;
}
.first-guide-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding: 10px 12px;
  border: 1px solid #fca5a5;
  border-radius: 10px;
  background: linear-gradient(180deg, rgba(254, 226, 226, 0.9), rgba(254, 242, 242, 0.9));
}
.first-guide-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 800;
  color: #991b1b;
}
.first-guide-body {
  color: #374151;
  line-height: 1.5;
}
.first-guide-dismiss.el-button--success.is-plain {
  color: #166534;
  background: rgba(255, 255, 255, 0.96);
  border-color: #86efac;
}
.first-guide-dismiss.el-button--success.is-plain:hover,
.first-guide-dismiss.el-button--success.is-plain:focus {
  color: #15803d;
  background: #ffffff;
  border-color: #4ade80;
}
.unbind-block-tip {
  color: #b45309;
  margin-top: 4px;
}
.mapping-state-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.mapping-state-tip {
  color: #92400e;
  line-height: 1.4;
}
.section-inline-title {
  display: flex;
  align-items: center;
  gap: 6px;
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
.section-block {
  position: relative;
  margin-left: 18px;
  padding: 6px 0 0 18px;
  border-left: 1px dashed #bfdbfe;
}
.section-block::before {
  content: "";
  position: absolute;
  top: 0;
  left: -5px;
  width: 9px;
  height: 9px;
  border-radius: 999px;
  background: #2563eb;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.14);
}
.section-surface {
  padding: 12px 14px;
  border: 1px solid #dbeafe;
  border-radius: 12px;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.96), rgba(239, 246, 255, 0.96));
}
.account-tabs {
  margin-top: 8px;
}
.path-grid {
  display: grid;
  grid-template-columns: minmax(280px, 0.9fr) minmax(320px, 1.1fr);
  gap: 12px;
}
.path-panel {
  padding: 14px;
  border: 1px solid #dbeafe;
  border-radius: 10px;
  background: #ffffff;
}
.panel-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}
.compact-form :deep(.el-form-item) {
  margin-bottom: 12px;
}
.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.challenge-command-box {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.challenge-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.command-line {
  padding: 10px;
  border-radius: 8px;
  background: #0f172a;
  color: #e2e8f0;
  overflow-x: auto;
}
.command-line code {
  color: inherit;
  white-space: nowrap;
}
.empty-tip {
  color: #64748b;
  font-size: 13px;
}
.section-divider {
  margin: 0 0 12px;
}
.editor {
  margin-top: 0;
}
.mb { margin-bottom: 12px; }
.strong-tip {
  border: 2px dashed #f59e0b;
}
.challenge-blue-tip {
  border: 1px solid #93c5fd;
  background: linear-gradient(180deg, rgba(219, 234, 254, 0.9), rgba(239, 246, 255, 0.9));
}
.challenge-countdown-value {
  font-weight: 800;
}
.challenge-countdown-value.is-safe {
  color: #0f766e;
}
.challenge-countdown-value.is-warn {
  color: #b45309;
}
.challenge-countdown-value.is-danger {
  color: #b91c1c;
}
.challenge-countdown-value.is-expired {
  color: #991b1b;
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
.payload-actions {
  margin-top: 8px;
  display: flex;
  gap: 8px;
}
.quota-tip-box {
  padding: 14px 16px;
  border: 1px solid #bfdbfe;
  border-radius: 14px;
  background: linear-gradient(180deg, #f8fbff 0%, #eaf3ff 100%);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.8);
}
.quota-tip-head {
  margin: 0 0 8px;
  font-size: 13px;
  font-weight: 800;
  letter-spacing: 0.04em;
  color: #1e3a8a;
}
.quota-tip-line {
  margin: 0;
  color: #1f2937;
  line-height: 1.6;
}
.quota-tip-line + .quota-tip-line {
  margin-top: 6px;
}
.quota-tip-subline {
  color: #334155;
}
.blue-keyword {
  color: #1d4ed8;
  font-weight: 800;
}

@media (max-width: 900px) {
  .head {
    flex-wrap: wrap;
  }
  .section-block {
    margin-left: 0;
    padding-left: 0;
    border-left: none;
  }
  .section-block::before {
    display: none;
  }
  .section-surface {
    padding: 10px;
  }
  .path-grid,
  .form-row {
    grid-template-columns: 1fr;
  }
  .panel-head,
  .challenge-meta {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
