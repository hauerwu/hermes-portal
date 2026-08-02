import { useCallback, useEffect, useState } from "react";
import { Loader2, Plus, Trash2, Users as UsersIcon } from "lucide-react";
import { api, type User } from "@/lib/api";

const roleLabel: Record<string, string> = {
  super_admin: "超级管理员",
  tenant_admin: "实例管理员",
  member: "成员",
};

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      setUsers(await api.listUsers());
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const remove = async (u: User) => {
    if (!confirm(`确认删除用户「${u.username}」？`)) return;
    await api.deleteUser(u.id);
    load();
  };

  return (
    <div className="mx-auto max-w-4xl p-6">
      <div className="mb-5 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">用户管理</h1>
          <p className="text-sm text-zinc-500">租户内的用户与角色分配</p>
        </div>
        <button
          onClick={() => setCreateOpen(true)}
          className="flex items-center gap-1.5 rounded-md bg-amber-500 px-3 py-1.5 text-sm font-medium text-zinc-950 hover:bg-amber-400"
        >
          <Plus className="h-4 w-4" /> 新建用户
        </button>
      </div>

      {error && <div className="mb-4 rounded-md border border-red-800 bg-red-500/10 p-3 text-sm text-red-300">{error}</div>}

      {loading ? (
        <div className="flex justify-center py-16 text-zinc-500"><Loader2 className="h-6 w-6 animate-spin" /></div>
      ) : users.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-zinc-800 py-16 text-zinc-500">
          <UsersIcon className="mb-2 h-8 w-8" /> 暂无用户
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-zinc-800">
          <table className="w-full text-sm">
            <thead className="bg-zinc-900 text-left text-xs text-zinc-500">
              <tr>
                <th className="px-4 py-2.5">用户名</th>
                <th className="px-4 py-2.5">邮箱</th>
                <th className="px-4 py-2.5">角色</th>
                <th className="px-4 py-2.5">状态</th>
                <th className="px-4 py-2.5">最近登录</th>
                <th className="px-4 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {users.map((u) => (
                <tr key={u.id} className="bg-zinc-950/40">
                  <td className="px-4 py-2.5 font-medium">{u.username}</td>
                  <td className="px-4 py-2.5 text-xs text-zinc-400">{u.email || "-"}</td>
                  <td className="px-4 py-2.5">
                    <span className="rounded-full bg-zinc-800 px-2 py-0.5 text-[11px] text-zinc-300">{roleLabel[u.role] || u.role}</span>
                  </td>
                  <td className="px-4 py-2.5">
                    <span className={`rounded-full px-2 py-0.5 text-[11px] ${u.active ? "bg-emerald-500/15 text-emerald-300" : "bg-zinc-700/40 text-zinc-500"}`}>
                      {u.active ? "启用" : "停用"}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-zinc-500">{u.last_login ? new Date(u.last_login).toLocaleString() : "-"}</td>
                  <td className="px-4 py-2.5 text-right">
                    <button onClick={() => remove(u)} className="text-red-400 hover:text-red-300" title="删除">
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {createOpen && <CreateUserModal onClose={() => setCreateOpen(false)} onCreated={load} />}
    </div>
  );
}

function CreateUserModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.createUser({ username, password, email, role: role as User["role"] });
      onCreated();
      onClose();
    } catch (err: any) {
      setError(err.message || "创建失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div className="w-full max-w-md rounded-xl border border-zinc-700 bg-zinc-900 p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-4 text-lg font-semibold">新建用户</h2>
        <form onSubmit={submit} className="space-y-4">
          <div>
            <label className="mb-1 block text-xs text-zinc-400">用户名</label>
            <input value={username} onChange={(e) => setUsername(e.target.value)} required
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
          </div>
          <div>
            <label className="mb-1 block text-xs text-zinc-400">密码（SSO 用户可留空）</label>
            <input value={password} onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
          </div>
          <div>
            <label className="mb-1 block text-xs text-zinc-400">邮箱</label>
            <input value={email} onChange={(e) => setEmail(e.target.value)}
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500" />
          </div>
          <div>
            <label className="mb-1 block text-xs text-zinc-400">角色</label>
            <select value={role} onChange={(e) => setRole(e.target.value)}
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500">
              <option value="member">成员（只读）</option>
              <option value="tenant_admin">实例管理员</option>
            </select>
          </div>
          {error && <div className="text-sm text-red-400">{error}</div>}
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="rounded-md border border-zinc-700 px-4 py-1.5 text-sm hover:bg-zinc-800">取消</button>
            <button type="submit" disabled={busy}
              className="flex items-center gap-1.5 rounded-md bg-amber-500 px-4 py-1.5 text-sm font-medium text-zinc-950 hover:bg-amber-400 disabled:opacity-50">
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />} 创建
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
