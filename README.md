# Hermes Portal ☤

Hermes Agent 管理门户：统一管理**多个 hermes-agent 实例**（本机 Docker 容器 / 远程 URL 纳管），内嵌 **hermes dashboard 全部 UI 交互**，并通过**统一网关**对外暴露 hermes-agent 的 OpenAI API 与 channel webhook 回调地址。

**零侵入**：不修改 hermes-agent 工程任何代码，完全复用其官方 Docker 镜像与原生协议。

---

## 功能总览

| 能力 | 说明 |
|---|---|
| 实例管理 | 创建 / 修改 / 启动 / 停止 / 重启 / 销毁（修改：名称、slug、镜像、内存限制、额外环境变量、远程 URL；本机实例修改后自动重建容器，数据卷保留） |
| 本机实例 | 每个实例 = 一个独立 hermes-agent 容器（portal 通过 Docker Socket 管理） |
| 远程纳管 | 通过远程 URL 纳管（dashboard 地址 + OpenAI API 地址），健康检查 + 统一代理 |
| 用户认证 | 用户名/密码（scrypt，与 hermes 同款算法）+ JWT 会话 |
| OIDC SSO | 标准授权码流程，兼容 Keycloak / Dex / Okta / Entra ID / Auth0 |
| 多租户 | 租户级数据隔离；超级管理员管所有实例，实例管理员只管本租户实例 |
| 审计日志 | 全量管理操作留痕，按动作/目标/操作者筛选 + 分页，租户隔离可见 |
| 内嵌 Dashboard | hermes dashboard 全部功能（聊天、配置、渠道、会话、定时任务、技能、MCP、文件、日志…）在 portal 内嵌使用 |
| 统一网关 | ① OpenAI API：`X-API-Key` 鉴权，portal 页内管理 Key；② channel webhook 回调固定 URL |
| 模型配置 | 模型库：维护多个推理端点（URL/模型名/Key），设置默认模型，创建实例时选择使用 |

## 技术栈（与 hermes-agent 对齐 + 工程约定）

| 层 | 技术 |
|---|---|
| 后端 | **Go 1.23** + Gin + GORM + SQLite（纯 Go 驱动 `glebarez/sqlite`，与 hermes 一致的 SQLite 持久化） |
| 前端 | **React 19** + TypeScript + Vite + Tailwind 4（与 hermes `web/` SPA 相同栈，依赖版本对齐） |
| 会话 | golang-jwt/v5，密码 scrypt（与 hermes `dashboard_auth/basic` 同算法同格式） |
| OIDC | coreos/go-oidc + oauth2 |
| 运行时 | 全部容器化（portal + 本机 hermes 实例均为 Docker 容器） |

---

## 架构

```
                        ┌──────────────────────────────────────────────┐
                        │                 Hermes Portal                │
                        │  ┌──────────┐   ┌───────────┐   ┌─────────┐  │
  浏览器 ────────────────►│  React SPA│   │ Gin API   │   │ 网关代理 │  │
  (portal 登录)          │  (内嵌)    │   │  RBAC/租户 │   │ openapi │  │
                        │  └────┬─────┘   │  +OIDC    │   │ webhook │  │
                        │       │         └─────┬─────┘   └────┬────┘  │
                        │       └───────────────┼──────────────┤       │
                        │      /instances/{id}/dashboard/       │       │
                        └───────┬───────────────┼──────────────┼───────┘
                         docker │ sock          │ 容器名:9119   │ 容器名:8642
                        ┌───────▼───────────────▼──────────────▼───────┐
                        │          Docker 网络 hermes-portal-net        │
                        │  hermes-inst-1    hermes-inst-2    …          │
                        │  (gateway+dashboard 单容器, s6 托管)           │
                        └──────────────────────────────────────────────┘
```

**内嵌 Dashboard 原理（零侵入关键）**：

1. 每个本地实例 = 一个容器，同时跑 `gateway run`（s6 托管）与 dashboard 服务（`HERMES_DASHBOARD=1`）。
2. dashboard 以非回环地址绑定 → hermes 原生 auth gate 生效（`dashboard.basic_auth`，凭证由 portal 生成）。
3. portal 反向代理 `/instances/{id}/dashboard/*`，注入 `X-Forwarded-Prefix` —— hermes SPA 原生读取 `__HERMES_BASE_PATH__`，**无需重建前端**即可工作在任何前缀下。
4. portal 持有实例会话：用生成的凭证调用 hermes 原生 `POST /auth/password-login`，捕获 session cookie 注入每个代理请求（HTTP + WebSocket），并透传 token 刷新；实例的 `Set-Cookie` 不会泄漏到浏览器（多实例同源互不冲突，cookie 按前缀路径隔离）。

**统一网关**：

```
/api/v1/gateway/{slug}/openapi/v1/*        → 实例 api_server（容器内 :8642）
/api/v1/gateway/{slug}/webhook/{channel}/* → 实例 channel webhook 服务器
```

- OpenAI API 用 portal 下发的 `hp_` API Key（`X-API-Key` / `Authorization: Bearer`）鉴权，portal 校验租户/实例作用域后，用实例私有的 `API_SERVER_KEY` 转发。
- webhook 回调由外部平台（Meta/微信等）调用，实例侧用自身 HMAC/verify token 校验，与 hermes 原生 webhook 模型一致。

---

## 快速开始

### 1. 准备 hermes-agent 镜像

```bash
# 方式 A：从 hermes-agent 源码构建（未做任何修改）
docker build -t hermes-agent ../hermes-agent

# 方式 B：使用已有镜像（在 .env 里配置 PORTAL_HERMES_IMAGE）
docker tag nousresearch/hermes-agent hermes-agent
```

### 2. 启动 portal

```bash
cd hermes-portal
cp .env.example .env            # 修改 PORTAL_ADMIN_PASSWORD / PORTAL_JWT_SECRET 等
docker compose up -d --build
```

首次启动自动创建默认租户 + 超级管理员（来自 `PORTAL_ADMIN_USERNAME/PASSWORD`）。

访问 `http://localhost:8080` → 登录 → **实例管理** → 新建实例。

### 3. 创建第一个实例

- **本机 Docker**：填名称，镜像默认 `hermes-agent`。portal 自动创建容器 `hermes-inst-<id>`、数据卷、接入 `hermes-portal-net` 网络。
- **远程纳管**：填远程 URL（dashboard 地址）+ OpenAI API 地址（可选，默认 `URL/v1`）。

创建后点进实例详情即可看到**内嵌的完整 hermes dashboard**。

### 5. 模型配置（模型库）

「模型配置」菜单维护租户的推理端点库：显示名称、Provider（custom/OpenAI/OpenRouter/…）、
端点 URL、模型名、API Key，可设置**默认模型**（创建实例时自动预选）。

创建实例时在「使用模型库配置」下拉中选择一个模型，portal 会将其快照到实例并自动写入
实例的 `config.yaml`（`model.provider/base_url/default/api_key`）。

> 提示：`custom`（OpenAI 兼容端点）会原样保留你填写的模型名与 URL；选择内置 Provider
> （openai/deepseek/…）时，hermes 会按其 provider 规范处理模型与鉴权。

### 4. 使用统一网关

在「API Keys」页面创建 Key，然后：

```bash
curl http://localhost:8080/api/v1/gateway/<slug>/openapi/v1/models \
  -H "X-API-Key: hp_..."

curl http://localhost:8080/api/v1/gateway/<slug>/openapi/v1/chat/completions \
  -H "X-API-Key: hp_..." -H "Content-Type: application/json" \
  -d '{"model":"hermes-agent","messages":[{"role":"user","content":"hi"}]}'
```

Webhook 回调地址（实例详情页可查）：

```
http://<portal>/api/v1/gateway/<slug>/webhook/whatsapp/
http://<portal>/api/v1/gateway/<slug>/webhook/webhook/
http://<portal>/api/v1/gateway/<slug>/webhook/bluebubbles/
http://<portal>/api/v1/gateway/<slug>/webhook/msgraph/
```

（channel → 实例 webhook 端口映射：whatsapp `8090`、generic webhook `8644`、bluebubbles `8645`、msgraph `8646`，与 hermes 默认一致。）

---

## OIDC 单点登录

在 `.env` 配置：

```
PORTAL_OIDC_ENABLED=true
PORTAL_OIDC_ISSUER=https://keycloak.example.com/realms/master
PORTAL_OIDC_CLIENT_ID=hermes-portal
PORTAL_OIDC_CLIENT_SECRET=...
PORTAL_OIDC_AUTO_PROVISION=true
PORTAL_OIDC_ADMIN_CLAIM=hermes_portal_admin
```

- 登录页出现「使用 SSO 单点登录」入口。
- `PORTAL_OIDC_AUTO_PROVISION=true` 时，首次 SSO 登录的用户自动在默认租户创建为 `member`。
- **管理员映射**：`PORTAL_OIDC_ADMIN_CLAIM` 指定 ID Token / userinfo 中的 claim 名（如 `hermes_portal_admin`、`roles`、`groups`）。当该 claim 为真值（`true` / `"true"` / `"1"` / `"yes"` / 非零数字 / 列表含真值元素）时，SSO 用户被授予 **tenant_admin** 角色；已存在的 member 在下次登录时自动升级（不会降级）。
- 已在 portal 建号（同一 `sub`）的用户按 OIDC issuer+subject 匹配，不重复创建。

## 审计日志

「审计日志」页面记录全部管理操作（登录、租户/用户/实例/API Key 的增删改、启停销毁等），支持按动作、目标、操作者筛选与分页：

- 超级管理员可查看所有租户的审计记录；
- 实例管理员 / 成员只能查看本租户的审计记录（后端强制租户隔离）。

API：`GET /api/audit?action=&target=&actor=&limit=&offset=`、`GET /api/audit/actions`。

## 权限模型

| 角色 | 租户 | 能力 |
|---|---|---|
| `super_admin` | 无（全局） | 管理所有租户/用户/实例/Key；给实例指定租户 |
| `tenant_admin` | 绑定一个租户 | 管理本租户用户、实例（创建/启停/销毁）、API Key |
| `member` | 绑定一个租户 | 只读实例/Key，可调用本租户实例的统一网关 |

所有实例/Key 访问均做租户隔离（后端强制校验，前端仅作展示）。

## 安全设计

- 密码 scrypt 存储（与 hermes 同格式 `scrypt$n$r$p$salt$dk`）；JWT HS256 + 刷新令牌。
- API Key 明文仅创建时展示一次，库中只存 SHA-256 哈希。
- dashboard 会话 cookie 由 portal 持有并注入，浏览器与实例之间不直连；实例的 `Set-Cookie` 不会落到浏览器。
- 内嵌 dashboard 与实例 API 均要求 portal 登录（Bearer 或 HttpOnly session cookie）。
- 实例的 API_SERVER_KEY / dashboard 密码为每个实例独立随机生成，portal 不展示明文（实例详情 `config` 已脱敏）。

## 目录结构

```
hermes-portal/
├── Dockerfile                  # 多阶段：前端构建 → Go 编译 → 运行时
├── docker-compose.yml          # portal + docker.sock + hermes-portal-net
├── .env.example
├── Makefile
├── backend/                    # Go 后端
│   ├── cmd/portal/main.go
│   └── internal/
│       ├── api/                # Gin 处理器（认证/租户/用户/实例/Key/SSO）
│       ├── config/             # 12-factor 配置
│       ├── database/           # GORM + SQLite + 种子数据
│       ├── middleware/         # Bearer/cookie 认证 + RBAC
│       ├── models/             # GORM 模型
│       ├── proxy/              # 内嵌 dashboard 反向代理 + 统一网关
│       ├── router/             # 路由装配 + SPA 托管
│       ├── security/           # scrypt / JWT / API Key
│       └── services/           # Docker 客户端、实例生命周期、会话缓存
└── frontend/                   # React 19 + Vite + Tailwind 门户 UI
    └── src/{lib,pages,components}
```

## 常见问题

- **创建实例报「docker is not reachable」**：portal 容器必须挂载 `/var/run/docker.sock`（compose 已配置；macOS 上需确保 Docker Desktop 运行中）。
- **内嵌 dashboard 显示异常**：确认实例容器状态为 `running`，可先「全屏打开」验证。
- **修改镜像/环境变量**：编辑实例（PUT）会自动重建容器（保留数据卷）。
- **销毁 vs 停止**：销毁会删除容器 + 数据卷；如误删，可用 `keep_volume=1` 查询参数保留卷。

## 开发

```bash
make dev-backend    # Go 后端（需先 cd frontend && npm run build）
make dev-frontend   # Vite 热更新，/api 代理到本地后端
make test           # Go 单元 + 集成测试
```
