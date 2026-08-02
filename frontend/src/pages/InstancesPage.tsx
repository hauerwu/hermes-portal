import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router";
import {
  Boxes,
  ExternalLink,
  Loader2,
  Play,
  Plus,
  RefreshCw,
  Square,
  Trash2,
} from "lucide-react";
import Modal from "@/components/Modal";
import { api, type Instance, type ModelConfig } from "@/lib/api";

const statusColor: Record<string, string> = {
  running: "bg-emerald-500/15 text-emerald-300",
  starting: "bg-amber-500/15 text-amber-300",
  stopped: "bg-zinc-600/30 text-zinc-400",
  error: "bg-red-500/15 text-red-300",
  created: "bg-sky-500/15 text-sky-300",
  destroyed: "bg-zinc-700/30 text-zinc-500",
};

export default function InstancesPage() {
  const [instances, setInstances] = useState<Instance[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setInstances(await api.listInstances());
      setError("");
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  // 每 15 秒静默轮询健康状态：starting → running 自动流转，无需进入详情页。
  useEffect(() => {
    const timer = setInterval(async () => {
      try {
        const list = await api.listInstances();
        await Promise.all(
          list
            .filter((i) => i.status === "starting" || i.status === "running")
            .map((i) => api.instanceHealth(i.id).catch(() => null))
        );
        setInstances(await api.listInstances());
      } catch {
        /* 静默 */
      }
    }, 15000);
    return () => clearInterval(timer);
  }, []);

  const act = async (inst: Instance, action: "start" | "stop" | "restart") => {
    await api.instanceAction(inst.id, action);
    load();
  };

  const destroy = async (inst: Instance) => {
    if (!confirm(`确认销毁实例「${inst.name}」？容器与数据卷将被删除。`)) return;
    await api.destroyInstance(inst.id);
    load();
  };

  return (
    <div className="mx-auto max-w-6xl p-6">
      <div className="mb-5 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">实例管理</h1>
          <p className="text-sm text-zinc-500">本机 Docker 容器实例与远程 URL 纳管实例</p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={load}
            className="flex items-center gap-1.5 rounded-md border border-zinc-700 px-3 py-1.5 text-sm hover:bg-zinc-800"
          >
            <RefreshCw className="h-3.5 w-3.5" /> 刷新
          </button>
          <button
            onClick={() => setCreateOpen(true)}
            className="flex items-center gap-1.5 rounded-md bg-amber-500 px-3 py-1.5 text-sm font-medium text-zinc-950 hover:bg-amber-400"
          >
            <Plus className="h-4 w-4" /> 新建实例
          </button>
        </div>
      </div>

      {error && <div className="mb-4 rounded-md border border-red-800 bg-red-500/10 p-3 text-sm text-red-300">{error}</div>}

      {loading ? (
        <div className="flex justify-center py-16 text-zinc-500">
          <Loader2 className="h-6 w-6 animate-spin" />
        </div>
      ) : instances.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-zinc-800 py-16 text-zinc-500">
          <Boxes className="mb-2 h-8 w-8" />
          暂无实例，点击「新建实例」创建
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {instances.map((inst) => (
            <div key={inst.id} className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-4">
              <div className="mb-2 flex items-start justify-between">
                <Link to={`/instances/${inst.id}`} className="font-medium hover:text-amber-300">
                  {inst.name}
                </Link>
                <div className="flex items-center gap-1.5">
                  <span className={`rounded-full px-2 py-0.5 text-[11px] ${statusColor[inst.status] || "bg-zinc-800 text-zinc-400"}`}>
                    {inst.status}
                  </span>
                </div>
              </div>
              <div className="mb-3 space-y-0.5 text-xs text-zinc-500">
                <div>slug: {inst.slug}</div>
                <div>
                  {inst.mode === "docker" ? `容器 ${inst.container_name}` : `远程 ${inst.remote_url}`}
                </div>
                <div className="truncate">image: {inst.image || "-"}</div>
              </div>
              <div className="mb-2 flex flex-wrap gap-1.5">
                <Link
                  to={`/instances/${inst.id}/dashboard`}
                  className="flex flex-1 items-center justify-center gap-1.5 rounded-md bg-amber-500/15 px-2 py-1.5 text-xs font-medium text-amber-300 hover:bg-amber-500/25"
                  title="进入该实例的 Hermes Dashboard（内嵌视图）"
                >
                  <ExternalLink className="h-3.5 w-3.5" /> 打开 Dashboard
                </Link>
              </div>
              <div className="flex gap-1.5">
                <button
                  onClick={() => act(inst, "start")}
                  disabled={inst.status === "running" || inst.mode !== "docker"}
                  className="flex items-center gap-1 rounded-md border border-zinc-700 px-2 py-1 text-xs hover:bg-zinc-800 disabled:opacity-40"
                  title="启动（仅本地实例）"
                >
                  <Play className="h-3 w-3" /> 启动
                </button>
                <button
                  onClick={() => act(inst, "stop")}
                  disabled={inst.status !== "running" || inst.mode !== "docker"}
                  className="flex items-center gap-1 rounded-md border border-zinc-700 px-2 py-1 text-xs hover:bg-zinc-800 disabled:opacity-40"
                >
                  <Square className="h-3 w-3" /> 停止
                </button>
                <button
                  onClick={() => act(inst, "restart")}
                  disabled={inst.mode !== "docker"}
                  className="flex items-center gap-1 rounded-md border border-zinc-700 px-2 py-1 text-xs hover:bg-zinc-800 disabled:opacity-40"
                >
                  <RefreshCw className="h-3 w-3" /> 重启
                </button>
                <button
                  onClick={() => destroy(inst)}
                  disabled={inst.mode !== "docker"}
                  className="ml-auto flex items-center gap-1 rounded-md border border-red-900 px-2 py-1 text-xs text-red-400 hover:bg-red-500/10 disabled:opacity-40"
                >
                  <Trash2 className="h-3 w-3" /> 销毁
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {createOpen && <CreateModal onClose={() => setCreateOpen(false)} onCreated={load} />}
    </div>
  );
}

function CreateModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [models, setModels] = useState<ModelConfig[]>([]);
  const [modelId, setModelId] = useState<number | "">("");
  const [mode, setMode] = useState<"docker" | "remote">("docker");
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [image, setImage] = useState("nousresearch/hermes-agent");
  const [remoteUrl, setRemoteUrl] = useState("");
  const [openapiUrl, setOpenapiUrl] = useState("");
  const [modelUrl, setModelUrl] = useState("");
  const [modelName, setModelName] = useState("");
  const [modelKey, setModelKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api.listModels().then((m) => {
      setModels(m);
      const def = m.find((x) => x.is_default);
      if (def) setModelId(def.id);
    }).catch(() => {});
  }, []);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const body: Record<string, unknown> = { name, mode, slug };
      if (mode === "docker") {
        body.image = image;
        if (modelId !== "") {
          body.model_id = modelId; // 从模型库快照
        } else if (modelUrl.trim() && modelName.trim()) {
          body.default_model = {
            url: modelUrl.trim(),
            model: modelName.trim(),
            key: modelKey.trim() || undefined,
          };
        }
      } else {
        body.remote_url = remoteUrl;
        if (openapiUrl) body.openapi_url = openapiUrl;
      }
      await api.createInstance(body);
      onCreated();
      onClose();
    } catch (err: any) {
      setError(err.message || "创建失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose} title="新建实例">
      <form onSubmit={submit} className="space-y-4">
          <div className="grid grid-cols-2 gap-2">
            <button
              type="button"
              onClick={() => setMode("docker")}
              className={`rounded-md border px-3 py-2 text-sm ${mode === "docker" ? "border-amber-500 bg-amber-500/10 text-amber-300" : "border-zinc-700 text-zinc-400"}`}
            >
              本机 Docker
            </button>
            <button
              type="button"
              onClick={() => setMode("remote")}
              className={`rounded-md border px-3 py-2 text-sm ${mode === "remote" ? "border-amber-500 bg-amber-500/10 text-amber-300" : "border-zinc-700 text-zinc-400"}`}
            >
              远程 URL 纳管
            </button>
          </div>
          <div>
            <label className="mb-1 block text-xs text-zinc-400">名称</label>
            <input value={name} onChange={(e) => setName(e.target.value)} required
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
          </div>
          <div>
            <label className="mb-1 block text-xs text-zinc-400">Slug（留空自动生成）</label>
            <input value={slug} onChange={(e) => setSlug(e.target.value)}
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
          </div>
          {mode === "docker" ? (
            <>
              <div>
                <label className="mb-1 block text-xs text-zinc-400">镜像</label>
                <input value={image} onChange={(e) => setImage(e.target.value)}
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
              </div>
              <div>
                <label className="mb-1 block text-xs text-zinc-400">使用模型库配置</label>
                <select
                  value={modelId}
                  onChange={(e) => setModelId(e.target.value === "" ? "" : Number(e.target.value))}
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500"
                >
                  <option value="">不选（手动配置）</option>
                  {models.map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.name}（{m.model}{m.is_default ? " · 默认" : ""}）
                    </option>
                  ))}
                </select>
                {models.length === 0 && (
                  <div className="mt-1 text-[11px] text-zinc-600">模型库为空，可到「模型配置」页添加，或下方手动填写</div>
                )}
              </div>
              <details className="rounded-md border border-zinc-800 bg-zinc-950/40 p-3">
                <summary className="cursor-pointer text-xs font-medium text-zinc-400 hover:text-amber-300">
                  手动配置默认模型参数（可选）：端点 URL / 模型名 / API Key
                </summary>
                <div className="mt-3 space-y-3">
                  <div>
                    <label className="mb-1 block text-xs text-zinc-500">端点 URL</label>
                    <input value={modelUrl} onChange={(e) => setModelUrl(e.target.value)}
                      placeholder="https://api.example.com/v1"
                      className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs text-zinc-500">模型名</label>
                    <input value={modelName} onChange={(e) => setModelName(e.target.value)}
                      placeholder="gpt-4o / deepseek-chat / qwen-max …"
                      className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs text-zinc-500">API Key</label>
                    <input type="password" value={modelKey} onChange={(e) => setModelKey(e.target.value)}
                      placeholder="sk-…"
                      className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
                  </div>
                </div>
              </details>
            </>
          ) : (
            <>
              <div>
                <label className="mb-1 block text-xs text-zinc-400">远程 URL（dashboard/网关地址）</label>
                <input value={remoteUrl} onChange={(e) => setRemoteUrl(e.target.value)} placeholder="https://hermes.example.com" required
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
              </div>
              <div>
                <label className="mb-1 block text-xs text-zinc-400">OpenAI API URL（可选，默认 /v1）</label>
                <input value={openapiUrl} onChange={(e) => setOpenapiUrl(e.target.value)} placeholder="https://hermes.example.com/v1"
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
              </div>
            </>
          )}
          {error && <div className="text-sm text-red-400">{error}</div>}
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="rounded-md border border-zinc-700 px-4 py-1.5 text-sm hover:bg-zinc-800">取消</button>
            <button type="submit" disabled={busy}
              className="flex items-center gap-1.5 rounded-md bg-amber-500 px-4 py-1.5 text-sm font-medium text-zinc-950 hover:bg-amber-400 disabled:opacity-50">
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 创建
            </button>
          </div>
      </form>
    </Modal>
  );
}
