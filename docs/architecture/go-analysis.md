# Go 网关架构

## 当前职责

Go 是 VenHybird 的公网入口与后续业务主后端，运行在 Fiber 之上。

- 对外提供页面、静态资源和未来的业务 API；
- 向 Node SSR worker 提交异步渲染任务；
- 接收 Node 的渲染回调，并将 HTML 返回给原始浏览器请求；
- 不扫描页面文件、不维护静态/动态页面表、不参与页面路由匹配。

页面路由的唯一权威在 Node。

## 目录

```text
frame/go/
├── main.go
└── internal/
    ├── config/       # 环境变量配置
    ├── ssr/          # Go ↔ Node 渲染协议、任务关联、Node client
    └── httpserver/   # Fiber 初始化、路由与页面代理
```

## HTTP 路由顺序

```text
POST /_internal/render-callback  Node 内部回调
GET  /healthz                    健康检查
GET  /assets/*                   SPA 构建产物
/api/*                           预留业务 API
/auth/*                          预留认证 API
GET/HEAD /*                      页面 SSR catch-all
```

页面 catch-all 必须最后注册，避免吞掉 API、静态资源和内部回调。

## 首屏渲染时序

```text
Browser GET /home
  → Go 创建 hookId，并注册 PendingRegistry waiter
  → Go POST Node /render
  ← Node 202 Accepted
  → Node 匹配页面并后台 SSR
  → Node POST Go /_internal/render-callback
  → Go 按 hookId 唤醒原请求
  ← Go 200 text/html
```

`PendingRegistry` 是进程内的一次性等待器：不持久化、不缓存历史、不重试 callback，也不等同于未来的 ISR cache。

## 配置

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `VEN_LISTEN_ADDR` | `:8080` | Go 公网监听地址 |
| `VEN_NODE_WORKER_URL` | `http://127.0.0.1:3000` | Node SSR worker 地址 |
| `VEN_NODE_SUBMIT_TIMEOUT` | `5s` | Go 提交渲染任务的超时 |
| `VEN_RENDER_TIMEOUT` | `20s` | Go 等待 Node callback 的超时 |
| `VEN_INTERNAL_TOKEN` | `development-token` | Go/Node 内部请求 token |
| `VEN_MAX_PENDING_RENDERS` | `100` | 同时等待的页面渲染数 |
| `VEN_ASSETS_DIR` | `../node/build` | Node 构建产物目录 |

后续开发将主要落在 Go：业务 API、数据 provider、服务端鉴权、ISR cache 与 `DataChange` 失效事件。
