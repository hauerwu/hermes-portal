/** Typed API client for the Hermes Portal backend. */
export interface User {
  id: number;
  tenant_id: number | null;
  username: string;
  email: string;
  role: "super_admin" | "tenant_admin" | "member";
  active: boolean;
  last_login: string | null;
  created_at: string;
}

export interface Tenant {
  id: number;
  name: string;
  slug: string;
  description: string;
  created_at: string;
}

export interface Instance {
  id: number;
  tenant_id: number;
  name: string;
  slug: string;
  mode: "docker" | "remote";
  image: string;
  container_name: string;
  status: string;
  remote_url: string;
  openapi_url: string;
  model_id: number | null;
  config: {
    extra_env?: Record<string, string>;
    mem_limit?: string;
    default_model?: { url: string; model: string; provider?: string } | null;
  };
  last_heartbeat: string | null;
  created_at: string;
  updated_at: string;
}

export interface ApiKey {
  id: number;
  tenant_id: number;
  instance_id: number | null;
  name: string;
  key_prefix: string;
  scopes: string[];
  active: boolean;
  expires_at: string | null;
  last_used: string | null;
  created_at: string;
}

export interface ModelConfig {
  id: number;
  tenant_id: number;
  name: string;
  slug: string;
  provider: string;
  url: string;
  model: string;
  has_key: boolean;
  is_default: boolean;
  created_at: string;
}

export interface AuditEntry {
  id: number;
  tenant_id: number | null;
  actor_id: number | null;
  actor: string;
  action: string;
  target: string;
  detail: string;
  ip: string;
  created_at: string;
}

export interface HealthResult {
  ok: boolean;
  error?: string;
  status?: string;
  container_state?: string;
  health?: Record<string, unknown>;
}

export interface GatewayUrls {
  dashboard: string;
  openapi_base: string;
  openapi_example: { endpoint: string; headers: Record<string, string>; body: unknown };
  webhook_channels: Record<string, string>;
}

const TOKEN_KEY = "portal.access";
const REFRESH_KEY = "portal.refresh";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}
export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_KEY);
}
export function setTokens(access: string, refresh: string) {
  localStorage.setItem(TOKEN_KEY, access);
  localStorage.setItem(REFRESH_KEY, refresh);
}
export function clearTokens() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_KEY);
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, options: RequestInit = {}, retry = true): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(options.headers as Record<string, string>),
  };
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;

  const res = await fetch(path, { ...options, headers });
  if (res.status === 401 && retry) {
    const refreshed = await tryRefresh();
    if (refreshed) return request<T>(path, options, false);
  }
  if (!res.ok) {
    let message = `HTTP ${res.status}`;
    try {
      const body = await res.json();
      message = body.error || body.detail || message;
    } catch {
      /* ignore */
    }
    throw new ApiError(res.status, message);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

async function tryRefresh(): Promise<boolean> {
  const rt = getRefreshToken();
  if (!rt) return false;
  try {
    const res = await fetch("/api/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: rt }),
    });
    if (!res.ok) return false;
    const data = await res.json();
    setTokens(data.access_token, data.refresh_token);
    return true;
  } catch {
    return false;
  }
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "POST", body: body !== undefined ? JSON.stringify(body) : undefined }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "PUT", body: body !== undefined ? JSON.stringify(body) : undefined }),
  del: <T>(path: string) => request<T>(path, { method: "DELETE" }),

  // auth
  login: (username: string, password: string) =>
    request<{ access_token: string; refresh_token: string; user: User }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  me: () => request<User>("/api/auth/me"),
  logout: () => request<{ ok: boolean }>("/api/auth/logout", { method: "POST" }),
  oidcStatus: () => request<{ enabled: boolean; issuer: string }>("/api/auth/oidc/status"),
  ssoExchange: (accessToken: string, refreshToken: string) =>
    request<{ user: User }>("/api/auth/sso/exchange", {
      method: "POST",
      body: JSON.stringify({ access_token: accessToken, refresh_token: refreshToken }),
    }),

  // tenants / users / instances / keys
  listTenants: () => request<Tenant[]>("/api/tenants"),
  createTenant: (b: { name: string; slug?: string; description?: string }) =>
    request<Tenant>("/api/tenants", { method: "POST", body: JSON.stringify(b) }),
  deleteTenant: (id: number) => request<{ ok: boolean }>(`/api/tenants/${id}`, { method: "DELETE" }),

  listUsers: () => request<User[]>("/api/users"),
  createUser: (b: Partial<User> & { password?: string }) =>
    request<User>("/api/users", { method: "POST", body: JSON.stringify(b) }),
  updateUser: (id: number, b: Partial<User> & { password?: string }) =>
    request<User>(`/api/users/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  deleteUser: (id: number) => request<{ ok: boolean }>(`/api/users/${id}`, { method: "DELETE" }),

  listAPIKeys: () => request<ApiKey[]>("/api/apikeys"),
  listInstances: () => request<Instance[]>("/api/instances"),
  createInstance: (b: Record<string, unknown>) =>
    request<Instance>("/api/instances", { method: "POST", body: JSON.stringify(b) }),
  getInstance: (id: number) => request<Instance>(`/api/instances/${id}`),
  updateInstance: (id: number, b: Record<string, unknown>) =>
    request<Instance>(`/api/instances/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  instanceHealth: (id: number) => request<HealthResult>(`/api/instances/${id}/health`),
  instanceLogs: (id: number) => request<{ logs: string }>(`/api/instances/${id}/logs`),
  instanceAction: (id: number, action: "start" | "stop" | "restart") =>
    request<Instance>(`/api/instances/${id}/${action}`, { method: "POST" }),
  destroyInstance: (id: number, keepVolume = false) =>
    request<{ ok: boolean }>(`/api/instances/${id}?keep_volume=${keepVolume ? 1 : 0}`, { method: "DELETE" }),
  gatewayUrls: (id: number) => request<GatewayUrls>(`/api/instances/${id}/gateway-urls`),

  // model library
  listModels: () => request<ModelConfig[]>("/api/models"),
  createModel: (b: Record<string, unknown>) =>
    request<ModelConfig>("/api/models", { method: "POST", body: JSON.stringify(b) }),
  updateModel: (id: number, b: Record<string, unknown>) =>
    request<ModelConfig>(`/api/models/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  setDefaultModel: (id: number) =>
    request<ModelConfig>(`/api/models/${id}/default`, { method: "POST" }),
  testModel: (id: number) =>
    request<{ ok: boolean; method?: string; elapsed_ms?: number; status?: number; error?: string }>(
      `/api/models/${id}/test`,
      { method: "POST" }
    ),
  deleteModel: (id: number) => request<{ ok: boolean }>(`/api/models/${id}`, { method: "DELETE" }),

  // audit
  listAudit: (params: Record<string, string | number> = {}) => {
    const qs = new URLSearchParams(Object.entries(params).map(([k, v]) => [k, String(v)])).toString();
    return request<{ items: AuditEntry[]; total: number; limit: number; offset: number }>(`/api/audit?${qs}`);
  },
  auditActions: () => request<{ actions: string[] }>("/api/audit/actions"),
};
