import { useEffect, useState } from "react";
import { Navigate, useLocation } from "react-router";
import { api, setTokens } from "@/lib/api";
import { useAuth } from "@/lib/auth";

/** OIDC callback returns tokens in the URL fragment (#/auth/sso?...). */
export default function SSOCallbackPage() {
  const location = useLocation();
  const { refresh } = useAuth();
  const [done, setDone] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const access = params.get("access_token");
    const refreshToken = params.get("refresh_token");
    if (!access || !refreshToken) {
      setError("SSO 回调缺少令牌");
      return;
    }
    (async () => {
      try {
        setTokens(access, refreshToken);
        await api.ssoExchange(access, refreshToken); // sets the HttpOnly portal cookie
        await refresh();
        setDone(true);
      } catch (e: any) {
        setError(e.message || "SSO 登录失败");
      }
    })();
  }, [location, refresh]);

  if (done) return <Navigate to="/instances" replace />;
  if (error) return <div className="flex h-full items-center justify-center text-red-400">{error}</div>;
  return <div className="flex h-full items-center justify-center text-zinc-400">SSO 登录中…</div>;
}
