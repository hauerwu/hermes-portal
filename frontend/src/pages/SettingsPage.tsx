import { useEffect, useState } from "react";
import { Settings as SettingsIcon } from "lucide-react";
import { api } from "@/lib/api";

export default function SettingsPage() {
  const [oidc, setOidc] = useState<{ enabled: boolean; issuer: string; admin_claim?: string; auto_provision?: boolean } | null>(null);
  const [docker, setDocker] = useState(false);

  useEffect(() => {
    api.oidcStatus().then(setOidc).catch(() => {});
    api.get<{ docker: boolean }>("/api/health").then((h) => setDocker(h.docker)).catch(() => {});
  }, []);

  return (
    <div className="mx-auto max-w-3xl p-6">
      <div className="mb-5 flex items-center gap-2">
        <SettingsIcon className="h-5 w-5 text-amber-400" />
        <h1 className="text-xl font-semibold">设置</h1>
      </div>

      <div className="space-y-4">
        <section className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-5">
          <h2 className="mb-3 text-sm font-semibold">OIDC 单点登录</h2>
          <p className="mb-3 text-sm text-zinc-400">
            通过环境变量配置（PORTAL_OIDC_ENABLED / PORTAL_OIDC_ISSUER / PORTAL_OIDC_CLIENT_ID /
            PORTAL_OIDC_CLIENT_SECRET），支持 Keycloak、Dex、Okta、Entra ID、Auth0 等标准 OIDC 服务商。
          </p>
          <div className="space-y-2 rounded-md border border-zinc-800 bg-zinc-950 p-3 text-sm">
            {oidc === null ? (
              <span className="text-zinc-500">查询中…</span>
            ) : oidc.enabled ? (
              <>
                <div className="text-emerald-400">已启用 — {oidc.issuer}</div>
                <div className="text-zinc-400">
                  管理员映射 claim：
                  <code className="ml-1 rounded bg-zinc-800 px-1.5 py-0.5 text-xs text-amber-300">{oidc.admin_claim || "（未配置）"}</code>
                  {oidc.admin_claim && <span className="ml-2 text-xs text-zinc-500">→ 命中即授予 tenant_admin</span>}
                </div>
                <div className="text-zinc-400">
                  自动开户：<span className={oidc.auto_provision ? "text-emerald-400" : "text-zinc-500"}>{oidc.auto_provision ? "开启" : "关闭"}</span>
                </div>
              </>
            ) : (
              <span className="text-zinc-400">未启用</span>
            )}
          </div>
        </section>

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
