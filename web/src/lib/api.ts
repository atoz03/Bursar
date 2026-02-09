export type ApiError = {
  message: string;
  status?: number;
  body?: string;
};

function normalizeServerError(status: number, bodyText: string): ApiError {
  const text = (bodyText ?? "").trim();
  let serverMsg = "";
  if (text) {
    try {
      const j = JSON.parse(text);
      serverMsg = String(j?.error ?? j?.message ?? "").trim();
    } catch {
      serverMsg = text;
    }
  }
  const m: Record<string, string> = {
    unauthorized: "未授权，请重新登录后重试",
    csrf_required: "登录状态已过期，请刷新页面后重试",
    invalid_credentials: "用户名或密码错误",
    session_disabled: "当前未启用登录会话",
    not_found: "请求的资源不存在",
    forbidden: "当前账号没有权限执行该操作",
  };
  const msg = m[serverMsg] || serverMsg || `请求失败（状态码 ${status}）`;
  return { message: msg, status };
}

export type AuthMeResp = {
  authenticated: boolean;
  username?: string;
  role?: string;
  can_view_board?: boolean;
  can_view_nodes?: boolean;
  can_review_requests?: boolean;
  expires_at?: string;
  csrf_token?: string;
};

export type UserProfile = {
  username: string;
  email?: string;
  real_name?: string;
  student_id?: string;
  advisor?: string;
  expected_graduation_year?: number;
  phone?: string;
  role: string;
};

export type BalanceResp = {
  username: string;
  balance: number;
  status: "normal" | "warning" | "limited" | "blocked" | string;
};

export type UsageRecord = {
  node_id: string;
  username: string;
  local_username?: string;
  billing_username?: string;
  registered?: boolean;
  timestamp: string;
  pid?: number;
  cpu_percent: number;
  memory_mb: number;
  gpu_count?: number;
  command?: string;
  gpu_usage: string;
  cost: number;
};

export type UsageUserSummary = {
  username: string;
  usage_records: number;
  gpu_process_records: number;
  cpu_process_records: number;
  total_cpu_percent: number;
  total_memory_mb: number;
  total_cost: number;
};

export type UsageMonthlySummary = {
  month: string;
  username: string;
  usage_records: number;
  gpu_process_records: number;
  cpu_process_records: number;
  total_cpu_percent: number;
  total_memory_mb: number;
  total_cost: number;
};

export type RechargeSummary = {
  username: string;
  recharge_count: number;
  recharge_total: number;
  last_recharge: string;
};

export type NodeStatus = {
  node_id: string;
  last_seen_at: string;
  last_report_id: string;
  last_report_ts: string;
  interval_seconds: number;
  cpu_model?: string;
  cpu_count?: number;
  gpu_model?: string;
  gpu_count?: number;
  net_rx_mb_month?: number;
  net_tx_mb_month?: number;
  gpu_process_count: number;
  cpu_process_count: number;
  usage_records_count: number;
  ssh_active_count?: number;
  cost_total: number;
  updated_at: string;
};

export type UserNodeAccount = {
  node_id: string;
  local_username: string;
  billing_username: string;
  created_at: string;
  updated_at: string;
};

export type SSHWhitelistEntry = {
  node_id: string;
  local_username: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type SSHBlacklistEntry = {
  node_id: string;
  local_username: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type SSHExemptionEntry = {
  node_id: string;
  local_username: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type PriceRow = { Model?: string; Price?: number; model?: string; price?: number };

export type RegistryResolveResp = {
  registered: boolean;
  billing_username?: string;
  blacklisted?: boolean;
  exempted?: boolean;
};

export type UserRequest = {
  request_id: number;
  request_type: "bind" | "open" | string;
  billing_username: string;
  node_id: string;
  local_username: string;
  message: string;
  status: "pending" | "approved" | "rejected" | string;
  reviewed_by?: string;
  reviewed_at?: string;
  created_at: string;
  updated_at: string;
  apply_count_by_billing?: number;
  duplicate_flag?: boolean;
  duplicate_reason?: string;
};

export type Announcement = {
  announcement_id: number;
  title: string;
  content: string;
  pinned: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type AdminUserDetail = {
  username: string;
  role: string;
  can_view_board: boolean;
  can_view_nodes: boolean;
  can_review_requests: boolean;
  email: string;
  student_id: string;
  real_name: string;
  advisor: string;
  expected_graduation_year: number;
  phone: string;
  balance: number;
  status: string;
  usage_records: number;
  total_cost: number;
  last_usage_at: string;
  node_accounts: UserNodeAccount[];
};

export type ProfileChangeRequest = {
  request_id: number;
  billing_username: string;
  old_username: string;
  old_email: string;
  old_student_id: string;
  new_username: string;
  new_email: string;
  new_student_id: string;
  reason: string;
  status: "pending" | "approved" | "rejected" | string;
  reviewed_by?: string;
  reviewed_at?: string;
  created_at: string;
  updated_at: string;
};

export type PlatformUsageUserSummary = {
  platform_username: string;
  usage_records: number;
  gpu_process_records: number;
  cpu_process_records: number;
  total_cpu_percent: number;
  total_memory_mb: number;
  total_cost: number;
};

export type PlatformUsageNodeDetail = {
  node_id: string;
  cpu_model: string;
  cpu_count: number;
  gpu_model: string;
  gpu_count: number;
  last_seen_at: string;
  usage_records: number;
  total_cpu_percent: number;
  total_memory_mb: number;
  total_cost: number;
  last_usage_at: string;
};

export type PowerUser = {
  username: string;
  can_view_board: boolean;
  can_view_nodes: boolean;
  can_review_requests: boolean;
  created_by: string;
  updated_by: string;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
};

function trimSlashRight(v: string): string {
  return v.replace(/\/+$/, "");
}

export class ApiClient {
  private readonly adminToken: string;
  private csrfToken: string;

  constructor(
    private readonly baseUrl: string,
    private readonly opts: { adminToken?: string; csrfToken?: string } = {},
  ) {
    this.adminToken = this.opts.adminToken?.trim() ?? "";
    this.csrfToken = this.opts.csrfToken?.trim() ?? "";
  }

  private url(path: string): string {
    const base = this.baseUrl?.trim() ? trimSlashRight(this.baseUrl.trim()) : window.location.origin;
    return base + path;
  }

  private adminHeaders(): Record<string, string> {
    if (!this.adminToken) return {};
    return { Authorization: `Bearer ${this.adminToken}` };
  }

  private csrfHeaders(): Record<string, string> {
    if (!this.csrfToken) return {};
    return { "X-CSRF-Token": this.csrfToken };
  }

  private async readText(res: Response): Promise<string> {
    try {
      return await res.text();
    } catch {
      return "";
    }
  }

  private async getJson<T>(path: string, headers: Record<string, string> = {}): Promise<T> {
    const res = await fetch(this.url(path), { headers, credentials: "include" });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as T;
  }

  private async postJson<T>(path: string, body: unknown, headers: Record<string, string> = {}): Promise<T> {
    const doReq = async (): Promise<Response> => {
      return await fetch(this.url(path), {
        method: "POST",
        headers: { "Content-Type": "application/json", ...this.csrfHeaders(), ...headers },
        body: JSON.stringify(body),
        credentials: "include",
      });
    };

    let res = await doReq();
    if (!res.ok) {
      const text = await this.readText(res);

      // Web 登录会话下：可能是 CSRF token 过期（服务端会返回 csrf_required）。
      // 仅在“未使用 Bearer admin_token”时尝试刷新一次 CSRF。
      if (res.status === 403 && !this.adminToken && text.includes("csrf_required")) {
        try {
          const me = await this.authMe();
          if (me.authenticated && me.csrf_token) {
            this.csrfToken = me.csrf_token;
            res = await doReq();
            if (res.ok) return (await res.json()) as T;
          }
        } catch {
          // 忽略刷新失败，走原始错误输出
        }
      }

      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as T;
  }

  async healthz(): Promise<{ ok: boolean }> {
    return await this.getJson("/healthz");
  }

  async metricsText(): Promise<string> {
    const res = await fetch(this.url("/metrics"), { credentials: "include" });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return await res.text();
  }

  async authMe(): Promise<AuthMeResp> {
    return await this.getJson("/api/auth/me");
  }

  async authLogin(username: string, password: string): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/login", { username, password });
  }

  async authRegister(payload: {
    email: string;
    username: string;
    password: string;
    real_name: string;
    student_id: string;
    advisor: string;
    expected_graduation_year: number;
    phone: string;
  }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/register", payload);
  }

  async authForgotPassword(email: string): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/forgot-password", { email });
  }

  async authResetPassword(payload: { username: string; token: string; new_password: string }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/reset-password", payload);
  }

  async authChangePassword(currentPassword: string, newPassword: string): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/change-password", {
      current_password: currentPassword,
      new_password: newPassword,
    });
  }

  async authLogout(): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/logout", {});
  }

  async announcements(limit = 20): Promise<{ announcements: Announcement[] }> {
    return await this.getJson(`/api/announcements?limit=${limit}`);
  }

  async userBalance(username: string): Promise<BalanceResp> {
    return await this.getJson(`/api/users/${encodeURIComponent(username)}/balance`);
  }

  async userUsage(username: string, limit: number): Promise<{ records: UsageRecord[] }> {
    return await this.getJson(`/api/users/${encodeURIComponent(username)}/usage?limit=${limit}`);
  }

  async userMe(): Promise<UserProfile> {
    return await this.getJson("/api/user/me");
  }

  async userUpdateProfile(payload: {
    email: string;
    username: string;
    student_id: string;
    real_name: string;
    advisor: string;
    expected_graduation_year: number;
    phone: string;
    change_reason: string;
  }): Promise<{ ok: boolean; profile_updated: boolean; request_submitted: boolean; message: string }> {
    return await this.postJson("/api/user/me/profile", payload);
  }

  async userProfileChangeRequests(limit: number): Promise<{ requests: ProfileChangeRequest[] }> {
    return await this.getJson(`/api/user/me/profile-change-requests?limit=${limit}`);
  }

  async userMyBalance(): Promise<BalanceResp> {
    return await this.getJson("/api/user/me/balance");
  }

  async userMyUsage(limit: number): Promise<{ records: UsageRecord[] }> {
    return await this.getJson(`/api/user/me/usage?limit=${limit}`);
  }

  async userAccounts(): Promise<{ accounts: UserNodeAccount[] }> {
    return await this.getJson("/api/user/accounts");
  }

  async userUpsertAccount(nodeId: string, localUsername: string): Promise<{ ok: boolean }> {
    return await this.postJson("/api/user/accounts", { node_id: nodeId, local_username: localUsername });
  }

  async userUpdateAccount(payload: {
    old_node_id: string;
    old_local_username: string;
    new_node_id: string;
    new_local_username: string;
  }): Promise<{ ok: boolean }> {
    const res = await fetch(this.url("/api/user/accounts"), {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...this.csrfHeaders() },
      credentials: "include",
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async userDeleteAccount(nodeId: string, localUsername: string): Promise<{ ok: boolean }> {
    const q = new URLSearchParams({ node_id: nodeId, local_username: localUsername });
    const res = await fetch(this.url(`/api/user/accounts?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async registryResolve(nodeId: string, localUsername: string): Promise<RegistryResolveResp> {
    const q = new URLSearchParams();
    q.set("node_id", nodeId.trim());
    q.set("local_username", localUsername.trim());
    return await this.getJson(`/api/registry/resolve?${q.toString()}`);
  }

  async userRequests(billingUsername: string, limit: number): Promise<{ requests: UserRequest[] }> {
    const q = new URLSearchParams();
    q.set("billing_username", billingUsername.trim());
    q.set("limit", String(limit));
    return await this.getJson(`/api/requests?${q.toString()}`);
  }

  async createBindRequests(
    billingUsername: string,
    items: Array<{ node_id: string; local_username: string }>,
    message: string,
  ): Promise<{ ok: boolean; request_ids: number[] }> {
    return await this.postJson("/api/requests/bind", { billing_username: billingUsername, items, message });
  }

  async createOpenRequest(
    billingUsername: string,
    nodeId: string,
    localUsername: string,
    message: string,
  ): Promise<{ ok: boolean; request_id: number }> {
    return await this.postJson("/api/requests/open", {
      billing_username: billingUsername,
      node_id: nodeId,
      local_username: localUsername,
      message,
    });
  }

  async adminUsers(): Promise<{ users: Array<{ Username?: string; Balance?: number; Status?: string; username?: string; balance?: number; status?: string }> }> {
    return await this.getJson("/api/admin/users", this.adminHeaders());
  }

  async adminUsersDetails(limit = 1000): Promise<{ users: AdminUserDetail[] }> {
    return await this.getJson(`/api/admin/users/details?limit=${limit}`, this.adminHeaders());
  }

  async adminPrices(): Promise<{ prices: Array<{ Model?: string; Price?: number; model?: string; price?: number }> }> {
    return await this.getJson("/api/admin/prices", this.adminHeaders());
  }

  async adminSetPrice(model: string, pricePerMinute: number): Promise<{ ok: boolean }> {
    return await this.postJson(
      "/api/admin/prices",
      { gpu_model: model, price_per_minute: pricePerMinute },
      this.adminHeaders(),
    );
  }

  async adminRecharge(username: string, amount: number, method: string): Promise<BalanceResp> {
    return await this.postJson(
      `/api/users/${encodeURIComponent(username)}/recharge`,
      { amount, method },
      this.adminHeaders(),
    );
  }

  async adminUsage(params: { billingUsername?: string; localUsername?: string; unregisteredOnly?: boolean; limit: number }): Promise<{ records: UsageRecord[] }> {
    const q = new URLSearchParams();
    if ((params.billingUsername ?? "").trim()) q.set("billing_username", (params.billingUsername ?? "").trim());
    if ((params.localUsername ?? "").trim()) q.set("local_username", (params.localUsername ?? "").trim());
    if (params.unregisteredOnly) q.set("unregistered_only", "1");
    q.set("limit", String(params.limit));
    return await this.getJson(`/api/admin/usage?${q.toString()}`, this.adminHeaders());
  }

  async adminNodes(limit: number): Promise<{ nodes: NodeStatus[] }> {
    return await this.getJson(`/api/admin/nodes?limit=${limit}`, this.adminHeaders());
  }

  async adminDisconnectNodeSSH(nodeId: string): Promise<{ ok: boolean; node_id: string; ssh_active_count: number; message: string }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/ssh/disconnect-all`, {}, this.adminHeaders());
  }

  async adminRequests(params: { status?: string; limit?: number }): Promise<{ requests: UserRequest[] }> {
    const q = new URLSearchParams();
    if (params.status?.trim()) q.set("status", params.status.trim());
    q.set("limit", String(params.limit ?? 200));
    return await this.getJson(`/api/admin/requests?${q.toString()}`, this.adminHeaders());
  }

  async adminApproveRequest(requestId: number): Promise<{ ok: boolean; request: UserRequest }> {
    return await this.postJson(`/api/admin/requests/${requestId}/approve`, {}, this.adminHeaders());
  }

  async adminRejectRequest(requestId: number): Promise<{ ok: boolean; request: UserRequest }> {
    return await this.postJson(`/api/admin/requests/${requestId}/reject`, {}, this.adminHeaders());
  }

  async adminBatchReview(requestIds: number[], newStatus: "approved" | "rejected"): Promise<{ ok: boolean; ok_count: number; fail_count: number; fail_items: Array<{request_id:number;error:string}> }> {
    return await this.postJson(`/api/admin/requests/batch-review`, { request_ids: requestIds, new_status: newStatus }, this.adminHeaders());
  }

  async adminProfileChangeRequests(params: { status?: string; username?: string; limit?: number }): Promise<{ requests: ProfileChangeRequest[] }> {
    const q = new URLSearchParams();
    if (params.status?.trim()) q.set("status", params.status.trim());
    if (params.username?.trim()) q.set("username", params.username.trim());
    q.set("limit", String(params.limit ?? 500));
    return await this.getJson(`/api/admin/profile-change-requests?${q.toString()}`, this.adminHeaders());
  }

  async adminApproveProfileChange(requestId: number): Promise<{ ok: boolean; request: ProfileChangeRequest }> {
    return await this.postJson(`/api/admin/profile-change-requests/${requestId}/approve`, {}, this.adminHeaders());
  }

  async adminRejectProfileChange(requestId: number): Promise<{ ok: boolean; request: ProfileChangeRequest }> {
    return await this.postJson(`/api/admin/profile-change-requests/${requestId}/reject`, {}, this.adminHeaders());
  }

  async adminCreateAnnouncement(payload: { title: string; content: string; pinned: boolean }): Promise<{ ok: boolean }> {
    return await this.postJson(`/api/admin/announcements`, payload, this.adminHeaders());
  }

  async adminDeleteAnnouncement(id: number): Promise<{ ok: boolean }> {
    const res = await fetch(this.url(`/api/admin/announcements/${id}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminQueue(): Promise<{ queue: Array<{ username: string; gpu_type: string; count: number; timestamp: string }> }> {
    return await this.getJson("/api/admin/gpu/queue", this.adminHeaders());
  }

  async adminExportUsageCSV(params: { username?: string; from?: string; to?: string; limit?: number }): Promise<Blob> {
    const q = new URLSearchParams();
    if (params.username?.trim()) q.set("username", params.username.trim());
    if (params.from?.trim()) q.set("from", params.from.trim());
    if (params.to?.trim()) q.set("to", params.to.trim());
    q.set("limit", String(params.limit ?? 20000));
    const res = await fetch(this.url(`/api/admin/usage/export.csv?${q.toString()}`), {
      headers: { ...this.adminHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return await res.blob();
  }

  async adminGetMailSettings(): Promise<{
    smtp_host: string;
    smtp_port: number;
    smtp_user: string;
    smtp_password_set: boolean;
    from_email: string;
    from_name: string;
  }> {
    return await this.getJson("/api/admin/mail/settings", this.adminHeaders());
  }

  async adminSetMailSettings(payload: {
    smtp_host: string;
    smtp_port: number;
    smtp_user: string;
    smtp_pass: string;
    update_pass: boolean;
    from_email: string;
    from_name: string;
  }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/mail/settings", payload, this.adminHeaders());
  }

  async adminMailTest(username: string): Promise<{ ok: boolean; email: string }> {
    return await this.postJson("/api/admin/mail/test", { username }, this.adminHeaders());
  }

  async adminAccounts(billingUsername = ""): Promise<{ accounts: UserNodeAccount[] }> {
    const q = new URLSearchParams();
    if (billingUsername.trim()) q.set("billing_username", billingUsername.trim());
    return await this.getJson(`/api/admin/accounts?${q.toString()}`, this.adminHeaders());
  }

  async adminUpsertAccount(payload: { billing_username: string; node_id: string; local_username: string }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/accounts", payload, this.adminHeaders());
  }

  async adminUpdateAccount(payload: {
    old_billing_username: string;
    old_node_id: string;
    old_local_username: string;
    new_billing_username: string;
    new_node_id: string;
    new_local_username: string;
  }): Promise<{ ok: boolean }> {
    const res = await fetch(this.url("/api/admin/accounts"), {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminDeleteAccount(params: { billing_username: string; node_id: string; local_username: string }): Promise<{ ok: boolean }> {
    const q = new URLSearchParams(params);
    const res = await fetch(this.url(`/api/admin/accounts?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminWhitelist(nodeId = ""): Promise<{ entries: SSHWhitelistEntry[] }> {
    const q = new URLSearchParams();
    if (nodeId) q.set("node_id", nodeId);
    return await this.getJson(`/api/admin/whitelist?${q.toString()}`, this.adminHeaders());
  }

  async adminUpsertWhitelist(nodeId: string, usernames: string[], billingUsernames: string[] = []): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/whitelist", { node_id: nodeId, usernames, billing_usernames: billingUsernames }, this.adminHeaders());
  }

  async adminDeleteWhitelist(nodeId: string, localUsername: string): Promise<{ ok: boolean }> {
    const q = new URLSearchParams({ node_id: nodeId, local_username: localUsername });
    const res = await fetch(this.url(`/api/admin/whitelist?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminBlacklist(nodeId = ""): Promise<{ entries: SSHBlacklistEntry[] }> {
    const q = new URLSearchParams();
    if (nodeId) q.set("node_id", nodeId);
    return await this.getJson(`/api/admin/blacklist?${q.toString()}`, this.adminHeaders());
  }

  async adminUpsertBlacklist(nodeId: string, usernames: string[], billingUsernames: string[] = []): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/blacklist", { node_id: nodeId, usernames, billing_usernames: billingUsernames }, this.adminHeaders());
  }

  async adminDeleteBlacklist(nodeId: string, localUsername: string): Promise<{ ok: boolean }> {
    const q = new URLSearchParams({ node_id: nodeId, local_username: localUsername });
    const res = await fetch(this.url(`/api/admin/blacklist?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminExemptions(nodeId = ""): Promise<{ entries: SSHExemptionEntry[] }> {
    const q = new URLSearchParams();
    if (nodeId) q.set("node_id", nodeId);
    return await this.getJson(`/api/admin/exemptions?${q.toString()}`, this.adminHeaders());
  }

  async adminUpsertExemptions(nodeId: string, usernames: string[], billingUsernames: string[] = []): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/exemptions", { node_id: nodeId, usernames, billing_usernames: billingUsernames }, this.adminHeaders());
  }

  async adminDeleteExemptions(nodeId: string, localUsername: string): Promise<{ ok: boolean }> {
    const q = new URLSearchParams({ node_id: nodeId, local_username: localUsername });
    const res = await fetch(this.url(`/api/admin/exemptions?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminPowerUsers(limit = 1000): Promise<{ users: PowerUser[] }> {
    return await this.getJson(`/api/admin/power-users?limit=${limit}`, this.adminHeaders());
  }

  async adminCreatePowerUser(payload: {
    username: string;
    password: string;
    can_view_board: boolean;
    can_view_nodes: boolean;
    can_review_requests: boolean;
  }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/power-users", payload, this.adminHeaders());
  }

  async adminUpdatePowerUserPermissions(
    username: string,
    payload: { can_view_board: boolean; can_view_nodes: boolean; can_review_requests: boolean },
  ): Promise<{ ok: boolean }> {
    const res = await fetch(this.url(`/api/admin/power-users/${encodeURIComponent(username)}/permissions`), {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminDeletePowerUser(username: string): Promise<{ ok: boolean }> {
    const res = await fetch(this.url(`/api/admin/power-users/${encodeURIComponent(username)}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminStatsUsers(params: { from?: string; to?: string; limit?: number }): Promise<{ from: string; to: string; rows: UsageUserSummary[] }> {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    q.set("limit", String(params.limit ?? 1000));
    return await this.getJson(`/api/admin/stats/users?${q.toString()}`, this.adminHeaders());
  }

  async adminStatsPlatformUsers(params: { from?: string; to?: string; limit?: number }): Promise<{ from: string; to: string; rows: PlatformUsageUserSummary[] }> {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    q.set("limit", String(params.limit ?? 1000));
    return await this.getJson(`/api/admin/stats/platform-users?${q.toString()}`, this.adminHeaders());
  }

  async adminStatsPlatformUserNodes(username: string, params: { from?: string; to?: string; limit?: number }): Promise<{ from: string; to: string; username: string; rows: PlatformUsageNodeDetail[] }> {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    q.set("limit", String(params.limit ?? 2000));
    return await this.getJson(`/api/admin/stats/platform-users/${encodeURIComponent(username)}/nodes?${q.toString()}`, this.adminHeaders());
  }

  async adminStatsMonthly(params: { from?: string; to?: string; limit?: number }): Promise<{ from: string; to: string; rows: UsageMonthlySummary[] }> {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    q.set("limit", String(params.limit ?? 20000));
    return await this.getJson(`/api/admin/stats/monthly?${q.toString()}`, this.adminHeaders());
  }

  async adminStatsRecharges(params: { from?: string; to?: string; limit?: number }): Promise<{ from: string; to: string; rows: RechargeSummary[] }> {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    q.set("limit", String(params.limit ?? 1000));
    return await this.getJson(`/api/admin/stats/recharges?${q.toString()}`, this.adminHeaders());
  }
}
