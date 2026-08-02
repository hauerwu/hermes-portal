import { useCallback, useEffect, useState } from "react";
import { Cpu, FlaskConical, Loader2, Pencil, Plus, Star, Trash2 } from "lucide-react";
import Modal from "@/components/Modal";
import { useConfirm } from "@/lib/confirm";
import { api, type ModelConfig } from "@/lib/api";

const providerLabel: Record<string, string> = {
  custom: "自定义端点",
  openai: "OpenAI",
  openrouter: "OpenRouter",
  anthropic: "Anthropic",
  gemini: "Gemini",
  deepseek: "DeepSeek",
  "": "—",
};

export default function ModelConfigsPage() {
  const confirmDialog = useConfirm();
  const [models, setModels] = useState<ModelConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<ModelConfig | null>(null);
  const [testing, setTesting] = useState<Record<number, "loading" | { ok: boolean; label: string }>>({});

  const load = useCallback(async () => {
    try {
      setModels(await api.listModels());
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

  const remove = async (m: ModelConfig) => {
    const ok = await confirmDialog({
      title: "删除模型",
      message: <>确认删除模型 <b className="text-amber-300">「{m.name}」</b>？已关联的实例不受影响。</>,
      confirmText: "删除",
      danger: true,
    });
    if (!ok) return;
    await api.deleteModel(m.id);
    load();
  };

  const makeDefault = async (m: ModelConfig) => {
    await api.setDefaultModel(m.id);
    load();
  };

  const runTest = async (m: ModelConfig) => {
    setTesting((prev) => ({ ...prev, [m.id]: "loading" }));
    try {
      const res = await api.testModel(m.id);
      setTesting((prev) => ({
        ...prev,
        [m.id]: res.ok
          ? { ok: true, label: `✓ 可用（${res.method ?? "chat"} · ${res.elapsed_ms ?? 0}ms）` }
          : { ok: false, label: `✗ ${res.error ?? `HTTP ${res.status ?? "?"}`}` },
      }));
    } catch (e: any) {
      setTesting((prev) => ({ ...prev, [m.id]: { ok: false, label: `✗ ${e.message}` } }));
    }
    // 保留结果 8 秒后自动清除
    setTimeout(() => {
      setTesting((prev) => {
        const next = { ...prev };
        delete next[m.id];
        return next;
      });
    }, 8000);
  };

  return (
    <div className="mx-auto max-w-5xl p-6">
      <div className="mb-5 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">模型配置</h1>
          <p className="text-sm text-zinc-500">模型库：维护多个推理端点，创建实例时选择使用</p>
        </div>
        <button
          onClick={() => setCreateOpen(true)}
          className="flex items-center gap-1.5 rounded-md bg-amber-500 px-3 py-1.5 text-sm font-medium text-black hover:bg-amber-400"
        >
          <Plus className="h-4 w-4" /> 新建模型
        </button>
      </div>

      {error && <div className="mb-4 rounded-md border border-red-800 bg-red-500/10 p-3 text-sm text-red-300">{error}</div>}

      {loading ? (
        <div className="flex justify-center py-16 text-zinc-500"><Loader2 className="h-6 w-6 animate-spin" /></div>
      ) : models.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-zinc-800 py-16 text-zinc-500">
          <Cpu className="mb-2 h-8 w-8" /> 暂无模型配置，点击「新建模型」添加
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {models.map((m) => {
            const testState = testing[m.id];
            return (
            <div
              key={m.id}
              className={`rounded-xl border p-4 ${m.is_default ? "border-amber-600/60 bg-amber-500/5" : "border-zinc-800 bg-zinc-900/60"}`}
            >
              <div className="mb-2 flex items-start justify-between">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{m.name}</span>
                  {m.is_default && (
                    <span className="flex items-center gap-1 rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] text-amber-300">
                      <Star className="h-3 w-3" /> 默认
                    </span>
                  )}
                </div>
                <div className="flex gap-1">
                  <button
                    onClick={() => runTest(m)}
                    disabled={testing[m.id] === "loading"}
                    className="rounded-md p-1.5 text-sky-300 hover:bg-sky-500/10 disabled:opacity-40"
                    title="测试端点连通性与凭证"
                  >
                    {testing[m.id] === "loading" ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <FlaskConical className="h-3.5 w-3.5" />
                    )}
                  </button>
                  <button onClick={() => setEditing(m)} className="rounded-md p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200" title="编辑">
                    <Pencil className="h-3.5 w-3.5" />
                  </button>
                  <button onClick={() => remove(m)} className="rounded-md p-1.5 text-red-400 hover:bg-red-500/10" title="删除">
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
              <div className="space-y-1 text-xs text-zinc-500">
                <div className="truncate">
                  <span className="text-zinc-400">{m.model}</span>
                  <span className="mx-1.5 text-zinc-700">←</span>
                  <span className="text-sky-300/80">{m.url}</span>
                </div>
                <div className="flex items-center gap-3">
                  <span>{providerLabel[m.provider] || m.provider || "—"}</span>
                  <span className={m.has_key ? "text-emerald-400" : "text-zinc-600"}>
                    {m.has_key ? "● 已配置 Key" : "○ 无 Key"}
                  </span>
                </div>
                {testState && testState !== "loading" && (
                  <div
                    className={`mt-2 rounded-md border px-2 py-1 text-[11px] leading-relaxed break-all ${
                      testState.ok
                        ? "border-emerald-700/50 bg-emerald-500/10 text-emerald-300"
                        : "border-red-800/60 bg-red-500/10 text-red-300"
                    }`}
                  >
                    {testState.label}
                  </div>
                )}
              </div>
              {!m.is_default && (
                <button
                  onClick={() => makeDefault(m)}
                  className="mt-3 rounded-md border border-zinc-700 px-2.5 py-1 text-xs text-zinc-400 hover:border-amber-500 hover:text-amber-300"
                >
                  设为默认
                </button>
              )}
            </div>
            );
          })}
        </div>
      )}

      {(createOpen || editing) && (
        <ModelModal
          model={editing}
          onClose={() => { setCreateOpen(false); setEditing(null); }}
          onSaved={() => { setCreateOpen(false); setEditing(null); load(); }}
        />
      )}
    </div>
  );
}

function ModelModal({ model, onClose, onSaved }: { model: ModelConfig | null; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(model?.name ?? "");
  const [provider, setProvider] = useState(model?.provider ?? "custom");
  const [url, setUrl] = useState(model?.url ?? "");
  const [modelName, setModelName] = useState(model?.model ?? "");
  const [key, setKey] = useState("");
  const [isDefault, setIsDefault] = useState(model?.is_default ?? false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const body: Record<string, unknown> = { name, provider, url, model: modelName, is_default: isDefault };
      if (key.trim()) body.key = key.trim(); // 留空保留原 Key
      if (model) {
        await api.updateModel(model.id, body);
      } else {
        await api.createModel(body);
      }
      onSaved();
    } catch (err: any) {
      setError(err.message || "保存失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose} title={model ? "编辑模型" : "新建模型"}>
      <form onSubmit={submit} className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="mb-1 block text-xs text-zinc-400">显示名称</label>
              <input value={name} onChange={(e) => setName(e.target.value)} required
                className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
            </div>
            <div>
              <label className="mb-1 block text-xs text-zinc-400">Provider</label>
              <select value={provider} onChange={(e) => setProvider(e.target.value)}
                className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500">
                <option value="custom">自定义端点 (custom)</option>
                <option value="openai">OpenAI</option>
                <option value="openrouter">OpenRouter</option>
                <option value="anthropic">Anthropic</option>
                <option value="gemini">Gemini</option>
                <option value="deepseek">DeepSeek</option>
              </select>
            </div>
          </div>
          <div>
            <label className="mb-1 block text-xs text-zinc-400">端点 URL</label>
            <input value={url} onChange={(e) => setUrl(e.target.value)} required placeholder="https://api.example.com/v1"
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
          </div>
          <div>
            <label className="mb-1 block text-xs text-zinc-400">模型名</label>
            <input value={modelName} onChange={(e) => setModelName(e.target.value)} required placeholder="gpt-4o / deepseek-chat …"
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
          </div>
          <div>
            <label className="mb-1 block text-xs text-zinc-400">API Key{model ? "（留空则沿用原 Key）" : ""}</label>
            <input type="password" value={key} onChange={(e) => setKey(e.target.value)} placeholder="sk-…"
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
          </div>
          <label className="flex items-center gap-2 text-sm text-zinc-300">
            <input type="checkbox" checked={isDefault} onChange={(e) => setIsDefault(e.target.checked)}
              className="h-4 w-4 accent-amber-500" />
            设为默认模型
          </label>
          {error && <div className="text-sm text-red-400">{error}</div>}
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
