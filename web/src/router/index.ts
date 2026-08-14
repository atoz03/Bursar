import { createRouter, createWebHistory } from "vue-router";
import { authState, refreshAuth } from "../lib/authStore";

const Layout = () => import("../views/Layout.vue");
const Login = () => import("../views/pages/Login.vue");
const UserBalance = () => import("../views/pages/UserBalance.vue");
const UserUsage = () => import("../views/pages/UserUsage.vue");
const UserProfile = () => import("../views/pages/UserProfile.vue");
const UserNotices = () => import("../views/pages/UserNotices.vue");
const UserRegister = () => import("../views/pages/UserRegister.vue");
const ForgotPassword = () => import("../views/pages/ForgotPassword.vue");
const ResetPassword = () => import("../views/pages/ResetPassword.vue");
const ChangePassword = () => import("../views/pages/ChangePassword.vue");
const UserAccounts = () => import("../views/pages/UserAccounts.vue");
const AdminUsers = () => import("../views/pages/AdminUsers.vue");
const AdminNodes = () => import("../views/pages/AdminNodes.vue");
const AdminStatus = () => import("../views/pages/AdminStatus.vue");
const AdminUsage = () => import("../views/pages/AdminUsage.vue");
const AdminRequests = () => import("../views/pages/AdminRequests.vue");
const AdminMailSettings = () => import("../views/pages/AdminMailSettings.vue");
const AdminBoard = () => import("../views/pages/AdminBoard.vue");
const AdminAccounts = () => import("../views/pages/AdminAccounts.vue");
const AdminAccountProvision = () => import("../views/pages/AdminAccountProvision.vue");
const AdminWhitelist = () => import("../views/pages/AdminWhitelist.vue");
const AdminAnnouncements = () => import("../views/pages/AdminAnnouncements.vue");
const AdminGuideline = () => import("../views/pages/AdminGuideline.vue");
const AdminNotebook = () => import("../views/pages/AdminNotebook.vue");
const AdminPowerUsers = () => import("../views/pages/AdminPowerUsers.vue");
const AdminHA = () => import("../views/pages/AdminHA.vue");
const AdminPoints = () => import("../views/pages/AdminPoints.vue");
const AdminProfile = () => import("../views/pages/AdminProfile.vue");

function hasPointsAccess(): boolean {
  return !!(
    authState.canManagePoints ||
    authState.canPointsUsers ||
    authState.canPointsBatchFiltered ||
    authState.canPointsBatchAll ||
    authState.canPointsRecords ||
    authState.canPointsMonthly ||
    authState.canPointsSpecialRules
  );
}

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/login", component: Login },
    { path: "/register", component: UserRegister },
    { path: "/forgot-password", component: ForgotPassword },
    { path: "/reset-password", component: ResetPassword },
    {
      path: "/key-decryptor",
      redirect: (to) => ({
        path: "/user/accounts",
        query: { ...to.query, tool: "key-decryptor" },
      }),
    },
    {
      path: "/",
      component: Layout,
      meta: { requiresAuth: true },
      children: [
        { path: "", redirect: "/user/balance" },
        { path: "user/balance", component: UserBalance },
        { path: "user/profile", component: UserProfile },
        { path: "user/usage", component: UserUsage },
        { path: "user/notices", component: UserNotices },
        { path: "user/change-password", component: ChangePassword },
        { path: "user/accounts", component: UserAccounts },
        { path: "admin/users", component: AdminUsers },
        { path: "admin/points", component: AdminPoints },
        { path: "admin/nodes", component: AdminNodes },
        { path: "admin/status", component: AdminStatus },
        { path: "admin/accounts", component: AdminAccounts },
        { path: "admin/account-provision", component: AdminAccountProvision },
        { path: "admin/whitelist", component: AdminWhitelist },
        { path: "admin/announcements", component: AdminAnnouncements },
        { path: "admin/guideline", component: AdminGuideline },
        { path: "admin/notebook", component: AdminNotebook },
        { path: "admin/power-users", component: AdminPowerUsers },
        { path: "admin/board", component: AdminBoard },
        { path: "admin/usage", component: AdminUsage },
        { path: "admin/requests", component: AdminRequests },
        { path: "admin/mail", component: AdminMailSettings },
        { path: "admin/ha", component: AdminHA },
        { path: "admin/change-password", redirect: "/user/change-password" },
        { path: "admin/profile", component: AdminProfile },
      ],
    },
  ],
});

router.beforeEach(async (to) => {
  if (!authState.checked) {
    try {
      await refreshAuth();
    } catch {
      authState.checked = true;
      authState.authenticated = false;
    }
  }

  const publicPaths = new Set(["/login", "/register", "/forgot-password", "/reset-password", "/key-decryptor"]);
  const isPublic = publicPaths.has(to.path);
  const isAdminRoute = to.path.startsWith("/admin");

  if (to.path === "/") {
    if (!authState.authenticated) return { path: "/login" };
    if (authState.role === "admin") return { path: "/admin/board" };
    if (authState.role === "power_user") {
      if (authState.canViewBoard) return { path: "/admin/board" };
      if (authState.canViewNodes) return { path: "/admin/nodes" };
      if (hasPointsAccess()) return { path: "/admin/points" };
      if (authState.canManagePlatformUsers) return { path: "/admin/users" };
      if (authState.canReviewRequests) return { path: "/admin/requests" };
      return { path: "/user/balance" };
    }
    return { path: "/user/balance" };
  }

  if (!isPublic && !authState.authenticated) {
    return { path: "/login" };
  }

  if (isAdminRoute) {
    if (authState.role === "admin") {
      return true;
    }
    if (authState.role === "power_user") {
      const p = to.path;
      if (p.startsWith("/admin/board") && authState.canViewBoard) return true;
      if (p.startsWith("/admin/nodes") && authState.canViewNodes) return true;
      if (p.startsWith("/admin/points") && hasPointsAccess()) return true;
      if (p.startsWith("/admin/users") && authState.canManagePlatformUsers) return true;
      if (p.startsWith("/admin/requests") && authState.canReviewRequests) return true;
      if (p.startsWith("/admin/profile")) return { path: "/user/profile" };
      if (p.startsWith("/admin/change-password")) return { path: "/user/change-password" };
      return { path: "/user/balance" };
    }
    return { path: "/user/balance" };
  }

  if (to.path === "/login" && authState.authenticated) {
    if (authState.role === "admin") return { path: "/admin/board" };
    if (authState.role === "power_user") {
      if (authState.canViewBoard) return { path: "/admin/board" };
      if (authState.canViewNodes) return { path: "/admin/nodes" };
      if (hasPointsAccess()) return { path: "/admin/points" };
      if (authState.canManagePlatformUsers) return { path: "/admin/users" };
      if (authState.canReviewRequests) return { path: "/admin/requests" };
      return { path: "/user/balance" };
    }
    return { path: "/user/balance" };
  }
  return true;
});
