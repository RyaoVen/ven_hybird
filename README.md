# VenHybird

Go（Fiber）网关 + Node SSR Worker 的混合渲染框架。后端取 Spring 思想（声明式注册、AOP 式织入、显式声明失效），前端取 Next 思想（文件路由、SSR、ISR）；首屏 SSR 直出后由内置 SPA router 接管。

- **Node 是页面路由唯一真相源**：`src/**/page.tsx` 即路由（`[id]` → `:id`，支持多层动态），Go 启动时拉取校验
- **hybrid / internal / build 三层分离**：业务只接触 hybrid 胶水层的少量 API，internal 永不暴露
- **单实例到集群零改造**：不配 Redis 是内存单实例；配上 Redis 即成集群（见 [docs/cluster.md](docs/cluster.md)）

## 架构

```text
Browser
  │
  ▼
Go Fiber :8080  ─── 唯一公网入口
  ├─ /assets/*                  静态资源（VEN_ASSETS_DIR）
  ├─ /_internal/render-callback 渲染回调（Node → Go，内部令牌校验）
  ├─ /healthz                   健康检查 + 缓存计数
  ├─ ISR 直发中间件              命中物化文件直接返回（不执行业务 handler）
  ├─ 业务页面 /api/* /auth/*     hybrid 注册（鉴权 + SSR/JSON）
  └─ /*                         兜底 SSR（未注册路径走默认渲染）
       │
       │ POST /render（异步，202 Accepted）
       ▼
Node SSR Worker :3000  ─── 仅内部访问
  ├─ GET /pages          返回全部页面路由模式（Go 启动时拉取）
  ├─ POST /render        接收渲染任务，React SSR → 回调 Go
  └─ src/**/page.tsx     页面路由（唯一真相源）
```

**渲染协议**：Go 提交渲染任务（202 异步接受），Node 渲染完 POST 回调；Go 侧 pending registry 按 hookID 匹配等待中的请求，超时/失败映射为 404/502/504。SSR bundle external 了 react/react-dom（与渲染器保持同一份 React）。

## 快速开始

**终端一 — Node SSR Worker**（先起，Go 启动依赖它拉路由表）：

```bash
cd frame/node
npm install
npm run build      # 生成路由注册表 + tsc 编译
node dist/main.js  # 127.0.0.1:3000
```

**终端二 — Go 网关**：

```bash
cd frame/go
go run .           # :8080
```

启动后在 `src/` 下新建 `<路径>/page.tsx` 即得页面，在 `frame/go/build/` 的 `Register` 里注册角色/页面/API（当前为空骨架；用法见 [PROMPT.md](PROMPT.md)）。

**检查命令**：Go 端 `go build ./... && go vet ./... && go test ./...`；Node 端 `npm run typecheck && npm test`（vitest，纯逻辑单元测试）。

## hybrid API

业务注册全部通过 `hybrid.App`（`frame/go/build/` 有完整示例）：

| 方法 | 说明 |
|---|---|
| `Page(pattern, roles, handler)` | 动态页（GET+HEAD）。roles 为空即公开；鉴权 + 页面缓存 + SSR/JSON 双模式 |
| `StaticPage(pattern, maxPages, smartLoad, handler)` | 静态页（ISR）。物化落盘直发；`maxPages` 上限（0=不限）；`smartLoad` 全局更新时按热度预渲染 Top-N，否则 LRU 懒删除 |
| `Get/Post/Put/Delete(pattern, roles, handler)` | 业务 API，自动 `/api` 前缀（写了反而报错），全 JSON 响应 |
| `RegisterRole(role, parents)` | 注册角色，可继承父角色（须在 Page 前完成） |
| `DataChange(pattern, ...params)` | 显式声明数据变更 → ISR 失效（永远异步即时返回，详见下文） |
| `InvalidatePage(path)` | 使某路径的页面缓存失效 |
| `SetLoginRedirect(path)` | 401 时的登录跳转目标（默认 `/login`） |
| `Listen(addr)` | 注册兜底路由并启动（兜底强制最后注册） |

页面与 API 的 pattern 都不允许 `/api` 前缀冲突；页面 pattern 必须与 Node 路由表一致（启动校验，失败即报错）。

### PageCtx（页面 handler，数据被截流）

`Param(key)` / `Query(key)` / `JSON(data)` 设置数据 / `Render()` 强制 SSR / `NotFound()` 404。

框架按请求头决定输出：`X-Ven-Data-Only: true` → 裸 JSON（SPA 取数）；否则 SSR 渲染整页 HTML。

### ApiCtx（API handler，直接响应）

`Param(key)` / `Query(key)` / `Bind(&v)` 解析 JSON body / `Body()` 原始请求体 / `JSON(status, data)` / `Error(status, message)`。

## 鉴权与守卫

角色支持继承（`RegisterRole("author", []string{"reader"})` = author 继承 reader）：页面声明所需角色名，命中语义为"用户 is-a 声明角色"——子角色可访问父角色的页面，父角色**不能**访问子角色的页面。

**会话**：登录校验通过后 `Server.GrantAuth(ctx, role)` 生成会话令牌（24h TTL），下发双 cookie——`ven_auth`（HttpOnly，后端鉴权唯一依据）与 `ven_role`（JS 可读，前端守卫显示用）；`Server.RevokeAuth(ctx)` 注销。存储是 `auth.Backend` KV 接口，配 Redis 即跨实例共享。

**守卫行为**：

- HTML 导航 401 → `302 {loginPath}?next={原始URL}`；403 → 原地渲染 `/403` 错误页（URL 不变）
- data-only 请求 401/403 → 裸 JSON，401 带 `X-Ven-Login-Path` 头（SPA router 统一拦截跳转）
- API 401/403 → 永远裸 JSON

## 页面缓存（动态页）

SSR HTML 按 `路径 + 规范化 query + 数据指纹` 缓存（内存实现，1min TTL，上限 1000 条）。命中直接返回 HTML 不回 Node；同 key 并发防击穿只回源一次；仅缓存成功渲染（404/502/504 不缓存）。数据变更后调 `app.InvalidatePage(path)`。后端是 `pagecache.Backend` 接口，配 Redis 即跨实例共享。

## 静态页 ISR 与事件总线

`StaticPage` 声明的页面在首次 SSR 后物化到 `VEN_ISR_DIR`（原子 temp+rename），之后中间件直接发文件。

**失效语义**：`DataChange(pattern, ...params)` 永远异步、即时返回——

- 参数粒度：不给 = 全局失效、给满 = 单页、给一部分 = 子树（支持多层动态）
- **debounce 合批**：静默窗口 5s（持续变更最多等 30s 强制 flush）
- **批内先删后渲**：先删物化文件 + 清内存缓存，再由 smartLoad 声明按访问热度后台再生 Top-N
- **批间流水 + 页面级代际**：再生渲染期间可并行处理下一代删除，老代渲染不得覆盖新代
- **map 去重**：同页重复变更、范围重叠（全局吞局部、子树吞单页）一轮只处理一次
- 未再生页面下次访问懒回源重新物化；服务**重启清空 ISR 目录**（不沿用旧产物）

`VEN_ISR_ENABLED=false` 可整体关闭（dev 用）；query 不参与 ISR。

## SPA router

SSR 直出后由内置 SPA router 接管：registry 驱动路由匹配、链接点击拦截、data-only 取数（带 `X-Ven-Data-Only` 头）、401 统一按 `X-Ven-Login-Path` 跳登录页、滚动恢复与竞态/加载态处理。无需业务侧写路由表。

## 实时推送（SSE）

`/_internal/sse?route=<pattern>&<query>` 端点按页面订阅数据变更：事件总线每次 flush（`DataChange` / `InvalidatePage` 生效时）向匹配连接的订阅推送 `page-data` 事件，载荷与首屏 `PageBootstrap` 同形（`route`/`params`/`query`/`initialState`）。

内置 entry-client 会自动为当前路由建立订阅，收到推送后走 SPA router 既有的 `setState` 通道无感刷新——**页面代码零改动**，SSR/SPA/ISR 页面一视同仁。ISR 静态页因此不会让用户看到"一块老一块新"：文件再生完成的同时新数据已推到浏览器。

- 慢客户端丢帧不保活（推送是敦促更新，不是可靠投递）；连接以浏览器 EventSource 自动重连
- 关停时先 drain 再关连接，客户端自动重连到新实例；多实例下事件经 Redis 广播，天然全实例生效

## 日志与观测

统一请求日志（方法/路径/状态/耗时）；渲染事件日志（缓存 hit/miss/shared、Node 耗时）；鉴权拒绝日志（401/403 含角色与页面）；ISR 失效/再生/淘汰/去重日志。`/healthz` 返回页面缓存命中/回源/共享计数。

## 集群部署

多实例 = 单实例行为 + Redis 两类共享：会话/页面缓存走 Redis KV，`DataChange` 事件经 Redis Pub/Sub 广播（允许丢，重启重载兜底）。Go↔Node 必须 1:1 配对（回调须回到提交者实例），LB 只架在用户流量侧，ISR 目录各实例自持。完整拓扑、配置对照表见 [docs/cluster.md](docs/cluster.md)。

## 配置

**Go 网关**（`frame/go/internal/config`）：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `VEN_LISTEN_ADDR` | `:8080` | 监听地址 |
| `VEN_NODE_WORKER_URL` | `http://127.0.0.1:3000` | Node Worker 地址 |
| `VEN_NODE_SUBMIT_TIMEOUT` | `5s` | 任务提交超时 |
| `VEN_RENDER_TIMEOUT` | `20s` | 渲染总超时（须大于提交超时） |
| `VEN_INTERNAL_TOKEN` | `development-token` | 内部认证令牌（生产必须改） |
| `VEN_MAX_PENDING_RENDERS` | `100` | 最大并发 pending 渲染数 |
| `VEN_ASSETS_DIR` | `../node/build` | 静态资源目录 |
| `VEN_ISR_DIR` | `./isr-pages` | ISR 物化目录 |
| `VEN_ISR_ENABLED` | `true` | ISR 开关（dev 可 false） |
| `VEN_REDIS_ADDR` | 空 | Redis 地址（空 = 内存单实例模式） |
| `VEN_REDIS_PASSWORD` / `VEN_REDIS_DB` | 空 / `0` | Redis 密码 / 库编号 |
| `VEN_SESSION_TTL` | `24h` | 会话有效期 |
| `VEN_PAGE_CACHE_TTL` | `1m` | 动态页内存缓存有效期 |
| `VEN_EVENT_QUIET_WINDOW` | `5s` | 事件总线 debounce 静默窗口 |
| `VEN_EVENT_MAX_WAIT` | `30s` | 持续变更最大等待（须大于静默窗口） |

**Node Worker**（`frame/node/config.ts`）：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `VEN_NODE_PORT` | `3000` | Node Worker 监听端口 |
| `VEN_RENDER_CALLBACK_URL` | `http://127.0.0.1:8080/_internal/render-callback` | 渲染回调地址（须指回配对的 Go） |
| `VEN_INTERNAL_TOKEN` | `development-token` | 与 Go 侧一致 |

## 目录结构

```text
ven_hybird/
├── src/                        # 前端页面（Node 构建，路由唯一真相源）
│   ├── <页面目录>/page.tsx      #   你的页面：文件路径即路由，[id] → :id（多层动态同理）
│   ├── app/                    #   SPA 运行时（PageApp / router / 类型，框架持有）
│   ├── entry-client.tsx        #   SPA 入口
│   └── entry-server.tsx        #   SSR 入口
├── frame/
│   ├── go/                     # Go 网关
│   │   ├── main.go             #   启动入口（配置→Node client→拉路由表→注册→Listen）
│   │   ├── hybrid/             #   胶水层：App、Page/StaticPage/API、PageCtx/ApiCtx
│   │   ├── build/              #   业务注册入口（Register 空壳，业务写这里）
│   │   └── internal/
│   │       ├── httpserver/     #   Fiber 服务器、路由、SSR 代理、ISR 接线
│   │       ├── auth/           #   角色注册表、会话存储（Backend 接口）
│   │       ├── pagecache/      #   页面缓存（Backend 接口 + 内存实现 + 防击穿）
│   │       ├── isr/            #   ISR 文件层（落盘、直发、匹配器、LRU）
│   │       ├── event/          #   变更事件总线（debounce、先删后渲、代际去重）
│   │       ├── redis/          #   Redis 后端与事件 Pub/Sub（集群可选）
│   │       ├── pagepattern/    #   页面 pattern 校验器
│   │       ├── ssr/            #   SSR 客户端、pending 注册中心、HookID
│   │       └── config/         #   环境变量配置加载
│   └── node/                   # Node SSR Worker
│       ├── main.ts             #   启动入口
│       ├── http-transport/     #   HTTP 控制器、渲染执行门
│       ├── page-builder/       #   SPA/SSR 构建器、页面注册表生成
│       └── config.ts           #   配置加载
└── docs/                       # 文档
```

## 文档

- [设计哲学](vision.md)
- [PROMPT.md](PROMPT.md)（项目介绍提示词：拉本仓作为框架开发时，喂给 AI 代理的开场 briefing）
- [AGENTS.md](AGENTS.md)（协作/代理开发指南：布局纪律、检查命令、测试风格、坑与红线）
- [集群部署](docs/cluster.md)
- [Go 与 Node 渲染协议](docs/architecture/go-http-handler.md)
- [Node 渲染流程](docs/architecture/node-flow.md)
