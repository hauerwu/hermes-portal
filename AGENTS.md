# AGENTS.md — Hermes Portal

Hermes Agent 管理门户：统一管理多个 hermes-agent 实例（本机 Docker 容器 / 远程 URL 纳管），内嵌 hermes dashboard 全部 UI 交互，并通过统一网关对外暴露 OpenAI API 与 channel webhook 回调地址。

**铁律：对 `hermes-agent` 工程零侵入**——绝不修改其代码。所有集成都通过其原生能力完成（Docker 镜像、`/auth/password-login`、`X-Forwarded-Prefix`、`POST /api/model/set`、`/api/config` 等）。hermes-agent 是**只读参照物**（位于 `../hermes-agent`，其内部实现细节是理解集成协议的来源）。

---

## 技术栈

| 层 | 技术 | 关键约定 |
|---|---|---|
| 后端 | Go 1.23 + Gin + GORM + SQLite（`glebarez/sqlite` 纯 Go 驱动，无 CGO） | `go build -buildvcs=false`（本仓库非 VCS 根） |
| 前端 | React 19 + TypeScript + Vite 8 + Tailwind 4（`@tailwindcss/vite`）+ react-router 8（HashRouter） | 依赖版本对齐 hermes-agent `web/package.json` |
| 认证 | golang-jwt/v5（HS256）、密码 scrypt（与 hermes `dashboard_auth/basic` 同格式 `scrypt$n$r$p$salt$dk`） | |
| SSO | coreos/go-oidc + oauth2，**配置存 SQLite 可由页面修改**（环境变量仅作初始默认） | |
| 运行时 | 全部 Docker 容器化；portal 挂载 `/var/run/docker.sock` 管理本机实例 | |

## 目录结构

```
backend/
  cmd/portal/main.go                 入口：加载配置 → 开库 → 装配路由 → 启动
  internal/
    api/                             Gin 处理器
      handlers.go                    认证/租户/用户/实例/API Key CRUD + publicXxx 序列化
      oidc.go                        OIDC 回调、admin claim 映射、token 签发
      oidc_settings.go               OIDC 页面配置（portal_settings 表 + 热更新）
      models.go                      模型库 CRUD + 端点测试（testOpenAIEndpoint）
      audit.go                       审计日志查询
    services/
      instance_service.go            实例生命周期核心（Create/waitReady/configureDefaultModel）
      docker_client.go               薄 Docker Engine API 客户端（unix socket 直连，无 SDK）
      session_cache.go               实例 dashboard 会话托管（/auth/password-login 引导 + cookie 注入）
    proxy/
      proxy.go                       内嵌 dashboard 反向代理 + 统一网关（OpenAI/webhook）
    router/router.go                 全部路由装配 + SPA 托管（history fallback）
    middleware/                      Bearer/cookie 认证 + RBAC（super_admin/tenant_admin/member）
    security/                        scrypt / JWT / API Key（hp_ 前缀，SHA-256 哈希存储）
    database/                        GORM 开库 + AutoMigrate + 种子（默认租户 + 超级管理员）
frontend/
  src/
    lib/api.ts                       类型化 API 客户端（JWT 自动刷新、HttpOnly cookie）
    lib/confirm.tsx                  useConfirm() 统一确认弹窗（禁 window.confirm）
    lib/auth.tsx                     AuthProvider
    lib/theme.ts                     主题（dark/light，data-theme + localStorage）
    components/Modal.tsx             通用模态（ESC/× 关闭、滚动锁定、焦点管理）
    components/ApiDoc.tsx            OpenAI API 调用文档（API Keys 页）
    pages/                           登录/实例/实例详情/Dashboard 全屏视图/模型配置/API Keys/用户/租户/审计/设置
```

## 常用命令

```bash
cd backend && go build -buildvcs=false ./...     # 编译
cd backend && go test -buildvcs=false ./...      # 全部测试（单元 + 集成）
cd frontend && npm run build                     # 前端产物 → frontend/dist
docker build --build-arg BASE_REGISTRY=docker.m.daocloud.io/library -t hermes-portal .   # 构建镜像（本环境 docker.io 不可直连，须用镜像源参数）
docker compose up -d / down                      # 部署 / 停止
```

部署流程（改代码后）：`npm run build` → `docker build` → `docker compose down && docker compose up -d`。后端改动需重新构建镜像；纯前端改动同样（SPA 打进镜像）。

## 核心架构与关键机制（改动前必读）

### 1. 内嵌 Dashboard（proxy.go + session_cache.go）
- 浏览器访问 `/instances/{id}/dashboard/` → portal 反向代理到实例容器 `http://hermes-inst-{id}:9119/`。
- **会话托管**：portal 用实例生成的 basic-auth 凭据调用 hermes 原生 `POST /auth/password-login`，捕获 session cookie 注入每个代理请求（HTTP + WebSocket）；`Set-Cookie` 被 portal 消费、不落到浏览器（多实例同源不冲突，cookie 按前缀路径隔离）。
- **X-Forwarded-Prefix**：必须注入 `/instances/{id}/dashboard`，hermes SPA 据此生成 `__HERMES_BASE_PATH__`（这是零侵入内嵌的关键）。
- **必须剥离** `Authorization` 头再转发（否则实例按自身 bearer 校验会拒绝）。
- 401 时 `retryRoundTripper` 自动重新登录重试一次；`ModifyResponse` 改写 Location 保持前缀内跳转。
- WS（/api/ws、/api/pty 等）走 `httputil.ReverseProxy` 原生 upgrade；gated 模式 SPA 用单次 ticket（`POST /api/auth/ws-ticket`）握手。
- 每次代理请求都会 `CookieHeader()` 拉取缓存会话；实例重启后自动重新引导。

### 2. 实例生命周期（instance_service.go + docker_client.go）
- 一个本地实例 = **一个容器**：`gateway run`（s6 托管）+ `HERMES_DASHBOARD=1` 让 dashboard 作为 s6 服务同容器运行。
- 容器环境（containerEnv）：`API_SERVER_HOST=0.0.0.0` + `API_SERVER_KEY`（≥16 字符，否则 hermes 不启用 api_server）、`HERMES_DASHBOARD_BASIC_AUTH_*`、`HERMES_UID/GID`。
- **waitReady goroutine**：创建后每 3s 探测 `/api/health` 直至 running（180s 超时），随后自动 `configureDefaultModel`（调用实例 `POST /api/model/set` 写 config.yaml）。
- 数据卷 `hermes-inst-{id}-data` → `/opt/data`；销毁时删容器+卷并**释放 slug**（`slug-del-{id}`，否则同名无法重建——unique 约束）。
- 编辑实例（PUT）会**重建容器**（保留数据卷）并重新 waitReady + 配置模型。
- 远程实例（mode=remote）仅纳管：URL + 健康检查 + 代理，无生命周期操作。

### 3. 统一网关（proxy.go）
- `POST /api/v1/gateway/{slug}/openapi/v1/...` → 实例 `:8642` 的 OpenAI 兼容 API。调用方用 portal API Key（`X-API-Key` 或 Bearer），portal 校验后换成实例私有 `API_SERVER_KEY` 转发。
- `POST /api/v1/gateway/{slug}/webhook/{channel}/...` → 实例 webhook 服务器（端口映射：whatsapp 8090、generic 8644、bluebubbles 8645、msgraph 8646）。
- 流式（SSE）必须 `FlushInterval=-1` 立即 flush。

### 4. 默认模型配置（重要：内置 provider 只认环境变量）
- 模型库条目（url/model/key/provider）→ 实例 `default_model` 快照 → `POST /api/model/set` 写实例 config.yaml 的 `model.*`。
- **hermes 内置 provider（deepseek/openai/anthropic/…）的凭证只从环境变量读取**（`hermes_cli/auth.py` 的 `api_key_env_vars`，如 `DEEPSEEK_API_KEY`）。因此 `applyProviderEnv` 必须把 key 注入实例容器 env（映射表 `providerEnvVars`）。`custom` provider 走 config.yaml 的 `model.api_key`（模型名/URL 原样保留），**不注入 env**。
- 新增 provider 时记得补充 `providerEnvVars` 映射。

### 5. 多租户与 RBAC（middleware + deps）
- `super_admin`（无租户，管全局）→ `tenant_admin`（绑租户，管本租户实例/用户/Key）→ `member`（只读 + 可用网关）。
- 后端**强制租户隔离**（每个列表/详情/操作都校验 `tenant_id`），前端仅展示。
- 新建 API 必须考虑：租户过滤、`require_tenant_admin`/`RequireRole` 守卫、审计 `database.Audit`、`publicXxx` 序列化脱敏。

### 6. 安全约定（不可破坏）
- API Key 明文只展示一次（创建响应），库中只存 SHA-256 哈希 + 前缀。
- `publicInstance`/`publicAPIKey`/模型库响应必须脱敏（不返回 `api_server_key`、dashboard 密码、模型 key）。
- 密码 scrypt；JWT access(1h) + refresh(30d)；HttpOnly cookie `portal_session`（iframe 会话需要）。
- dashboard 会话 cookie 在服务端缓存（内存），不进浏览器。

### 7. 主题（前端）
- 组件全部用 `zinc-*`/`amber-*` 等语义色板；亮/暗主题通过 `[data-theme="light"]` 覆盖 **CSS 变量**实现，**不要硬编码颜色**。
- 主操作按钮文字用 `text-black`（不要 `text-zinc-950`，浅色主题下会变白字）。
- 日志/代码块用 `neutral-*`（固定深色，不随主题）。

### 8. 前端交互约定
- 弹窗一律用 `components/Modal.tsx`（ESC/× 关闭，**点击外部不关闭**）。
- 确认一律用 `useConfirm()`（禁 `window.confirm`/`alert`）。
- 新增页面需在 `App.tsx` 注册路由 + 侧边栏菜单。

## 测试

- `backend/internal/security`：scrypt/JWT/API Key 单元测试。
- `backend/internal/proxy`：slugify/容器命名/webhook 端口映射。
- `backend/internal/integration`：用 httptest 全栈测试（登录、RBAC、租户隔离、审计、网关 API Key 鉴权、dashboard 代理认证）。新增 API 尽量补这里的用例。
- 运行：`cd backend && go test -buildvcs=false ./...`

## 常见陷阱速查

| 现象 | 原因/处理 |
|---|---|
| `instance slug 'xxx' already exists` | 旧实例已销毁但 slug 被占用（destroyed 行 + unique 约束）。销毁路径会自动 `slug-del-{id}`；手工改库需同样处理 |
| 实例对话报 `No usable credentials found for provider 'xxx'` | 内置 provider 需 env 注入：检查 `providerEnvVars` 映射 + 容器 env |
| 实例状态一直 starting | 等 waitReady 自动就绪（≤180s）；容器内 dashboard 未起来时检查 hermes 日志 |
| dashboard 内嵌 401/302 | 代理未剥离 `Authorization`、会话缓存失效（实例重启后自动重引导）、或 `X-Forwarded-Prefix` 缺失 |
| 浅色主题下按钮文字不可见 | 按钮文字用了 `text-zinc-950`，应改 `text-black` |
| Docker 镜像构建拉不到 base | 本环境须 `--build-arg BASE_REGISTRY=docker.m.daocloud.io/library` |
| `go build` 报 VCS 错误 | 用 `-buildvcs=false` |

## 部署形态

```bash
docker compose up -d   # portal 容器 + portal-data 卷 + /var/run/docker.sock + hermes-portal-net
```
- 本机实例容器由 portal 通过 docker.sock 动态创建，加入 `hermes-portal-net`（portal 按容器名访问实例 :9119/:8642）。
- `.env`（来自 `.env.example`）：必须设置 `PORTAL_ADMIN_PASSWORD`、`PORTAL_JWT_SECRET`；`PORTAL_HERMES_IMAGE` 默认 `hermes-agent`。
- 数据：SQLite 在 `PORTAL_DATA_DIR`（默认 `/app/data`）挂 `portal-data` 卷。
