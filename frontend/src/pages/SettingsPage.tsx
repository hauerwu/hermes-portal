import { useCallback, useEffect, useState } from "react";
import { Loader2, Moon, Settings as SettingsIcon, Sun } from "lucide-react";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { applyTheme, getTheme, type Theme } from "@/lib/theme";

interface OIDCSettings {
  enabled: boolean;
  issuer: string;
  client_id: string;
  client_secret: string;
  scopes: string;
  admin_claim: string;
  auto_provision: boolean;
  redirect_uri: string;
  editable: boolean;
}

const emptyOIDC: OIDCSettings = {
  enabled: false,
  issuer: "",
  client_id: "",
  client_secret: "",
  scopes: "openid profile email",
  admin_claim: "hermes_portal_admin",
  auto_provision: false,
  redirect_uri: "",
  editable: false,
};

export default function SettingsPage() {
  const { user } = useAuth();
  const [docker, setDocker] = useState(false);
  const [theme, setTheme] = useState<Theme>(getTheme);
  const [oidc, setOidc] = useState<OIDCSettings | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const loadOIDC = useCallback(async () => {
    try {
      setOidc(await api.getOidcSettings());
    } catch {
      setOidc(emptyOIDC);
    }
  }, []);

  useEffect(() => {
    loadOIDC();
    api.get<{ docker: boolean }>("/api/health").then((h) => setDocker(h.docker)).catch(() => {});
  }, [loadOIDC]);

  const saveOIDC = async () => {
    if (!oidc) return;
    setSaving(true);
    setSaveMsg(null);
    try {
      const res = await api.updateOidcSettings({
        enabled: oidc.enabled,
        issuer: oidc.issuer,
        client_id: oidc.client_id,
        client_secret: oidc.client_secret,
        scopes: oidc.scopes,
        admin_claim: oidc.admin_claim,
        auto_provision: oidc.auto_provision,
      });
      if (res.discovery_ok) {
        setSaveMsg({ ok: true, text: "已保存并生效。" });
      } else {
        setSaveMsg({ ok: false, text: `配置已保存，但 OIDC 服务商发现失败：${res.error ?? "请检查 Issuer 地址"}` });
      }
      await loadOIDC();
    } catch (e: any) {
      setSaveMsg({ ok: false, text: e.message || "保存失败" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="mx-auto max-w-3xl p-6">
      <div className="mb-5 flex items-center gap-2">
        <SettingsIcon className="h-5 w-5 text-amber-400" />
        <h1 className="text-xl font-semibold">设置</h1>
      </div>

      <div className="space-y-4">
        {/* ── 界面风格 ── */}
        <section className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-5">
          <h2 className="mb-3 text-sm font-semibold">界面风格</h2>
          <p className="mb-3 text-sm text-zinc-400">选择 portal 的显示风格，选择会立即生效并记住。</p>
          <div className="grid grid-cols-2 gap-3">
            <button
              onClick={() => { setTheme("dark"); applyTheme("dark"); }}
              className={`flex items-center justify-center gap-2 rounded-lg border px-4 py-3 text-sm ${
                theme === "dark"
                  ? "border-amber-500 bg-amber-500/10 text-amber-300"
                  : "border-zinc-700 text-zinc-400 hover:bg-zinc-800"
              }`}
            >
              <Moon className="h-4 w-4" /> 深色风格
            </button>
            <button
              onClick={() => { setTheme("light"); applyTheme("light"); }}
              className={`flex items-center justify-center gap-2 rounded-lg border px-4 py-3 text-sm ${
                theme === "light"
                  ? "border-amber-500 bg-amber-500/10 text-amber-300"
                  : "border-zinc-700 text-zinc-400 hover:bg-zinc-800"
              }`}
            >
              <Sun className="h-4 w-4" /> 浅色风格
            </button>
          </div>
        </section>

        {/* ── OIDC 单点登录 ── */}
        <section className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-5">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold">OIDC 单点登录</h2>
            <span
              className={`rounded-full px-2 py-0.5 text-[11px] ${
                oidc?.enabled ? "bg-emerald-500/15 text-emerald-300" : "bg-zinc-700/40 text-zinc-500"
              }`}
            >
              {oidc?.enabled ? "已启用" : "未启用"}
            </span>
          </div>
          <p className="mb-3 text-sm text-zinc-400">
            支持 Keycloak、Dex、Okta、Entra ID、Auth0 等标准 OIDC 服务商。配置保存后立即生效，无需重启。
          </p>

          {oidc === null ? (
            <div className="py-4 text-center text-zinc-500"><Loader2 className="mx-auto h-5 w-5 animate-spin" /></div>
          ) : oidc.editable ? (
            <div className="space-y-3">
              <label className="flex items-center gap-2 text-sm text-zinc-300">
                <input
                  type="checkbox"
                  checked={oidc.enabled}
                  onChange={(e) => setOidc({ ...oidc, enabled: e.target.checked })}
                  className="h-4 w-4 accent-amber-500"
                />
                启用 OIDC 单点登录
              </label>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <Field label="Issuer（服务商地址）" wide>
                  <input
                    value={oidc.issuer}
                    onChange={(e) => setOidc({ ...oidc, issuer: e.target.value })}
                    placeholder="https://keycloak.example.com/realms/master"
                    className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500"
                  />
                </Field>
                <Field label="Client ID">
                  <input
                    value={oidc.client_id}
                    onChange={(e) => setOidc({ ...oidc, client_id: e.target.value })}
                    placeholder="hermes-portal"
                    className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500"
                  />
                </Field>
                <Field label="Client Secret">
                  <input
                    type="password"
                    value={oidc.client_secret}
                    onChange={(e) => setOidc({ ...oidc, client_secret: e.target.value })}
                    className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500"
                  />
                </Field>
                <Field label="Scopes">
                  <input
                    value={oidc.scopes}
                    onChange={(e) => setOidc({ ...oidc, scopes: e.target.value })}
                    className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500"
                  />
                </Field>
                <Field label="管理员映射 Claim">
                  <input
                    value={oidc.admin_claim}
                    onChange={(e) => setOidc({ ...oidc, admin_claim: e.target.value })}
                    placeholder="hermes_portal_admin"
                    className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500"
                  />
                </Field>
                <label className="flex items-end gap-2 pb-2 text-sm text-zinc-300">
                  <input
                    type="checkbox"
                    checked={oidc.auto_provision}
                    onChange={(e) => setOidc({ ...oidc, auto_provision: e.target.checked })}
                    className="h-4 w-4 accent-amber-500"
                  />
                  首次登录自动开户（member）
                </label>
              </div>

              {oidc.redirect_uri && (
                <div className="rounded-md border border-zinc-800 bg-zinc-950 p-3 text-xs text-zinc-400">
                  请在 IdP 的客户端配置中登记回调地址：
                  <code className="ml-1 break-all text-sky-300/90">{oidc.redirect_uri}</code>
                </div>
              )}

              {saveMsg && (
                <div className={`rounded-md border p-3 text-sm ${saveMsg.ok ? "border-emerald-700/50 bg-emerald-500/10 text-emerald-300" : "border-red-800/60 bg-red-500/10 text-red-300"}`}>
                  {saveMsg.text}
                </div>
              )}
              <div className="flex justify-end">
                <button
                  onClick={saveOIDC}
                  disabled={saving}
                  className="flex items-center gap-1.5 rounded-md bg-amber-500 px-4 py-1.5 text-sm font-medium text-black hover:bg-amber-400 disabled:opacity-50"
                >
                  {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 保存并生效
                </button>
              </div>
            </div>
          ) : (
            <div className="space-y-2 rounded-md border border-zinc-800 bg-zinc-950 p-3 text-sm">
              <div className="text-zinc-400">
                {oidc.enabled ? <span className="text-emerald-400">已启用 — {oidc.issuer}</span> : "未启用"}
              </div>
              {oidc.enabled && (
                <>
                  <div className="text-zinc-400">
                    管理员映射 claim：
                    <code className="ml-1 rounded bg-zinc-800 px-1.5 py-0.5 text-xs text-amber-300">{oidc.admin_claim || "（未配置）"}</code>
                  </div>
                  <div className="text-zinc-400">
                    自动开户：<span className={oidc.auto_provision ? "text-emerald-400" : "text-zinc-500"}>{oidc.auto_provision ? "开启" : "关闭"}</span>
                  </div>
                </>
              )}
              {user?.role !== "super_admin" && (
                <div className="text-xs text-zinc-600">仅超级管理员可修改 OIDC 配置。</div>
              )}
            </div>
          )}
        </section>

        {/* ── Docker 运行时 ── */}
        <section className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-5">
          <h2 className="mb-3 text-sm font-semibold">Docker 运行时</h2>
          <p className="mb-3 text-sm text-zinc-400">
            本机实例通过挂载的 Docker Socket 管理（/var/run/docker.sock）。每个本地实例是一个独立的
            hermes-agent 容器，共享 hermes-portal-net 网络。
          </p>
          <div className="rounded-md border border-zinc-800 bg-zinc-950 p-3 text-sm">
            {docker ? (
              <span className="text-emerald-400">Docker 可用 — 本机实例管理已启用</span>
            ) : (
              <span className="text-red-400">Docker 不可达 — 仅支持远程实例纳管</span>
            )}
          </div>
        </section>

        {/* ── 架构说明 ── */}
        <section className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-5">
          <h2 className="mb-3 text-sm font-semibold">架构说明</h2>
          <ul className="list-disc space-y-1 pl-5 text-sm text-zinc-400">
            <li>后端：Go + Gin + SQLite（GORM，纯 Go 驱动）</li>
            <li>前端：React 19 + TypeScript + Vite + Tailwind 4</li>
            <li>内嵌 Dashboard：反向代理 + 会话托管，对 hermes-agent 零侵入</li>
            <li>统一网关：OpenAI API（X-API-Key 鉴权）+ Channel Webhook 回调</li>
          </ul>
        </section>
      </div>
    </div>
  );
}

function Field({ label, children, wide = false }: { label: string; children: React.ReactNode; wide?: boolean }) {
  return (
    <div className={wide ? "sm:col-span-2" : ""}>
      <label className="mb-1 block text-xs text-zinc-400">{label}</label>
      {children}
    </div>
  );
}
