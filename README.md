# VenHybird

Go + Node 混合渲染框架。Go 负责网关与数据层，Node 负责页面路由与 SSR 渲染，首屏 SSR 直出后由 SPA 接管。

## 架构

```text
Browser
  │
  ▼
Go Fiber :8080  ─── 唯一公网入口
  ├─ /assets/*           静态资源
  ├─ /_internal/*        内部端点（渲染回调、健康检查）
  ├─ /auth/*             鉴权端点
  ├─ /home, /about ...   业务页面（hybrid 注册，带鉴权 + SSR/JSON 双模式）
  └─ /*                  兜底 SSR（未注册的页面走默认渲染）
       │
       │ POST /render (async, 202)
       ▼
Node SSR Worker :3000  ─── 仅内部访问
  ├─ GET /pages          返回全部页面路由模式（Go 启动时拉取校验）
  ├─ POST /render        接收渲染任务，React SSR → 回调 Go
  └─ src/**/page.tsx     页面路由（唯一真相源）
```

**核心流程**：Go 启动时从 Node 拉取页面路由模式列表 → 注册业务页面路由 → 请求到达时 cookie 鉴权 → 权限校验 → 执行 handler 拿到数据 → 查页面缓存（命中直接返回 HTML，不回 Node）→ 提交 SSR 渲染并回填缓存 → 返回 HTML 或 JSON。

**页面缓存**：SSR HTML 在 Go 端按 `路径 + 规范化 query + 数据指纹` 缓存（内存实现，1 分钟 TTL，上限 1000 条），相同请求并发时防击穿只回源一次。仅缓存成功渲染（404/502/504 不缓存）。业务数据变更后调 `app.InvalidatePage(path)` 手动失效；存储后端是 `pagecache.Backend` 接口，预留 Redis 切换。

**静态页 ISR**：`app.StaticPage(pattern, maxPages, smartLoad, handler)` 声明的公开页面，SSR 产物物化到 `VEN_ISR_DIR`（默认 `./isr-pages`），之后由中间件直接发文件（不再回 Node）。失效靠业务显式声明 `app.DataChange(pattern, ...params)`——不给参数全局失效、给满局部单页、给一部分子树（支持 `/user/blog/:id` 多层动态），删除文件与内存缓存并写日志。`smartLoad` 开启时全局更新按访问热度预重渲染 Top-N；关闭且设上限时按 LRU 懒删除。query 不参与 ISR；`VEN_ISR_ENABLED=false` 可整体关闭（dev 用）。

**日志**：统一请求日志（方法/路径/状态/耗时）、渲染事件日志（缓存 hit/miss/shared、Node 耗时）、鉴权拒绝日志（401/403 含角色与页面）；`/healthz` 暴露缓存命中/回源/共享计数。

## 目录结构

```text
ven_hybird/
├── src/                        # 前端页面（Node 构建，路由唯一真相源）
│   ├── home/page.tsx           #   /home
│   ├── about/page.tsx          #   /about
│   ├── blog/[id]/page.tsx      #   /blog/:id
│   ├── app/                    #   页面容器与类型
│   ├── entry-client.tsx        #   SPA 入口
│   └── entry-server.tsx        #   SSR 入口
├── frame/
│   ├── go/                     # Go 网关
│   │   ├── main.go             #   启动入口
│   │   ├── hybrid/             #   胶水层：App、Page、PageCtx
│   │   ├── build/              #   业务注册：角色 + 页面 handler
│   │   └── internal/
│   │       ├── httpserver/     #   Fiber 服务器、路由、SSR 代理
│   │       ├── auth/           #   权限等级注册表、cookie 鉴权
│   │       ├── pagepattern/    #   页面 pattern 校验器
│   │       ├── ssr/            #   SSR 客户端、pending 注册中心、HookID
│   │       └── config/         #   环境变量配置加载
│   └── node/                   # Node SSR Worker
│       ├── main.ts             #   启动入口
│       ├── http-transport/     #   HTTP 控制器、渲染执行门
│       ├── build/              #   SPA/SSR 构建器、页面注册表生成
│       └── config.ts           #   配置加载
└── docs/                       # 文档
```

## 页面注册

页面在 Go 端通过 `hybrid.App` 注册，pattern 必须与 Node 页面路由一致：

```go
// frame/go/build/pages.go
func Register(a *hybrid.App) error {
    a.RegisterRole("guest", nil)

    a.Page("/home", nil, homePage)              // 无需鉴权
    a.Page("/about", nil, aboutPage)            // 无需鉴权
    a.Page("/blog/:id", []string{"guest"}, blogPage)  // 需要 guest 角色
    return nil
}
```

**Page handler 签名**：

```go
func homePage(c *hybrid.PageCtx) error {
    // c.JSON(data)   — 设置数据，框架根据请求头决定返回 JSON 或 SSR 页面
    // c.Render()     — 强制 SSR 渲染
    // c.Param(key)   — 读取路径参数
    // c.Query(key)   — 读取查询参数
    // c.NotFound()   — 返回 404
    return c.JSON(map[string]any{"title": "VenHybird"})
}
```

**请求处理流程**：cookie 鉴权 → 权限校验 → handler 设置数据 → `X-Ven-Data-Only: true` 返回 JSON，否则走 SSR 渲染返回 HTML。

## 权限系统

角色支持层级继承，注册时指定父角色：

```go
a.RegisterRole("admin", []string{"user"})  // admin 继承 user
a.RegisterRole("user", []string{"guest"})  // user 继承 guest
```

页面通过角色名数组声明所需权限，框架解析为等级列表后做命中比较。

**cookie 鉴权**：登录校验通过后调用放行函数 `Server.GrantAuth(ctx, role)`，生成随机会话令牌存入服务端会话缓存（`token → role`），并下发两个 cookie：

- `ven_auth`（HttpOnly）：会话令牌，后端鉴权唯一依据，每次请求拿它到会话缓存比对；
- `ven_role`（JS 可读）：角色名明文，供前端路由守卫使用（守卫注入待后续实现）。

会话存储后端是 `auth.Backend` KV 接口（`Set/Get/Delete`），当前为内存实现（24h 过期），预留 Redis 等外部存储切换空间。登出调用 `Server.RevokeAuth(ctx)` 注销会话并清除 cookie。

## 本地运行

**终端一 — Node SSR Worker**：

```bash
cd frame/node
npm install
npm run build
node dist/main.js
```

**终端二 — Go 网关**：

```bash
cd frame/go
go run .
```

访问 `http://127.0.0.1:8080/home`。

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `VEN_LISTEN_ADDR` | `:8080` | Go 网关监听地址 |
| `VEN_NODE_WORKER_URL` | `http://127.0.0.1:3000` | Node SSR Worker 地址 |
| `VEN_NODE_SUBMIT_TIMEOUT` | `5s` | 任务提交超时 |
| `VEN_RENDER_TIMEOUT` | `20s` | 渲染总超时（须大于提交超时） |
| `VEN_INTERNAL_TOKEN` | `development-token` | 内部认证令牌 |
| `VEN_MAX_PENDING_RENDERS` | `100` | 最大并发渲染数 |
| `VEN_ASSETS_DIR` | `../node/build` | 静态资源目录 |

## 文档

- [Go 与 Node 渲染协议](docs/architecture/go-http-handler.md)
- [Node 渲染流程](docs/architecture/node-flow.md)
- [HTTP Transport API](docs/api/http-transport.md)
