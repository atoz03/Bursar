<template>
  <el-container class="shell" :class="{ 'mobile-shell': isMobile, 'mobile-menu-open': mobileMenuOpen }">
    <div v-if="isMobile && mobileMenuOpen" class="mobile-mask" @click="mobileMenuOpen = false" />
    <el-aside width="250px" class="aside">
      <div class="brand">
        <el-icon :size="30"><Cpu /></el-icon>
        <div>
          <div class="brand-title">GPU Ops</div>
          <div class="brand-sub">{{ t("GPU 运维平台", "GPU Operations Platform") }}</div>
        </div>
      </div>

      <el-menu :default-active="activePath" :default-openeds="defaultOpeneds" router class="menu" @select="onMenuSelect">
        <template v-if="authState.role === 'admin'">
          <el-sub-menu index="grp-ops">
            <template #title><el-icon><DataBoard /></el-icon><span>{{ t("运营分析", "Operations") }}</span></template>
            <el-menu-item index="/admin/board">{{ t("运营看板", "Dashboard") }}</el-menu-item>
            <el-menu-item index="/admin/usage">{{ t("进程记录", "Process Records") }}</el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-resource">
            <template #title><el-icon><Monitor /></el-icon><span>{{ t("资源管理", "Resources") }}</span></template>
            <el-menu-item index="/admin/status">{{ t("状态总览", "Status Overview") }}</el-menu-item>
            <el-menu-item index="/admin/nodes">{{ t("节点状态", "Node Status") }}</el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-points">
            <template #title><el-icon><WalletFilled /></el-icon><span>{{ t("积分管理", "Points") }}</span></template>
            <el-menu-item index="/admin/points">{{ t("积分管理", "Points") }}</el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-access">
            <template #title><el-icon><UserFilled /></el-icon><span>{{ t("账号与访问", "Accounts & Access") }}</span></template>
            <el-menu-item index="/admin/users">{{ t("平台用户管理", "Platform Users") }}</el-menu-item>
            <el-menu-item index="/admin/accounts">
              <el-badge :value="accountMappingTodoCount" :hidden="accountMappingTodoCount === 0">
                <span>{{ t("账号映射", "Account Mapping") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/admin/account-provision">
              <el-badge :value="accountProvisionTodoCount" :hidden="accountProvisionTodoCount === 0" type="danger">
                <span>{{ t("节点账号开通", "Node Account Provision") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/admin/requests">
              <el-badge :is-dot="reviewTodoCount > 0" type="danger">
                <span>{{ t("平台账号注册审核", "Registration Review") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/admin/power-users">{{ t("高级用户", "Power Users") }}</el-menu-item>
            <el-menu-item index="/admin/whitelist">{{ t("SSH名单", "SSH Lists") }}</el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-system">
            <template #title><el-icon><Setting /></el-icon><span>{{ t("系统与容灾", "System") }}</span></template>
            <el-menu-item index="/admin/ha">{{ t("容灾同步", "HA Sync") }}</el-menu-item>
            <el-menu-item index="/admin/announcements">{{ t("公告管理", "Announcements") }}</el-menu-item>
            <el-menu-item index="/admin/guideline">{{ t("用户准则", "Guidelines") }}</el-menu-item>
            <el-menu-item index="/admin/notebook">{{ t("管理员记事本", "Notebook") }}</el-menu-item>
            <el-menu-item index="/admin/mail">{{ t("邮件设置", "Mail Settings") }}</el-menu-item>
            <el-menu-item index="/admin/profile">{{ t("个人信息", "Profile") }}</el-menu-item>
            <el-menu-item index="/admin/change-password">{{ t("修改密码", "Change Password") }}</el-menu-item>
          </el-sub-menu>
        </template>
        <template v-else-if="authState.role === 'power_user'">
          <el-sub-menu index="grp-power">
            <template #title><el-icon><DataBoard /></el-icon><span>{{ t("授权功能", "Authorized") }}</span></template>
            <el-menu-item v-if="authState.canViewBoard" index="/admin/board">{{ t("运营看板", "Dashboard") }}</el-menu-item>
            <el-menu-item v-if="authState.canViewNodes" index="/admin/nodes">{{ t("节点状态", "Node Status") }}</el-menu-item>
            <el-menu-item v-if="authState.canManagePoints || authState.canPointsUsers || authState.canPointsBatchFiltered || authState.canPointsBatchAll || authState.canPointsRecords || authState.canPointsMonthly || authState.canPointsSpecialRules" index="/admin/points">{{ t("积分管理", "Points") }}</el-menu-item>
            <el-menu-item v-if="authState.canManagePlatformUsers" index="/admin/users">{{ t("平台用户管理", "Platform Users") }}</el-menu-item>
            <el-menu-item v-if="authState.canReviewRequests" index="/admin/requests">
              <el-badge :is-dot="reviewTodoCount > 0" type="danger">
                <span>{{ t("平台账号注册审核", "Registration Review") }}</span>
              </el-badge>
            </el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-user">
            <template #title><el-icon><WalletFilled /></el-icon><span>{{ t("我的中心", "My Center") }}</span></template>
            <el-menu-item index="/user/notices">
              <el-badge :is-dot="userNoticeHasNew">
                <span>{{ t("公告与用户准则", "Notices & Guidelines") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/user/balance">
              <el-badge :is-dot="userPointsUnreadCount > 0" type="danger">
                <span>{{ t("我的积分", "My Points") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/user/usage">{{ t("我的用量", "My Usage") }}</el-menu-item>
            <el-menu-item index="/user/profile">{{ t("个人资料", "Profile") }}</el-menu-item>
            <el-menu-item index="/user/accounts">
              <el-badge :is-dot="userAccountProvisionHasNew" type="danger">
                <span>{{ t("节点账号", "Node Accounts") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/user/change-password">{{ t("修改密码", "Change Password") }}</el-menu-item>
          </el-sub-menu>
        </template>
        <template v-else>
          <el-sub-menu index="grp-user">
            <template #title><el-icon><WalletFilled /></el-icon><span>{{ t("我的中心", "My Center") }}</span></template>
            <el-menu-item index="/user/notices">
              <el-badge :is-dot="userNoticeHasNew">
                <span>{{ t("公告与用户准则", "Notices & Guidelines") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/user/balance">
              <el-badge :is-dot="userPointsUnreadCount > 0" type="danger">
                <span>{{ t("我的积分", "My Points") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/user/usage">{{ t("我的用量", "My Usage") }}</el-menu-item>
            <el-menu-item index="/user/profile">{{ t("个人资料", "Profile") }}</el-menu-item>
            <el-menu-item index="/user/accounts">
              <el-badge :is-dot="userAccountProvisionHasNew" type="danger">
                <span>{{ t("节点账号", "Node Accounts") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/user/change-password">{{ t("修改密码", "Change Password") }}</el-menu-item>
          </el-sub-menu>
        </template>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header">
        <div class="header-left">
          <el-button v-if="isMobile" text class="mobile-menu-toggle" @click="toggleMobileMenu">
            <el-icon><Menu /></el-icon>
            <span>{{ t("菜单", "Menu") }}</span>
          </el-button>
          <template v-if="authState.role === 'admin'">
            <el-button text class="controller-toggle" @click="showControllerConfig = !showControllerConfig">
              <el-icon><Link /></el-icon>
              <span>{{ showControllerConfig ? t("收起控制器地址", "Hide Controller URL") : t("控制器地址（高级）", "Controller URL") }}</span>
            </el-button>
            <div v-if="showControllerConfig" class="controller-editor">
              <span class="muted">{{ t("控制器地址", "Controller URL") }}</span>
              <el-input
                v-model="settingsState.baseUrl"
                :placeholder="t('留空表示当前站点', 'Leave empty to use current site')"
                style="max-width: 320px"
                @change="persist"
                clearable
              />
              <el-button @click="persist" type="primary" size="small">
                <el-icon><Check /></el-icon>
                {{ t("保存", "Save") }}
              </el-button>
            </div>
          </template>
        </div>
        <div class="header-right">
          <el-button text @click="toggleUiLanguage">{{ uiLocaleState.language === "en" ? "中" : "EN" }}</el-button>
          <el-tag type="success" effect="light">
            {{ authState.role === 'admin' ? t('管理员', 'Admin') : (authState.role === 'power_user' ? t('高级用户', 'Power User') : t('用户', 'User')) }}
          </el-tag>
          <el-tag effect="plain">{{ authState.username }}</el-tag>
          <el-button @click="doLogout">
            <el-icon><SwitchButton /></el-icon>
            {{ t("退出", "Logout") }}
          </el-button>
        </div>
      </el-header>

      <el-main class="main">
        <el-alert
          v-if="showApiBaseWarning"
          :title="t(`当前页面 API 实际连接到：${effectiveApiBase}`, `Current API base: ${effectiveApiBase}`)"
          type="warning"
          show-icon
          :closable="false"
          style="margin-bottom: 12px"
        />
        <router-view :key="activePath" />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { persistSettings, settingsState } from "../lib/settingsStore";
import { authState, logout } from "../lib/authStore";
import { ElMessage } from "element-plus";
import { ApiClient, type UserNodeAccountMappingRisk, type UserRequest } from "../lib/api";
import { toServerEpochMs } from "../lib/time";
import { pickText, toggleUiLanguage, uiLocaleState } from "../lib/uiLocale";
import {
  Check,
  Cpu,
  DataBoard,
  Link,
  Menu,
  Monitor,
  SwitchButton,
  Setting,
  UserFilled,
  WalletFilled,
} from "@element-plus/icons-vue";

const route = useRoute();
const router = useRouter();
const activePath = computed(() => route.path);
const defaultOpeneds = computed(() => ["grp-ops", "grp-resource", "grp-points", "grp-access", "grp-system", "grp-user", "grp-power"]);
const reviewTodoCount = ref(0);
const accountOpenTodoCount = ref(0);
const accountRiskTodoCount = ref(0);
const accountUnbindTodoCount = ref(0);
const accountMappingTodoCount = computed(() =>
  Number(accountRiskTodoCount.value || 0) + Number(accountUnbindTodoCount.value || 0),
);
const accountProvisionTodoCount = computed(() => Number(accountOpenTodoCount.value || 0));
const effectiveApiBase = computed(() => settingsState.baseUrl?.trim() || window.location.origin);
const showApiBaseWarning = computed(() => {
  const saved = settingsState.baseUrl?.trim() || "";
  const origin = window.location.origin;
  return !!saved && saved !== origin;
});
const showControllerConfig = ref(false);
const userNoticeHasNew = ref(false);
const userPointsUnreadCount = ref(0);
const userAccountProvisionHasNew = ref(false);
const isMobile = ref(false);
const mobileMenuOpen = ref(false);
let reviewTodoTimer: ReturnType<typeof setInterval> | null = null;
let userNoticeTimer: ReturnType<typeof setInterval> | null = null;
let userPointsTimer: ReturnType<typeof setInterval> | null = null;
let userAccountProvisionTimer: ReturnType<typeof setInterval> | null = null;

function t(zh: string, en: string): string {
  return pickText(zh, en);
}

function persist() {
  persistSettings();
  ElMessage.success(t("保存成功", "Saved"));
}

async function doLogout() {
  await logout();
  await router.push("/login");
}

function syncMobileState() {
  const next = window.innerWidth <= 920;
  isMobile.value = next;
  if (!next) {
    mobileMenuOpen.value = false;
  }
}

function toggleMobileMenu() {
  if (!isMobile.value) return;
  mobileMenuOpen.value = !mobileMenuOpen.value;
}

function onMenuSelect() {
  if (isMobile.value) {
    mobileMenuOpen.value = false;
  }
}

function clearReviewTodoTimer() {
  if (reviewTodoTimer) {
    clearInterval(reviewTodoTimer);
    reviewTodoTimer = null;
  }
}

function clearUserNoticeTimer() {
  if (userNoticeTimer) {
    clearInterval(userNoticeTimer);
    userNoticeTimer = null;
  }
}

function clearUserPointsTimer() {
  if (userPointsTimer) {
    clearInterval(userPointsTimer);
    userPointsTimer = null;
  }
}

function clearUserAccountProvisionTimer() {
  if (userAccountProvisionTimer) {
    clearInterval(userAccountProvisionTimer);
    userAccountProvisionTimer = null;
  }
}

function canLoadReviewTodo(): boolean {
  if (!authState.authenticated) return false;
  if (authState.role === "admin") return true;
  return authState.role === "power_user" && !!authState.canReviewRequests;
}

function canLoadAccountOpenTodo(): boolean {
  return !!authState.authenticated && authState.role === "admin";
}

function canLoadUserNotice(): boolean {
  return !!authState.authenticated && (authState.role === "user" || authState.role === "power_user");
}

function canLoadUserPoints(): boolean {
  return !!authState.authenticated && (authState.role === "user" || authState.role === "power_user");
}

function canLoadUserAccountProvision(): boolean {
  return !!authState.authenticated && (authState.role === "user" || authState.role === "power_user");
}

function isDocumentVisible(): boolean {
  if (typeof document === "undefined") return true;
  return !document.hidden;
}

function userAnnouncementSeenKey(): string {
  const u = String(authState.username || "").trim() || "anonymous";
  return `gpuops_seen_announcement_ts_${u}`;
}

function userPointsSeenKey(): string {
  const u = String(authState.username || "").trim() || "anonymous";
  return `gpuops_seen_points_recharge_id_${u}`;
}

async function loadReviewTodoCount() {
  if (!canLoadReviewTodo()) {
    reviewTodoCount.value = 0;
    if (!canLoadAccountOpenTodo()) {
      accountOpenTodoCount.value = 0;
      accountRiskTodoCount.value = 0;
      accountUnbindTodoCount.value = 0;
    }
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const [regRes, profileRes, openRes, riskRes] = await Promise.allSettled([
      client.adminRegistrationRequestsOverview({ limit: 2000 }),
      client.adminProfileChangeRequests({ status: "pending", limit: 2000 }),
      canLoadAccountOpenTodo()
        ? client.adminRequests({ status: "pending", limit: 5000 })
        : Promise.resolve<{ requests: UserRequest[] }>({ requests: [] }),
      canLoadAccountOpenTodo()
        ? client.adminAccountMappingRisks({ days: 30, min_switches: 2, limit: 5000 })
        : Promise.resolve<{ days: number; min_switches: number; total_risky: number; risky_accounts: UserNodeAccountMappingRisk[] }>({
            days: 30,
            min_switches: 2,
            total_risky: 0,
            risky_accounts: [],
          }),
    ]);
    let total = 0;
    if (regRes.status === "fulfilled") {
      total += Number((regRes.value.pending ?? []).length + (regRes.value.conflicts ?? []).length);
    }
    if (profileRes.status === "fulfilled") {
      total += Number((profileRes.value.requests ?? []).length);
    }
    reviewTodoCount.value = total;
    if (openRes.status === "fulfilled") {
      accountOpenTodoCount.value = Number((openRes.value.requests ?? []).filter((x) => String(x.request_type || "").trim() === "open").length);
      accountUnbindTodoCount.value = Number((openRes.value.requests ?? []).filter((x) => String(x.request_type || "").trim() === "unbind").length);
    } else if (!canLoadAccountOpenTodo()) {
      accountOpenTodoCount.value = 0;
      accountUnbindTodoCount.value = 0;
    } else {
      accountOpenTodoCount.value = 0;
      accountUnbindTodoCount.value = 0;
    }
    if (riskRes.status === "fulfilled") {
      const totalRisk = Number(riskRes.value.total_risky || 0);
      accountRiskTodoCount.value = Number.isFinite(totalRisk)
        ? Math.max(0, Math.floor(totalRisk))
        : Number((riskRes.value.risky_accounts ?? []).length);
    } else if (!canLoadAccountOpenTodo()) {
      accountRiskTodoCount.value = 0;
    } else {
      accountRiskTodoCount.value = 0;
    }
  } catch (e: any) {
    if (e?.status === 404 || e?.status === 403 || e?.status === 401) {
      reviewTodoCount.value = 0;
      accountOpenTodoCount.value = 0;
      accountRiskTodoCount.value = 0;
      accountUnbindTodoCount.value = 0;
      return;
    }
  }
}

function resetReviewTodoPolling() {
  clearReviewTodoTimer();
  if (!isDocumentVisible()) return;
  loadReviewTodoCount();
  if (!canLoadReviewTodo()) return;
  reviewTodoTimer = setInterval(() => {
    if (!isDocumentVisible()) return;
    loadReviewTodoCount();
  }, 30000);
}

async function loadUserNoticeState() {
  if (!canLoadUserNotice()) {
    userNoticeHasNew.value = false;
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.announcements(1);
    const latest = String(r.announcements?.[0]?.created_at || "").trim();
    if (!latest) {
      userNoticeHasNew.value = false;
      return;
    }
    const seen = String(localStorage.getItem(userAnnouncementSeenKey()) || "").trim();
    if (!seen) {
      userNoticeHasNew.value = true;
      return;
    }
    const latestTs = toServerEpochMs(latest);
    const seenTs = toServerEpochMs(seen);
    if (Number.isNaN(latestTs) || Number.isNaN(seenTs)) {
      userNoticeHasNew.value = latest !== seen;
      return;
    }
    userNoticeHasNew.value = latestTs > seenTs;
  } catch {
    // ignore
  }
}

function resetUserNoticePolling() {
  clearUserNoticeTimer();
  if (!isDocumentVisible()) return;
  loadUserNoticeState();
  if (!canLoadUserNotice()) return;
  userNoticeTimer = setInterval(() => {
    if (!isDocumentVisible()) return;
    loadUserNoticeState();
  }, 30000);
}

async function loadUserPointsState() {
  if (!canLoadUserPoints()) {
    userPointsUnreadCount.value = 0;
    return;
  }
  try {
    const raw = String(localStorage.getItem(userPointsSeenKey()) || "").trim();
    const seenID = Number(raw);
    const sinceID = Number.isFinite(seenID) && seenID > 0 ? Math.floor(seenID) : 0;
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.userMyPointsIncrements({ sinceId: sinceID, limit: 200 });
    const unread = Number(r.unread_count || 0);
    userPointsUnreadCount.value = Number.isFinite(unread) ? Math.max(0, Math.floor(unread)) : 0;
  } catch (e: any) {
    if (e?.status === 404 || e?.status === 403 || e?.status === 401) {
      userPointsUnreadCount.value = 0;
      return;
    }
    userPointsUnreadCount.value = 0;
  }
}

function resetUserPointsPolling() {
  clearUserPointsTimer();
  if (!isDocumentVisible()) return;
  loadUserPointsState();
  if (!canLoadUserPoints()) return;
  userPointsTimer = setInterval(() => {
    if (!isDocumentVisible()) return;
    loadUserPointsState();
  }, 30000);
}

function isProvisionMessageDestroyedLike(row: any): boolean {
  if (!row) return true;
  if (String(row.destroyed_at || "").trim()) return true;
  if (!String(row.encrypted_payload || "").trim()) return true;
  const destroyAfterText = String(row.destroy_after_at || "").trim();
  if (!destroyAfterText) return false;
  const destroyAfterMs = toServerEpochMs(destroyAfterText);
  if (Number.isNaN(destroyAfterMs)) return false;
  return Date.now() >= destroyAfterMs;
}

async function loadUserAccountProvisionState() {
  if (!canLoadUserAccountProvision()) {
    userAccountProvisionHasNew.value = false;
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.userProvisionMessages(60);
    const items = Array.isArray(r.messages) ? r.messages : [];
    userAccountProvisionHasNew.value = items.some((x) => {
      if (isProvisionMessageDestroyedLike(x)) return false;
      return !String(x?.first_decrypted_at || "").trim();
    });
  } catch (e: any) {
    if (e?.status === 404 || e?.status === 403 || e?.status === 401) {
      userAccountProvisionHasNew.value = false;
      return;
    }
    userAccountProvisionHasNew.value = false;
  }
}

function resetUserAccountProvisionPolling() {
  clearUserAccountProvisionTimer();
  if (!isDocumentVisible()) return;
  loadUserAccountProvisionState();
  if (!canLoadUserAccountProvision()) return;
  userAccountProvisionTimer = setInterval(() => {
    if (!isDocumentVisible()) return;
    loadUserAccountProvisionState();
  }, 30000);
}

function onVisibilityChange() {
  if (!isDocumentVisible()) {
    clearReviewTodoTimer();
    clearUserNoticeTimer();
    clearUserPointsTimer();
    clearUserAccountProvisionTimer();
    return;
  }
  resetReviewTodoPolling();
  resetUserNoticePolling();
  resetUserPointsPolling();
  resetUserAccountProvisionPolling();
}

watch(
  () => [authState.authenticated, authState.role, authState.canReviewRequests, settingsState.baseUrl, authState.csrfToken],
  () => {
    resetReviewTodoPolling();
    resetUserNoticePolling();
    resetUserPointsPolling();
    resetUserAccountProvisionPolling();
  },
  { immediate: true },
);

watch(
  () => route.path,
  () => {
    if (isMobile.value) {
      mobileMenuOpen.value = false;
    }
  },
);

onMounted(() => {
  syncMobileState();
  resetReviewTodoPolling();
  resetUserNoticePolling();
  resetUserPointsPolling();
  resetUserAccountProvisionPolling();
  window.addEventListener("resize", syncMobileState);
  window.addEventListener("gpuops-announcement-seen", loadUserNoticeState);
  window.addEventListener("gpuops-points-seen", loadUserPointsState);
  document.addEventListener("visibilitychange", onVisibilityChange);
});

onBeforeUnmount(() => {
  clearReviewTodoTimer();
  clearUserNoticeTimer();
  clearUserPointsTimer();
  clearUserAccountProvisionTimer();
  window.removeEventListener("resize", syncMobileState);
  window.removeEventListener("gpuops-announcement-seen", loadUserNoticeState);
  window.removeEventListener("gpuops-points-seen", loadUserPointsState);
  document.removeEventListener("visibilitychange", onVisibilityChange);
});
</script>

<style scoped>
.shell {
  position: relative;
  min-height: 100vh;
  overflow: hidden;
  background: linear-gradient(160deg, #edf5ff 0%, #f3f8ff 52%, #f8fbff 100%);
}
.aside {
  border-right: 1px solid #c7d2fe;
  background: linear-gradient(190deg, #0f172a 0%, #1e3a8a 100%);
  position: relative;
  z-index: 1;
  box-shadow: 2px 0 14px rgba(15, 23, 42, 0.08);
}
.brand {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 18px 16px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.35);
  color: #f8fafc;
}
.brand-title {
  font-weight: 800;
  letter-spacing: 0.3px;
}
.brand-sub {
  font-size: 12px;
  color: rgba(226, 232, 240, 0.8);
}
.menu {
  border-right: none;
  padding: 8px;
  background: transparent;
}
.menu :deep(.el-menu) {
  border-right: none !important;
  background: transparent !important;
}
.menu :deep(.el-sub-menu .el-menu) {
  margin: 4px 0 10px;
  padding: 6px;
  border-radius: 12px;
  background: #1e293b !important;
}
.menu :deep(.el-sub-menu__title) {
  color: #ecfeff !important;
  border-radius: 10px;
  font-weight: 700;
}
.menu :deep(.el-sub-menu__icon-arrow) {
  color: rgba(236, 254, 255, 0.9) !important;
}
.menu :deep(.el-menu-item) {
  color: #dff8f5 !important;
  border-radius: 10px;
}
.menu :deep(.el-sub-menu .el-menu-item) {
  min-width: auto;
  margin-left: 6px;
}
.menu :deep(.el-menu-item:hover) {
  color: #ffffff !important;
  background: #1d4ed8 !important;
}
.menu :deep(.el-menu-item.is-active) {
  color: #67e8f9 !important;
  background: #0f2d73 !important;
  border-radius: 10px;
  font-weight: 700;
  box-shadow: inset 3px 0 0 #38bdf8;
}
.menu :deep(.el-menu-item.is-disabled) {
  color: rgba(226, 232, 240, 0.7) !important;
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border-bottom: 1px solid #d7e2f0;
  background: #ffffff;
  border-radius: 0 0 14px 14px;
  box-shadow: 0 8px 18px rgba(15, 23, 42, 0.08);
  position: relative;
  z-index: 1;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}
.controller-toggle {
  color: #0f766e;
  font-weight: 700;
  padding: 0;
}
.controller-editor {
  display: flex;
  align-items: center;
  gap: 8px;
}
.muted {
  color: #475569;
}
.header-right {
  display: flex;
  gap: 8px;
  align-items: center;
}
.main {
  padding: 18px;
  position: relative;
  z-index: 1;
}
.mobile-mask {
  position: fixed;
  inset: 0;
  z-index: 1100;
  background: rgba(15, 23, 42, 0.45);
  backdrop-filter: blur(1px);
}
.mobile-menu-toggle {
  color: #0f172a;
  font-weight: 700;
  padding: 0;
}

@media (max-width: 920px) {
  .shell {
    overflow-x: hidden;
  }
  .mobile-shell .aside {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    width: 250px !important;
    z-index: 1200;
    overflow-y: auto;
    transform: translateX(-104%);
    transition: transform 0.2s ease;
  }
  .mobile-shell.mobile-menu-open .aside {
    transform: translateX(0);
  }
  .mobile-shell .header {
    border-radius: 0;
  }
  .mobile-shell .main {
    padding: 10px;
  }
  .header {
    flex-wrap: wrap;
  }
  .header-left,
  .header-right {
    width: 100%;
  }
  .header-right {
    justify-content: flex-start;
  }
  .controller-editor {
    width: 100%;
    flex-wrap: wrap;
  }
}
</style>
