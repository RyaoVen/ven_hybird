# 重构计划：Node 统一页面路由，Go 收敛为业务网关与 SSR 协调层

## 已确定的架构决策

```text
Node：页面目录扫描、路由匹配、React 页面模块选择、SSR、SPA bundle 构建
Go：  Fiber 服务、业务 API、鉴权、数据访问、静态资源、ISR/事件（后续）、页面 catch-all
```

- Go 不再扫描、匹配或注册每个页面路由。
- Node `PageRouter` 保留，成为唯一页面路由权威。
- Go 仅转发真实路径；Node 返回匹配到的模板路由、页面名和 HTML。
- Go 与 Node 保留异步 `202 + callback` 协议：Go 等待回调后向浏览器返回 HTML；未来 ISR 后台再生可复用该协议。

---

# 一、删除的 Go 冗余模块

删除：

```text
frame/go/router/generate.go
frame/go/router/controller.go
frame/go/router/preload.go
frame/go/router/defender.go
frame/go/types/types.go
frame/go/http/handler.go
frame/go/http/controller.go
frame/go/config.go
```

随后删除清空目录：

```text
frame/go/router/
frame/go/types/
frame/go/http/
```

这些文件属于旧设计：Go 扫描 `page` 文件、维护静态/动态页、逐页注册 Fiber handler，并以错误的 `GET Node /route` 协议取得 HTML。Fiber 本身保留，后续 Go 的 API、鉴权、数据层与 ISR 都继续以 Fiber 为基座。

---

# 二、目标 Go 目录与职责

```text
frame/go/
├── main.go
├── go.mod
├── go.sum
└── internal/
    ├── config/
    │   └── config.go
    ├── ssr/
    │   ├── types.go
    │   ├── hook_id.go
    │   ├── pending.go
    │   └── client.go
    └── httpserver/
        ├── server.go
        ├── page_proxy.go
        └── routes.go
```

## `internal/config/config.go`

```go
type Config struct {
    ListenAddr        string
    NodeWorkerURL     string
    NodeSubmitTimeout time.Duration
    RenderTimeout     time.Duration
    InternalToken     string
    MaxPendingRenders int
    AssetsDir         string
}

func Load() (Config, error)
```

从环境变量读取并校验 `RenderTimeout > NodeSubmitTimeout`。Fiber 的 `WriteTimeout` 设置为大于 `RenderTimeout`。不再配置页面根目录、页面列表或页面白黑名单。

环境变量：

```text
VEN_LISTEN_ADDR=:8080
VEN_NODE_WORKER_URL=http://127.0.0.1:3000
VEN_NODE_SUBMIT_TIMEOUT=5s
VEN_RENDER_TIMEOUT=20s
VEN_INTERNAL_TOKEN=development-token
VEN_MAX_PENDING_RENDERS=100
VEN_ASSETS_DIR=../node/build
```

## `internal/ssr/types.go`

```go
type PageBootstrap struct {
    Route        string            `json:"route"`
    Params       map[string]string `json:"params"`
    Query        map[string]string `json:"query"`
    InitialState any               `json:"initialState"`
}

type RenderTask struct {
    HookID       string        `json:"hookId"`
    RequestRoute string        `json:"requestRoute"`
    Payload      PageBootstrap `json:"payload"`
}

type RenderError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

type RenderCallback struct {
    HookID       string       `json:"hookId"`
    RequestRoute string       `json:"requestRoute"`
    MatchedRoute string       `json:"matchedRoute,omitempty"`
    PageName     string       `json:"pageName,omitempty"`
    HTML         string       `json:"html"`
    Error        *RenderError `json:"error,omitempty"`
    Duration     int64        `json:"duration,omitempty"`
}
```

`hookId` 由 Go 生成并关联 callback；`requestRoute` 是真实访问路径；`matchedRoute`、`pageName` 由 Node 计算；Go 不再传 `pagename`。

## `internal/ssr/hook_id.go`

```go
type HookIDGenerator interface { New() (string, error) }
type CryptoHookIDGenerator struct{}
```

用 `crypto/rand` 生成 URL-safe 随机 ID；测试注入固定 generator。

## `internal/ssr/pending.go`

```go
type PendingRegistry struct {
    mu         sync.Mutex
    waiters    map[string]chan RenderCallback
    maxPending int
}

func NewPendingRegistry(maxPending int) *PendingRegistry
func (r *PendingRegistry) Register(hookID string) (
    result <-chan RenderCallback,
    remove func(),
    err error,
)
func (r *PendingRegistry) Resolve(callback RenderCallback) bool
func (r *PendingRegistry) Count() int
```

仅保存正在等待 callback 的页面请求：

- 使用容量为 1 的 buffered channel；
- 先 Register，再 POST Node，防止极速 callback 丢失；
- `Resolve` 先删除 map 条目再投递，保证单次完成；
- 超时/回调竞争以先获得锁的一方为准；
- 不持久化、不保存历史、不重试、不是 ISR cache。

## `internal/ssr/client.go`

```go
type Client interface {
    Submit(ctx context.Context, task RenderTask) error
}

type NodeClient struct {
    baseURL string
    client  *http.Client
    token   string
}
```

标准库 `net/http` 实现。只允许 Node `202 Accepted` 表示任务受理；非 202、不可达或提交超时返回 error。该超时只覆盖提交，不覆盖 SSR 完成。

## `internal/httpserver/server.go`

```go
type Server struct {
    app     *fiber.App
    config  config.Config
    ssr     ssr.Client
    pending *ssr.PendingRegistry
    hookIDs ssr.HookIDGenerator
}
```

创建唯一 Fiber app、安装 body limit/错误处理/日志并控制路由顺序。

## `internal/httpserver/page_proxy.go`

```go
func (s *Server) HandlePage(ctx *fiber.Ctx) error
func (s *Server) HandleRenderCallback(ctx *fiber.Ctx) error
```

`HandlePage` 是最后注册的 `GET/HEAD /*`：

```text
真实 path/query
→ hookId
→ pending.Register
→ PageBootstrap{route, query, initialState:{}}
→ 限时提交 Node task
→ 等 callback 或 RenderTimeout
→ PAGE_NOT_FOUND: 404
→ RENDER_FAILED: 502
→ 成功：text/html + HTML
```

`HandleRenderCallback` 为 `POST /_internal/render-callback`：token 常量时间校验、body 大小限制、decode/validate callback、Resolve 成功回 204，未知/重复/迟到 hook 回 404。

## `internal/httpserver/routes.go`

固定顺序：

```text
1. POST /_internal/render-callback
2. GET  /healthz
3. GET  /assets/*
4. /api/*      （预留后续 Go 业务）
5. /auth/*     （预留后续鉴权）
6. GET  /*     （页面 catch-all，最后）
7. HEAD /*
```

Go 以 `app.Static("/assets", cfg.AssetsDir)` 提供 Node build 输出。

## `main.go`

只负责配置加载与依赖组装：

```text
config.Load → ssr.NewNodeClient → PendingRegistry → httpserver.New → RegisterRoutes → Listen
```

---

# 三、Node：保留 PageRouter，补齐单 bundle SSR worker

## Node 目录

```text
frame/node/
├── main.ts
├── config.ts
├── package.json
├── tsconfig.json
├── .generated/
│   └── pageRegistry.ts
├── http-transport/
│   ├── httpClient.ts
│   ├── httpController.ts
│   ├── renderExecutionGate.ts
│   └── types.ts
└── page-builder/
    ├── pageRouter.ts
    ├── pageRegistryGenerator.ts
    ├── pageBuilder.ts
    ├── spaBuilder.ts
    ├── ssrBuilder.ts
    ├── ssrLoader.ts
    └── ssrRenderer.ts
```

删除：

```text
frame/node/page-builder/hybridRenderer.ts
frame/node/page-builder/ssrRenderController.ts
frame/node/configLoad.ts
```

`hybridRenderer` 与 `ssrRenderer` 的 document/data/script 注入重复；`ssrRenderController` 混合页面缓存/预加载/慢加载/过滤并反向持有 HTTP controller，和未来 Go ISR 责任冲突；`configLoad` 无配置文件与调用方。

## `http-transport/types.ts`

定义与 Go 同语义的 `PageBootstrap`、`RenderTask`、`RenderError`、`RenderCallback`。Node 请求不再需要 `pagename`；匹配成功后才在 callback 回填 `matchedRoute` 和 `pageName`。

## `pageRouter.ts`

保留为唯一页面路由权威并增强：

```ts
interface SSRPage {
  id: string;       // route template 或显式 metadata
  name: string;     // 观测字段，非全局唯一主键
  route: string;    // 如 /blog/:slug
  filePath: string;
  enabled: boolean;
}

interface MatchedPage {
  page: SSRPage;
  params: Record<string, string>;
}
```

实现：

- `page.tsx/page.jsx` 扫描；
- `[slug] -> :slug`；
- 参数提取；
- 根路由 `/`；
- exact 静态路由优先；
- 动态路由按静态段更多、参数段更少、稳定 route 字符排序；
- 重复模板路由构建失败；
- `id` 不使用 basename，避免同名页面冲突。

新增 Node 单测覆盖 root、静态、动态、参数、优先级、冲突与 not found。

## `pageRegistryGenerator.ts`

构建前由扫描结果生成 `.generated/pageRegistry.ts`：

- 每个 page 的静态 import；
- 稳定排序后的 registry；
- `matchPage(route)`、`getPageModule(route)`；
- 与 PageRouter 相同的 params/优先级逻辑。

静态 import 使 esbuild 能将全部页面打入单一 SSR/SPA bundle；不再在构建产物里按页面名 require TypeScript 文件。

`.gitignore` 加入 `/frame/node/.generated/`。

## 页面入口与 PageApp

新建 `src/app/pageApp.tsx`，让 server/client 使用同一个 `PageApp`：

```tsx
<PageApp bootstrap={bootstrap} />
```

PageApp 通过生成 registry 选择真实 Page 组件并传递唯一 bootstrap。

`src/entry-server.tsx` 变为单 SSR entry，导出 `getPageModule(route)` 和 `PageApp`。

`src/entry-client.tsx` 只读取 `window.__VEN_BOOTSTRAP__`，再：

```tsx
hydrateRoot(root, <PageApp bootstrap={bootstrap} />)
```

不再出现 `__SPA_DATA__`、`__INITIAL_DATA__` 双协议，也不从 client import server entry。

## `ssrLoader.ts`

重写为单 bundle resolver：只加载 `build/entry-server.js`，dev 清对应 require cache，调用 bundle 的 `getPageModule(route)`；不存在抛 `PageNotFoundError`。删除 pageName 文件猜测、路由映射和批量按页加载。

## `ssrRenderer.ts`

成为唯一 document / bootstrap / client script 注入点：

```text
load page module
→ optional getInitialProps(bootstrap)
→ merge initial state
→ renderToString(<PageApp bootstrap={finalBootstrap} />)
→ document:
  div#root
  一次 __VEN_BOOTSTRAP__ 安全注入
  一次 /assets/entry-client.js
```

安全转义 `<`、`>`、`&`、U+2028、U+2029、`</script`。

## `pageBuilder.ts`

职责收缩为：

```text
scan pages → router → generate registry → build SPA/SSR → render(route, bootstrap)
```

- build 前生成 registry；
- render 通过 PageRouter 获得 MatchedPage/params；
- 不存在页抛 PageNotFoundError；
- 输出 html、requestRoute、matchedRoute、pageName；
- 删除 preload、slowLoad、filter、Node 页面缓存、render-controller API。

## 异步 worker

新建 `renderExecutionGate.ts`，限制 Node SSR 并发：重复 hook 返回 409，满载返回 503，阶段一不设内存队列。

`httpController.ts` 保留 `/health` 与 `/render`：

```text
token 校验
→ task 校验
→ gate.acquire
→ 后台 PageBuild.render
→ success / PAGE_NOT_FOUND / RENDER_FAILED callback Go
→ callback 仅 2xx 成功
→ finally release gate
→ 立即返回 202
```

移除旧 `pagename` 校验、`ResponseConfig`、Node 回打自身地址。

`main.ts` 仅组装 PageBuild 与 HttpController：Node 根据 `requestRoute` 自己判断页面。

## 配置与构建

```ts
SPAClientConfig.publicPath = "/assets/entry-client.js"
PageBuildDefaultConfig.pagesDir = "../../src"
HttpServerConfig.host = "127.0.0.1"
RenderWorkerConfig.callbackURL = "http://127.0.0.1:8080/_internal/render-callback"
```

更新 `tsconfig` 覆盖 `.ts/.tsx` 与生成 registry；`package.json` 增加 `generate:routes`，并让 typecheck/build 先生成 registry。

---

# 四、实施顺序与验收

## Step 1：Node 页面路由与单 bundle

1. PageRouter 动态段、params、优先级、冲突；
2. 路由测试；
3. registry generator；
4. PageApp 与 server/client entry；
5. single bundle SSRLoader；
6. SSR 唯一 bootstrap 注入。

验收：Node `/home` 真正渲染 `src/home/page.tsx`；不存在页是 PAGE_NOT_FOUND；SSR/CSR bootstrap schema 一致。

## Step 2：Node worker transport

1. 新 transport 类型；
2. execution gate；
3. callback URL/token/2xx 检查；
4. async `/render` task；
5. 更新 main/config/scripts。

验收：合法任务立即 202；callback 包含 Node 算出的路由信息；重复/满载为 409/503。

## Step 3：Go internal 网关

1. 新 config、SSR protocol/hook/pending/client；
2. 新 Fiber server、callback、catch-all；
3. 新 main；
4. 删除旧 Go router/types/http/config；
5. `go mod tidy`。

验收：浏览器访问 Go `/home`，Go submit Node task，Node callback，Go 返回 200 HTML。

## Step 4：静态资源与 hydration

1. Go `/assets/*` 服务 Node build；
2. HTML 引用 `/assets/entry-client.js`；
3. 浏览器能下载 bundle；
4. 使用 `__VEN_BOOTSTRAP__` hydration；
5. 无 hydration mismatch 和首屏二次请求。

## Step 5：文档与回归

更新架构/API 文档并新增 README；文档明确 ISR、DataChange、业务鉴权、博客 CRUD 尚属下一阶段。

---

# 五、验证

Node：PageRouter、registry、loader、renderer、worker 的单测；类型检查和构建。

Go：PendingRegistry 并发（`go test -race`）、NodeClient 202 校验、callback token、page proxy 的 200/404/502/503/504、assets/API 不被 catch-all 吞掉。

最终：启动 Go/Node，验证 `/home`、不存在路由、Node 不可用、token 错误、assets 与浏览器 hydration。