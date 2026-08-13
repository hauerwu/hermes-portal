import { useCallback, useEffect, useState } from "react";
import { Loader2, Plus, Shield, Trash2 } from "lucide-react";
import Modal from "@/components/Modal";
import { useConfirm } from "@/lib/confirm";
import { api, type Tenant } from "@/lib/api";

export default function TenantsPage() {
  const confirmDialog = useConfirm();
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      setTenants(await api.listTenants());
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const remove = async (t: Tenant) => {
    const ok = await confirmDialog({
      title: "删除租户",
      message: <>确认删除租户 <b className="text-amber-300">「{t.name}」</b>？<br />其下的实例、用户与数据将一并删除。</>,
      confirmText: "删除",
      danger: true,
    });
    if (!ok) return;
    await api.deleteTenant(t.id);
    load();
  };

  return (
    <div className="mx-auto max-w-4xl p-4 sm:p-6">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">租户管理</h1>
          <p className="text-sm text-zinc-500">多租户数据隔离边界（超级管理员）</p>
        </div>
        <button
          onClick={() => setCreateOpen(true)}
          className="flex items-center gap-1.5 rounded-md bg-amber-500 px-3 py-1.5 text-sm font-medium text-black hover:bg-amber-400"
        >
          <Plus className="h-4 w-4" /> 新建租户
        </button>
      </div>

      {error && <div className="mb-4 rounded-md border border-red-800 bg-red-500/10 p-3 text-sm text-red-300">{error}</div>}

      {loading ? (
        <div className="flex justify-center py-16 text-zinc-500"><Loader2 className="h-6 w-6 animate-spin" /></div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-zinc-800">
          <table className="w-full min-w-[640px] text-sm">
            <thead className="bg-zinc-900 text-left text-xs text-zinc-500">
              <tr>
                <th className="px-4 py-2.5">租户</th>
                <th className="px-4 py-2.5">Slug</th>
                <th className="px-4 py-2.5">描述</th>
                <th className="px-4 py-2.5">创建时间</th>
                <th className="px-4 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {tenants.map((t) => (
                <tr key={t.id} className="bg-zinc-950/40">
                  <td className="px-4 py-2.5 font-medium">{t.name}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-zinc-400">{t.slug}</td>
                  <td className="px-4 py-2.5 text-xs text-zinc-500">{t.description || "-"}</td>
                  <td className="px-4 py-2.5 text-xs text-zinc-500">{new Date(t.created_at).toLocaleString()}</td>
                  <td className="px-4 py-2.5 text-right">
                    <button onClick={() => remove(t)} className="text-red-400 hover:text-red-300" title="删除">
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {createOpen && <CreateTenantModal onClose={() => setCreateOpen(false)} onCreated={load} />}
    </div>
  );
}

function CreateTenantModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.createTenant({ name, slug, description });
      onCreated();
      onClose();
    } catch (err: any) {
      setError(err.message || "创建失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose} title="新建租户">
      <div className="mb-3 flex items-center gap-2 text-amber-400">
        <Shield className="h-4 w-4" />
      </div>
      <form onSubmit={submit} className="space-y-4">
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
          <div>
            <label className="mb-1 block text-xs text-zinc-400">描述</label>
            <input value={description} onChange={(e) => setDescription(e.target.value)}
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
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
    </Modal>
  );
}
