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
import Modal from "@/components/Modal";
import { useConfirm } from "@/lib/confirm";
import { api, type GatewayUrls, type HealthResult, type Instance, type ModelConfig } from "@/lib/api";

export default function InstanceDetailPage() {
  const confirmDialog = useConfirm();
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
    } catch (e: any) {
      setError(e.message || "操作失败");
    } finally {
      setBusy(false);
    }
  };

  const fetchLogs = async () => {
    try {
      const res = await api.instanceLogs(instanceId);
      setLogs(res.logs);
      setShowLogs(true);
    } catch (e: any) {
      setError(e.message || "读取日志失败");
    }
  };

  const destroy = async () => {
    const isDocker = inst?.mode === "docker";
    const ok = await confirmDialog({
      title: "销毁实例",
      message: (
        <>
          确认销毁实例 <b className="text-amber-300">「{inst?.name}」</b>？
          <br />
          {isDocker
            ? "容器与数据卷将被删除，且不可恢复。"
            : "该实例将从门户移除纳管（远程实例本身不受影响）。"}
        </>
      ),
      confirmText: isDocker ? "销毁" : "移除",
      danger: true,
    });
    if (!ok) return;
    try {
      await api.destroyInstance(instanceId);
      window.location.hash = "#/instances";
    } catch (e: any) {
      setError(e.message || "销毁失败");
    }
  };

  if (!inst) {
    return (
      <div className="flex justify-center py-20 text-zinc-500">
        <Loader2 className="h-6 w-6 animate-spin" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl p-4 sm:p-6">
      <div className="mb-4 flex flex-wrap items-center gap-x-3 gap-y-2">
        <Link to="/instances" className="shrink-0 text-zinc-400 hover:text-zinc-200">
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <h1 className="min-w-0 truncate text-xl font-semibold">{inst.name}</h1>
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
        <button onClick={fetchLogs} disabled={inst.mode !== "docker"}
          className="flex items-center gap-1.5 rounded-md border border-zinc-700 px-3 py-1.5 text-sm hover:bg-zinc-800 disabled:opacity-40">
          <Terminal className="h-3.5 w-3.5" /> 容器日志
        </button>
        <Link
          to={`/instances/${instanceId}/dashboard`}
          className="flex items-center gap-1.5 rounded-md bg-amber-500 px-3 py-1.5 text-sm font-medium text-black hover:bg-amber-400"
        >
          <ExternalLink className="h-3.5 w-3.5" /> 打开 Dashboard
        </Link>
        <button onClick={() => setEditOpen(true)}
          className="flex items-center gap-1.5 rounded-md border border-zinc-700 px-3 py-1.5 text-sm hover:bg-zinc-800">
          <Pencil className="h-3.5 w-3.5" /> 编辑
        </button>
        <button onClick={destroy}
          className="ml-auto flex items-center gap-1.5 rounded-md border border-red-900 px-3 py-1.5 text-sm text-red-400 hover:bg-red-500/10">
          <Trash2 className="h-3.5 w-3.5" /> {inst.mode === "docker" ? "销毁实例" : "移除纳管"}
        </button>
      </div>

      {showLogs && (
        <div className="mb-4 rounded-lg border border-neutral-800 bg-neutral-950/90 p-3">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-xs text-zinc-400">容器日志（最近 500 行）</span>
            <button onClick={() => setShowLogs(false)} className="text-xs text-zinc-500 hover:text-zinc-300">关闭</button>
          </div>
          <pre className="max-h-64 overflow-auto whitespace-pre-wrap font-mono text-[11px] leading-relaxed text-neutral-300">{logs}</pre>
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
          onClick={() => setEmbed(!embed)}
          className={embed ? "font-medium text-amber-300" : "text-zinc-400 hover:text-zinc-200"}
        >
          {embed ? "隐藏内嵌预览" : "显示内嵌预览"}
        </button>
        <Link to={`/instances/${instanceId}/dashboard`} className="text-zinc-400 hover:text-amber-300">
          进入全屏 Dashboard 视图 →
        </Link>
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
  const [models, setModels] = useState<ModelConfig[]>([]);
  const [modelId, setModelId] = useState<number | "">(instance.model_id ?? "");
  const [modelUrl, setModelUrl] = useState(instance.config?.default_model?.url || "");
  const [modelName, setModelName] = useState(instance.config?.default_model?.model || "");
  const [modelKey, setModelKey] = useState("");
  const [modelTouched, setModelTouched] = useState(false);
  const [modelCleared, setModelCleared] = useState(false);

  useEffect(() => {
    api.listModels().then(setModels).catch(() => {});
  }, []);
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
      if (modelTouched) {
        // 手动配置优先：脱离模型库关联，使用手动填写的端点参数
        body.model_id = null;
        if (modelUrl.trim() && modelName.trim()) {
          body.default_model = { url: modelUrl.trim(), model: modelName.trim(), key: modelKey.trim() || undefined };
        } else {
          body.default_model = { url: "", model: "" }; // 清空
        }
      } else if (modelId !== "") {
        body.model_id = Number(modelId); // 从模型库切换
      } else if (modelCleared) {
        body.model_id = null; // 取消关联
        body.default_model = { url: "", model: "" }; // 清空
      }
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
    <Modal open onClose={onClose} title="编辑实例" width="max-w-lg">
      <form onSubmit={submit} className="space-y-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
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
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
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

          {instance.mode === "docker" && (
            <div className="rounded-md border border-zinc-800 bg-zinc-950/40 p-3">
              <div className="mb-2 flex items-center justify-between">
                <span className="text-xs font-medium text-zinc-400">默认模型参数</span>
                {!modelTouched && (
                  <button type="button" onClick={() => setModelTouched(true)}
                    className="text-[11px] text-amber-300 hover:text-amber-200">手动修改</button>
                )}
              </div>
              <div className="mb-2">
                <select
                  value={modelId}
                  onChange={(e) => {
                    const v = e.target.value === "" ? "" : Number(e.target.value);
                    setModelId(v);
                    setModelCleared(v === "");
                    // 选择模型库条目后，退出“手动修改”模式，避免手动输入被覆盖
                    if (v !== "") setModelTouched(false);
                  }}
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 text-sm outline-none focus:border-amber-500"
                >
                  <option value="">无（未关联模型库）</option>
                  {models.map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.name}（{m.model}{m.is_default ? " · 默认" : ""}）
                    </option>
                  ))}
                </select>
              </div>
              {modelTouched ? (
                <div className="space-y-2">
                  <input value={modelUrl} onChange={(e) => setModelUrl(e.target.value)}
                    placeholder="端点 URL，如 https://api.example.com/v1"
                    className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 text-sm outline-none focus:border-amber-500" />
                  <input value={modelName} onChange={(e) => setModelName(e.target.value)}
                    placeholder="模型名，如 gpt-4o"
                    className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 text-sm outline-none focus:border-amber-500" />
                  <input value={modelKey} onChange={(e) => setModelKey(e.target.value)}
                    placeholder="API Key（留空则沿用原 Key）"
                    className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 text-sm outline-none focus:border-amber-500" />
                </div>
              ) : (
                <div className="text-xs text-zinc-500">
                  {instance.config?.default_model ? (
                    <>
                      <div className="truncate">{instance.config.default_model.model} ← {instance.config.default_model.url}</div>
                      {instance.config.default_model.provider && (
                        <div className="text-[11px] text-zinc-600">provider: {instance.config.default_model.provider}</div>
                      )}
                    </>
                  ) : (
                    <span className="text-zinc-600">未配置（实例将使用其默认模型）</span>
                  )}
                </div>
              )}
            </div>
          )}

          <div>
            <label className="mb-1 block text-xs text-zinc-400">额外环境变量（注入容器 / 网关配置）</label>
            <div className="space-y-2">
              {envPairs.length === 0 && <div className="text-xs text-zinc-600">暂无额外环境变量</div>}
              {envPairs.map(([k, v]) => (
                <div key={k} className="flex gap-2">
                  <input value={k} readOnly
                    className="w-1/3 rounded-md border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none sm:w-2/5" />
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
              className="flex items-center gap-1.5 rounded-md bg-amber-500 px-4 py-1.5 text-sm font-medium text-black hover:bg-amber-400 disabled:opacity-50">
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 保存
            </button>
          </div>
      </form>
    </Modal>
  );
}
