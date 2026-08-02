import { useState } from "react";
import { NavLink, Navigate, Route, Routes, useNavigate } from "react-router";
import {
  Boxes,
  Cpu,
  History,
  KeyRound,
  LayoutDashboard,
  LogOut,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Shield,
  Users,
} from "lucide-react";
import { AuthProvider, useAuth } from "@/lib/auth";
import { ConfirmProvider } from "@/lib/confirm";
import LoginPage from "@/pages/LoginPage";
import SSOCallbackPage from "@/pages/SSOCallbackPage";
import InstancesPage from "@/pages/InstancesPage";
import InstanceDetailPage from "@/pages/InstanceDetailPage";
import DashboardPage from "@/pages/DashboardPage";
import ApiKeysPage from "@/pages/ApiKeysPage";
import UsersPage from "@/pages/UsersPage";
import TenantsPage from "@/pages/TenantsPage";
import AuditLogsPage from "@/pages/AuditLogsPage";
import ModelConfigsPage from "@/pages/ModelConfigsPage";
import SettingsPage from "@/pages/SettingsPage";

const SIDEBAR_KEY = "portal.sidebar.collapsed";

function Shell() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem(SIDEBAR_KEY) === "1");

  if (!user) return <Navigate to="/auth/login" replace />;

  const toggleSidebar = () => {
    setCollapsed((v) => {
      localStorage.setItem(SIDEBAR_KEY, v ? "0" : "1");
      return !v;
    });
  };

  const nav = [
    { to: "/instances", label: "实例管理", icon: Boxes },
    { to: "/models", label: "模型配置", icon: Cpu },
    { to: "/apikeys", label: "API Keys", icon: KeyRound },
    ...(user.role === "super_admin" || user.role === "tenant_admin"
      ? [{ to: "/users", label: "用户管理", icon: Users }]
      : []),
    ...(user.role === "super_admin" ? [{ to: "/tenants", label: "租户管理", icon: Shield }] : []),
    { to: "/audit", label: "审计日志", icon: History },
    { to: "/settings", label: "设置", icon: Settings },
  ];

  const roleLabel =
    user.role === "super_admin" ? "超级管理员" : user.role === "tenant_admin" ? "实例管理员" : "成员";

  return (
    <div className="flex h-full">
      <aside
        className={`${
          collapsed ? "w-14" : "w-56"
        } flex shrink-0 flex-col border-r border-zinc-800 bg-zinc-900/60 transition-[width] duration-200 ease-in-out`}
      >
        {/* ── 顶栏：Logo + 收放按钮 ── */}
        <div className="flex items-center justify-between border-b border-zinc-800 px-3 py-3.5">
          <div className="flex min-w-0 items-center gap-2">
            <LayoutDashboard className="h-5 w-5 shrink-0 text-amber-400" />
            {!collapsed && (
              <div className="min-w-0">
                <div className="truncate font-semibold leading-tight">Hermes Portal</div>
                <div className="truncate text-[11px] text-zinc-500">{user.username}</div>
              </div>
            )}
          </div>
          <button
            onClick={toggleSidebar}
            className="shrink-0 rounded-md p-1.5 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200"
            title={collapsed ? "展开菜单" : "收起菜单"}
          >
            {collapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
          </button>
        </div>

        {/* ── 导航菜单 ── */}
        <nav className="flex-1 space-y-1 px-2 py-3">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              title={item.label}
              className={({ isActive }) =>
                `flex items-center gap-2.5 rounded-md py-2 text-sm transition-colors ${
                  collapsed ? "justify-center px-0" : "px-3"
                } ${
                  isActive
                    ? "bg-amber-500/15 text-amber-300"
                    : "text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200"
                }`
              }
            >
              <item.icon className="h-4 w-4 shrink-0" />
              {!collapsed && <span className="truncate">{item.label}</span>}
            </NavLink>
          ))}
        </nav>

        {/* ── 底部：角色 + 退出 ── */}
        <div className="space-y-2 border-t border-zinc-800 px-3 py-3">
          {!collapsed && (
            <div className="px-1 text-[11px] leading-relaxed text-zinc-500">
              角色：{roleLabel}
              {user.role !== "super_admin" && user.tenant_id != null && <div>租户 #{user.tenant_id}</div>}
            </div>
          )}
          <button
            onClick={async () => {
              await logout();
              navigate("/auth/login");
            }}
            title="退出登录"
            className={`flex w-full items-center gap-2 rounded-md py-1.5 text-sm text-zinc-400 hover:bg-zinc-800 hover:text-red-300 ${
              collapsed ? "justify-center px-0" : "px-3"
            }`}
          >
            <LogOut className="h-4 w-4 shrink-0" />
            {!collapsed && <span>退出登录</span>}
          </button>
        </div>
      </aside>

      <main className="flex-1 overflow-auto">
        <Routes>
          <Route path="/instances" element={<InstancesPage />} />
          <Route path="/instances/:id" element={<InstanceDetailPage />} />
          <Route path="/instances/:id/dashboard" element={<DashboardPage />} />
          <Route path="/models" element={<ModelConfigsPage />} />
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
    <ConfirmProvider>
      <AuthProvider>
      <Routes>
        <Route path="/auth/login" element={<LoginPage />} />
        <Route path="/auth/sso" element={<SSOCallbackPage />} />
        <Route path="/*" element={<Shell />} />
      </Routes>
      </AuthProvider>
    </ConfirmProvider>
  );
}
