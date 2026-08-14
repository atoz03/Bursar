<template>
  <el-container class="shell" :class="{ 'mobile-shell': isMobile, 'mobile-menu-open': mobileMenuOpen }">
    <div v-if="isMobile && mobileMenuOpen" class="mobile-mask" @click="mobileMenuOpen = false" />
    <el-aside width="244px" class="aside">
      <div class="brand">
        <span class="brand-mark"><el-icon :size="23"><Cpu /></el-icon></span>
        <div class="brand-copy">
          <div class="brand-title">{{ authState.platformName }}</div>
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
            <el-menu-item index="/admin/setup">{{ t("系统设置", "System Setup") }}</el-menu-item>
            <el-menu-item index="/admin/ha">{{ t("容灾同步", "HA Sync") }}</el-menu-item>
            <el-menu-item index="/admin/announcements">{{ t("公告管理", "Announcements") }}</el-menu-item>
            <el-menu-item index="/admin/guideline">{{ t("用户准则", "Guidelines") }}</el-menu-item>
            <el-menu-item index="/admin/notebook">{{ t("管理员记事本", "Notebook") }}</el-menu-item>
            <el-menu-item index="/admin/mail">{{ t("邮件设置", "Mail Settings") }}</el-menu-item>
            <el-menu-item index="/admin/demo" class="demo-menu-item">
              <span>{{ t("界面演示", "Interface Demo") }}</span>
              <el-tag class="menu-demo-tag" size="small" effect="dark" type="warning">DEMO</el-tag>
            </el-menu-item>
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
        <div class="header-search">
          <el-autocomplete
            ref="featureSearchRef"
            v-model="featureSearchText"
            :fetch-suggestions="queryFeatureSearch"
            :trigger-on-focus="true"
            clearable
            fit-input-width
            popper-class="feature-search-popper"
            :placeholder="t('搜索功能 / 设置', 'Search features / settings')"
            @select="openSearchEntry"
          >
            <template #prefix><el-icon><Search /></el-icon></template>
            <template #suffix><kbd>{{ shortcutLabel }}</kbd></template>
            <template #default="{ item }">
              <div class="feature-search-option">
                <div><strong>{{ item.title }}</strong><span>{{ item.group }}</span></div>
                <small>{{ item.description }}</small>
              </div>
            </template>
          </el-autocomplete>
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
  Search,
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
type FeatureSearchEntry = {
  value: string;
  title: string;
  group: string;
  description: string;
  path: string;
  keywords: string;
};

const featureSearchRef = ref<any>(null);
const featureSearchText = ref("");
const shortcutLabel = computed(() => /Mac|iPhone|iPad/i.test(navigator.platform) ? "⌘K" : "Ctrl K");

function searchEntry(title: string, group: string, path: string, description: string, keywords = ""): FeatureSearchEntry {
  return { value: `${title} · ${group}`, title, group, path, description, keywords };
}

const userSearchEntries: FeatureSearchEntry[] = [
  searchEntry("我的积分", "我的中心", "/user/balance", "查看通用、结转、专属积分和变动记录", "积分余额 充值 结转 专属 jf points balance"),
  searchEntry("我的用量", "我的中心", "/user/usage", "查询个人 CPU、GPU 使用与积分消耗", "使用记录 进程 cpu gpu yl usage"),
  searchEntry("节点账号", "我的中心", "/user/accounts", "查看节点账号、申请开通和解密密钥", "ssh 开通 密钥 解密 账号 jd key"),
  searchEntry("公告与用户准则", "我的中心", "/user/notices", "查看平台公告、维护信息和使用规范", "通知 公告 准则 notice guideline"),
  searchEntry("个人资料", "账号安全", "/user/profile", "查看和修改个人身份资料", "姓名 邮箱 学号 导师 profile grzl"),
  searchEntry("修改密码", "账号安全", "/user/change-password", "修改当前登录密码", "密码 安全 password mm"),
];

const adminSearchEntries: FeatureSearchEntry[] = [
  searchEntry("运营看板", "总览", "/admin/board", "查看资源使用、积分消耗和用户概览", "dashboard 看板 统计 yy kb"),
  searchEntry("数据留存与删除", "运营看板 · 数据工具", "/admin/board", "配置自动清理天数或按日期删除用量记录", "保留天数 自动删除 清理 retention delete sjlc"),
  searchEntry("用量数据导出", "运营看板", "/admin/board", "按统计区间导出 CSV", "csv export 导出 运营"),
  searchEntry("集群状态", "总览", "/admin/status", "查看节点 CPU 与每张 GPU 的实时状态", "监控 显卡 温度 显存 在线 gpu cpu jc jq"),
  searchEntry("节点管理", "资源与计费", "/admin/nodes", "管理节点策略、版本和运行状态", "节点 同步 agent jd node"),
  searchEntry("节点限速策略", "节点管理", "/admin/nodes", "配置低积分和欠费 CPU 限速", "cpu quota throttle 限制 xscl"),
  searchEntry("节点磁盘配额", "节点管理", "/admin/nodes", "配置用户磁盘软硬配额", "disk quota home mnt 硬盘 pe"),
  searchEntry("节点计费参数", "节点管理", "/admin/nodes", "配置 GPU 与 CPU 单价", "价格 单价 billing price jf cs"),
  searchEntry("GPU 可见与独享", "节点管理", "/admin/nodes", "配置用户可见 GPU 和节点独享规则", "显卡 可见 独占 exclusive visibility gpu"),
  searchEntry("节点安全事件", "节点管理", "/admin/nodes", "查看端口扫描、挖矿和可疑进程", "安全 恶意 扫描 挖矿 security aq"),
  searchEntry("进程审计", "资源与计费", "/admin/usage", "查询用户与节点进程使用记录", "进程 记录 usage audit csv jc"),
  searchEntry("积分管理", "资源与计费", "/admin/points", "查看余额并调整用户积分", "加分 扣分 余额 points jf"),
  searchEntry("积分月度规则", "积分管理", "/admin/points", "配置每月积分、结转和欠费上限", "博士 硕士 月初 结转 monthly rule"),
  searchEntry("积分特殊规则", "积分管理", "/admin/points", "配置单个用户的特殊积分规则", "special rule 特殊 用户 jf"),
  searchEntry("平台用户", "账号与访问", "/admin/users", "管理平台账号、状态和资料", "用户 删除 恢复 黑名单 csv yh"),
  searchEntry("账号映射", "账号与访问", "/admin/accounts", "维护平台账号与节点账号绑定", "绑定 解绑 mapping bind zh"),
  searchEntry("绑定安全策略", "账号映射", "/admin/accounts", "配置绑定挑战、冷却和试用资源限制", "challenge cooldown trial 风险 安全"),
  searchEntry("账号开通", "账号与访问", "/admin/account-provision", "开通节点账号并下发 SSH 密钥", "ssh 密钥 邮件 provision kt"),
  searchEntry("开通历史", "账号开通", "/admin/account-provision", "查看节点账号开通与邮件发送记录", "history 日志 记录 ktls"),
  searchEntry("注册与资料审核", "账号与访问", "/admin/requests", "审核注册、资料修改、开通和解绑申请", "注册 审核 申请 review zc sh"),
  searchEntry("高级用户权限", "账号与访问", "/admin/power-users", "管理高级用户的功能授权", "授权 权限 power user gjyh"),
  searchEntry("SSH 名单", "账号与访问", "/admin/whitelist", "管理黑名单、白名单、豁免和临时账号", "blacklist whitelist exemption temporary ssh hbm"),
  searchEntry("容灾同步", "系统与容灾", "/admin/ha", "配置主备控制器与同步周期", "ha dr backup 灾备 同步 rz"),
  searchEntry("公告管理", "系统与容灾", "/admin/announcements", "发布和维护平台公告", "通知 announcement gg"),
  searchEntry("用户准则", "系统与容灾", "/admin/guideline", "编辑注册和资源使用规范", "规则 guideline zhunze yz"),
  searchEntry("管理员记事本", "系统与容灾", "/admin/notebook", "记录内部运维事项", "note notebook 记录 jsb"),
  searchEntry("邮件设置", "系统与容灾", "/admin/mail", "配置 SMTP 与通知邮件", "smtp mail 邮箱 发信 yj"),
  searchEntry("界面演示", "系统与容灾", "/admin/demo", "使用 Mock 数据预览不同身份页面", "demo mock 预览 身份 ys"),
  searchEntry("管理员资料", "账号安全", "/admin/profile", "查看管理员资料和安全状态", "profile 2fa grzl"),
];

const searchableEntries = computed(() => {
  if (authState.role === "admin") return [...adminSearchEntries, ...userSearchEntries.filter((item) => item.path.endsWith("change-password"))];
  if (authState.role !== "power_user") return userSearchEntries;
  const authorized: FeatureSearchEntry[] = [];
  if (authState.canViewBoard) authorized.push(...adminSearchEntries.filter((item) => item.path === "/admin/board"));
  if (authState.canViewNodes) authorized.push(...adminSearchEntries.filter((item) => item.path === "/admin/nodes"));
  if (authState.canManagePoints || authState.canPointsUsers || authState.canPointsBatchFiltered || authState.canPointsBatchAll || authState.canPointsRecords || authState.canPointsMonthly || authState.canPointsSpecialRules) authorized.push(...adminSearchEntries.filter((item) => item.path === "/admin/points"));
  if (authState.canManagePlatformUsers) authorized.push(...adminSearchEntries.filter((item) => item.path === "/admin/users"));
  if (authState.canReviewRequests) authorized.push(...adminSearchEntries.filter((item) => item.path === "/admin/requests"));
  return [...authorized, ...userSearchEntries];
});

function t(zh: string, en: string): string {
  return pickText(zh, en);
}

function normalizeSearchText(value: string): string {
  return String(value || "")
    .normalize("NFKC")
    .toLowerCase()
    .replace(/[\s·/_:：()（）\-]+/g, "")
    .replace(/[^\p{L}\p{N}]/gu, "");
}

function editDistance(a: string, b: string): number {
  if (a === b) return 0;
  if (!a.length) return b.length;
  if (!b.length) return a.length;
  let previous = Array.from({ length: b.length + 1 }, (_, index) => index);
  for (let i = 1; i <= a.length; i += 1) {
    const current = [i];
    for (let j = 1; j <= b.length; j += 1) {
      current[j] = Math.min(
        current[j - 1] + 1,
        previous[j] + 1,
        previous[j - 1] + (a[i - 1] === b[j - 1] ? 0 : 1),
      );
    }
    previous = current;
  }
  return previous[b.length];
}

function subsequenceRatio(needle: string, haystack: string): number {
  if (!needle || !haystack) return 0;
  let matched = 0;
  for (const char of haystack) {
    if (char === needle[matched]) matched += 1;
    if (matched === needle.length) return matched / haystack.length;
  }
  return 0;
}

function fuzzyTermScore(term: string, candidates: string[]): number {
  let best = -1;
  for (const candidate of candidates) {
    if (!candidate) continue;
    if (candidate === term) best = Math.max(best, 120);
    else if (candidate.startsWith(term)) best = Math.max(best, 104 - Math.min(18, candidate.length - term.length));
    else {
      const index = candidate.indexOf(term);
      if (index >= 0) best = Math.max(best, 88 - Math.min(24, index));
    }
    if (term.length >= 2 && Math.abs(candidate.length - term.length) <= 3) {
      const distance = editDistance(term, candidate);
      const allowed = Math.max(1, Math.floor(Math.max(term.length, candidate.length) * .34));
      if (distance <= allowed) best = Math.max(best, 72 - distance * 12);
    }
    const sequence = subsequenceRatio(term, candidate);
    if (sequence > 0) best = Math.max(best, 42 + Math.round(sequence * 22));
  }
  return best;
}

function featureSearchScore(query: string, item: FeatureSearchEntry): number {
  const rawTerms = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const terms = rawTerms.map(normalizeSearchText).filter(Boolean);
  if (!terms.length) return 1;
  const title = normalizeSearchText(item.title);
  const group = normalizeSearchText(item.group);
  const description = normalizeSearchText(item.description);
  const keywordParts = item.keywords.split(/\s+/).map(normalizeSearchText).filter(Boolean);
  const whole = normalizeSearchText(`${item.title}${item.group}${item.description}${item.keywords}`);
  let score = 0;
  for (const term of terms) {
    const partScore = fuzzyTermScore(term, [title, group, description, whole, ...keywordParts]);
    if (partScore < 0) return -1;
    score += partScore;
    if (title.includes(term)) score += 28;
    if (group.includes(term)) score += 10;
  }
  return score;
}

function queryFeatureSearch(query: string, callback: (items: FeatureSearchEntry[]) => void) {
  const value = query.trim();
  if (!value) {
    callback(searchableEntries.value.slice(0, 10));
    return;
  }
  const results = searchableEntries.value
    .map((item, index) => ({ item, index, score: featureSearchScore(value, item) }))
    .filter((row) => row.score >= 0)
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .slice(0, 12)
    .map((row) => row.item);
  callback(results);
}

async function openSearchEntry(item: FeatureSearchEntry) {
  featureSearchText.value = "";
  if (route.path !== item.path) await router.push(item.path);
  if (isMobile.value) mobileMenuOpen.value = false;
}

function onFeatureSearchShortcut(event: KeyboardEvent) {
  if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== "k") return;
  event.preventDefault();
  featureSearchRef.value?.focus?.();
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
  if (route.path.startsWith("/admin/demo")) return;
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
  if (route.path.startsWith("/admin/demo")) return;
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
  if (route.path.startsWith("/admin/demo")) return;
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
  if (route.path.startsWith("/admin/demo")) return;
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
    resetReviewTodoPolling();
    resetUserNoticePolling();
    resetUserPointsPolling();
    resetUserAccountProvisionPolling();
  },
);

onMounted(() => {
  syncMobileState();
  window.addEventListener("resize", syncMobileState);
  window.addEventListener("gpuops-announcement-seen", loadUserNoticeState);
  window.addEventListener("gpuops-points-seen", loadUserPointsState);
  document.addEventListener("visibilitychange", onVisibilityChange);
  window.addEventListener("keydown", onFeatureSearchShortcut);
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
  window.removeEventListener("keydown", onFeatureSearchShortcut);
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
  overflow: hidden;
  border-right: 1px solid rgba(125, 159, 203, .14);
  background:
    radial-gradient(340px 260px at -18% -3%, rgba(37, 99, 235, .32), transparent 68%),
    radial-gradient(280px 360px at 118% 62%, rgba(8, 145, 178, .16), transparent 72%),
    linear-gradient(175deg, #121d32 0%, #0d1729 52%, #091321 100%);
  box-shadow: 7px 0 28px rgba(15, 23, 42, .11), inset -1px 0 0 rgba(255, 255, 255, .025);
}
.brand {
  position: relative;
  z-index: 1;
  display: flex;
  gap: 11px;
  align-items: center;
  min-height: 78px;
  padding: 15px 13px 14px 15px;
  border-bottom: 1px solid rgba(148, 163, 184, .12);
  color: #f8fafc;
  background: linear-gradient(180deg, rgba(255, 255, 255, .035), transparent);
}
.brand-mark {
  width: 39px;
  height: 39px;
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  border: 1px solid rgba(165, 243, 252, .24);
  border-radius: 12px;
  color: #ecfeff;
  background:
    radial-gradient(circle at 25% 15%, rgba(255, 255, 255, .28), transparent 42%),
    linear-gradient(135deg, rgba(37, 99, 235, .92), rgba(8, 145, 178, .8));
  box-shadow: 0 9px 22px rgba(2, 132, 199, .2), inset 0 1px 0 rgba(255, 255, 255, .28);
}
.brand-copy {
  min-width: 0;
  flex: 1;
}
.brand-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #f8fbff;
  font-size: 13px;
  font-weight: 800;
  letter-spacing: .055em;
}
.brand-sub {
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: rgba(185, 204, 228, .72);
}
.menu {
  position: relative;
  z-index: 1;
  flex: 1;
  min-height: 0;
  overflow-y: scroll;
  border-right: none;
  padding: 12px 9px 28px;
  background: transparent;
  scrollbar-width: thin;
  scrollbar-color: rgba(125, 155, 194, .46) transparent;
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
  background: rgba(125, 155, 194, .46);
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
  margin: 1px 0 9px 13px;
  padding: 3px 3px 4px 7px;
  border-left: 1px solid rgba(125, 159, 203, .16);
  border-radius: 0 10px 10px 0;
  background: linear-gradient(90deg, rgba(255, 255, 255, .025), transparent) !important;
}
.menu :deep(.el-sub-menu__title) {
  height: 44px;
  margin: 1px 0;
  padding: 0 12px !important;
  color: #c5d2e4 !important;
  border: 1px solid transparent;
  border-radius: 11px;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: .01em;
  transition: color .16s ease, background .16s ease, border-color .16s ease;
}
.menu :deep(.el-sub-menu__title:hover) {
  color: #ffffff !important;
  border-color: rgba(125, 159, 203, .12);
  background: rgba(255, 255, 255, .045) !important;
}
.menu :deep(.el-sub-menu.is-active > .el-sub-menu__title) {
  color: #edf6ff !important;
}
.menu :deep(.el-sub-menu__icon-arrow) {
  color: rgba(166, 190, 221, .8) !important;
  font-size: 11px;
}
.menu :deep(.el-menu-item) {
  position: relative;
  height: 38px;
  padding: 0 11px !important;
  color: #9eafc5 !important;
  border: 1px solid transparent;
  border-radius: 9px;
  font-size: 12px;
  transition: color .16s ease, background .16s ease, border-color .16s ease, transform .16s ease;
}
.menu :deep(.el-sub-menu .el-menu-item) {
  min-width: auto;
  margin: 2px 0;
}
.menu :deep(.el-sub-menu .el-menu-item::before) {
  content: "";
  width: 5px;
  height: 5px;
  margin-right: 9px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: #526780;
  transition: background .16s ease, box-shadow .16s ease;
}
.menu :deep(.el-menu-item:hover) {
  color: #ffffff !important;
  border-color: rgba(96, 165, 250, .1);
  background: rgba(59, 130, 246, .11) !important;
  transform: translateX(1px);
}
.menu :deep(.el-menu-item.is-active) {
  color: #ffffff !important;
  border-color: rgba(125, 211, 252, .16);
  background:
    radial-gradient(120px 42px at 12% 50%, rgba(34, 211, 238, .17), transparent 72%),
    linear-gradient(90deg, rgba(37, 99, 235, .82), rgba(37, 99, 235, .48)) !important;
  border-radius: 9px;
  font-weight: 700;
  box-shadow: 0 7px 18px rgba(30, 64, 175, .18), inset 0 1px 0 rgba(255, 255, 255, .12);
  transform: none;
}
.menu :deep(.el-sub-menu .el-menu-item.is-active::before) {
  background: #67e8f9;
  box-shadow: 0 0 0 4px rgba(34, 211, 238, .12);
}
.menu :deep(.demo-menu-item) {
  justify-content: flex-start;
}
.menu :deep(.menu-demo-tag) {
  height: 18px;
  margin-left: auto;
  padding: 0 5px;
  border-color: rgba(251, 191, 36, .24) !important;
  color: #fde68a !important;
  background: rgba(180, 83, 9, .32) !important;
  font-size: 8px;
  font-weight: 850;
  letter-spacing: .06em;
}
.menu :deep(.el-badge__content.is-fixed) {
  top: 7px;
  right: 1px;
  border: 2px solid #122039;
  box-shadow: 0 2px 7px rgba(0, 0, 0, .22);
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
  background: rgba(255, 255, 255, .94);
  box-shadow: 0 4px 18px rgba(15, 23, 42, .045), inset 0 -1px 0 rgba(148, 163, 184, .08);
}
.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1 1 260px;
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
.header-search {
  width: min(430px, 34vw);
  min-width: 250px;
  flex: 0 1 430px;
}
.header-search :deep(.el-autocomplete) {
  width: 100%;
}
.header-search :deep(.el-input__wrapper) {
  min-height: 38px;
  padding-left: 12px;
  border: 1px solid rgba(203, 213, 225, .62);
  border-radius: 12px !important;
  background:
    radial-gradient(150px 40px at 8% 0%, rgba(59, 130, 246, .08), transparent 72%),
    rgba(248, 250, 252, .88) !important;
  box-shadow: 0 4px 14px rgba(15, 23, 42, .035) !important;
}
.header-search :deep(.el-input__wrapper:hover) {
  border-color: rgba(96, 165, 250, .58);
  box-shadow: 0 5px 16px rgba(37, 99, 235, .08) !important;
}
.header-search :deep(.el-input__wrapper.is-focus) {
  border-color: rgba(59, 130, 246, .72);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, .1), 0 7px 18px rgba(37, 99, 235, .08) !important;
}
.header-search kbd {
  padding: 2px 6px;
  border: 1px solid #d8e0eb;
  border-bottom-width: 2px;
  border-radius: 6px;
  color: #8290a3;
  background: rgba(255, 255, 255, .8);
  font-family: inherit;
  font-size: 9px;
  line-height: 1.25;
  white-space: nowrap;
}
:global(.feature-search-popper) {
  border: 1px solid rgba(203, 213, 225, .78) !important;
  border-radius: 14px !important;
  background: linear-gradient(145deg, rgba(255,255,255,.99), rgba(245,248,253,.98)) !important;
  box-shadow: 0 18px 48px rgba(15, 23, 42, .15) !important;
}
:global(.feature-search-popper .el-autocomplete-suggestion__wrap) {
  max-height: min(62vh, 460px);
  padding: 7px;
}
:global(.feature-search-popper .el-autocomplete-suggestion li) {
  height: auto;
  min-height: 52px;
  margin: 2px 0;
  padding: 8px 10px;
  border-radius: 9px;
  line-height: 1.35;
}
:global(.feature-search-popper .el-autocomplete-suggestion li.highlighted),
:global(.feature-search-popper .el-autocomplete-suggestion li:hover) {
  background: linear-gradient(90deg, rgba(219, 234, 254, .76), rgba(238, 242, 255, .58));
}
.feature-search-option {
  min-width: 0;
  display: grid;
  gap: 3px;
}
.feature-search-option > div {
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.feature-search-option strong {
  color: #1e293b;
  font-size: 13px;
}
.feature-search-option span {
  overflow: hidden;
  color: #3b82f6;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.feature-search-option small {
  overflow: hidden;
  color: #8491a4;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.header-right {
  display: flex;
  gap: 8px;
  align-items: center;
  flex: 0 0 auto;
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
    width: 244px !important;
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
  .header-left {
    width: auto;
    flex: 1 1 auto;
  }
  .header-right {
    width: auto;
    justify-content: flex-end;
    overflow-x: auto;
  }
  .header-search {
    order: 3;
    width: 100%;
    min-width: 0;
    flex: 1 0 100%;
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
