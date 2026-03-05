<template>
  <el-card>
    <template #header>
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><UserFilled /></el-icon></span>
          <div>
          <div class="title">平台用户管理</div>
          <div class="sub">平台账号资料、状态控制、删除恢复与重复身份排查</div>
          </div>
        </div>
        <div class="row">
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
        </div>
      </div>
    </template>

    <div class="content-stack">
      <el-alert v-if="error" :title="error" type="error" show-icon />
      <el-alert v-if="success" :title="success" type="success" show-icon />
      <div class="section-title-wrap">
        <span class="section-icon tone-list"><el-icon><List /></el-icon></span>
        <div>
          <div class="title">平台账号列表</div>
          <div class="sub">支持筛选、加分、拉黑/解黑、删除和重复身份排查。</div>
        </div>
      </div>
      <el-form inline>
        <el-form-item label="平台账号查询">
          <el-input v-model="keyword" placeholder="输入平台账号过滤" clearable />
        </el-form-item>
      </el-form>

      <el-table :data="filteredRows" stripe height="520" empty-text="暂无数据">
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="expand-wrap">
              <el-descriptions :column="3" border>
                <el-descriptions-item label="真实姓名">{{ row.real_name || "-" }}</el-descriptions-item>
                <el-descriptions-item label="导师">{{ row.advisor || "-" }}</el-descriptions-item>
                <el-descriptions-item label="预计毕业">{{ fmtGrad(row.expected_graduation_year, row.expected_graduation_month) }}</el-descriptions-item>
                <el-descriptions-item label="手机号">{{ row.phone || "-" }}</el-descriptions-item>
                <el-descriptions-item label="累计记录">{{ row.usage_records }}</el-descriptions-item>
                <el-descriptions-item label="累计积分消耗">{{ fmt2(row.total_cost) }}</el-descriptions-item>
                <el-descriptions-item label="最后使用时间">{{ fmtTime(row.last_usage_at) }}</el-descriptions-item>
                <el-descriptions-item label="运营看板权限">{{ row.can_view_board ? "有" : "无" }}</el-descriptions-item>
                <el-descriptions-item label="节点状态权限">{{ row.can_view_nodes ? "有" : "无" }}</el-descriptions-item>
                <el-descriptions-item label="注册审核权限">{{ row.can_review_requests ? "有" : "无" }}</el-descriptions-item>
              </el-descriptions>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="平台账号" width="160">
          <template #default="{ row }">
            <span class="username-cell">
              <button type="button" class="username-link" @click="openProfile(row.username)">{{ row.username }}</button>
              <span v-if="isBlack(row)" class="black-badge">黑</span>
              <span v-if="isWhite(row)" class="white-badge">白</span>
              <span v-if="isExempt(row)" class="exempt-badge">豁</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="real_name" label="真实姓名" width="120" />
        <el-table-column label="账号类型" width="110">
          <template #default="{ row }">
            {{ roleText(row.role) }}
          </template>
        </el-table-column>
        <el-table-column prop="student_id" label="学号" width="160" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column label="预计毕业" width="120">
          <template #default="{ row }">{{ fmtGrad(row.expected_graduation_year, row.expected_graduation_month) }}</template>
        </el-table-column>
        <el-table-column label="积分余额" width="120">
          <template #default="{ row }">{{ fmt2(row.balance) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">{{ effectiveStatus(row) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="420" fixed="right">
          <template #default="{ row }">
            <el-space>
              <el-button size="small" :disabled="row.role !== 'user'" @click="openRecharge(row)">加分</el-button>
              <el-button v-if="!isBlack(row)" size="small" type="danger" :disabled="row.role !== 'user'" @click="blockUser(row)">拉黑</el-button>
              <el-button v-else size="small" type="success" :disabled="row.role !== 'user'" @click="unblockUser(row)">解黑</el-button>
              <el-button size="small" type="warning" :disabled="row.role !== 'user'" @click="deleteUser(row)">删除</el-button>
              <el-button size="small" @click="queryDuplicates(row)">查重</el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <el-divider />
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon tone-delete"><el-icon><Delete /></el-icon></span>
          <div>
          <div class="title">已删除平台账号（可恢复）</div>
          <div class="sub">恢复时会校验当前已注册与待审核注册申请，冲突会明确提示。</div>
          </div>
        </div>
      </div>
      <el-table :data="deletedRows" stripe height="280" empty-text="暂无数据">
        <el-table-column prop="deleted_id" label="删除ID" width="90" />
        <el-table-column prop="username" label="平台账号" width="150" />
        <el-table-column prop="real_name" label="真实姓名" width="120" />
        <el-table-column prop="student_id" label="学号" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column prop="delete_reason" label="删除原因" min-width="220" />
        <el-table-column prop="deleted_by" label="删除人" width="120" />
        <el-table-column label="删除时间" width="180">
          <template #default="{ row }">{{ fmtTime(row.deleted_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="restoreUser(row)">恢复</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-divider />
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon tone-remind"><el-icon><Bell /></el-icon></span>
          <div>
          <div class="title">毕业到期提醒（两个月后可能清理数据）</div>
          <div class="sub">查询已达到预计毕业时间的用户，可单发或批量发送备份提醒邮件。</div>
          </div>
        </div>
        <div class="row">
          <el-button :loading="dueLoading" @click="loadGraduationDueUsers">查询毕业到期用户</el-button>
          <el-button :loading="sendLoading" type="primary" @click="sendSelectedDueUsers">发送已选中</el-button>
          <el-button :loading="sendLoading" type="warning" @click="sendAllDueUsers">发送全部到期用户</el-button>
        </div>
      </div>
      <el-table :data="dueRows" stripe height="320" @selection-change="onDueSelectionChange" empty-text="暂无数据">
        <el-table-column type="selection" width="48" />
        <el-table-column prop="username" label="用户名" width="140" />
        <el-table-column prop="student_id" label="学号" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column label="预计毕业" width="120">
          <template #default="{ row }">{{ fmtGrad(row.expected_graduation_year, row.expected_graduation_month) }}</template>
        </el-table-column>
        <el-table-column prop="overdue_months" label="已到期(月)" width="100" />
        <el-table-column label="发送状态" min-width="220">
          <template #default="{ row }">
            <span :style="{ color: row.send_success ? '#16a34a' : (row.send_error ? '#dc2626' : '#475569') }">
              {{ row.send_success ? "发送成功" : (row.send_error || "未发送") }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" type="primary" :loading="sendLoading" @click="sendSingleDueUser(row)">发送</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="rechargeVisible" title="积分调整" width="500px">
      <el-form label-width="90px">
        <el-form-item label="用户名">
          <el-input v-model="rechargeUser" disabled />
        </el-form-item>
        <el-form-item label="当前积分">
          <div class="recharge-balance-wrap">
            <el-tag type="success" effect="plain">通用 {{ fmt2(rechargeGeneralBalance) }}</el-tag>
            <el-tag type="info" effect="plain">结转 {{ fmt2(rechargeCarryoverBalance) }}</el-tag>
            <el-tag type="warning" effect="plain">专属 {{ fmt2(rechargeExclusiveBalance) }}</el-tag>
            <el-tag type="primary" effect="plain">总计 {{ fmt2(rechargeTotalBalance) }}</el-tag>
          </div>
          <div v-if="rechargeBalanceLoading" class="sub">正在同步最新积分...</div>
          <div v-if="rechargeBalanceError" class="sub recharge-balance-error">{{ rechargeBalanceError }}</div>
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

    <el-dialog v-model="profileVisible" width="680px">
      <template #header>
        <div class="dialog-title-wrap">
          <span class="section-icon tone-profile"><el-icon><UserFilled /></el-icon></span>
          <span class="title">平台账号信息</span>
        </div>
      </template>
      <el-alert v-if="profileError" :title="profileError" type="error" show-icon />
      <el-skeleton v-else-if="profileLoading" :rows="6" animated />
      <el-descriptions v-else-if="profileData" :column="2" border>
        <el-descriptions-item label="平台账号">{{ profileData.username }}</el-descriptions-item>
        <el-descriptions-item label="真实姓名">{{ profileData.real_name || "-" }}</el-descriptions-item>
        <el-descriptions-item label="学号">{{ profileData.student_id || "-" }}</el-descriptions-item>
        <el-descriptions-item label="邮箱">{{ profileData.email || "-" }}</el-descriptions-item>
        <el-descriptions-item label="导师">{{ profileData.advisor || "-" }}</el-descriptions-item>
        <el-descriptions-item label="电话">{{ profileData.phone || "-" }}</el-descriptions-item>
        <el-descriptions-item label="预计毕业">{{ fmtGrad(profileData.expected_graduation_year, profileData.expected_graduation_month) }}</el-descriptions-item>
        <el-descriptions-item label="通用积分">{{ fmt2(profileData.general_balance ?? profileData.balance) }}</el-descriptions-item>
        <el-descriptions-item label="结转积分">{{ fmt2(profileData.carryover_balance ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="节点专属积分">{{ fmt2(profileData.exclusive_balance ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="总可用积分">{{ fmt2(profileData.total_balance ?? ((profileData.general_balance ?? profileData.balance) + (profileData.carryover_balance ?? 0) + (profileData.exclusive_balance ?? 0))) }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ profileData.status || "-" }}</el-descriptions-item>
        <el-descriptions-item label="角色">{{ profileData.role || "-" }}</el-descriptions-item>
      </el-descriptions>
      <template v-if="profileData">
        <div class="section-title-wrap mt10">
          <span class="section-icon tone-map"><el-icon><Connection /></el-icon></span>
          <div class="title">节点账号映射</div>
        </div>
        <el-table :data="profileData.node_accounts || []" stripe max-height="220" empty-text="暂无映射">
          <el-table-column prop="node_id" label="节点编号" width="140" />
          <el-table-column prop="local_username" label="节点账号" width="160" />
          <el-table-column label="更新时间" min-width="180">
            <template #default="{ row }">{{ fmtTime(row.updated_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="120">
            <template #default="{ row: acc }">
              <el-button
                v-if="!isNodeAccountBlack(acc?.node_id, acc?.local_username)"
                size="small"
                type="danger"
                @click="disableNodeAccountMapping(acc)"
              >禁用</el-button>
              <el-button
                v-else
                size="small"
                type="success"
                @click="enableNodeAccountMapping(acc)"
              >解除禁用</el-button>
            </template>
          </el-table-column>
        </el-table>
      </template>
    </el-dialog>

    <el-dialog v-model="duplicatesVisible" width="860px">
      <template #header>
        <div class="dialog-title-wrap">
          <span class="section-icon tone-dup"><el-icon><WarningFilled /></el-icon></span>
          <span class="title">重复身份查询结果</span>
        </div>
      </template>
      <el-alert
        title="仅在点击“查重”时发起查询，平时不查询。"
        type="info"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-skeleton v-if="duplicatesLoading" :rows="4" animated />
      <template v-else>
        <div class="section-title-wrap">
          <span class="section-icon tone-active"><el-icon><UserFilled /></el-icon></span>
          <div class="title">当前在用平台账号</div>
        </div>
        <el-table :data="duplicateActiveRows" stripe max-height="220" empty-text="暂无数据">
          <el-table-column prop="username" label="平台账号" width="150" />
          <el-table-column prop="real_name" label="真实姓名" width="120" />
          <el-table-column prop="student_id" label="学号" width="140" />
          <el-table-column prop="email" label="邮箱" min-width="220" />
        </el-table>
        <div class="section-title-wrap mt10">
          <span class="section-icon tone-delete"><el-icon><Delete /></el-icon></span>
          <div class="title">已删除平台账号</div>
        </div>
        <el-table :data="duplicateDeletedRows" stripe max-height="220" empty-text="暂无数据">
          <el-table-column prop="deleted_id" label="删除ID" width="90" />
          <el-table-column prop="username" label="平台账号" width="150" />
          <el-table-column prop="real_name" label="真实姓名" width="120" />
          <el-table-column prop="student_id" label="学号" width="140" />
          <el-table-column prop="email" label="邮箱" min-width="220" />
          <el-table-column label="删除时间" width="180">
            <template #default="{ row }">{{ fmtTime(row.deleted_at) }}</template>
          </el-table-column>
        </el-table>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ApiClient, type AdminUserDetail, type DeletedUserAccount, type GraduationDueUser, type UserProfile } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { formatServerDateTime } from "../../lib/time";
import { Bell, Connection, Delete, List, UserFilled, WarningFilled } from "@element-plus/icons-vue";

const loading = ref(false);
const error = ref("");
const success = ref("");
const rows = ref<AdminUserDetail[]>([]);
const deletedRows = ref<DeletedUserAccount[]>([]);
const blacklistSet = ref<Set<string>>(new Set());
const blacklistKeySet = ref<Set<string>>(new Set());
const whitelistSet = ref<Set<string>>(new Set());
const exemptionSet = ref<Set<string>>(new Set());
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
const rechargeGeneralBalance = ref(0);
const rechargeCarryoverBalance = ref(0);
const rechargeExclusiveBalance = ref(0);
const rechargeTotalBalance = ref(0);
const rechargeBalanceLoading = ref(false);
const rechargeBalanceError = ref("");
const dueLoading = ref(false);
const sendLoading = ref(false);
const profileVisible = ref(false);
const profileLoading = ref(false);
const profileError = ref("");
const profileData = ref<{
  username: string;
  email: string;
  real_name: string;
  student_id: string;
  advisor: string;
  expected_graduation_year: number;
  expected_graduation_month: number;
  phone: string;
  role: string;
  balance: number;
  general_balance?: number;
  carryover_balance?: number;
  exclusive_balance?: number;
  total_balance?: number;
  status: string;
  node_accounts: Array<{ node_id: string; local_username: string; updated_at: string }>;
} | null>(null);
const duplicatesVisible = ref(false);
const duplicatesLoading = ref(false);
const duplicateActiveRows = ref<UserProfile[]>([]);
const duplicateDeletedRows = ref<DeletedUserAccount[]>([]);
type GraduationDueView = GraduationDueUser & { send_success?: boolean; send_error?: string };
const dueRows = ref<GraduationDueView[]>([]);
const selectedDueRows = ref<GraduationDueView[]>([]);

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

function fmt2(v: number): string {
  return Number(v ?? 0).toFixed(2);
}

function setRechargeBalances(generalBalance: number, carryoverBalance: number, exclusiveBalance: number, totalBalance?: number) {
  const g = Number(generalBalance || 0);
  const c = Number(carryoverBalance || 0);
  const e = Number(exclusiveBalance || 0);
  rechargeGeneralBalance.value = g;
  rechargeCarryoverBalance.value = c;
  rechargeExclusiveBalance.value = e;
  rechargeTotalBalance.value = Number(totalBalance ?? (g + c + e));
}

function roleText(role: string): string {
  if (role === "admin") return "管理员";
  if (role === "power_user") return "高级用户";
  return "普通用户";
}

function pushIdentity(set: Set<string>, v: string): void {
  const x = String(v || "").trim();
  if (x) set.add(x);
}

function buildIdentitySet(entries: Array<{
  local_username?: string;
  billing_username?: string;
  source_platform_username?: string;
}>): Set<string> {
  const out = new Set<string>();
  for (const e of entries ?? []) {
    const sourcePlatform = String(e?.source_platform_username || "").trim();
    const billing = String(e?.billing_username || "").trim();
    if (sourcePlatform) {
      pushIdentity(out, sourcePlatform);
      continue;
    }
    if (billing) {
      pushIdentity(out, billing);
    }
  }
  return out;
}

function isBlack(row: AdminUserDetail): boolean {
  const u = String(row.username || "").trim();
  return blacklistSet.value.has(u);
}

function isWhite(row: AdminUserDetail): boolean {
  const u = String(row.username || "").trim();
  return whitelistSet.value.has(u);
}

function isExempt(row: AdminUserDetail): boolean {
  const u = String(row.username || "").trim();
  return exemptionSet.value.has(u);
}

function accountBlacklistKey(nodeID: string, localUsername: string): string {
  return `${String(nodeID || "").trim()}|${String(localUsername || "").trim()}`;
}

function isNodeAccountBlack(nodeID: string, localUsername: string): boolean {
  const node = String(nodeID || "").trim();
  const local = String(localUsername || "").trim();
  if (!node || !local) return false;
  return blacklistKeySet.value.has(accountBlacklistKey(node, local)) || blacklistKeySet.value.has(accountBlacklistKey("*", local));
}

function effectiveStatus(row: AdminUserDetail): string {
  const current = String(row.status || "").trim();
  if (isBlack(row)) return "blocked";
  if (current === "blocked") return "normal";
  return current || "normal";
}

function fmtGrad(year: number, month: number): string {
  if (!year || !month) return "-";
  return `${year}-${String(month).padStart(2, "0")}`;
}

function fmtTime(v: string): string {
  return formatServerDateTime(v);
}

async function reload() {
  loading.value = true;
  error.value = "";
  success.value = "";
  rows.value = [];
  try {
    const r1 = await client().adminUsersDetails(2000);
    rows.value = r1.users ?? [];
    try {
      const r2 = await client().adminDeletedUsers(2000, false);
      deletedRows.value = r2.users ?? [];
    } catch (e: any) {
      // 兼容旧后端：未提供删除恢复接口时不阻塞主页面展示。
      if (e?.status === 404) {
        deletedRows.value = [];
      } else {
        throw e;
      }
    }
    try {
      const [bl, wl, ex] = await Promise.all([
        client().adminBlacklist(""),
        client().adminWhitelist(""),
        client().adminExemptions(""),
      ]);
      blacklistKeySet.value = new Set((bl.entries ?? []).map((x) => accountBlacklistKey(x.node_id, x.local_username)).filter((x) => x !== "|"));
      blacklistSet.value = buildIdentitySet(bl.entries ?? []);
      whitelistSet.value = buildIdentitySet(wl.entries ?? []);
      exemptionSet.value = buildIdentitySet(ex.entries ?? []);
    } catch {
      blacklistKeySet.value = new Set();
      blacklistSet.value = new Set();
      whitelistSet.value = new Set();
      exemptionSet.value = new Set();
    }
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function syncRechargeCurrentBalance(username: string) {
  const u = String(username || "").trim();
  if (!u) return;
  rechargeBalanceLoading.value = true;
  rechargeBalanceError.value = "";
  try {
    const r = await client().adminPlatformUserDetail(u);
    if (String(rechargeUser.value || "").trim() !== u) return;
    const user = r.user;
    const general = Number(user.general_balance ?? user.balance ?? 0);
    const carryover = Number(user.carryover_balance ?? 0);
    const exclusive = Number(user.exclusive_balance ?? 0);
    const total = Number(user.total_balance ?? (general + carryover + exclusive));
    setRechargeBalances(general, carryover, exclusive, total);
  } catch (e: any) {
    if (String(rechargeUser.value || "").trim() !== u) return;
    rechargeBalanceError.value = `同步当前积分失败：${e?.message ?? String(e)}`;
  } finally {
    if (String(rechargeUser.value || "").trim() === u) {
      rechargeBalanceLoading.value = false;
    }
  }
}

function openRecharge(row: AdminUserDetail) {
  rechargeUser.value = String(row.username || "").trim();
  rechargeAmount.value = 100;
  rechargeMethod.value = "admin";
  rechargeBalanceError.value = "";
  rechargeBalanceLoading.value = false;
  const general = Number(row.balance ?? 0);
  const carryover = Number(row.carryover_balance ?? 0);
  const exclusive = Number(row.exclusive_balance ?? 0);
  const total = Number(row.total_balance ?? (general + carryover + exclusive));
  setRechargeBalances(general, carryover, exclusive, total);
  rechargeVisible.value = true;
  void syncRechargeCurrentBalance(rechargeUser.value);
}

async function openProfile(username: string) {
  profileVisible.value = true;
  profileLoading.value = true;
  profileError.value = "";
  profileData.value = null;
  try {
    const r = await client().adminPlatformUserDetail(username);
    profileData.value = r.user;
  } catch (e: any) {
    profileError.value = e?.message ?? String(e);
  } finally {
    profileLoading.value = false;
  }
}

async function disableNodeAccountMapping(acc: { node_id: string; local_username: string }) {
  const billing = String(profileData.value?.username || "").trim();
  const nodeID = String(acc?.node_id || "").trim();
  const local = String(acc?.local_username || "").trim();
  if (!billing || !nodeID || !local) {
    error.value = "禁用失败：映射信息不完整";
    return;
  }
  try {
    await ElMessageBox.confirm(
      `确认禁用该节点账号映射吗？\n平台账号：${billing}\n节点编号：${nodeID}\n节点账号：${local}\n\n禁用后将无法 SSH 登录，并会强制下线且终止该账号当前全部进程。`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认禁用", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  let reason = "";
  try {
    const promptRes = await ElMessageBox.prompt("请输入禁用理由（可留空）", "禁用理由", {
      confirmButtonText: "确认",
      cancelButtonText: "取消",
      inputPlaceholder: "默认空",
    });
    reason = String((promptRes as any)?.value || "").trim();
  } catch {
    return;
  }
  try {
    await client().adminDisableAccountMapping({
      billing_username: billing,
      node_id: nodeID,
      local_username: local,
      reason,
    });
    success.value = `已禁用映射：${nodeID}/${local}（已下发强制下线与进程终止）`;
    await openProfile(billing);
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function enableNodeAccountMapping(acc: { node_id: string; local_username: string }) {
  const billing = String(profileData.value?.username || "").trim();
  const nodeID = String(acc?.node_id || "").trim();
  const local = String(acc?.local_username || "").trim();
  if (!billing || !nodeID || !local) {
    error.value = "解除禁用失败：映射信息不完整";
    return;
  }
  try {
    await ElMessageBox.confirm(
      `确认解除该节点账号映射的禁用吗？\n平台账号：${billing}\n节点编号：${nodeID}\n节点账号：${local}`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认解除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  let reason = "";
  try {
    const promptRes = await ElMessageBox.prompt("请输入解除禁用理由（可留空）", "解除禁用理由", {
      confirmButtonText: "确认",
      cancelButtonText: "取消",
      inputPlaceholder: "默认空",
    });
    reason = String((promptRes as any)?.value || "").trim();
  } catch {
    return;
  }
  try {
    try {
      await client().adminEnableAccountMapping({
        billing_username: billing,
        node_id: nodeID,
        local_username: local,
        reason,
      });
    } catch (e: any) {
      if (e?.status === 404) {
        await client().adminDeleteBlacklist(nodeID, local);
      } else {
        throw e;
      }
    }
    success.value = `已解除禁用映射：${nodeID}/${local}`;
    await openProfile(billing);
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function blockUser(row: AdminUserDetail) {
  if (isBlack(row)) {
    ElMessage.warning(`账号 ${row.username} 已在黑名单`);
    return;
  }
  let reason = "";
  try {
    await ElMessageBox.confirm(`确认拉黑平台账号 ${row.username} 吗？`, "二次确认", { type: "warning", confirmButtonText: "确认拉黑", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    const promptRes = await ElMessageBox.prompt("请输入拉黑理由（可留空）", "拉黑理由", {
      confirmButtonText: "确认",
      cancelButtonText: "取消",
      inputPlaceholder: "默认空",
    });
    reason = String((promptRes as any)?.value || "").trim();
  } catch {
    return;
  }
  try {
    try {
      await client().adminBlockUser(row.username, reason);
    } catch (e: any) {
      // 兼容：旧后端没有 /users/:username/block 时，直接写 SSH 黑名单（全节点）。
      if (e?.status === 404) {
        await client().adminUpsertBlacklist("*", [], [row.username], reason);
      } else {
        throw e;
      }
    }
    success.value = `已拉黑：${row.username}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function unblockUser(row: AdminUserDetail) {
  if (!isBlack(row)) {
    ElMessage.warning(`账号 ${row.username} 当前未拉黑`);
    return;
  }
  try {
    await ElMessageBox.confirm(`确认解黑平台账号 ${row.username} 吗？`, "二次确认", { type: "warning", confirmButtonText: "确认解黑", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    try {
      await client().adminUnblockUser(row.username);
    } catch (e: any) {
      if (e?.status !== 404) throw e;
    }
    // 解黑时同步尝试从全局 SSH 黑名单移除
    try {
      await client().adminDeleteBlacklist("*", row.username);
    } catch (e: any) {
      if (e?.status !== 404) throw e;
    }
    success.value = `已解黑：${row.username}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function deleteUser(row: AdminUserDetail) {
  try {
    await ElMessageBox.confirm(`确认删除平台账号 ${row.username} 吗？删除后可在“已删除平台账号”中恢复。`, "二次确认", { type: "warning", confirmButtonText: "确认删除", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    await client().adminDeleteUserCompat(row.username, "管理员手动删除");
    success.value = `已删除：${row.username}`;
    await reload();
  } catch (e: any) {
    if (e?.status === 404 && String(e?.message || "").trim() === "请求的资源不存在") {
      const base = (settingsState.baseUrl || window.location.origin || "").trim() || window.location.origin;
      error.value = `删除接口不可用：当前控制器地址为 ${base}，请确认该实例已更新并重启。`;
    } else {
      error.value = e?.message ?? String(e);
    }
  }
}

async function restoreUser(row: DeletedUserAccount) {
  try {
    await ElMessageBox.confirm(`确认恢复平台账号 ${row.username} 吗？`, "二次确认", { type: "warning", confirmButtonText: "确认恢复", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    await client().adminRestoreDeletedUser(row.deleted_id);
    success.value = `已恢复：${row.username}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function queryDuplicates(row: AdminUserDetail) {
  duplicatesVisible.value = true;
  duplicatesLoading.value = true;
  duplicateActiveRows.value = [];
  duplicateDeletedRows.value = [];
  try {
    const r = await client().adminFindUserDuplicates({
      username: row.username,
      email: row.email,
      student_id: row.student_id,
      limit: 200,
    });
    duplicateActiveRows.value = r.active_users ?? [];
    duplicateDeletedRows.value = r.deleted_users ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    duplicatesLoading.value = false;
  }
}

async function doRecharge() {
  rechargeLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminRecharge(rechargeUser.value, rechargeAmount.value, rechargeMethod.value);
    const general = Number(r.general_balance ?? r.balance ?? rechargeGeneralBalance.value);
    const carryover = Number(r.carryover_balance ?? rechargeCarryoverBalance.value);
    const exclusive = Number(r.exclusive_balance ?? rechargeExclusiveBalance.value);
    const total = Number(r.total_balance ?? (general + carryover + exclusive));
    setRechargeBalances(general, carryover, exclusive, total);
    rechargeVisible.value = false;
    success.value = `积分调整成功，当前通用积分 ${fmt2(general)}、结转积分 ${fmt2(carryover)}，总积分 ${fmt2(total)}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    rechargeLoading.value = false;
  }
}

async function loadGraduationDueUsers() {
  dueLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminGraduationDueUsers(5000);
    dueRows.value = (r.users ?? []).map((x) => ({ ...x }));
    selectedDueRows.value = [];
    success.value = `查询完成：共 ${dueRows.value.length} 人已到期`;
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    dueLoading.value = false;
  }
}

function onDueSelectionChange(rows: GraduationDueView[]) {
  selectedDueRows.value = rows ?? [];
}

async function sendReminder(usernames: string[]) {
  if (usernames.length === 0) {
    ElMessage.warning("请先选择用户");
    return;
  }
  sendLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminSendGraduationReminders(usernames);
    const m = new Map((r.results ?? []).map((x) => [x.username, x]));
    dueRows.value = dueRows.value.map((row) => {
      const rr = m.get(row.username);
      if (!rr) return row;
      return {
        ...row,
        send_success: rr.success,
        send_error: rr.success ? "" : (rr.error || "发送失败"),
      };
    });
    success.value = `发送完成：成功 ${r.success}，失败 ${r.failed}`;
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    sendLoading.value = false;
  }
}

async function sendSelectedDueUsers() {
  await sendReminder(selectedDueRows.value.map((x) => x.username));
}

async function sendAllDueUsers() {
  await sendReminder(dueRows.value.map((x) => x.username));
}

async function sendSingleDueUser(row: GraduationDueView) {
  await sendReminder([row.username]);
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
.tone-delete {
  background: linear-gradient(135deg, #991b1b, #dc2626);
  color: #fee2e2;
}
.tone-list {
  background: linear-gradient(135deg, #0f766e, #14b8a6);
  color: #ccfbf1;
}
.tone-profile {
  background: linear-gradient(135deg, #1e3a8a, #2563eb);
  color: #dbeafe;
}
.tone-map {
  background: linear-gradient(135deg, #312e81, #4f46e5);
  color: #e0e7ff;
}
.tone-dup {
  background: linear-gradient(135deg, #9a3412, #ea580c);
  color: #ffedd5;
}
.tone-active {
  background: linear-gradient(135deg, #166534, #16a34a);
  color: #dcfce7;
}
.tone-remind {
  background: linear-gradient(135deg, #a16207, #d97706);
  color: #fef3c7;
}
.dialog-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.mt10 {
  margin-top: 10px;
}
.sub {
  margin-top: 4px;
  font-size: 12px;
  color: #6b7280;
}
.recharge-balance-wrap {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.recharge-balance-error {
  color: var(--error-color);
}
.expand-wrap {
  padding: 8px 12px;
}
.mb {
  margin-bottom: 10px;
}
.username-link {
  padding: 0;
  border: none;
  background: transparent;
  color: #16a34a;
  font-weight: 600;
  cursor: pointer;
}
.username-link:hover {
  text-decoration: underline;
}
.username-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.black-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
  background: #dc2626;
}
.white-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
  background: #2563eb;
}
.exempt-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
  background: #7c3aed;
}
</style>
