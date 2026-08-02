import { useCallback, useEffect, useState } from "react";
import { Copy, KeyRound, Loader2, Plus, Trash2 } from "lucide-react";
import ApiDoc from "@/components/ApiDoc";
import Modal from "@/components/Modal";
import { useConfirm } from "@/lib/confirm";
import { api, type ApiKey, type Instance } from "@/lib/api";

export default function ApiKeysPage() {
  const confirmDialog = useConfirm();
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [instances, setInstances] = useState<Instance[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      const [k, i] = await Promise.all([api.listAPIKeys(), api.listInstances()]);
      setKeys(k);
      setInstances(i);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const revoke = async (key: ApiKey) => {
    const ok = await confirmDialog({
      title: "吊销 API Key",
      message: <>确认吊销 <b className="text-amber-300">「{key.name}」</b>？使用该 Key 的调用将立即失败。</>,
      confirmText: "吊销",
      danger: true,
    });
    if (!ok) return;
    await api.del<{ ok: boolean }>(`/api/apikeys/${key.id}`);
    load();
  };

  const instanceName = (id: number | null) => instances.find((i) => i.id === id)?.name || "（全租户）";

  return (
    <div className="mx-auto max-w-4xl p-6">
      <div className="mb-5 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">API Keys</h1>
          <p className="text-sm text-zinc-500">用于统一网关 OpenAI API 的鉴权凭据（仅显示一次）</p>
        </div>
        <button
          onClick={() => setCreateOpen(true)}
          className="flex items-center gap-1.5 rounded-md bg-amber-500 px-3 py-1.5 text-sm font-medium text-black hover:bg-amber-400"
        >
          <Plus className="h-4 w-4" /> 新建 Key
        </button>
      </div>

      {error && <div className="mb-4 rounded-md border border-red-800 bg-red-500/10 p-3 text-sm text-red-300">{error}</div>}

      <ApiDoc />

      {loading ? (
        <div className="flex justify-center py-16 text-zinc-500">
          <Loader2 className="h-6 w-6 animate-spin" />
        </div>
      ) : keys.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-zinc-800 py-16 text-zinc-500">
          <KeyRound className="mb-2 h-8 w-8" /> 暂无 API Key
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-zinc-800">
          <table className="w-full text-sm">
            <thead className="bg-zinc-900 text-left text-xs text-zinc-500">
              <tr>
                <th className="px-4 py-2.5">名称</th>
                <th className="px-4 py-2.5">前缀</th>
                <th className="px-4 py-2.5">作用域</th>
                <th className="px-4 py-2.5">状态</th>
                <th className="px-4 py-2.5">最后使用</th>
                <th className="px-4 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {keys.map((key) => (
                <tr key={key.id} className="bg-zinc-950/40">
                  <td className="px-4 py-2.5">
                    <div className="font-medium">{key.name}</div>
                    <div className="text-xs text-zinc-500">{instanceName(key.instance_id)}</div>
                  </td>
                  <td className="px-4 py-2.5 font-mono text-xs text-zinc-400">{key.key_prefix}…</td>
                  <td className="px-4 py-2.5 text-xs text-zinc-400">
                    {Array.isArray(key.scopes) ? key.scopes.join(", ") : String(key.scopes)}
                  </td>
                  <td className="px-4 py-2.5">
                    <span className={`rounded-full px-2 py-0.5 text-[11px] ${key.active ? "bg-emerald-500/15 text-emerald-300" : "bg-red-500/15 text-red-300"}`}>
                      {key.active ? "启用" : "已吊销"}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-zinc-500">{key.last_used ? new Date(key.last_used).toLocaleString() : "-"}</td>
                  <td className="px-4 py-2.5 text-right">
                    <button
                      onClick={() => revoke(key)}
                      disabled={!key.active}
                      className="text-red-400 hover:text-red-300 disabled:opacity-30"
                      title="吊销"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {createOpen && (
        <CreateKeyModal
          instances={instances}
          onClose={() => setCreateOpen(false)}
          onCreated={load}
        />
      )}
    </div>
  );
}

function CreateKeyModal({
  instances,
  onClose,
  onCreated,
}: {
  instances: Instance[];
  onClose: () => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [instanceId, setInstanceId] = useState<number | "">("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [newKey, setNewKey] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const res = await api.post<{ key: string }>("/api/apikeys", {
        name,
        instance_id: instanceId === "" ? null : instanceId,
      });
      setNewKey(res.key);
    } catch (err: any) {
      setError(err.message || "创建失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose} title={newKey ? "Key 创建成功" : "新建 API Key"}>
      {newKey ? (
          <div>
            <p className="mb-2 text-xs text-amber-300">请立即复制保存，明文只显示这一次：</p>
            <div className="mb-4 flex items-center gap-2 rounded-md border border-amber-700/60 bg-amber-500/10 p-3">
              <code className="flex-1 break-all font-mono text-sm text-amber-200">{newKey}</code>
              <button
                onClick={() => navigator.clipboard.writeText(newKey)}
                className="text-zinc-400 hover:text-zinc-200"
                title="复制"
              >
                <Copy className="h-4 w-4" />
              </button>
            </div>
            <button
              onClick={() => {
                onCreated();
                onClose();
              }}
              className="w-full rounded-md bg-amber-500 py-2 text-sm font-medium text-black hover:bg-amber-400"
            >
              完成
            </button>
          </div>
        ) : (
          <>
            <form onSubmit={submit} className="space-y-4">
              <div>
                <label className="mb-1 block text-xs text-zinc-400">名称</label>
                <input value={name} onChange={(e) => setName(e.target.value)} required
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
              </div>
              <div>
                <label className="mb-1 block text-xs text-zinc-400">作用域（留空 = 全租户实例）</label>
                <select
                  value={instanceId}
                  onChange={(e) => setInstanceId(e.target.value === "" ? "" : Number(e.target.value))}
                  className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500"
                >
                  <option value="">全部实例（本租户）</option>
                  {instances.map((i) => (
                    <option key={i.id} value={i.id}>{i.name} ({i.slug})</option>
                  ))}
                </select>
              </div>
              {error && <div className="text-sm text-red-400">{error}</div>}
              <div className="flex justify-end gap-2 pt-2">
                <button type="button" onClick={onClose} className="rounded-md border border-zinc-700 px-4 py-1.5 text-sm hover:bg-zinc-800">取消</button>
                <button type="submit" disabled={busy}
                  className="flex items-center gap-1.5 rounded-md bg-amber-500 px-4 py-1.5 text-sm font-medium text-black hover:bg-amber-400 disabled:opacity-50">
                  {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 创建
                </button>
              </div>
            </form>
          </>
        )}
    </Modal>
  );
}
