<template>
  <el-card>
    <template #header>
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><UserFilled /></el-icon></span>
          <div>
          <div class="title">平台账号注册审核</div>
          <div class="sub">注册申请需审核通过后才创建平台账号</div>
          </div>
        </div>
        <el-button :loading="loading" type="primary" @click="reloadAll">刷新全部</el-button>
      </div>
    </template>

    <div class="content-stack">
      <el-alert v-if="error" :title="error" type="error" show-icon />
      <el-alert
        :title="`待处理平台账号注册审核：${registrationPendingRows.length + registrationConflictRows.length} 人`"
        type="info"
        :closable="false"
      />
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

      <div class="section-inline-title">
        <span class="section-icon tone-pending"><el-icon><Clock /></el-icon></span>
        <span class="title">待审核申请</span>
      </div>
      <el-table :data="registrationPendingRows" stripe height="280">
        <el-table-column prop="request_id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" width="150" />
        <el-table-column prop="real_name" label="真实姓名" width="120" />
        <el-table-column prop="student_id" label="学号" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column prop="advisor" label="导师" width="120" />
        <el-table-column prop="phone" label="电话" width="140" />
        <el-table-column prop="created_at" label="提交时间" min-width="170" />
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

      <div class="section-inline-title">
        <span class="section-icon tone-risk"><el-icon><WarningFilled /></el-icon></span>
        <span class="title">唯一性冲突申请（自动筛出）</span>
      </div>
      <el-table :data="registrationConflictRows" stripe height="240" :row-class-name="conflictRowClassName">
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

      <div class="section-inline-title">
        <span class="section-icon tone-reject"><el-icon><Document /></el-icon></span>
        <span class="title">退回申请（作废）</span>
      </div>
      <el-table :data="registrationRejectedRows" stripe height="220">
        <el-table-column prop="request_id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" width="150" />
        <el-table-column prop="real_name" label="真实姓名" width="120" />
        <el-table-column prop="student_id" label="学号" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column prop="reject_reason" label="退回原因" min-width="260" />
        <el-table-column prop="reviewed_by" label="处理人" width="120" />
        <el-table-column prop="reviewed_at" label="处理时间" min-width="170" />
      </el-table>

      <el-divider />
      <div class="section-inline-title">
        <span class="section-icon tone-security"><el-icon><Lock /></el-icon></span>
        <span class="title">注册安全防护查询</span>
      </div>
      <el-alert
        :title="`当前策略：IP窗口 ${registerSecurityPolicy.ip_window_seconds || '-'}s（上限 ${registerSecurityPolicy.ip_limit || '-'}），邮箱窗口 ${registerSecurityPolicy.email_window_seconds || '-'}s（上限 ${registerSecurityPolicy.email_limit || '-'}），IP冷却 ${registerSecurityPolicy.ip_cooldown_seconds || '-'}s，邮箱冷却 ${registerSecurityPolicy.email_cooldown_seconds || '-'}s`"
        type="info"
        :closable="false"
      />
      <el-form inline class="compact-form">
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
      <el-table :data="registerSecurityEvents" stripe height="280">
        <el-table-column prop="event_id" label="ID" width="90" />
        <el-table-column prop="created_at" label="时间" min-width="170" />
        <el-table-column prop="client_ip" label="IP" width="150" />
        <el-table-column prop="username" label="用户名" width="130" />
        <el-table-column prop="student_id" label="学号" width="130" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column prop="decision" label="结果" width="90" />
        <el-table-column prop="reason" label="命中原因" min-width="220" />
      </el-table>

      <div class="section-inline-title">
        <span class="section-icon tone-security"><el-icon><Lock /></el-icon></span>
        <span class="title">临时邮箱域名黑名单</span>
      </div>
      <el-form inline class="compact-form">
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
      <el-table :data="disposableDomains" stripe height="220">
        <el-table-column prop="domain" label="域名" min-width="220" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">{{ row.enabled ? "启用" : "禁用" }}</template>
        </el-table-column>
        <el-table-column prop="note" label="备注" min-width="220" />
        <el-table-column prop="updated_by" label="更新人" width="120" />
        <el-table-column prop="updated_at" label="更新时间" min-width="170" />
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-space>
              <el-button size="small" @click="toggleDisposableDomain(row, !row.enabled)">{{ row.enabled ? "设为禁用" : "启用" }}</el-button>
              <el-button size="small" type="danger" @click="removeDisposableDomain(row.domain)">删除</el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <el-divider />

      <div class="section-inline-title">
        <span class="section-icon tone-link"><el-icon><Connection /></el-icon></span>
        <span class="title">节点账号开通/绑定申请审核</span>
      </div>
      <el-form inline>
        <el-form-item label="状态">
          <el-select v-model="status" style="width: 160px" @change="reloadRequests">
            <el-option label="待审核" value="pending" />
            <el-option label="已通过" value="approved" />
            <el-option label="已拒绝" value="rejected" />
            <el-option label="全部" value="" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button :disabled="selectedIds.length===0" :loading="batchLoading" type="success" @click="batchApprove">批量通过</el-button>
          <el-button :disabled="selectedIds.length===0" :loading="batchLoading" type="danger" @click="batchReject">批量拒绝</el-button>
          <el-button :loading="requestLoading" @click="reloadRequests">刷新</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="rows" stripe height="360" @selection-change="onSelectionChange" :row-class-name="requestRowClassName">
        <el-table-column type="selection" width="45" />
        <el-table-column prop="request_id" label="ID" width="80" />
        <el-table-column prop="request_type" label="类型" width="100" />
        <el-table-column label="平台账号" width="160">
          <template #default="{ row }">
            <el-button link type="primary" @click="openProfile(row.billing_username)">{{ row.billing_username }}</el-button>
          </template>
        </el-table-column>
        <el-table-column prop="node_id" label="端口" width="110" />
        <el-table-column prop="local_username" label="节点账号" width="160" />
        <el-table-column prop="message" label="开通/绑定理由" min-width="240" />
        <el-table-column prop="status" label="状态" width="120" />
        <el-table-column prop="apply_count_by_billing" label="申请次数" width="100" />
        <el-table-column prop="duplicate_reason" label="风险提示" width="220" />
        <el-table-column prop="created_at" label="提交时间" min-width="180" />
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
      <div class="section-inline-title">
        <span class="section-icon tone-profile"><el-icon><Document /></el-icon></span>
        <span class="title">平台账号资料关键信息变更审核</span>
      </div>
      <el-form inline>
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
      <el-table :data="profileRows" stripe height="320">
        <el-table-column prop="request_id" label="ID" width="80" />
        <el-table-column label="申请用户" width="140">
          <template #default="{ row }">
            <el-button link type="primary" @click="openProfile(row.billing_username)">{{ row.billing_username }}</el-button>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100" />
        <el-table-column prop="old_username" label="原用户名" width="120" />
        <el-table-column prop="new_username" label="新用户名" width="120" />
        <el-table-column prop="old_email" label="原邮箱" min-width="170" />
        <el-table-column prop="new_email" label="新邮箱" min-width="170" />
        <el-table-column prop="old_student_id" label="原学号" width="120" />
        <el-table-column prop="new_student_id" label="新学号" width="120" />
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
import { onMounted, reactive, ref } from "vue";
import { ElMessageBox } from "element-plus";
import {
  ApiClient,
  type ProfileChangeRequest,
  type RegistrationDisposableEmailDomain,
  type RegistrationRequest,
  type RegistrationRequestView,
  type RegistrationSecurityEvent,
  type UserRequest,
} from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { Clock, Connection, Document, Lock, UserFilled, WarningFilled } from "@element-plus/icons-vue";

const loading = ref(false);
const error = ref("");

const registrationPendingRows = ref<RegistrationRequestView[]>([]);
const registrationConflictRows = ref<RegistrationRequestView[]>([]);
const registrationRejectedRows = ref<RegistrationRequest[]>([]);
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
  email_cooldown_seconds: 0,
  captcha_ttl_seconds: 0,
  allowed_email_domains: [] as string[],
});
const disposableDomainLoading = ref(false);
const disposableDomainKeyword = ref("");
const disposableDomains = ref<RegistrationDisposableEmailDomain[]>([]);
const newDisposableDomain = ref("");
const newDisposableDomainNote = ref("");

const requestLoading = ref(false);
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
const profileVisible = ref(false);
const selectedProfileUsername = ref("");

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

function formatConflictFields(fields?: string[]): string {
  const label = (f: string) => (f === "username" ? "用户名" : (f === "email" ? "邮箱" : (f === "student_id" ? "学号" : f)));
  return (fields ?? []).map(label).join("、") || "-";
}

function conflictRowClassName() {
  return "dup-row";
}

function requestRowClassName({ row }: { row: UserRequest }) {
  return row.duplicate_flag ? "dup-row" : "";
}

async function reloadRegistration() {
  const load = async () => {
    const r = await client().adminRegistrationRequestsOverview({
      limit: 50000,
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
      error.value = "平台账号注册审核接口不可用：请确认控制器已更新到最新版本并重启。";
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
      limit: 1000,
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
      limit: 2000,
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
      "请填写退回理由（可留空）。",
      "退回注册申请",
      {
        confirmButtonText: "确认退回",
        cancelButtonText: "取消",
        inputType: "textarea",
        inputValue: reason,
        inputPlaceholder: "例如：请补充真实姓名/修改重复邮箱（可留空）",
      },
    );
    reason = String(r?.value || "").trim();
  } catch {
    return;
  }
  registrationActionId.value = id;
  error.value = "";
  try {
    await client().adminRejectRegistrationRequest(id, reason);
    await reloadRegistration();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    registrationActionId.value = null;
  }
}

async function reloadRequests() {
  requestLoading.value = true;
  error.value = "";
  rows.value = [];
  try {
    const r = await client().adminRequests({ status: status.value, limit: 500 });
    rows.value = r.requests ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    requestLoading.value = false;
  }
}

async function approve(id: number) {
  actionLoadingId.value = id;
  error.value = "";
  try {
    await client().adminApproveRequest(id);
    await reloadRequests();
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
    await client().adminRejectRequest(id);
    await reloadRequests();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    actionLoadingId.value = null;
  }
}

function onSelectionChange(v: UserRequest[]) {
  selectedIds.value = (v ?? []).map((x) => x.request_id);
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
    await client().adminBatchReview(selectedIds.value, newStatus);
    selectedIds.value = [];
    await reloadRequests();
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
    const r = await client().adminProfileChangeRequests({
      status: profileStatus.value,
      username: profileUsername.value,
      limit: 500,
    });
    profileRows.value = r.requests ?? [];
  } catch (e: any) {
    if (e?.status === 404) {
      profileRows.value = [];
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
    await reloadRegistration();
    await reloadRegisterSecurityPolicy();
    await reloadRegisterSecurityEvents();
    await reloadDisposableDomains();
    await reloadRequests();
    await reloadProfileChanges();
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
.tone-link {
  background: linear-gradient(135deg, #115e59, #0f766e);
  color: #ccfbf1;
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
:deep(.dup-row > td) {
  background: #fee2e2 !important;
}
</style>
