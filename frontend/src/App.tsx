import { NavLink, Navigate, Route, Routes, useNavigate } from "react-router";
import {
  Boxes,
  GitBranch,
  History,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Settings,
  Shield,
  Users,
} from "lucide-react";
import { AuthProvider, useAuth } from "@/lib/auth";
import LoginPage from "@/pages/LoginPage";
import SSOCallbackPage from "@/pages/SSOCallbackPage";
import InstancesPage from "@/pages/InstancesPage";
import InstanceDetailPage from "@/pages/InstanceDetailPage";
import ApiKeysPage from "@/pages/ApiKeysPage";
import UsersPage from "@/pages/UsersPage";
import TenantsPage from "@/pages/TenantsPage";
import AuditLogsPage from "@/pages/AuditLogsPage";
import SettingsPage from "@/pages/SettingsPage";

function Shell() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  if (!user) return <Navigate to="/auth/login" replace />;

  const nav = [
    { to: "/instances", label: "实例管理", icon: Boxes },
    { to: "/apikeys", label: "API Keys", icon: KeyRound },
    ...(user.role === "super_admin" || user.role === "tenant_admin"
      ? [{ to: "/users", label: "用户管理", icon: Users }]
      : []),
    ...(user.role === "super_admin" ? [{ to: "/tenants", label: "租户管理", icon: Shield }] : []),
    { to: "/audit", label: "审计日志", icon: History },
    { to: "/settings", label: "设置", icon: Settings },
  ];

  return (
    <div className="flex h-full">
      <aside className="w-56 shrink-0 border-r border-zinc-800 bg-zinc-900/60 flex flex-col">
        <div className="flex items-center gap-2 px-4 py-4 border-b border-zinc-800">
          <LayoutDashboard className="h-5 w-5 text-amber-400" />
          <div>
            <div className="font-semibold leading-tight">Hermes Portal</div>
            <div className="text-[11px] text-zinc-500">{user.username}</div>
          </div>
        </div>
        <nav className="flex-1 px-2 py-3 space-y-1">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                `flex items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors ${
                  isActive
                    ? "bg-amber-500/15 text-amber-300"
                    : "text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200"
                }`
              }
            >
              <item.icon className="h-4 w-4" />
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="border-t border-zinc-800 px-4 py-3 space-y-2">
          <div className="text-[11px] text-zinc-500">
            角色：{user.role === "super_admin" ? "超级管理员" : user.role === "tenant_admin" ? "实例管理员" : "成员"}
            {user.role !== "super_admin" && user.tenant_id != null && (
              <div>租户 #{user.tenant_id}</div>
            )}
          </div>
          <button
            onClick={async () => {
              await logout();
              navigate("/auth/login");
            }}
            className="flex w-full items-center gap-2 rounded-md px-3 py-1.5 text-sm text-zinc-400 hover:bg-zinc-800 hover:text-red-300"
          >
            <LogOut className="h-4 w-4" /> 退出登录
          </button>
        </div>
      </aside>
      <main className="flex-1 overflow-auto">
        <Routes>
          <Route path="/instances" element={<InstancesPage />} />
          <Route path="/instances/:id" element={<InstanceDetailPage />} />
          <Route path="/apikeys" element={<ApiKeysPage />} />
          <Route path="/users" element={<UsersPage />} />
          <Route path="/tenants" element={<TenantsPage />} />
          <Route path="/audit" element={<AuditLogsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/instances" replace />} />
        </Routes>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/auth/login" element={<LoginPage />} />
        <Route path="/auth/sso" element={<SSOCallbackPage />} />
        <Route path="/*" element={<Shell />} />
      </Routes>
    </AuthProvider>
  );
}
