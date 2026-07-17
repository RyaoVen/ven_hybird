# VenHybird

VenHybird 是一个 Go + Node 的 SSR / SPA 混合渲染实验框架。

- **Go / Fiber** 是唯一公网入口，承载静态资源、后续业务 API、鉴权、数据聚合和 ISR；
- **Node / React** 是仅内部可访问的页面路由与 SSR worker；
- 首屏通过 SSR HTML 直出，浏览器随后加载 SPA bundle 并用相同 bootstrap 数据 hydration。

## 架构

```text
Browser
  ↓
Go Fiber :8080
  ├─ /assets/*
  ├─ /api/*
  ├─ /auth/*
  └─ GET /*
       ↓ POST /render (202)
Node SSR worker :3000
  ├─ src/**/page.tsx routing
  ├─ React SSR
  └─ POST /_internal/render-callback
       ↓
Go returns HTML
```

Node 的页面路由是唯一真相源；Go 不扫描页面目录。

## 本地运行

终端一：

```bash
cd frame/node
npm install
npm run build
node dist/main.js
```

终端二：

```bash
cd frame/go
go run .
```

访问：

```text
http://127.0.0.1:8080/home
```

## 页面约定

```text
src/home/page.tsx             → /home
src/blog/[slug]/page.tsx      → /blog/:slug
src/page.tsx                  → /
```

## 重要环境变量

```text
VEN_NODE_WORKER_URL=http://127.0.0.1:3000
VEN_RENDER_CALLBACK_URL=http://127.0.0.1:8080/_internal/render-callback
VEN_INTERNAL_TOKEN=development-token
```

完整协议参见 [Go 与 Node 渲染协议](docs/architecture/go-http-handler.md)。

## 下一阶段

- Go data provider 与首屏 `initialState`；
- Fiber 业务 API、博客 CRUD、服务端鉴权；
- 事件驱动 ISR、`DataChange(tags/routes)`、缓存失效和后台再生；
- 前端 guard manifest；
- 用博客验证框架落地。
