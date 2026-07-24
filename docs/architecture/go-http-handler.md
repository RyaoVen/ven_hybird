# Go 与 Node 渲染协议

Go 网关和 Node SSR worker 使用内部异步任务协议。浏览器只访问 Go；Node 仅监听内部地址。

## 渲染请求

Go 向 Node 发起：

```text
POST {VEN_NODE_WORKER_URL}/render
Content-Type: application/json
X-Ven-Internal-Token: <token>
```

```json
{
  "hookId": "unique-random-id",
  "requestRoute": "/blog/hello",
  "payload": {
    "route": "/blog/hello",
    "params": {},
    "query": {},
    "initialState": {}
  }
}
```

Node 只在已获取执行容量时返回 `202 Accepted`。`hookId` 由 Go 使用随机值生成，是 callback 与原始页面请求的唯一关联键。

## 渲染回调

Node 在后台 SSR 完成后 POST 到：

```text
POST /_internal/render-callback
```

```json
{
  "hookId": "unique-random-id",
  "requestRoute": "/blog/hello",
  "matchedRoute": "/blog/:slug",
  "pageName": "slug",
  "html": "<!DOCTYPE html>...",
  "duration": 15
}
```

页面不存在时：

```json
{
  "hookId": "unique-random-id",
  "requestRoute": "/missing",
  "html": "",
  "error": {
    "code": "PAGE_NOT_FOUND",
    "message": "Page route not found: /missing"
  }
}
```

Go callback endpoint 会校验内部 token、关联 pending waiter 并返回 `204 No Content`。未知、超时或重复 hookId 会得到 `404`。

## 浏览器响应

Go 等待 callback：

- 成功：`200 text/html; charset=utf-8`；
- `PAGE_NOT_FOUND`：`404`；
- Node 不可达或渲染失败：`502`；
- Node 接单/渲染等待超时：`504`；
- Go pending 容量满：`503`。

该协议后续可复用于 ISR 后台再生任务；那时不创建浏览器 waiter，而由 Go cache/event 层消费 callback 结果。

## Go 端页面缓存

`RenderPage` 在提交渲染前先查页面缓存，命中直接返回 HTML 不回源 Node：

- **缓存 key**：`path + "|" + 规范化 query + "|" + sha256(json(data))`。handler 产出的 data 变化会得到新 key（"根据数据重新渲染"），旧条目随 TTL/容量自然淘汰。
- **只缓存成功结果**：`error == nil && HTML != ""`；`PAGE_NOT_FOUND`（避免遮蔽后注册页面）与 502/504（瞬时故障）不缓存。
- **防击穿**：同 key 并发 miss 只回源一次（flight group，检查点在注册 pending 之前，只占一个 pending 名额），follower 共享结果；失败共享错误且不写缓存。
- **后端接口**：`pagecache.Backend`（`Get/Set/Delete/DeletePrefix`），当前内存实现（容量上限 + 惰性过期），预留 Redis。
- **手动失效**：`Server.InvalidatePage(path)` / `hybrid.App.InvalidatePage(path)`，按路径前缀删除全部变体；未来 ISR/DataChange 事件挂同一入口。
- data-only 请求（`X-Ven-Data-Only`）不经过 RenderPage，天然 bypass。
