import { useCallback, useEffect, useState } from "react";
import { ChevronLeft, ChevronRight, History, Loader2, RefreshCw, Search } from "lucide-react";
import { api, type AuditEntry } from "@/lib/api";

const PAGE_SIZE = 50;

const actionLabel: Record<string, string> = {
  login: "登录",
  login_failed: "登录失败",
  login_oidc: "SSO 登录",
  tenant_create: "创建租户",
  tenant_update: "修改租户",
  tenant_delete: "删除租户",
  user_create: "创建用户",
  user_update: "修改用户",
  user_delete: "删除用户",
  instance_create: "创建实例",
  instance_update: "修改实例",
  instance_update_error: "修改实例失败",
  instance_start: "启动实例",
  instance_stop: "停止实例",
  instance_restart: "重启实例",
  instance_destroy: "销毁实例",
  apikey_create: "创建 API Key",
  apikey_revoke: "吊销 API Key",
};

function humanAction(action: string) {
  return actionLabel[action] || action;
}

export default function AuditLogsPage() {
  const [items, setItems] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [actions, setActions] = useState<string[]>([]);
  const [action, setAction] = useState("");
  const [target, setTarget] = useState("");
  const [actor, setActor] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string | number> = { limit: PAGE_SIZE, offset };
      if (action) params.action = action;
      if (target.trim()) params.target = target.trim();
      if (actor.trim()) params.actor = actor.trim();
      const res = await api.listAudit(params);
      setItems(res.items);
      setTotal(res.total);
      setError("");
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [offset, action, target, actor]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    api.auditActions().then((res) => setActions(res.actions)).catch(() => {});
  }, []);

  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const page = Math.floor(offset / PAGE_SIZE) + 1;

  return (
    <div className="mx-auto max-w-6xl p-6">
      <div className="mb-5 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">审计日志</h1>
          <p className="text-sm text-zinc-500">
            管理操作记录{total > 0 && `（共 ${total} 条）`}
          </p>
        </div>
        <button
          onClick={load}
          className="flex items-center gap-1.5 rounded-md border border-zinc-700 px-3 py-1.5 text-sm hover:bg-zinc-800"
        >
          <RefreshCw className="h-3.5 w-3.5" /> 刷新
        </button>
      </div>

      {/* filters */}
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <select
          value={action}
          onChange={(e) => { setAction(e.target.value); setOffset(0); }}
          className="rounded-md border border-zinc-700 bg-zinc-950 px-3 py-1.5 text-sm outline-none focus:border-amber-500"
        >
          <option value="">全部动作</option>
          {actions.map((a) => (
            <option key={a} value={a}>{humanAction(a)}</option>
          ))}
        </select>
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-zinc-600" />
          <input
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && setOffset(0)}
            placeholder="目标（实例/用户 slug）"
            className="w-56 rounded-md border border-zinc-700 bg-zinc-950 py-1.5 pl-8 pr-3 text-sm outline-none focus:border-amber-500"
          />
        </div>
        <input
          value={actor}
          onChange={(e) => setActor(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && setOffset(0)}
          placeholder="操作者用户名"
          className="w-40 rounded-md border border-zinc-700 bg-zinc-950 px-3 py-1.5 text-sm outline-none focus:border-amber-500"
        />
        {(action || target || actor) && (
          <button
            onClick={() => { setAction(""); setTarget(""); setActor(""); setOffset(0); }}
            className="rounded-md border border-zinc-700 px-3 py-1.5 text-xs text-zinc-400 hover:bg-zinc-800"
          >
            清除筛选
          </button>
        )}
      </div>

      {error && <div className="mb-4 rounded-md border border-red-800 bg-red-500/10 p-3 text-sm text-red-300">{error}</div>}

      {loading ? (
        <div className="flex justify-center py-16 text-zinc-500"><Loader2 className="h-6 w-6 animate-spin" /></div>
      ) : items.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-zinc-800 py-16 text-zinc-500">
          <History className="mb-2 h-8 w-8" /> 暂无审计记录
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-zinc-800">
          <table className="w-full text-sm">
            <thead className="bg-zinc-900 text-left text-xs text-zinc-500">
              <tr>
                <th className="px-4 py-2.5">时间</th>
                <th className="px-4 py-2.5">操作者</th>
                <th className="px-4 py-2.5">动作</th>
                <th className="px-4 py-2.5">目标</th>
                <th className="px-4 py-2.5">详情</th>
                <th className="px-4 py-2.5">IP</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {items.map((row) => (
                <tr key={row.id} className="bg-zinc-950/40 align-top">
                  <td className="whitespace-nowrap px-4 py-2.5 text-xs text-zinc-400">
                    {new Date(row.created_at).toLocaleString()}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-zinc-300">{row.actor || `#${row.actor_id ?? "-"}`}</td>
                  <td className="px-4 py-2.5">
                    <span className="rounded-full bg-zinc-800 px-2 py-0.5 text-[11px] text-zinc-300">
                      {humanAction(row.action)}
                    </span>
                  </td>
                  <td className="max-w-[180px] truncate px-4 py-2.5 font-mono text-xs text-sky-300/80">{row.target || "-"}</td>
                  <td className="max-w-[320px] truncate px-4 py-2.5 text-xs text-zinc-500">{row.detail || "-"}</td>
                  <td className="px-4 py-2.5 font-mono text-[11px] text-zinc-600">{row.ip || "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {total > PAGE_SIZE && (
        <div className="mt-4 flex items-center justify-center gap-3 text-sm">
          <button
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
            disabled={offset === 0}
            className="flex items-center gap-1 rounded-md border border-zinc-700 px-3 py-1.5 hover:bg-zinc-800 disabled:opacity-40"
          >
            <ChevronLeft className="h-3.5 w-3.5" /> 上一页
          </button>
          <span className="text-zinc-400">第 {page} / {pages} 页</span>
          <button
            onClick={() => setOffset(offset + PAGE_SIZE)}
            disabled={offset + PAGE_SIZE >= total}
            className="flex items-center gap-1 rounded-md border border-zinc-700 px-3 py-1.5 hover:bg-zinc-800 disabled:opacity-40"
          >
            下一页 <ChevronRight className="h-3.5 w-3.5" />
          </button>
        </div>
      )}
    </div>
  );
}
