import { createRouter, createWebHistory } from "vue-router";
import Layout from "../views/Layout.vue";
import Dashboard from "../views/pages/Dashboard.vue";
import Login from "../views/pages/Login.vue";
import UserBalance from "../views/pages/UserBalance.vue";
import UserUsage from "../views/pages/UserUsage.vue";
import UserProfile from "../views/pages/UserProfile.vue";
import UserNotices from "../views/pages/UserNotices.vue";
import UserRegister from "../views/pages/UserRegister.vue";
import ForgotPassword from "../views/pages/ForgotPassword.vue";
import ResetPassword from "../views/pages/ResetPassword.vue";
import ChangePassword from "../views/pages/ChangePassword.vue";
import UserAccounts from "../views/pages/UserAccounts.vue";
import AdminUsers from "../views/pages/AdminUsers.vue";
import AdminNodes from "../views/pages/AdminNodes.vue";
import AdminUsage from "../views/pages/AdminUsage.vue";
import AdminQueue from "../views/pages/AdminQueue.vue";
import AdminRequests from "../views/pages/AdminRequests.vue";
import AdminMailSettings from "../views/pages/AdminMailSettings.vue";
import AdminBoard from "../views/pages/AdminBoard.vue";
import AdminAccounts from "../views/pages/AdminAccounts.vue";
import AdminWhitelist from "../views/pages/AdminWhitelist.vue";
import AdminAnnouncements from "../views/pages/AdminAnnouncements.vue";
import AdminGuideline from "../views/pages/AdminGuideline.vue";
import AdminNotebook from "../views/pages/AdminNotebook.vue";
import AdminPowerUsers from "../views/pages/AdminPowerUsers.vue";
import AdminHA from "../views/pages/AdminHA.vue";
import AdminPoints from "../views/pages/AdminPoints.vue";
import AdminProfile from "../views/pages/AdminProfile.vue";
import { authState, refreshAuth } from "../lib/authStore";

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
        { path: "", component: Dashboard },
        { path: "user/balance", component: UserBalance },
        { path: "user/profile", component: UserProfile },
        { path: "user/usage", component: UserUsage },
        { path: "user/notices", component: UserNotices },
        { path: "user/change-password", component: ChangePassword },
        { path: "user/accounts", component: UserAccounts },
        { path: "admin/users", component: AdminUsers },
        { path: "admin/points", component: AdminPoints },
        { path: "admin/nodes", component: AdminNodes },
        { path: "admin/accounts", component: AdminAccounts },
        { path: "admin/whitelist", component: AdminWhitelist },
        { path: "admin/announcements", component: AdminAnnouncements },
        { path: "admin/guideline", component: AdminGuideline },
        { path: "admin/notebook", component: AdminNotebook },
        { path: "admin/power-users", component: AdminPowerUsers },
        { path: "admin/board", component: AdminBoard },
        { path: "admin/usage", component: AdminUsage },
        { path: "admin/queue", component: AdminQueue },
        { path: "admin/requests", component: AdminRequests },
        { path: "admin/mail", component: AdminMailSettings },
        { path: "admin/ha", component: AdminHA },
        { path: "admin/change-password", component: ChangePassword },
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
      if (authState.canReviewRequests) return { path: "/admin/requests" };
      return { path: "/user/balance" };
    }
    return { path: "/user/balance" };
  }
  return true;
});
