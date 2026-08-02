import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { ArrowLeft, ExternalLink, LayoutDashboard } from "lucide-react";
import { api, type Instance } from "@/lib/api";

/**
 * 全屏内嵌 Hermes Dashboard 视图。
 *
 * 顶部仅保留一条工具栏（返回 portal / 实例名 / 状态 / 新窗口打开），
 * 其余显示区域全部交给内嵌 iframe 的 hermes dashboard。
 */
export default function DashboardPage() {
  const { id } = useParams();
  const instanceId = Number(id);
  const [inst, setInst] = useState<Instance | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setInst(await api.getInstance(instanceId));
      setError("");
    } catch (e: any) {
      setError(e.message);
    }
  }, [instanceId]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="flex h-dvh flex-col bg-zinc-950">
      {/* ── 工具栏 ── */}
      <div className="flex h-12 shrink-0 items-center gap-3 border-b border-zinc-800 bg-zinc-900/90 px-3">
        <Link
          to="/instances"
          className="flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm text-zinc-300 hover:bg-zinc-800 hover:text-white"
          title="退出 Dashboard，返回 Portal 实例列表"
        >
          <ArrowLeft className="h-4 w-4" /> 返回
        </Link>
        <div className="mx-1 h-5 w-px bg-zinc-800" />
        <LayoutDashboard className="h-4 w-4 shrink-0 text-amber-400" />
        <span className="max-w-[30vw] truncate text-sm font-medium">{inst?.name ?? "加载中…"}</span>
        {inst && (
          <span
            className={`rounded-full px-2 py-0.5 text-[11px] ${
              inst.status === "running"
                ? "bg-emerald-500/15 text-emerald-300"
                : inst.status === "starting"
                ? "bg-amber-500/15 text-amber-300"
                : "bg-zinc-700/40 text-zinc-400"
            }`}
          >
            {inst.status}
          </span>
        )}
        <div className="flex-1" />
        <a
          href={`/instances/${instanceId}/dashboard/`}
          target="_blank"
          rel="noreferrer"
          className="flex items-center gap-1.5 rounded-md border border-zinc-700 px-2.5 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
          title="在新窗口打开 Dashboard"
        >
          <ExternalLink className="h-3.5 w-3.5" /> 新窗口
        </a>
      </div>

      {/* ── Dashboard 显示区域：占满剩余空间 ── */}
      <div className="min-h-0 flex-1">
        {error ? (
          <div className="flex h-full items-center justify-center text-red-400">{error}</div>
        ) : (
          <iframe
            src={`/instances/${instanceId}/dashboard/`}
            className="h-full w-full border-0"
            title={`${inst?.name ?? "instance"} dashboard`}
          />
        )}
      </div>
    </div>
  );
}
