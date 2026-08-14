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
          <el-sub-menu index="grp-overview">
            <template #title><el-icon><DataBoard /></el-icon><span>{{ t("总览", "Overview") }}</span></template>
            <el-menu-item index="/admin/board">{{ t("运营看板", "Dashboard") }}</el-menu-item>
            <el-menu-item index="/admin/status">{{ t("集群总览", "Cluster Overview") }}</el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-resource">
            <template #title><el-icon><Monitor /></el-icon><span>{{ t("资源与计费", "Resources & Billing") }}</span></template>
            <el-menu-item index="/admin/nodes">{{ t("节点管理", "Node Management") }}</el-menu-item>
            <el-menu-item index="/admin/usage">{{ t("进程审计", "Process Audit") }}</el-menu-item>
            <el-menu-item index="/admin/points">{{ t("积分管理", "Points") }}</el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-access">
            <template #title><el-icon><UserFilled /></el-icon><span>{{ t("账号与访问", "Accounts & Access") }}</span></template>
            <el-menu-item index="/admin/users">{{ t("平台用户", "Platform Users") }}</el-menu-item>
            <el-menu-item index="/admin/accounts">
              <el-badge :value="accountMappingTodoCount" :hidden="accountMappingTodoCount === 0">
                <span>{{ t("账号映射", "Account Mapping") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/admin/account-provision">
              <el-badge :value="accountProvisionTodoCount" :hidden="accountProvisionTodoCount === 0" type="danger">
                <span>{{ t("账号开通", "Account Provision") }}</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/admin/requests">
              <el-badge :is-dot="reviewTodoCount > 0" type="danger">
                <span>{{ t("注册与资料审核", "Account Review") }}</span>
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
          </el-sub-menu>
        </template>
        <template v-else-if="authState.role === 'power_user'">
          <el-sub-menu index="grp-power">
            <template #title><el-icon><DataBoard /></el-icon><span>{{ t("授权功能", "Authorized") }}</span></template>
            <el-menu-item v-if="authState.canViewBoard" index="/admin/board">{{ t("运营看板", "Dashboard") }}</el-menu-item>
            <el-menu-item v-if="authState.canViewNodes" index="/admin/nodes">{{ t("节点管理", "Node Management") }}</el-menu-item>
            <el-menu-item v-if="authState.canManagePoints || authState.canPointsUsers || authState.canPointsBatchFiltered || authState.canPointsBatchAll || authState.canPointsRecords || authState.canPointsMonthly || authState.canPointsSpecialRules" index="/admin/points">{{ t("积分管理", "Points") }}</el-menu-item>
            <el-menu-item v-if="authState.canManagePlatformUsers" index="/admin/users">{{ t("平台用户管理", "Platform Users") }}</el-menu-item>
            <el-menu-item v-if="authState.canReviewRequests" index="/admin/requests">
              <el-badge :is-dot="reviewTodoCount > 0" type="danger">
                <span>{{ t("注册与资料审核", "Account Review") }}</span>
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
            <el-menu-item index="/user/accounts">
              <el-badge :is-dot="userAccountProvisionHasNew" type="danger">
                <span>{{ t("节点账号", "Node Accounts") }}</span>
              </el-badge>
            </el-menu-item>
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
            <el-menu-item index="/user/accounts">
              <el-badge :is-dot="userAccountProvisionHasNew" type="danger">
                <span>{{ t("节点账号", "Node Accounts") }}</span>
              </el-badge>
            </el-menu-item>
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
              <span>{{ showControllerConfig ? t("收起连接设置", "Hide Connection Settings") : t("连接设置", "Connection Settings") }}</span>
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
          <el-dropdown trigger="click" @command="onUserCommand">
            <button type="button" class="user-menu-trigger">
              <span class="user-avatar">{{ userInitial }}</span>
              <span class="user-meta">
                <strong>{{ authState.username }}</strong>
                <small>{{ accountRoleLabel }}</small>
              </span>
              <el-icon><ArrowDown /></el-icon>
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="profile"><el-icon><User /></el-icon>{{ t("个人资料", "Profile") }}</el-dropdown-item>
                <el-dropdown-item command="password"><el-icon><Key /></el-icon>{{ t("修改密码", "Change Password") }}</el-dropdown-item>
                <el-dropdown-item divided command="logout"><el-icon><SwitchButton /></el-icon>{{ t("退出登录", "Logout") }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
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
import { ApiClient } from "../lib/api";
import { toServerEpochMs } from "../lib/time";
import { pickText, toggleUiLanguage, uiLocaleState } from "../lib/uiLocale";
import {
  ArrowDown,
  Check,
  Cpu,
  DataBoard,
  Key,
  Link,
  Menu,
  Monitor,
  SwitchButton,
  Setting,
  User,
  UserFilled,
  WalletFilled,
} from "@element-plus/icons-vue";

const route = useRoute();
const router = useRouter();
const activePath = computed(() => route.path);
const defaultOpeneds = computed(() => {
  if (authState.role === "admin") {
    return ["grp-overview", "grp-resource", "grp-access", "grp-system"];
  }
  if (authState.role === "power_user") return ["grp-power", "grp-user"];
  return ["grp-user"];
});
const reviewTodoCount = ref(0);
const accountOpenTodoCount = ref(0);
const accountUnbindTodoCount = ref(0);
const accountMappingTodoCount = computed(() => Number(accountUnbindTodoCount.value || 0));
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
const NAV_POLL_INTERVAL_MS = 60 * 1000;
const accountRoleLabel = computed(() => authState.role === "admin"
  ? t("管理员", "Admin")
  : authState.role === "power_user"
    ? t("高级用户", "Power User")
    : t("用户", "User"));
const userInitial = computed(() => String(authState.username || "U").trim().slice(0, 1).toUpperCase());

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

async function onUserCommand(command: string) {
  if (command === "logout") {
    await doLogout();
    return;
  }
  if (command === "profile") {
    await router.push(authState.role === "admin" ? "/admin/profile" : "/user/profile");
    return;
  }
  if (command === "password") {
    await router.push("/user/change-password");
  }
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
      accountUnbindTodoCount.value = 0;
    }
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    try {
      const summary = await client.adminNavigationSummary();
      reviewTodoCount.value = Math.max(0, Number(summary.review_pending || 0));
      accountOpenTodoCount.value = canLoadAccountOpenTodo() ? Math.max(0, Number(summary.open_pending || 0)) : 0;
      accountUnbindTodoCount.value = canLoadAccountOpenTodo() ? Math.max(0, Number(summary.unbind_pending || 0)) : 0;
      return;
    } catch (e: any) {
      if (e?.status !== 404) throw e;
    }

    const [registration, profile, accountRequests] = await Promise.all([
      client.adminRegistrationRequestsOverview({ limit: 2000 }),
      client.adminProfileChangeRequests({ status: "pending", limit: 2000 }),
      canLoadAccountOpenTodo() ? client.adminRequests({ status: "pending", limit: 5000 }) : Promise.resolve({ requests: [] }),
    ]);
    reviewTodoCount.value = Number((registration.pending ?? []).length + (registration.conflicts ?? []).length + (profile.requests ?? []).length);
    accountOpenTodoCount.value = Number((accountRequests.requests ?? []).filter((row) => String(row.request_type || "").trim() === "open").length);
    accountUnbindTodoCount.value = Number((accountRequests.requests ?? []).filter((row) => String(row.request_type || "").trim() === "unbind").length);
  } catch (e: any) {
    if (e?.status === 404 || e?.status === 403 || e?.status === 401) {
      reviewTodoCount.value = 0;
      accountOpenTodoCount.value = 0;
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
  }, NAV_POLL_INTERVAL_MS);
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
  }, NAV_POLL_INTERVAL_MS);
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
  }, NAV_POLL_INTERVAL_MS);
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
  }, NAV_POLL_INTERVAL_MS);
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
  background: transparent;
}
.aside {
  position: sticky;
  top: 0;
  z-index: 20;
  height: 100vh;
  display: flex;
  flex-direction: column;
  border-right: 1px solid rgba(148, 163, 184, .14);
  background: linear-gradient(180deg, #111827 0%, #0b1220 100%);
  box-shadow: 5px 0 24px rgba(15, 23, 42, .08);
}
.brand {
  display: flex;
  gap: 10px;
  align-items: center;
  min-height: 72px;
  padding: 17px 18px;
  border-bottom: 1px solid rgba(148, 163, 184, .14);
  color: #f8fafc;
}
.brand-title {
  font-weight: 800;
  letter-spacing: .06em;
}
.brand-sub {
  font-size: 12px;
  color: rgba(226, 232, 240, 0.8);
}
.menu {
  flex: 1;
  min-height: 0;
  overflow-y: scroll;
  border-right: none;
  padding: 10px 9px 24px;
  background: transparent;
  scrollbar-width: thin;
  scrollbar-color: rgba(148, 163, 184, .28) transparent;
}
.menu::-webkit-scrollbar {
  width: 8px;
}
.menu::-webkit-scrollbar-track {
  background: transparent;
}
.menu::-webkit-scrollbar-thumb {
  border: 2px solid transparent;
  border-radius: 999px;
  background: rgba(148, 163, 184, .42);
  background-clip: padding-box;
}
.menu::-webkit-scrollbar-thumb:hover {
  background: rgba(191, 219, 254, .62);
  background-clip: padding-box;
}
.menu :deep(.el-menu) {
  border-right: none !important;
  background: transparent !important;
}
.menu :deep(.el-sub-menu .el-menu) {
  margin: 2px 0 8px;
  padding: 3px;
  border-radius: 10px;
  background: rgba(255, 255, 255, .035) !important;
}
.menu :deep(.el-sub-menu__title) {
  height: 46px;
  color: #cbd5e1 !important;
  border-radius: 10px;
  font-weight: 650;
}
.menu :deep(.el-sub-menu__icon-arrow) {
  color: rgba(236, 254, 255, 0.9) !important;
}
.menu :deep(.el-menu-item) {
  height: 40px;
  color: #b8c3d4 !important;
  border-radius: 8px;
}
.menu :deep(.el-sub-menu .el-menu-item) {
  min-width: auto;
  margin-left: 6px;
}
.menu :deep(.el-menu-item:hover) {
  color: #ffffff !important;
  background: rgba(59, 130, 246, .14) !important;
}
.menu :deep(.el-menu-item.is-active) {
  color: #ffffff !important;
  background: linear-gradient(90deg, rgba(37, 99, 235, .9), rgba(59, 130, 246, .7)) !important;
  border-radius: 8px;
  font-weight: 700;
  box-shadow: 0 6px 16px rgba(37, 99, 235, .2);
}
.menu :deep(.el-menu-item.is-disabled) {
  color: rgba(226, 232, 240, 0.7) !important;
}
.header {
  position: sticky;
  top: 0;
  z-index: 19;
  min-height: 64px;
  height: auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 22px;
  border-bottom: 1px solid rgba(255, 255, 255, .7);
  background: rgba(255, 255, 255, .52);
  backdrop-filter: blur(20px) saturate(155%);
  -webkit-backdrop-filter: blur(20px) saturate(155%);
  box-shadow: 0 4px 18px rgba(15, 23, 42, .045), inset 0 -1px 0 rgba(148, 163, 184, .08);
}
.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}
.controller-toggle {
  color: #475569;
  font-weight: 650;
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
.user-menu-trigger {
  min-width: 148px;
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 6px 10px 6px 7px;
  border: 1px solid #dbe3ef;
  border-radius: 12px;
  color: #0f172a;
  background: rgba(255, 255, 255, .88);
  cursor: pointer;
  transition: border-color .2s ease, box-shadow .2s ease;
}
.user-menu-trigger:hover {
  border-color: #93b4e8;
  box-shadow: 0 5px 16px rgba(37, 99, 235, .1);
}
.user-avatar {
  width: 30px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border-radius: 9px;
  color: #fff;
  background: linear-gradient(135deg, #2563eb, #38bdf8);
  font-size: 13px;
  font-weight: 800;
}
.user-meta {
  min-width: 0;
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: flex-start;
  line-height: 1.2;
}
.user-meta strong {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}
.user-meta small {
  margin-top: 2px;
  color: #64748b;
  font-size: 11px;
}
.main {
  width: 100%;
  min-width: 0;
  padding: 22px;
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
    padding: 8px 12px;
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
    justify-content: space-between;
    overflow-x: auto;
  }
  .user-menu-trigger {
    min-width: 0;
    padding-right: 7px;
  }
  .user-meta {
    display: none;
  }
  .controller-editor {
    width: 100%;
    flex-wrap: wrap;
  }
}
</style>
