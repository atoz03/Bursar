<template>
  <el-card>
    <template #header>
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><UserFilled /></el-icon></span>
          <div>
          <div class="title">
            <el-badge :is-dot="registrationTodoCount + profilePendingCount > 0" type="danger">
              <span>注册与资料审核</span>
            </el-badge>
          </div>
          <div class="sub">集中处理账号注册、资料变更与注册安全。</div>
          </div>
        </div>
        <el-button :loading="loading" type="primary" @click="reloadAll">刷新全部</el-button>
      </div>
    </template>

    <div class="content-stack">
      <el-alert v-if="error" :title="error" type="error" show-icon />
      <el-form inline class="compact-form">
        <el-form-item label="字段筛选">
          <el-select v-model="registrationFilterField" style="width: 160px" @change="reloadRegistration">
            <el-option label="全部字段" value="all" />
            <el-option label="用户名" value="username" />
            <el-option label="邮箱" value="email" />
            <el-option label="学号" value="student_id" />
            <el-option label="真实姓名" value="real_name" />
            <el-option label="导师" value="advisor" />
            <el-option label="电话" value="phone" />
            <el-option label="状态" value="status" />
          </el-select>
        </el-form-item>
        <el-form-item label="关键词">
          <el-input v-model="registrationKeyword" placeholder="输入关键词并回车" clearable @keyup.enter="reloadRegistration" @clear="reloadRegistration" />
        </el-form-item>
        <el-form-item>
          <el-button :loading="loading" @click="reloadRegistration">筛选/刷新</el-button>
        </el-form-item>
      </el-form>

      <div class="section-inline-title order-pending-title">
        <span class="section-icon tone-pending"><el-icon><Clock /></el-icon></span>
        <el-badge :is-dot="registrationTodoCount > 0" type="danger">
          <span class="title">平台账号待审核申请</span>
        </el-badge>
        <el-tag v-if="registrationTodoCount > 0" type="danger" size="small">{{ registrationTodoCount }}</el-tag>
      </div>
      <el-table :data="registrationPendingRows" stripe height="280" class="order-pending-table">
        <el-table-column prop="request_id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" width="150" />
        <el-table-column prop="real_name" label="真实姓名" width="120" />
        <el-table-column prop="student_id" label="学号" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column prop="advisor" label="导师" width="120" />
        <el-table-column prop="phone" label="电话" width="140" />
        <el-table-column prop="created_at" label="提交时间" min-width="170" :formatter="tableTimeFormatter" />
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-space>
              <el-button
                size="small"
                type="success"
                :loading="registrationActionId === row.request_id"
                @click="approveRegistration(row.request_id)"
              >
                通过
              </el-button>
              <el-button
                size="small"
                type="danger"
                :loading="registrationActionId === row.request_id"
                @click="rejectRegistration(row.request_id)"
              >
                退回
              </el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <div class="section-inline-title order-conflict-title">
        <span class="section-icon tone-risk"><el-icon><WarningFilled /></el-icon></span>
        <span class="title">唯一性冲突</span>
      </div>
      <el-table :data="registrationConflictRows" stripe height="240" class="order-conflict-table" :row-class-name="conflictRowClassName">
        <el-table-column prop="request_id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" width="150" />
        <el-table-column prop="student_id" label="学号" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column label="冲突项" width="180">
          <template #default="{ row }">{{ formatConflictFields(row.conflict_fields) }}</template>
        </el-table-column>
        <el-table-column prop="conflict_reason" label="冲突原因" min-width="260" />
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button
              size="small"
              type="danger"
              :loading="registrationActionId === row.request_id"
              @click="rejectRegistration(row.request_id, '自动退回：学号/邮箱/用户名重复，请修改后重新提交')"
            >
              退回
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="section-inline-title order-rejected-title">
        <span class="section-icon tone-reject"><el-icon><Document /></el-icon></span>
        <span class="title">已作废申请</span>
      </div>
      <el-table :data="registrationRejectedRows" stripe height="220" class="order-rejected-table">
        <el-table-column prop="request_id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" width="150" />
        <el-table-column prop="real_name" label="真实姓名" width="120" />
        <el-table-column prop="student_id" label="学号" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column prop="reject_reason" label="退回原因" min-width="260" />
        <el-table-column label="邮件通知" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.reject_notify_mail_checked && row.reject_notify_mail_sent" type="success">成功</el-tag>
            <el-tag v-else-if="row.reject_notify_mail_checked" type="warning">未发送</el-tag>
            <el-tag v-else type="info">未记录</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="邮件结果" min-width="240">
          <template #default="{ row }">
            <span v-if="row.reject_notify_mail_checked">{{ row.reject_notify_mail_sent ? "已发送通知邮件" : (row.reject_notify_mail_error || "未发送") }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="reviewed_by" label="处理人" width="120" />
        <el-table-column prop="reviewed_at" label="处理时间" min-width="170" :formatter="tableTimeFormatter" />
      </el-table>

      <el-divider class="order-security-divider" />
      <div class="section-inline-title order-security-title">
        <span class="section-icon tone-security"><el-icon><Lock /></el-icon></span>
        <span class="title">注册安全防护查询</span>
      </div>
      <el-alert
        class="order-security-alert"
        :title="`注册防护：IP ${registerSecurityPolicy.ip_limit || '-'} 次 / ${registerSecurityPolicy.ip_window_seconds || '-'} 秒，邮箱 ${registerSecurityPolicy.email_limit || '-'} 次 / ${registerSecurityPolicy.email_window_seconds || '-'} 秒`"
        type="info"
        :closable="false"
      />
      <el-form inline class="compact-form order-security-form">
        <el-form-item label="查询字段">
          <el-select v-model="registerSecurityField" style="width: 140px" @change="reloadRegisterSecurityEvents">
            <el-option label="全部" value="all" />
            <el-option label="IP" value="client_ip" />
            <el-option label="邮箱" value="email" />
            <el-option label="用户名" value="username" />
            <el-option label="学号" value="student_id" />
            <el-option label="原因" value="reason" />
            <el-option label="UA" value="user_agent" />
          </el-select>
        </el-form-item>
        <el-form-item label="关键词">
          <el-input v-model="registerSecurityKeyword" placeholder="支持模糊匹配" clearable @keyup.enter="reloadRegisterSecurityEvents" @clear="reloadRegisterSecurityEvents" />
        </el-form-item>
        <el-form-item label="动作">
          <el-select v-model="registerSecurityAction" style="width: 150px" @change="reloadRegisterSecurityEvents">
            <el-option label="全部" value="" />
            <el-option label="register_submit" value="register_submit" />
          </el-select>
        </el-form-item>
        <el-form-item label="结果">
          <el-select v-model="registerSecurityDecision" style="width: 140px" @change="reloadRegisterSecurityEvents">
            <el-option label="全部" value="" />
            <el-option label="allow" value="allow" />
            <el-option label="deny" value="deny" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button :loading="registerSecurityLoading" type="primary" @click="reloadRegisterSecurityEvents">查询</el-button>
        </el-form-item>
      </el-form>
      <el-table :data="registerSecurityEvents" stripe height="280" class="order-security-table">
        <el-table-column prop="event_id" label="ID" width="90" />
        <el-table-column prop="created_at" label="时间" min-width="170" :formatter="tableTimeFormatter" />
        <el-table-column prop="client_ip" label="IP" width="150" />
        <el-table-column prop="username" label="用户名" width="130" />
        <el-table-column prop="student_id" label="学号" width="130" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column prop="decision" label="结果" width="90" />
        <el-table-column prop="reason" label="命中原因" min-width="220" />
        <el-table-column label="可再次申请时间" min-width="190">
          <template #default="{ row }">
            <span>{{ registerSecurityRetryText(row) }}</span>
          </template>
        </el-table-column>
      </el-table>

      <div class="section-inline-title order-disposable-title">
        <span class="section-icon tone-security"><el-icon><Lock /></el-icon></span>
        <span class="title">临时邮箱域名黑名单</span>
      </div>
      <el-form inline class="compact-form order-disposable-form">
        <el-form-item label="域名">
          <el-input v-model="newDisposableDomain" placeholder="例如 mailinator.com" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="newDisposableDomainNote" placeholder="来源/原因（可选）" />
        </el-form-item>
        <el-form-item>
          <el-button :loading="disposableDomainLoading" type="danger" @click="saveDisposableDomain(true)">加入黑名单</el-button>
          <el-button :loading="disposableDomainLoading" @click="saveDisposableDomain(false)">设为禁用</el-button>
        </el-form-item>
        <el-form-item label="筛选">
          <el-input v-model="disposableDomainKeyword" placeholder="按域名/备注筛选" clearable @keyup.enter="reloadDisposableDomains" @clear="reloadDisposableDomains" />
        </el-form-item>
        <el-form-item>
          <el-button :loading="disposableDomainLoading" @click="reloadDisposableDomains">刷新</el-button>
        </el-form-item>
      </el-form>
      <el-table :data="disposableDomains" stripe height="220" class="order-disposable-table">
        <el-table-column prop="domain" label="域名" min-width="220" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">{{ row.enabled ? "启用" : "禁用" }}</template>
        </el-table-column>
        <el-table-column prop="note" label="备注" min-width="220" />
        <el-table-column prop="updated_by" label="更新人" width="120" />
        <el-table-column prop="updated_at" label="更新时间" min-width="170" :formatter="tableTimeFormatter" />
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-space>
              <el-button size="small" @click="toggleDisposableDomain(row, !row.enabled)">{{ row.enabled ? "设为禁用" : "启用" }}</el-button>
              <el-button size="small" type="danger" @click="removeDisposableDomain(row.domain)">删除</el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <el-divider class="order-profile-divider" />
      <div class="section-inline-title order-profile-title">
        <span class="section-icon tone-profile"><el-icon><Document /></el-icon></span>
        <el-badge :is-dot="profilePendingCount > 0" type="danger">
          <span class="title">平台账号资料关键信息变更审核</span>
        </el-badge>
        <el-tag v-if="profilePendingCount > 0" type="danger" size="small">{{ profilePendingCount }}</el-tag>
      </div>
      <el-form inline class="order-profile-form">
        <el-form-item label="平台账号筛选">
          <el-input v-model="profileUsername" placeholder="按平台账号筛选" @keyup.enter="reloadProfileChanges" />
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
      <el-table :data="profileRows" stripe height="320" class="order-profile-table">
        <el-table-column prop="request_id" label="ID" width="80" />
        <el-table-column label="申请用户" width="140">
          <template #default="{ row }">
            <el-button link type="primary" @click="openProfile(row.billing_username)">{{ row.billing_username }}</el-button>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100" />
        <el-table-column label="原用户名" width="140">
          <template #default="{ row }">
            <span class="diff-text">
              <template v-for="(seg, idx) in diffOldTokens(row.old_username, row.new_username)" :key="`ou-${row.request_id}-${idx}`">
                <span :class="{ 'diff-changed': seg.changed }">{{ seg.text }}</span>
              </template>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="新用户名" width="140">
          <template #default="{ row }">
            <span class="diff-text">
              <template v-for="(seg, idx) in diffNewTokens(row.old_username, row.new_username)" :key="`nu-${row.request_id}-${idx}`">
                <span :class="{ 'diff-changed': seg.changed }">{{ seg.text }}</span>
              </template>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="原邮箱" min-width="190">
          <template #default="{ row }">
            <span class="diff-text">
              <template v-for="(seg, idx) in diffOldTokens(row.old_email, row.new_email)" :key="`oe-${row.request_id}-${idx}`">
                <span :class="{ 'diff-changed': seg.changed }">{{ seg.text }}</span>
              </template>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="新邮箱" min-width="190">
          <template #default="{ row }">
            <span class="diff-text">
              <template v-for="(seg, idx) in diffNewTokens(row.old_email, row.new_email)" :key="`ne-${row.request_id}-${idx}`">
                <span :class="{ 'diff-changed': seg.changed }">{{ seg.text }}</span>
              </template>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="原学号" width="140">
          <template #default="{ row }">
            <span class="diff-text">
              <template v-for="(seg, idx) in diffOldTokens(row.old_student_id, row.new_student_id)" :key="`os-${row.request_id}-${idx}`">
                <span :class="{ 'diff-changed': seg.changed }">{{ seg.text }}</span>
              </template>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="新学号" width="140">
          <template #default="{ row }">
            <span class="diff-text">
              <template v-for="(seg, idx) in diffNewTokens(row.old_student_id, row.new_student_id)" :key="`ns-${row.request_id}-${idx}`">
                <span :class="{ 'diff-changed': seg.changed }">{{ seg.text }}</span>
              </template>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="reason" label="变更备注" min-width="180" />
        <el-table-column label="操作" width="220">
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
    </div>
    <PlatformUserDetailDialog v-model="profileVisible" :username="selectedProfileUsername" />
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import {
  ApiClient,
  type ProfileChangeRequest,
  type RegistrationDisposableEmailDomain,
  type RegistrationRequest,
  type RegistrationRequestView,
  type RegistrationSecurityEvent,
} from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { Clock, Document, Lock, UserFilled, WarningFilled } from "@element-plus/icons-vue";
import { formatServerDateTime } from "../../lib/time";

const loading = ref(false);
const error = ref("");

const registrationPendingRows = ref<RegistrationRequestView[]>([]);
const registrationConflictRows = ref<RegistrationRequestView[]>([]);
const registrationRejectedRows = ref<RegistrationRequest[]>([]);
const registrationTodoCount = computed(() => Number(registrationPendingRows.value.length + registrationConflictRows.value.length));
const registrationActionId = ref<number | null>(null);
const registrationFilterField = ref("all");
const registrationKeyword = ref("");

const registerSecurityLoading = ref(false);
const registerSecurityField = ref("all");
const registerSecurityKeyword = ref("");
const registerSecurityAction = ref("");
const registerSecurityDecision = ref("");
const registerSecurityEvents = ref<RegistrationSecurityEvent[]>([]);
const registerSecurityPolicy = reactive({
  ip_window_seconds: 0,
  ip_limit: 0,
  email_window_seconds: 0,
  email_limit: 0,
  ip_cooldown_seconds: 0,
  ip_cooldown_failures: 0,
  email_cooldown_seconds: 0,
  captcha_ttl_seconds: 0,
  allowed_email_domains: [] as string[],
});
const disposableDomainLoading = ref(false);
const disposableDomainKeyword = ref("");
const disposableDomains = ref<RegistrationDisposableEmailDomain[]>([]);
const newDisposableDomain = ref("");
const newDisposableDomainNote = ref("");

const profileLoading = ref(false);
const profileActionId = ref<number | null>(null);
const profileRows = ref<ProfileChangeRequest[]>([]);
const profilePendingCount = ref(0);
const profileStatus = ref("pending");
const profileUsername = ref("");
const profileVisible = ref(false);
const selectedProfileUsername = ref("");

type DiffToken = {
  text: string;
  changed: boolean;
};

type DiffPair = {
  old: DiffToken[];
  neu: DiffToken[];
};

const diffPairCache = new Map<string, DiffPair>();

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

function tableTimeFormatter(_: unknown, __: unknown, cellValue: unknown): string {
  return formatServerDateTime(String(cellValue ?? ""));
}

function registerSecurityRetryText(row: RegistrationSecurityEvent): string {
  const retryAt = String(row?.retry_at || "").trim();
  if (!retryAt) return "-";
  return formatServerDateTime(retryAt);
}

function formatConflictFields(fields?: string[]): string {
  const label = (f: string) => (f === "username" ? "用户名" : (f === "email" ? "邮箱" : (f === "student_id" ? "学号" : f)));
  return (fields ?? []).map(label).join("、") || "-";
}

function conflictRowClassName() {
  return "dup-row";
}

function normalizeDiffValue(v: unknown): string {
  if (v == null) return "";
  return String(v);
}

function mergeDiffTokens(tokens: DiffToken[]): DiffToken[] {
  if (tokens.length === 0) return tokens;
  const out: DiffToken[] = [];
  for (const tok of tokens) {
    if (!tok.text) continue;
    const prev = out[out.length - 1];
    if (prev && prev.changed === tok.changed) {
      prev.text += tok.text;
      continue;
    }
    out.push({ text: tok.text, changed: tok.changed });
  }
  return out;
}

function buildCharDiffPair(oldValue: string, newValue: string): DiffPair {
  const a = Array.from(oldValue);
  const b = Array.from(newValue);
  const n = a.length;
  const m = b.length;
  const dp: number[][] = Array.from({ length: n + 1 }, () => Array.from({ length: m + 1 }, () => 0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      if (a[i] === b[j]) {
        dp[i][j] = dp[i + 1][j + 1] + 1;
      } else {
        dp[i][j] = Math.max(dp[i + 1][j], dp[i][j + 1]);
      }
    }
  }
  const oldTokens: DiffToken[] = [];
  const newTokens: DiffToken[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      oldTokens.push({ text: a[i], changed: false });
      newTokens.push({ text: b[j], changed: false });
      i++;
      j++;
      continue;
    }
    if (dp[i + 1][j] >= dp[i][j + 1]) {
      oldTokens.push({ text: a[i], changed: true });
      i++;
    } else {
      newTokens.push({ text: b[j], changed: true });
      j++;
    }
  }
  while (i < n) {
    oldTokens.push({ text: a[i], changed: true });
    i++;
  }
  while (j < m) {
    newTokens.push({ text: b[j], changed: true });
    j++;
  }
  return {
    old: mergeDiffTokens(oldTokens),
    neu: mergeDiffTokens(newTokens),
  };
}

function diffPair(oldValue: unknown, newValue: unknown): DiffPair {
  const oldText = normalizeDiffValue(oldValue);
  const newText = normalizeDiffValue(newValue);
  const key = `${oldText}\u0000${newText}`;
  const cached = diffPairCache.get(key);
  if (cached) return cached;
  const built = buildCharDiffPair(oldText, newText);
  if (diffPairCache.size > 2000) diffPairCache.clear();
  diffPairCache.set(key, built);
  return built;
}

function diffOldTokens(oldValue: unknown, newValue: unknown): DiffToken[] {
  const oldText = normalizeDiffValue(oldValue);
  const tokens = diffPair(oldValue, newValue).old;
  if (tokens.length > 0) return tokens;
  if (oldText) return [{ text: oldText, changed: false }];
  return [{ text: "-", changed: normalizeDiffValue(newValue) !== "" }];
}

function diffNewTokens(oldValue: unknown, newValue: unknown): DiffToken[] {
  const newText = normalizeDiffValue(newValue);
  const tokens = diffPair(oldValue, newValue).neu;
  if (tokens.length > 0) return tokens;
  if (newText) return [{ text: newText, changed: false }];
  return [{ text: "-", changed: normalizeDiffValue(oldValue) !== "" }];
}

async function reloadRegistration() {
  const load = async () => {
    const r = await client().adminRegistrationRequestsOverview({
      limit: 2000,
      field: registrationFilterField.value,
      keyword: registrationKeyword.value,
    });
    registrationPendingRows.value = r.pending ?? [];
    registrationConflictRows.value = r.conflicts ?? [];
    registrationRejectedRows.value = r.rejected ?? [];
  };
  try {
    await load();
  } catch (e: any) {
    if (e?.status === 404) {
      registrationPendingRows.value = [];
      registrationConflictRows.value = [];
      registrationRejectedRows.value = [];
      error.value = "注册审核服务不可用，请更新并重启 Controller。";
      return;
    }
    throw e;
  }
}

async function reloadRegisterSecurityPolicy() {
  try {
    const r = await client().adminRegisterSecurityPolicy();
    registerSecurityPolicy.ip_window_seconds = Number(r.ip_window_seconds || 0);
    registerSecurityPolicy.ip_limit = Number(r.ip_limit || 0);
    registerSecurityPolicy.email_window_seconds = Number(r.email_window_seconds || 0);
    registerSecurityPolicy.email_limit = Number(r.email_limit || 0);
    registerSecurityPolicy.ip_cooldown_seconds = Number(r.ip_cooldown_seconds || 0);
    registerSecurityPolicy.ip_cooldown_failures = Number(r.ip_cooldown_failures || 0);
    registerSecurityPolicy.email_cooldown_seconds = Number(r.email_cooldown_seconds || 0);
    registerSecurityPolicy.captcha_ttl_seconds = Number(r.captcha_ttl_seconds || 0);
    registerSecurityPolicy.allowed_email_domains = Array.isArray(r.allowed_email_domains) ? r.allowed_email_domains : [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function reloadRegisterSecurityEvents() {
  registerSecurityLoading.value = true;
  try {
    const r = await client().adminRegisterSecurityEvents({
      keyword: registerSecurityKeyword.value,
      field: registerSecurityField.value,
      action: registerSecurityAction.value,
      decision: registerSecurityDecision.value,
      limit: 200,
    });
    registerSecurityEvents.value = r.events ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    registerSecurityLoading.value = false;
  }
}

function normalizeDomainInput(v: string): string {
  return String(v || "").trim().toLowerCase().replace(/^@+/, "");
}

async function reloadDisposableDomains() {
  disposableDomainLoading.value = true;
  try {
    const r = await client().adminDisposableEmailDomains({
      keyword: disposableDomainKeyword.value,
      limit: 500,
    });
    disposableDomains.value = r.domains ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    disposableDomainLoading.value = false;
  }
}

async function saveDisposableDomain(enabled: boolean) {
  const domain = normalizeDomainInput(newDisposableDomain.value);
  if (!domain) {
    error.value = "请先填写域名";
    return;
  }
  disposableDomainLoading.value = true;
  error.value = "";
  try {
    await client().adminUpsertDisposableEmailDomain({
      domain,
      enabled,
      note: String(newDisposableDomainNote.value || "").trim(),
    });
    newDisposableDomain.value = "";
    newDisposableDomainNote.value = "";
    await reloadDisposableDomains();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    disposableDomainLoading.value = false;
  }
}

async function toggleDisposableDomain(row: RegistrationDisposableEmailDomain, enabled: boolean) {
  disposableDomainLoading.value = true;
  error.value = "";
  try {
    await client().adminUpsertDisposableEmailDomain({
      domain: row.domain,
      enabled,
      note: row.note || "",
    });
    await reloadDisposableDomains();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    disposableDomainLoading.value = false;
  }
}

async function removeDisposableDomain(domain: string) {
  const ok = await ElMessageBox.confirm(`确认删除域名黑名单 ${domain} 吗？`, "确认删除", {
    type: "warning",
    confirmButtonText: "删除",
    cancelButtonText: "取消",
  }).then(() => true).catch(() => false);
  if (!ok) return;

  disposableDomainLoading.value = true;
  error.value = "";
  try {
    await client().adminDeleteDisposableEmailDomain(domain);
    await reloadDisposableDomains();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    disposableDomainLoading.value = false;
  }
}

async function approveRegistration(id: number) {
  try {
    await ElMessageBox.confirm(
      "确认通过这条平台账号注册申请吗？通过后会立即创建平台账号。",
      "二次确认",
      { type: "warning", confirmButtonText: "确认通过", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  registrationActionId.value = id;
  error.value = "";
  try {
    await client().adminApproveRegistrationRequest(id);
    await reloadRegistration();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    registrationActionId.value = null;
  }
}

async function rejectRegistration(id: number, defaultReason = "") {
  let reason = String(defaultReason || "");
  try {
    const r: any = await ElMessageBox.prompt(
      "请填写退回理由（必填）。",
      "退回注册申请",
      {
        confirmButtonText: "确认退回",
        cancelButtonText: "取消",
        inputType: "textarea",
        inputValue: reason,
        inputPlaceholder: "例如：学号与已有账号冲突，请修改后重新提交",
        inputValidator: (v: string) => String(v || "").trim().length > 0 || "退回理由不能为空",
      },
    );
    reason = String(r?.value || "").trim();
  } catch {
    return;
  }
  registrationActionId.value = id;
  error.value = "";
  try {
    const r = await client().adminRejectRegistrationRequest(id, reason);
    await reloadRegistration();
    if (r.mail_sent) {
      ElMessage.success("退回成功，通知邮件已发送");
    } else {
      ElMessage.warning(`退回成功，但邮件未发送：${String(r.mail_error || "未知原因")}`);
    }
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    registrationActionId.value = null;
  }
}

async function reloadProfileChanges() {
  profileLoading.value = true;
  error.value = "";
  try {
    const reqs = [
      client().adminProfileChangeRequests({
        status: profileStatus.value,
        username: profileUsername.value,
        limit: 500,
      }),
      profileStatus.value === "pending"
        ? Promise.resolve<{ requests: ProfileChangeRequest[] } | null>(null)
        : client().adminProfileChangeRequests({ status: "pending", limit: 2000 }),
    ] as const;
    const [r, pendingResp] = await Promise.all(reqs);
    profileRows.value = r.requests ?? [];
    if (profileStatus.value === "pending") {
      profilePendingCount.value = profileRows.value.length;
    } else {
      profilePendingCount.value = Number((pendingResp?.requests ?? []).length);
    }
  } catch (e: any) {
    if (e?.status === 404) {
      profileRows.value = [];
      profilePendingCount.value = 0;
      return;
    }
    error.value = e?.message ?? String(e);
  } finally {
    profileLoading.value = false;
  }
}

function openProfile(username: string) {
  selectedProfileUsername.value = String(username || "").trim();
  if (!selectedProfileUsername.value) return;
  profileVisible.value = true;
}

async function approveProfileChange(id: number) {
  profileActionId.value = id;
  error.value = "";
  try {
    await client().adminApproveProfileChange(id);
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
    await client().adminRejectProfileChange(id);
    await reloadProfileChanges();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    profileActionId.value = null;
  }
}

async function reloadAll() {
  loading.value = true;
  error.value = "";
  try {
    await Promise.all([
      reloadRegistration(),
      reloadRegisterSecurityPolicy(),
      reloadRegisterSecurityEvents(),
      reloadDisposableDomains(),
      reloadProfileChanges(),
    ]);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  reloadAll();
});
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
.compact-form {
  margin-bottom: 2px;
}
.order-pending-title { order: 10; }
.order-pending-table { order: 11; }
.order-rejected-title { order: 20; }
.order-rejected-table { order: 21; }
.order-conflict-title { order: 40; }
.order-conflict-table { order: 41; }
.order-profile-divider { order: 50; }
.order-profile-title { order: 51; }
.order-profile-form { order: 52; }
.order-profile-table { order: 53; }
.order-security-divider { order: 60; }
.order-security-title { order: 61; }
.order-security-alert { order: 62; }
.order-security-form { order: 63; }
.order-security-table { order: 64; }
.order-disposable-title { order: 70; }
.order-disposable-form { order: 71; }
.order-disposable-table { order: 72; }
.title {
  font-weight: 700;
}
.section-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.section-icon {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #dbeafe;
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
  flex-shrink: 0;
}
.section-inline-title {
  display: flex;
  align-items: center;
  gap: 10px;
}
.tone-pending {
  background: linear-gradient(135deg, #1e3a8a, #2563eb);
  color: #dbeafe;
}
.tone-risk {
  background: linear-gradient(135deg, #b45309, #f59e0b);
  color: #fffbeb;
}
.tone-reject {
  background: linear-gradient(135deg, #991b1b, #dc2626);
  color: #fee2e2;
}
.tone-profile {
  background: linear-gradient(135deg, #312e81, #4f46e5);
  color: #e0e7ff;
}
.tone-security {
  background: linear-gradient(135deg, #7c2d12, #c2410c);
  color: #ffedd5;
}
.sub {
  margin-top: 4px;
  font-size: 12px;
  color: #6b7280;
}
.diff-text {
  display: inline-block;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.35;
}
.diff-changed {
  color: #b91c1c;
  background: #fee2e2;
  border-radius: 3px;
  padding: 0 1px;
}
:deep(.dup-row > td) {
  background: #fee2e2 !important;
}
</style>
