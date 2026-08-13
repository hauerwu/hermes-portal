import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from "react-router";
import {
  Boxes,
  Cpu,
  History,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Menu,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Shield,
  Sun,
  Users,
  X,
} from "lucide-react";
import { AuthProvider, useAuth } from "@/lib/auth";
import { ConfirmProvider } from "@/lib/confirm";
import { getTheme, toggleTheme, type Theme } from "@/lib/theme";
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
  const location = useLocation();
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem(SIDEBAR_KEY) === "1");
  // 移动端抽屉（< md）：顶栏汉堡按钮打开，路由变化 / ESC / 点遮罩关闭
  const [mobileOpen, setMobileOpen] = useState(false);
  const [theme, setTheme] = useState<Theme>(getTheme);

  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname]);

  // 抽屉打开时锁定 body 滚动
  useEffect(() => {
    if (!mobileOpen) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [mobileOpen]);

  // ESC 关闭抽屉
  useEffect(() => {
    if (!mobileOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMobileOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [mobileOpen]);

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

  // 桌面收起态只影响 md+ 的静态侧栏；移动端抽屉始终展示完整菜单
  const iconOnly = collapsed && !mobileOpen;

  return (
    <div className="flex h-full">
      {/* ── 移动端顶栏（< md）── */}
      <header className="fixed inset-x-0 top-0 z-40 flex h-12 shrink-0 items-center gap-2 border-b border-zinc-800 bg-zinc-900/90 px-3 backdrop-blur md:hidden">
        <button
          onClick={() => setMobileOpen(true)}
          aria-label="打开菜单"
          className="rounded-md p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200"
        >
          <Menu className="h-5 w-5" />
        </button>
        <LayoutDashboard className="h-5 w-5 shrink-0 text-amber-400" />
        <span className="truncate font-semibold">Hermes Portal</span>
        <div className="flex-1" />
        <button
          onClick={() => setTheme(toggleTheme())}
          className="rounded-md p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200"
          title={theme === "dark" ? "切换为浅色风格" : "切换为深色风格"}
        >
          {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </button>
      </header>

      {/* ── 移动端抽屉遮罩 ── */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/60 animate-fade-in md:hidden"
          onClick={() => setMobileOpen(false)}
        />
      )}

      {/* ── 侧栏：移动端 = 滑出抽屉；桌面端 = 静态可收起侧栏 ── */}
      <aside
        className={`fixed inset-y-0 left-0 z-50 flex w-64 transform flex-col border-r border-zinc-800 bg-zinc-900/95 backdrop-blur transition-transform duration-200 ease-in-out md:static md:z-auto md:translate-x-0 md:bg-zinc-900/60 md:backdrop-blur-none md:transition-[width] ${
          mobileOpen ? "translate-x-0" : "-translate-x-full"
        } ${collapsed ? "md:w-14" : "md:w-56"}`}
      >
        {/* ── 顶栏：Logo + 收放/关闭按钮 ── */}
        <div
          className={`flex items-center border-b border-zinc-800 py-3.5 ${
            iconOnly ? "md:justify-center md:px-0" : "justify-between px-3"
          }`}
        >
          {iconOnly ? (
            // 桌面收起态侧栏仅 56px 宽：只保留展开按钮并居中，避免按钮溢出面板
            <button
              onClick={toggleSidebar}
              className="hidden rounded-md p-1.5 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200 md:block"
              title="展开菜单"
            >
              <PanelLeftOpen className="h-4 w-4" />
            </button>
          ) : (
            <>
              <div className="flex min-w-0 items-center gap-2">
                <LayoutDashboard className="h-5 w-5 shrink-0 text-amber-400" />
                <div className="min-w-0">
                  <div className="truncate font-semibold leading-tight">Hermes Portal</div>
                  <div className="truncate text-[11px] text-zinc-500">{user.username}</div>
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <button
                  onClick={() => setTheme(toggleTheme())}
                  className="hidden rounded-md p-1.5 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200 md:block"
                  title={theme === "dark" ? "切换为浅色风格" : "切换为深色风格"}
                >
                  {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
                </button>
                <button
                  onClick={toggleSidebar}
                  className="hidden rounded-md p-1.5 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200 md:block"
                  title="收起菜单"
                >
                  <PanelLeftClose className="h-4 w-4" />
                </button>
                <button
                  onClick={() => setMobileOpen(false)}
                  className="rounded-md p-1.5 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200 md:hidden"
                  title="关闭菜单"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            </>
          )}
        </div>

        {/* ── 导航菜单 ── */}
        <nav className="flex-1 space-y-1 overflow-y-auto px-2 py-3">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              title={item.label}
              className={({ isActive }) =>
                `flex items-center gap-2.5 rounded-md py-2 text-sm transition-colors ${
                  iconOnly ? "justify-center px-0" : "px-3"
                } ${
                  isActive
                    ? "bg-amber-500/15 text-amber-300"
                    : "text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200"
                }`
              }
            >
              <item.icon className="h-4 w-4 shrink-0" />
              {!iconOnly && <span className="truncate">{item.label}</span>}
            </NavLink>
          ))}
        </nav>

        {/* ── 底部：角色 + 退出 ── */}
        <div className="space-y-2 border-t border-zinc-800 px-3 py-3">
          {!iconOnly && (
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
              iconOnly ? "justify-center px-0" : "px-3"
            }`}
          >
            <LogOut className="h-4 w-4 shrink-0" />
            {!iconOnly && <span>退出登录</span>}
          </button>
        </div>
      </aside>

      <main className="flex-1 overflow-auto pt-12 md:pt-0">
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
