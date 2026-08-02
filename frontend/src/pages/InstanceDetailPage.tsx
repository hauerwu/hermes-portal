import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import {
  ArrowLeft,
  ExternalLink,
  Loader2,
  Pencil,
  Play,
  RefreshCw,
  Square,
  Terminal,
  Trash2,
} from "lucide-react";
import { api, type GatewayUrls, type HealthResult, type Instance } from "@/lib/api";

export default function InstanceDetailPage() {
  const { id } = useParams();
  const instanceId = Number(id);
  const [inst, setInst] = useState<Instance | null>(null);
  const [health, setHealth] = useState<HealthResult | null>(null);
  const [urls, setUrls] = useState<GatewayUrls | null>(null);
  const [logs, setLogs] = useState("");
  const [showLogs, setShowLogs] = useState(false);
  const [embed, setEmbed] = useState(true);
  const [editOpen, setEditOpen] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [i, h, u] = await Promise.all([
        api.getInstance(instanceId),
        api.instanceHealth(instanceId).catch(() => null),
        api.gatewayUrls(instanceId),
      ]);
      setInst(i);
      setHealth(h);
      setUrls(u);
      setError("");
    } catch (e: any) {
      setError(e.message);
    }
  }, [instanceId]);

  useEffect(() => {
    load();
    const timer = setInterval(() => {
      api.instanceHealth(instanceId).then(setHealth).catch(() => {});
    }, 15000);
    return () => clearInterval(timer);
  }, [load, instanceId]);

  const act = async (action: "start" | "stop" | "restart") => {
    setBusy(true);
    try {
      await api.instanceAction(instanceId, action);
      await load();
    } finally {
      setBusy(false);
    }
  };

  const fetchLogs = async () => {
    const res = await api.instanceLogs(instanceId);
    setLogs(res.logs);
    setShowLogs(true);
  };

  const destroy = async () => {
    if (!confirm(`确认销毁实例「${inst?.name}」？容器与数据卷将被删除。`)) return;
    await api.destroyInstance(instanceId);
    window.location.hash = "#/instances";
  };

  if (!inst) {
    return (
      <div className="flex justify-center py-20 text-zinc-500">
        <Loader2 className="h-6 w-6 animate-spin" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl p-6">
      <div className="mb-4 flex items-center gap-3">
        <Link to="/instances" className="text-zinc-400 hover:text-zinc-200">
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <h1 className="text-xl font-semibold">{inst.name}</h1>
        <span className="rounded-full bg-zinc-800 px-2 py-0.5 text-[11px] text-zinc-400">{inst.status}</span>
        <span className="rounded-full bg-zinc-800 px-2 py-0.5 text-[11px] text-zinc-400">
          {inst.mode === "docker" ? "本机容器" : "远程纳管"}
        </span>
        {health?.ok && <span className="text-xs text-emerald-400">● 健康</span>}
        {health && !health.ok && <span className="text-xs text-red-400">● 异常</span>}
      </div>

      {error && <div className="mb-4 rounded-md border border-red-800 bg-red-500/10 p-3 text-sm text-red-300">{error}</div>}

      <div className="mb-4 flex flex-wrap gap-2">
        <button onClick={() => act("start")} disabled={busy || inst.mode !== "docker"}
          className="flex items-center gap-1.5 rounded-md border border-zinc-700 px-3 py-1.5 text-sm hover:bg-zinc-800 disabled:opacity-40">
          <Play className="h-3.5 w-3.5" /> 启动
        </button>
        <button onClick={() => act("stop")} disabled={busy || inst.mode !== "docker"}
          className="flex items-center gap-1.5 rounded-md border border-zinc-700 px-3 py-1.5 text-sm hover:bg-zinc-800 disabled:opacity-40">
          <Square className="h-3.5 w-3.5" /> 停止
        </button>
        <button onClick={() => act("restart")} disabled={busy || inst.mode !== "docker"}
          className="flex items-center gap-1.5 rounded-md border border-zinc-700 px-3 py-1.5 text-sm hover:bg-zinc-800 disabled:opacity-40">
          <RefreshCw className="h-3.5 w-3.5" /> 重启
        </button>
        <button onClick={fetchLogs}
          className="flex items-center gap-1.5 rounded-md border border-zinc-700 px-3 py-1.5 text-sm hover:bg-zinc-800 disabled:opacity-40">
          <Terminal className="h-3.5 w-3.5" /> 容器日志
        </button>
        <a
          href={`/instances/${instanceId}/dashboard/`}
          target="_blank"
          rel="noreferrer"
          className="flex items-center gap-1.5 rounded-md bg-amber-500 px-3 py-1.5 text-sm font-medium text-zinc-950 hover:bg-amber-400"
        >
          <ExternalLink className="h-3.5 w-3.5" /> 打开 Dashboard
        </a>
        <button onClick={() => setEditOpen(true)}
          className="flex items-center gap-1.5 rounded-md border border-zinc-700 px-3 py-1.5 text-sm hover:bg-zinc-800">
          <Pencil className="h-3.5 w-3.5" /> 编辑
        </button>
        <button onClick={destroy} disabled={inst.mode !== "docker"}
          className="ml-auto flex items-center gap-1.5 rounded-md border border-red-900 px-3 py-1.5 text-sm text-red-400 hover:bg-red-500/10 disabled:opacity-40">
          <Trash2 className="h-3.5 w-3.5" /> 销毁实例
        </button>
      </div>

      {showLogs && (
        <div className="mb-4 rounded-lg border border-zinc-800 bg-black/60 p-3">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-xs text-zinc-400">容器日志（最近 500 行）</span>
            <button onClick={() => setShowLogs(false)} className="text-xs text-zinc-500 hover:text-zinc-300">关闭</button>
          </div>
          <pre className="max-h-64 overflow-auto whitespace-pre-wrap font-mono text-[11px] leading-relaxed text-zinc-300">{logs}</pre>
        </div>
      )}

      {urls && (
        <div className="mb-4 rounded-xl border border-zinc-800 bg-zinc-900/60 p-4">
          <h2 className="mb-2 text-sm font-semibold">统一网关 URL</h2>
          <div className="space-y-1 text-xs">
            <div className="flex items-center gap-2">
              <span className="w-24 shrink-0 text-zinc-500">OpenAI API</span>
              <code className="truncate text-amber-300/90">{urls.openapi_base}/v1/chat/completions</code>
              <button
                onClick={() => navigator.clipboard.writeText(`${urls.openapi_base}/v1/chat/completions`)}
                className="text-zinc-500 hover:text-zinc-300">复制</button>
            </div>
            <div className="flex items-center gap-2">
              <span className="w-24 shrink-0 text-zinc-500">Webhook</span>
              <code className="truncate text-sky-300/90">
                {Object.entries(urls.webhook_channels)
                  .map(([ch, u]) => `${ch} → ${u}`)
                  .join("  ")}
              </code>
            </div>
            <div className="flex items-center gap-2">
              <span className="w-24 shrink-0 text-zinc-500">调用示例</span>
              <code className="truncate text-zinc-400">
                curl -H "X-API-Key: hp_..." {urls.openapi_example.endpoint}
              </code>
            </div>
          </div>
        </div>
      )}

      <div className="mb-2 flex items-center gap-3 text-sm">
        <button
          onClick={() => setEmbed(true)}
          className={embed ? "font-medium text-amber-300" : "text-zinc-400 hover:text-zinc-200"}
        >
          内嵌 Dashboard
        </button>
        <a
          href={`/instances/${instanceId}/dashboard/`}
          target="_blank"
          rel="noreferrer"
          className="flex items-center gap-1 text-zinc-400 hover:text-zinc-200"
        >
          <ExternalLink className="h-3.5 w-3.5" /> 全屏打开
        </a>
      </div>

      {embed && (
        <div className="h-[75vh] overflow-hidden rounded-xl border border-zinc-800 bg-zinc-950">
          <iframe
            src={`/instances/${instanceId}/dashboard/`}
            className="h-full w-full border-0"
            title={`${inst.name} dashboard`}
          />
        </div>
      )}

      {editOpen && (
        <EditInstanceModal
          instance={inst}
          onClose={() => setEditOpen(false)}
          onSaved={() => {
            setEditOpen(false);
            load();
          }}
        />
      )}
    </div>
  );
}

function EditInstanceModal({
  instance,
  onClose,
  onSaved,
}: {
  instance: Instance;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(instance.name);
  const [slug, setSlug] = useState(instance.slug);
  const [image, setImage] = useState(instance.image || "");
  const [remoteUrl, setRemoteUrl] = useState(instance.remote_url || "");
  const [openapiUrl, setOpenapiUrl] = useState(instance.openapi_url || "");
  const [memLimit, setMemLimit] = useState(instance.config?.mem_limit || "");
  const [extraEnv, setExtraEnv] = useState<Record<string, string>>(instance.config?.extra_env || {});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const body: Record<string, unknown> = {
        name,
        slug,
        extra_env: extraEnv,
        mem_limit: memLimit,
      };
      if (instance.mode === "docker") body.image = image;
      else {
        body.remote_url = remoteUrl;
        body.openapi_url = openapiUrl;
      }
      await api.updateInstance(instance.id, body);
      setNotice(
        instance.mode === "docker"
          ? "已保存。本机实例正在重建容器以应用新配置（数据卷保留）…"
          : "已保存。"
      );
      onSaved();
    } catch (err: any) {
      setError(err.message || "保存失败");
    } finally {
      setBusy(false);
    }
  };

  const envPairs = Object.entries(extraEnv);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div className="max-h-[85vh] w-full max-w-lg overflow-auto rounded-xl border border-zinc-700 bg-zinc-900 p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-4 text-lg font-semibold">编辑实例</h2>
        <form onSubmit={submit} className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="mb-1 block text-xs text-zinc-400">名称</label>
              <input value={name} onChange={(e) => setName(e.target.value)} required
                className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
            </div>
            <div>
              <label className="mb-1 block text-xs text-zinc-400">Slug</label>
              <input value={slug} onChange={(e) => setSlug(e.target.value)}
                className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
            </div>
          </div>

          {instance.mode === "docker" ? (
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="mb-1 block text-xs text-zinc-400">镜像</label>
                <input value={image} onChange={(e) => setImage(e.target.value)}
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
              </div>
              <div>
                <label className="mb-1 block text-xs text-zinc-400">内存限制（如 2g）</label>
                <input value={memLimit} onChange={(e) => setMemLimit(e.target.value)} placeholder="留空 = 不限"
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
              </div>
            </div>
          ) : (
            <>
              <div>
                <label className="mb-1 block text-xs text-zinc-400">远程 URL</label>
                <input value={remoteUrl} onChange={(e) => setRemoteUrl(e.target.value)}
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
              </div>
              <div>
                <label className="mb-1 block text-xs text-zinc-400">OpenAI API URL</label>
                <input value={openapiUrl} onChange={(e) => setOpenapiUrl(e.target.value)}
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
              </div>
            </>
          )}

          <div>
            <label className="mb-1 block text-xs text-zinc-400">额外环境变量（注入容器 / 网关配置）</label>
            <div className="space-y-2">
              {envPairs.length === 0 && <div className="text-xs text-zinc-600">暂无额外环境变量</div>}
              {envPairs.map(([k, v]) => (
                <div key={k} className="flex gap-2">
                  <input value={k} readOnly
                    className="w-2/5 rounded-md border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
                  <input value={v}
                    onChange={(e) => setExtraEnv((prev) => ({ ...prev, [k]: e.target.value }))}
                    className="flex-1 rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 font-mono text-xs outline-none focus:border-amber-500" />
                  <button type="button"
                    onClick={() => {
                      const next = { ...extraEnv };
                      delete next[k];
                      setExtraEnv(next);
                    }}
                    className="text-xs text-red-400 hover:text-red-300">删除</button>
                </div>
              ))}
              <button
                type="button"
                onClick={() => setExtraEnv((prev) => ({ ...prev, [`ENV_${envPairs.length + 1}`]: "" }))}
                className="rounded-md border border-dashed border-zinc-700 px-3 py-1.5 text-xs text-zinc-400 hover:border-amber-500 hover:text-amber-300"
              >
                + 添加变量
              </button>
            </div>
          </div>

          {error && <div className="text-sm text-red-400">{error}</div>}
          {notice && <div className="text-sm text-emerald-400">{notice}</div>}
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="rounded-md border border-zinc-700 px-4 py-1.5 text-sm hover:bg-zinc-800">取消</button>
            <button type="submit" disabled={busy}
              className="flex items-center gap-1.5 rounded-md bg-amber-500 px-4 py-1.5 text-sm font-medium text-zinc-950 hover:bg-amber-400 disabled:opacity-50">
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 保存
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
