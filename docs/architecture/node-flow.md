# Node SSR Worker 架构

## 当前职责

Node 是 VenHybird 的内部 React 页面 worker，不直接对公网暴露。

- 扫描 `src/**/page.tsx` 和 `page.jsx`；
- 维护唯一页面路由表；
- 将 `[param]` 目录转换为 `:param` 模板路由；
- 构建单一 SPA bundle 与单一 SSR bundle；
- 接收 Go 的异步 render task，执行 SSR 后 callback Go；
- 负责页面匹配、动态参数提取和真实 Page component 选择。

Go 不再扫描页面目录或逐页注册路由。

## 页面构建链

```text
src/**/page.tsx
  → PageRouter 扫描、排序、冲突检查
  → .generated/pageRegistry.ts（静态 import registry）
  → esbuild
     ├─ build/entry-client.js
     └─ build/entry-server.js
```

Registry 的静态 import 让 esbuild 将所有页面打进同一 bundle。运行时不会按页面名称猜测 `build/<page>.js`。

## 路由规则

```text
src/home/page.tsx             → /home
src/blog/[slug]/page.tsx      → /blog/:slug
src/page.tsx                  → /
```

匹配优先级：精确静态路由优先；动态路由按静态段更多、参数段更少、route 字符串稳定排序。重复模板路由会在构建时失败。

## Worker 协议

Go 提交：

```text
POST /render
→ 202 Accepted
```

任务包含：

```json
{
  "hookId": "opaque-id",
  "requestRoute": "/home",
  "payload": {
    "route": "/home",
    "params": {},
    "query": {},
    "initialState": {}
  }
}
```

Node 后台渲染后回调 Go：

```text
POST http://127.0.0.1:8080/_internal/render-callback
```

callback 包含 HTML、Node 计算出的 `matchedRoute` 和 `pageName`。同一 hookId 重复执行返回 409；worker 并发已满返回 503。

## SSR 与 hydration

`SSRRenderer` 是唯一 HTML document 生成点：

```html
<div id="root">...</div>
<script>window.__VEN_BOOTSTRAP__=...</script>
<script type="module" src="/assets/entry-client.js"></script>
```

客户端和服务端都使用 `PageApp` 与同一份 `PageBootstrap`，避免旧 `__INITIAL_DATA__` / `__SPA_DATA__` 双协议导致的 hydration mismatch。

## 运行配置

| 变量 | 默认值 |
|---|---|
| Worker 地址 | `127.0.0.1:3000` |
| callback URL | `http://127.0.0.1:8080/_internal/render-callback` |
| internal token | `development-token` |
| 最大并发渲染 | `4` |

ISR、页面缓存、业务鉴权与数据预取已由 Go 网关持有（见 README 与 cluster.md），Node 只负责页面匹配与渲染。
