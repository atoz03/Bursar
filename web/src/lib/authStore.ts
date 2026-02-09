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
  canReviewRequests: boolean;
  csrfToken: string;
  expiresAt: string;
};

export const authState = reactive<AuthState>({
  checked: false,
  authenticated: false,
  username: "",
  role: "",
  canViewBoard: false,
  canViewNodes: false,
  canReviewRequests: false,
  csrfToken: "",
  expiresAt: "",
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
  authState.canReviewRequests = !!me.can_review_requests;
  authState.csrfToken = me.csrf_token ?? "";
  authState.expiresAt = me.expires_at ?? "";
}

export async function login(username: string, password: string): Promise<void> {
  const client = new ApiClient(settingsState.baseUrl);
  await client.authLogin(username, password);
  await refreshAuth();
}

export async function logout(): Promise<void> {
  const client = new ApiClient(settingsState.baseUrl);
  await client.authLogout();
  await refreshAuth();
}
