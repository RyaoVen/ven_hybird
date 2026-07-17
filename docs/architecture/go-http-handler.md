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
