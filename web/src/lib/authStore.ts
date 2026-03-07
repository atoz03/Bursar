import { reactive } from "vue";
import { ApiClient } from "./api";
import { settingsState } from "./settingsStore";

export type AuthState = {
  checked: boolean;
  authenticated: boolean;
  username: string;
  role: string;
  canViewBoard: boolean;
  canViewNodes: boolean;
  canManageNodes: boolean;
  canManagePoints: boolean;
  canReviewRequests: boolean;
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
  canViewBoard: false,
  canViewNodes: false,
  canManageNodes: false,
  canManagePoints: false,
  canReviewRequests: false,
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
  authState.canViewBoard = !!me.can_view_board;
  authState.canViewNodes = !!me.can_view_nodes;
  authState.canManageNodes = !!me.can_manage_nodes;
  authState.canManagePoints = !!me.can_manage_points;
  authState.canReviewRequests = !!me.can_review_requests;
  authState.csrfToken = me.csrf_token ?? "";
  authState.expiresAt = me.expires_at ?? "";
  authState.serverNow = me.server_now ?? "";
  authState.serverTzName = me.server_tz_name ?? "";
  authState.serverTzOffsetMinutes = Number.isFinite(Number(me.server_tz_offset_minutes))
    ? Number(me.server_tz_offset_minutes)
    : Number.NaN;
}

export async function login(username: string, password: string, captchaID: string, captchaOption: number): Promise<void> {
  const client = new ApiClient(settingsState.baseUrl);
  await client.authLogin(username, password, captchaID, captchaOption);
  await refreshAuth();
}

export async function logout(): Promise<void> {
  const client = new ApiClient(settingsState.baseUrl);
  await client.authLogout();
  await refreshAuth();
}
