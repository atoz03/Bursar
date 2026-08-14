import { reactive } from "vue";
import { ApiClient } from "./api";
import { settingsState } from "./settingsStore";

export type AuthState = {
  checked: boolean;
  authenticated: boolean;
  username: string;
  role: string;
  twoFactorEnabled: boolean;
  canViewBoard: boolean;
  canViewNodes: boolean;
  canManageNodes: boolean;
  canManagePoints: boolean;
  canPointsUsers: boolean;
  canPointsBatchFiltered: boolean;
  canPointsBatchAll: boolean;
  canPointsRecords: boolean;
  canPointsMonthly: boolean;
  canPointsSpecialRules: boolean;
  canReviewRequests: boolean;
  canManagePlatformUsers: boolean;
  csrfToken: string;
  expiresAt: string;
  serverNow: string;
  serverTzName: string;
  serverTzOffsetMinutes: number;
};

export const authState = reactive<AuthState>({
  checked: false,
  authenticated: false,
  username: "",
  role: "",
  twoFactorEnabled: false,
  canViewBoard: false,
  canViewNodes: false,
  canManageNodes: false,
  canManagePoints: false,
  canPointsUsers: false,
  canPointsBatchFiltered: false,
  canPointsBatchAll: false,
  canPointsRecords: false,
  canPointsMonthly: false,
  canPointsSpecialRules: false,
  canReviewRequests: false,
  canManagePlatformUsers: false,
  csrfToken: "",
  expiresAt: "",
  serverNow: "",
  serverTzName: "",
  serverTzOffsetMinutes: Number.NaN,
});

export async function refreshAuth(): Promise<void> {
  const client = new ApiClient(settingsState.baseUrl);
  const me = await client.authMe();
  authState.checked = true;
  authState.authenticated = !!me.authenticated;
  authState.username = me.username ?? "";
  authState.role = me.role ?? "";
  authState.twoFactorEnabled = !!me.two_factor_enabled;
  authState.canViewBoard = !!me.can_view_board;
  authState.canViewNodes = !!me.can_view_nodes;
  authState.canManageNodes = !!me.can_manage_nodes;
  authState.canManagePoints = !!me.can_manage_points;
  authState.canPointsUsers = !!me.can_points_users;
  authState.canPointsBatchFiltered = !!me.can_points_batch_filtered;
  authState.canPointsBatchAll = !!me.can_points_batch_all;
  authState.canPointsRecords = !!me.can_points_records;
  authState.canPointsMonthly = !!me.can_points_monthly;
  authState.canPointsSpecialRules = !!me.can_points_special_rules;
  authState.canReviewRequests = !!me.can_review_requests;
  authState.canManagePlatformUsers = !!me.can_manage_platform_users;
  authState.csrfToken = me.csrf_token ?? "";
  authState.expiresAt = me.expires_at ?? "";
  authState.serverNow = me.server_now ?? "";
  authState.serverTzName = me.server_tz_name ?? "";
  authState.serverTzOffsetMinutes = Number.isFinite(Number(me.server_tz_offset_minutes))
    ? Number(me.server_tz_offset_minutes)
    : Number.NaN;
}

export async function login(username: string, password: string, captchaID: string, captchaOption: number, totpCode = "", captchaToken = ""): Promise<void> {
  const client = new ApiClient(settingsState.baseUrl);
  await client.authLogin(username, password, captchaID, captchaOption, totpCode, captchaToken);
  await refreshAuth();
}

export async function logout(): Promise<void> {
  const client = new ApiClient(settingsState.baseUrl);
  await client.authLogout();
  await refreshAuth();
}
