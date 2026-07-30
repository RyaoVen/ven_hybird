# VenHybird 框架介绍提示词

> 用法：`git clone` 本仓后，把下面「提示词正文」整段粘贴给你博客项目的 AI 代理作为开场 briefing。
> 提示词假设代理的工作区就是本仓根目录。

---

## 提示词正文

你要用 **VenHybird** 框架写一个博客系统（真实登录 + 文章 CRUD）。VenHybird 是 Go(Fiber) 网关 + Node SSR worker 的混合渲染框架：React 页面 SSR 直出、SPA 接管、静态页 ISR 物化、数据变更经事件总线失效并 SSE 推送到在线浏览器。框架已完整可用，你只写业务。

### 先读这三篇文档（就在仓里）

1. `README.md`——架构、快速开始、hybrid API、鉴权守卫、缓存/ISR、SSE、配置表
2. `AGENTS.md`——分层纪律、检查命令、测试风格、环境坑、git 工作流红线（**必须遵守**）
3. `vision.md`——设计哲学（变更事件=敦促更新等语义，帮你理解框架为什么这么设计）

### 启动方式（两个进程）

```bash
cd frame/node && npm install && npm run build && node dist/main.js   # :3000，先起
cd frame/go && go run .                                               # :8080，后起（依赖 Node 拉路由表）
```

### 业务边界：你能动哪里

- **能动**：`frame/go/build/`（Go 业务注册：角色、页面、API、登录）和 `src/`（React 页面）
- **不能动**：`frame/go/internal/`、`frame/go/hybrid/`、`frame/node/`——这是框架本体。觉得框架缺能力时先向我提出，不要私改框架
- 仓内是空业务骨架：`frame/go/build/` 只有一个 `Register` 空壳函数（main.go 的注册入口），`src/` 下没有任何页面——按下面的契约从零写你的博客业务

### 页面契约（前端）

- 新建 `src/<路径>/page.tsx` 即得路由，`src/posts/[id]/page.tsx` → `/posts/:id`（多层动态同理）。**文件路径就是路由**，无需注册、无需 metadata
- 页面组件默认导出，props 为 `{ bootstrap }`（类型 `PageAppProps`，见 `src/app/pageApp.tsx`）：`bootstrap.params` 路径参数、`bootstrap.query` 查询参数、`bootstrap.initialState` 服务端数据（就是 Go 侧 handler 给的数据）
- SPA 跳转、data-only 取数、401 跳登录、SSE 数据推送刷新全部由内置 router 自动处理，**页面代码零框架接入**
- `src/entry-client.tsx` / `src/entry-server.tsx` / `src/app/` 是运行时，别动

### Go 业务契约（hybrid API，全部在 `build/` 里调用）

```go
// 角色（先于页面注册）；parents 为继承的父角色
a.RegisterRole("author", []string{"reader"})

// 动态页：SSR + 内存缓存（1min TTL）；roles 为 nil 表示公开
a.Page("/posts/:id", []string{"reader"}, func(c *hybrid.PageCtx) error {
    id := c.Param("id")           // 路径参数；c.Query("page") 查询参数
    post, ok := lookup(id)
    if !ok { return c.NotFound() } // 404
    return c.JSON(post)            // 数据即页面的 initialState
    // return c.Render()           // 空数据渲染（极少用）
})

// 静态页（ISR）：SSR 后物化落盘直发；maxPages 上限、smartLoad 热门预渲染
a.StaticPage("/posts/:id", 1000, true, handler)

// 业务 API：框架自动加 /api 前缀（写 "/posts" 即 /api/posts）；Page/StaticPage 禁止 /api 前缀
a.Get("/posts", nil, listHandler)
a.Post("/posts", []string{"author"}, func(c *hybrid.ApiCtx) error {
    var in NewPost
    if err := c.Bind(&in); err != nil { return c.Error(400, "bad body") }
    // ...入库...
    _ = a.DataChange("/posts/:id", id)  // 声明数据变更：失效 ISR/缓存 + SSE 推送在线浏览器（异步，立即返回）
    return c.JSON(201, map[string]any{"id": id})
})
a.Put("/posts/:id", []string{"author"}, h)    // c.Param / c.Query / c.Bind / c.Body 可用
a.Delete("/posts/:id", []string{"author"}, h)
// handler 里 c.User() → (userID, role, ok) 取当前登录身份（评论/点赞/归属按人记时用）

// 登录：自己校验用户凭据，通过后调放行函数下发双 cookie（ven_auth/ven_role）
server := a.Server()
server.App().Post("/auth/login", func(ctx *fiber.Ctx) error {
    // ...校验用户名密码（接你的用户存储），拿到业务用户主键 user.ID...
    if err := server.GrantAuthWithUser(ctx, "author", user.ID); err != nil { /* 角色未注册 */ }
    // 不需要身份时用 server.GrantAuth(ctx, "author")（等价 userID 为空）
    return ctx.JSON(fiber.Map{"ok": true})
})
server.App().Post("/auth/logout", func(ctx *fiber.Ctx) error { server.RevokeAuth(ctx); return ctx.SendStatus(204) })

a.SetLoginRedirect("/login")  // 401 跳转目标（默认就是 /login）
a.InvalidatePage("/posts/1") // 动态页手动失效（StaticPage 请用 DataChange）
```

守卫行为：未登录访问受限页 → 302 `{loginPath}?next=`（data-only 请求则 401 + `X-Ven-Login-Path` 头）；已登录但角色不足 → 原地渲染 `/403` 页。`/login` 与 `/403` 两个页面需要你自己在 `src/` 下提供。

### 失效与实时语义（重要）

- `DataChange(pattern, ...params)` 是唯一失效入口：**不给参数 = 该 pattern 全局失效；给满 = 单页；给一部分 = 子树**。永远异步、立即返回，debounce 5s 合批（30s 上限），随后自动完成：删 ISR 文件 + 清缓存 → 后台再生 → SSE 推送新数据到在线浏览器
- 所以文章增删改后只需声明一次 `DataChange("/posts/:id", id)`（列表页变更用 `DataChange("/posts")`），不要手动清缓存
- 事件允许丢、重启 ISR 目录自动清空重建——不要为它做持久化

### 工程纪律

- 提交前必跑：`cd frame/go && go build ./... && go vet ./... && go test ./...`；`cd frame/node && npm run typecheck && npm test`
- git 工作流（红线见 AGENTS.md）：一单元一 issue → 分支 `<type>/issue-<N>-<slug>` → conventional commits 中文描述带 `(#N)` → 检查通过 → PR → squash 合并；**不直推 master、不 force-push**
- 配置走环境变量（见 README 配置表），生产必须改 `VEN_INTERNAL_TOKEN`

### 建议的第一步

在空骨架上搭出博客主链路：角色 `reader`/`author`；页面 `/posts`、`/posts/[id]`、`/login`、`/write`（`src/` 下对应 page.tsx）；`build/Register` 里注册页面与 API `GET/POST /posts`、`PUT/DELETE /posts/:id`；登录页表单 POST `/auth/login`（先接一个内存用户表，验证全链路后再换真实存储）。跑通"登录 → 发文 → 列表/详情更新（含开两个浏览器窗口验证 SSE 无感刷新）"即证明链路全通。
