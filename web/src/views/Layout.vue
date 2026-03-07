<template>
  <el-container class="shell">
    <el-aside width="250px" class="aside">
      <div class="brand">
        <el-icon :size="30"><Cpu /></el-icon>
        <div>
          <div class="brand-title">GPU Ops</div>
          <div class="brand-sub">GPU 运维平台</div>
        </div>
      </div>

      <el-menu :default-active="activePath" :default-openeds="defaultOpeneds" router class="menu">
        <template v-if="authState.role === 'admin'">
          <el-sub-menu index="grp-ops">
            <template #title><el-icon><DataBoard /></el-icon><span>运营分析</span></template>
            <el-menu-item index="/admin/board">运营看板</el-menu-item>
            <el-menu-item index="/admin/usage">使用记录</el-menu-item>
            <el-menu-item index="/admin/queue">排队队列</el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-resource">
            <template #title><el-icon><Monitor /></el-icon><span>资源管理</span></template>
            <el-menu-item index="/admin/nodes">节点状态</el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-points">
            <template #title><el-icon><WalletFilled /></el-icon><span>积分管理</span></template>
            <el-menu-item index="/admin/points">积分管理</el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-access">
            <template #title><el-icon><UserFilled /></el-icon><span>账号与访问</span></template>
            <el-menu-item index="/admin/users">平台用户管理</el-menu-item>
            <el-menu-item index="/admin/accounts">账号映射</el-menu-item>
            <el-menu-item index="/admin/requests">
              <el-badge :value="reviewTodoCount" :hidden="reviewTodoCount === 0">
                <span>平台账号注册审核</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/admin/power-users">高级用户</el-menu-item>
            <el-menu-item index="/admin/whitelist">SSH名单</el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-system">
            <template #title><el-icon><Setting /></el-icon><span>系统与容灾</span></template>
            <el-menu-item index="/admin/ha">容灾同步</el-menu-item>
            <el-menu-item index="/admin/announcements">公告管理</el-menu-item>
            <el-menu-item index="/admin/guideline">用户准则</el-menu-item>
            <el-menu-item index="/admin/notebook">管理员记事本</el-menu-item>
            <el-menu-item index="/admin/mail">邮件设置</el-menu-item>
            <el-menu-item index="/admin/profile">个人信息</el-menu-item>
            <el-menu-item index="/admin/change-password">修改密码</el-menu-item>
          </el-sub-menu>
        </template>
        <template v-else-if="authState.role === 'power_user'">
          <el-sub-menu index="grp-power">
            <template #title><el-icon><DataBoard /></el-icon><span>授权功能</span></template>
            <el-menu-item v-if="authState.canViewBoard" index="/admin/board">运营看板</el-menu-item>
            <el-menu-item v-if="authState.canViewNodes" index="/admin/nodes">节点状态</el-menu-item>
            <el-menu-item v-if="authState.canReviewRequests" index="/admin/requests">
              <el-badge :value="reviewTodoCount" :hidden="reviewTodoCount === 0">
                <span>平台账号注册审核</span>
              </el-badge>
            </el-menu-item>
          </el-sub-menu>
          <el-sub-menu index="grp-user">
            <template #title><el-icon><WalletFilled /></el-icon><span>我的中心</span></template>
            <el-menu-item index="/user/notices">
              <el-badge :is-dot="userNoticeHasNew">
                <span>公告与用户准则</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/user/balance">我的积分</el-menu-item>
            <el-menu-item index="/user/usage">我的用量</el-menu-item>
            <el-menu-item index="/user/profile">个人资料</el-menu-item>
            <el-menu-item index="/user/accounts">节点账号</el-menu-item>
            <el-menu-item index="/user/change-password">修改密码</el-menu-item>
          </el-sub-menu>
        </template>
        <template v-else>
          <el-sub-menu index="grp-user">
            <template #title><el-icon><WalletFilled /></el-icon><span>我的中心</span></template>
            <el-menu-item index="/user/notices">
              <el-badge :is-dot="userNoticeHasNew">
                <span>公告与用户准则</span>
              </el-badge>
            </el-menu-item>
            <el-menu-item index="/user/balance">我的积分</el-menu-item>
            <el-menu-item index="/user/usage">我的用量</el-menu-item>
            <el-menu-item index="/user/profile">个人资料</el-menu-item>
            <el-menu-item index="/user/accounts">节点账号</el-menu-item>
            <el-menu-item index="/user/change-password">修改密码</el-menu-item>
          </el-sub-menu>
        </template>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header">
        <div class="header-left">
          <template v-if="authState.role === 'admin'">
            <el-button text class="controller-toggle" @click="showControllerConfig = !showControllerConfig">
              <el-icon><Link /></el-icon>
              <span>{{ showControllerConfig ? "收起控制器地址" : "控制器地址（高级）" }}</span>
            </el-button>
            <div v-if="showControllerConfig" class="controller-editor">
              <span class="muted">控制器地址</span>
              <el-input
                v-model="settingsState.baseUrl"
                placeholder="留空表示当前站点"
                style="max-width: 320px"
                @change="persist"
                clearable
              />
              <el-button @click="persist" type="primary" size="small">
                <el-icon><Check /></el-icon>
                保存
              </el-button>
            </div>
          </template>
        </div>
        <div class="header-right">
          <el-tag type="success" effect="light">
            {{ authState.role === 'admin' ? '管理员' : (authState.role === 'power_user' ? '高级用户' : '用户') }}
          </el-tag>
          <el-tag effect="plain">{{ authState.username }}</el-tag>
          <el-button @click="doLogout">
            <el-icon><SwitchButton /></el-icon>
            退出
          </el-button>
        </div>
      </el-header>

      <el-main class="main">
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
import {
  Check,
  Cpu,
  DataBoard,
  Link,
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
const showControllerConfig = ref(false);
const userNoticeHasNew = ref(false);
let reviewTodoTimer: ReturnType<typeof setInterval> | null = null;
let userNoticeTimer: ReturnType<typeof setInterval> | null = null;

function persist() {
  persistSettings();
  ElMessage.success("保存成功");
}

async function doLogout() {
  await logout();
  await router.push("/login");
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

function canLoadReviewTodo(): boolean {
  if (!authState.authenticated) return false;
  if (authState.role === "admin") return true;
  return authState.role === "power_user" && !!authState.canReviewRequests;
}

function canLoadUserNotice(): boolean {
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

async function loadReviewTodoCount() {
  if (!canLoadReviewTodo()) {
    reviewTodoCount.value = 0;
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const [regRes, profileRes] = await Promise.allSettled([
      client.adminRegistrationRequestsOverview(2000),
      client.adminProfileChangeRequests({ status: "pending", limit: 2000 }),
    ]);
    let total = 0;
    if (regRes.status === "fulfilled") {
      total += Number((regRes.value.pending ?? []).length + (regRes.value.conflicts ?? []).length);
    }
    if (profileRes.status === "fulfilled") {
      total += Number((profileRes.value.requests ?? []).length);
    }
    reviewTodoCount.value = total;
  } catch (e: any) {
    if (e?.status === 404 || e?.status === 403 || e?.status === 401) {
      reviewTodoCount.value = 0;
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
    const latestTs = new Date(latest).getTime();
    const seenTs = new Date(seen).getTime();
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

function onVisibilityChange() {
  if (!isDocumentVisible()) {
    clearReviewTodoTimer();
    clearUserNoticeTimer();
    return;
  }
  resetReviewTodoPolling();
  resetUserNoticePolling();
}

watch(
  () => [authState.authenticated, authState.role, authState.canReviewRequests, settingsState.baseUrl, authState.csrfToken],
  () => {
    resetReviewTodoPolling();
    resetUserNoticePolling();
  },
  { immediate: true },
);

onMounted(() => {
  resetReviewTodoPolling();
  resetUserNoticePolling();
  window.addEventListener("gpuops-announcement-seen", loadUserNoticeState);
  document.addEventListener("visibilitychange", onVisibilityChange);
});

onBeforeUnmount(() => {
  clearReviewTodoTimer();
  clearUserNoticeTimer();
  window.removeEventListener("gpuops-announcement-seen", loadUserNoticeState);
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

@media (max-width: 920px) {
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
