import { useState } from "react";
import { useNavigate } from "react-router";
import { KeyRound, Loader2 } from "lucide-react";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

export default function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [oidc, setOidc] = useState<{ enabled: boolean; issuer: string } | null>(null);

  useState(() => {
    api.oidcStatus().then(setOidc).catch(() => setOidc({ enabled: false, issuer: "" }));
  });

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await login(username, password);
      navigate("/instances");
    } catch (err: any) {
      setError(err.message || "登录失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex h-full items-center justify-center bg-zinc-950">
      <div className="w-full max-w-sm rounded-2xl border border-zinc-800 bg-zinc-900 p-8 shadow-2xl">
        <div className="mb-6 flex items-center gap-2">
          <KeyRound className="h-6 w-6 text-amber-400" />
          <h1 className="text-xl font-semibold">Hermes Portal</h1>
        </div>
        <form onSubmit={submit} className="space-y-4">
          <div>
            <label className="mb-1 block text-sm text-zinc-400">用户名</label>
            <input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500"
              autoComplete="username"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm text-zinc-400">密码</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-amber-500"
              autoComplete="current-password"
            />
          </div>
          {error && <div className="text-sm text-red-400">{error}</div>}
          <button
            type="submit"
            disabled={busy}
            className="flex w-full items-center justify-center gap-2 rounded-md bg-amber-500 py-2 text-sm font-medium text-black hover:bg-amber-400 disabled:opacity-50"
          >
            {busy && <Loader2 className="h-4 w-4 animate-spin" />}
            登录
          </button>
        </form>
        {oidc?.enabled && (
          <a
            href="/api/auth/oidc/authorize"
            className="mt-4 block rounded-md border border-zinc-700 py-2 text-center text-sm text-zinc-300 hover:bg-zinc-800"
          >
            使用 SSO 单点登录（OIDC）
          </a>
        )}
      </div>
    </div>
  );
}
