# HTTP Client 使用说明

本文档介绍 `httpClient.ts` 模块的使用方法，包含 HTTP 客户端（发送请求）、HTTP 处理器（处理请求）、HTTP 服务器（网络层）三个核心组件。

## 目录

- [快速开始](#快速开始)
- [HttpClient - 发送 HTTP 请求](#httpclient---发送-http-请求)
- [HttpHandler - 处理 HTTP 请求](#httphandler---处理-http-请求)
- [HttpServer - 网络层服务器](#httpserver---网络层服务器)
- [完整示例](#完整示例)

---

## 快速开始

### 启动一个简单的 HTTP 服务器

```typescript
import { HttpHandler, HttpServer } from "./httpClient/httpClient";

// 创建处理器并注册路由
const handler = new HttpHandler();
handler.get("/hello", (ctx) => ({ message: "Hello World" }));

// 创建并启动服务器
const server = new HttpServer(handler, { port: 3000 });
await server.start();
// 服务器启动在 http://0.0.0.0:3000
```

### 发送一个 HTTP 请求

```typescript
import { HttpClient } from "./httpClient/httpClient";

const client = new HttpClient("https://api.example.com");
const response = await client.get("/users");
console.log(response.data); // 响应数据
```

---

## HttpClient - 发送 HTTP 请求

`HttpClient` 用于发送 HTTP 请求，支持 GET、POST、PUT、DELETE、PATCH 等方法。

### 创建客户端

```typescript
import { HttpClient, createHttpClient, getHttpClient } from "./httpClient/httpClient";

// 方式1: 直接创建实例
const client = new HttpClient("https://api.example.com", {
    headers: { "Authorization": "Bearer token" },
    timeout: 5000 // 5秒超时
});

// 方式2: 使用工厂函数
const client = createHttpClient("https://api.example.com");

// 方式3: 获取默认单例
const client = getHttpClient("https://api.example.com");
```

### 发送请求

#### GET 请求

```typescript
// 简单 GET 请求
const response = await client.get("/users");

// 带查询参数
const response = await client.get("/users", {
    query: { page: 1, limit: 10 }
});

// 带自定义请求头
const response = await client.get("/users", {
    headers: { "X-Custom-Header": "value" }
});
```

#### POST 请求

```typescript
// 发送 JSON 数据
const response = await client.post("/users", {
    name: "John",
    email: "john@example.com"
});

// 带额外配置
const response = await client.post("/users", { name: "John" }, {
    headers: { "Content-Type": "application/json" },
    timeout: 10000
});
```

#### PUT / PATCH / DELETE 请求

```typescript
// PUT - 更新资源
await client.put("/users/1", { name: "John Updated" });

// PATCH - 部分更新
await client.patch("/users/1", { email: "new@example.com" });

// DELETE - 删除资源
await client.delete("/users/1");
```

#### 通用 request 方法

```typescript
const response = await client.request<User[]>({
    url: "/users",
    method: "GET",
    query: { page: 1 },
    headers: { "Authorization": "Bearer token" },
    timeout: 5000,
    responseType: "json" // 可选: json, text, arraybuffer, blob
});
```

### 响应结构

```typescript
interface HttpResponse<T> {
    status: number;        // HTTP 状态码
    statusText: string;    // 状态文本
    headers: Record<string, string>;  // 响应头
    data: T;               // 响应数据
    config: HttpRequestConfig;        // 原始请求配置
}

// 使用示例
const response = await client.get<User>("/users/1");
console.log(response.status);     // 200
console.log(response.data.name);  // "John"
```

### 拦截器

拦截器用于在请求发送前或响应接收后进行统一处理。

#### 请求拦截器

```typescript
client.useRequestInterceptor((config) => {
    // 添加认证 token
    config.headers = {
        ...config.headers,
        "Authorization": `Bearer ${getToken()}`
    };
    return config;
});
```

#### 响应拦截器

```typescript
client.useResponseInterceptor((response) => {
    // 统一处理错误响应
    if (response.status === 401) {
        // 未授权，跳转登录
        redirectToLogin();
    }
    return response;
});
```

### 动态配置

```typescript
// 更新基础 URL
client.setBaseURL("https://new-api.example.com");

// 更新默认请求头
client.setDefaultHeaders({
    "Authorization": "Bearer new-token",
    "X-API-Version": "v2"
});
```

---

## HttpHandler - 处理 HTTP 请求

`HttpHandler` 用于定义路由和处理逻辑，是业务层组件。

### 创建处理器

```typescript
import { HttpHandler, createHttpHandler, getHttpHandler } from "./httpClient/httpClient";

// 方式1: 直接创建
const handler = new HttpHandler();

// 方式2: 工厂函数
const handler = createHttpHandler();

// 方式3: 获取默认单例
const handler = getHttpHandler();
```

### 注册路由

#### 基本路由

```typescript
handler.get("/users", (ctx) => {
    return { users: [{ id: 1, name: "John" }] };
});

handler.post("/users", (ctx) => {
    const body = ctx.body as { name: string };
    return { created: body, id: 2 };
});

handler.put("/users/:id", (ctx) => {
    return { updated: ctx.body, id: ctx.params.id };
});

handler.delete("/users/:id", (ctx) => {
    return { deleted: true, id: ctx.params.id };
});
```

#### 动态路由参数

路由支持动态参数，使用 `:paramName` 格式定义：

```typescript
handler.get("/users/:id", (ctx) => {
    // ctx.params 包含路径参数
    const userId = ctx.params.id;
    return { user: { id: userId } };
});

handler.get("/posts/:postId/comments/:commentId", (ctx) => {
    return {
        postId: ctx.params.postId,
        commentId: ctx.params.commentId
    };
});
```

### 请求上下文 (RequestContext)

```typescript
interface RequestContext {
    path: string;                   // 请求路径
    method: string;                 // HTTP 方法
    headers: Record<string, string>; // 请求头
    query: Record<string, string>;  // 查询参数
    body: unknown;                  // 请求体
    ip?: string;                    // 客户端 IP
    params?: Record<string, string>; // 路径参数（匹配后填充）
    rawRequest?: unknown;           // 原始请求对象
    rawResponse?: unknown;          // 原始响应对象
}

// 使用示例
handler.post("/search", (ctx) => {
    const keyword = ctx.query.keyword;
    const filters = ctx.body as { category?: string };
    const userAgent = ctx.headers["user-agent"];
    const clientIP = ctx.ip;

    return { results: [], keyword, filters };
});
```

### 中间件

中间件用于在请求处理前/后执行自定义逻辑，支持链式调用。

#### 添加中间件

```typescript
// 日志中间件
handler.use(async (ctx, next) => {
    console.log(`[${new Date().toISOString()}] ${ctx.method} ${ctx.path}`);
    await next(); // 继续执行下一个中间件或路由处理器
    console.log(`Response: ${JSON.stringify(ctx.body)}`);
});

// 认证中间件
handler.use(async (ctx, next) => {
    const token = ctx.headers["authorization"];
    if (!token) {
        ctx.body = { error: "Unauthorized" };
        return; // 不调用 next()，终止请求
    }
    await next();
});

// CORS 中间件
handler.use(async (ctx, next) => {
    ctx.headers["access-control-allow-origin"] = "*";
    await next();
});
```

#### 中间件执行顺序

中间件按添加顺序依次执行：

```typescript
handler.use(async (ctx, next) => {
    console.log("Middleware 1 - before");
    await next();
    console.log("Middleware 1 - after");
});

handler.use(async (ctx, next) => {
    console.log("Middleware 2 - before");
    await next();
    console.log("Middleware 2 - after");
});

handler.get("/test", (ctx) => {
    console.log("Handler");
    return { ok: true };
});

// 执行顺序:
// Middleware 1 - before
// Middleware 2 - before
// Handler
// Middleware 2 - after
// Middleware 1 - after
```

### 错误处理

```typescript
handler.setErrorHandler((error, ctx) => {
    console.error(`Error on ${ctx.path}:`, error.message);
    return {
        error: "Internal Server Error",
        message: error.message,
        path: ctx.path,
        timestamp: Date.now()
    };
});

// 路由中抛出错误
handler.get("/error", (ctx) => {
    throw new Error("Something went wrong");
});
```

### 获取已注册路由

```typescript
const routes = handler.getRoutes();
routes.forEach(route => {
    console.log(`${route.method} ${route.path}`);
});
```

---

## HttpServer - 网络层服务器

`HttpServer` 使用 Node.js `http/https` 模块实现完整的网络层，负责端口监听、请求解析、响应写入。

### 创建服务器

```typescript
import { HttpServer, createHttpServer, getHttpServer } from "./httpClient/httpClient";

const handler = new HttpHandler();
handler.get("/hello", (ctx) => ({ message: "Hello" }));

// 方式1: 直接创建
const server = new HttpServer(handler, {
    port: 3000,
    host: "0.0.0.0"
});

// 方式2: 工厂函数
const server = createHttpServer(handler, { port: 3000 });

// 方式3: 获取默认单例
const server = getHttpServer(handler, { port: 3000 });
```

### 服务器配置选项

```typescript
interface HttpServerOptions {
    port?: number;          // 监听端口，默认 3000
    host?: string;          // 监听主机，默认 "0.0.0.0"
    ssl?: boolean;          // 是否启用 HTTPS，默认 false
    certPath?: string;      // SSL 证书路径（HTTPS 时必需）
    keyPath?: string;       // SSL 密钥路径（HTTPS 时必需）
    maxBodySize?: number;   // 请求体最大大小（字节），默认 10MB
    timeout?: number;       // 服务器超时时间（毫秒），默认 120000
    keepAlive?: boolean;    // 是否保持连接，默认 true
    keepAliveTimeout?: number; // 保持连接超时时间（毫秒），默认 5000
}
```

### 启动服务器

```typescript
const server = new HttpServer(handler, { port: 3000 });

await server.start();
console.log("Server started on port 3000");

// 访问: http://localhost:3000/hello
```

### HTTPS 服务器

```typescript
const server = new HttpServer(handler, {
    port: 443,
    ssl: true,
    certPath: "./cert.pem",
    keyPath: "./key.pem"
});

await server.start();
console.log("HTTPS Server started on port 443");
```

### 停止和重启服务器

```typescript
// 停止服务器
await server.stop();
console.log("Server stopped");

// 重启服务器
await server.restart();
console.log("Server restarted");
```

### 获取服务器状态

```typescript
const status = server.getStatus();

console.log(status.running);          // 是否运行中
console.log(status.port);             // 监听端口
console.log(status.host);             // 监听主机
console.log(status.startTime);        // 启动时间
console.log(status.requestsHandled);  // 已处理请求总数
```

### 动态更换处理器

```typescript
// 无需重启服务器，直接更换处理器
const newHandler = new HttpHandler();
newHandler.get("/new-route", (ctx) => ({ new: true }));

server.setHandler(newHandler);
```

### 获取底层 Node.js 服务器

```typescript
const rawServer = server.getRawServer();
if (rawServer) {
    // 可以绑定底层事件
    rawServer.on("connection", (socket) => {
        console.log("New connection");
    });
}
```

### 快速启动

```typescript
import { quickStart } from "./httpClient/httpClient";

// 最简单的方式启动服务器
const server = await quickStart(3000);
// 使用默认处理器，所有请求返回 404

// 带路由配置快速启动
const handler = new HttpHandler();
handler.get("/hello", (ctx) => ({ hello: "world" }));
const server = await quickStart(3000, handler);
```

---

## 完整示例

### 示例1: RESTful API 服务器

```typescript
import { HttpHandler, HttpServer } from "./httpClient/httpClient";

const handler = new HttpHandler();

// 日志中间件
handler.use(async (ctx, next) => {
    console.log(`${ctx.method} ${ctx.path} - ${ctx.ip}`);
    await next();
});

// CORS 中间件
handler.use(async (ctx, next) => {
    await next();
});

// 用户 API
let users = [{ id: 1, name: "John" }];

handler.get("/users", (ctx) => users);

handler.get("/users/:id", (ctx) => {
    const user = users.find(u => u.id === parseInt(ctx.params.id));
    if (!user) throw new Error("User not found");
    return user;
});

handler.post("/users", (ctx) => {
    const body = ctx.body as { name: string };
    const newUser = { id: users.length + 1, name: body.name };
    users.push(newUser);
    return newUser;
});

handler.put("/users/:id", (ctx) => {
    const id = parseInt(ctx.params.id);
    const user = users.find(u => u.id === id);
    if (!user) throw new Error("User not found");
    const body = ctx.body as { name?: string };
    user.name = body.name ?? user.name;
    return user;
});

handler.delete("/users/:id", (ctx) => {
    const id = parseInt(ctx.params.id);
    users = users.filter(u => u.id !== id);
    return { deleted: true };
});

// 错误处理
handler.setErrorHandler((error, ctx) => ({
    error: error.message,
    path: ctx.path
}));

// 启动服务器
const server = new HttpServer(handler, { port: 3000 });
await server.start();
```

### 示例2: 客户端调用 API

```typescript
import { HttpClient } from "./httpClient/httpClient";

const client = new HttpClient("http://localhost:3000");

// 获取用户列表
const usersResponse = await client.get<{ id: number; name: string }[]>("/users");
console.log(usersResponse.data);

// 创建用户
const createResponse = await client.post("/users", { name: "Alice" });
console.log(createResponse.data);

// 获取单个用户
const userResponse = await client.get("/users/1");
console.log(userResponse.data);

// 更新用户
await client.put("/users/1", { name: "Alice Updated" });

// 删除用户
await client.delete("/users/1");
```

### 示例3: 客户端与服务端交互

```typescript
import { HttpClient, HttpHandler, HttpServer } from "./httpClient/httpClient";

// 服务端
const handler = new HttpHandler();
handler.get("/api/data", (ctx) => ({
    data: [1, 2, 3],
    timestamp: Date.now()
}));

const server = new HttpServer(handler, { port: 3000 });
await server.start();

// 客户端
const client = new HttpClient("http://localhost:3000");

// 添加请求拦截器记录日志
client.useRequestInterceptor((config) => {
    console.log(`Request: ${config.method} ${config.url}`);
    return config;
});

// 发送请求
const response = await client.get("/api/data");
console.log(response.data);

// 关闭服务器
await server.stop();
```

---

## 类型导出

```typescript
// 接口和类型
export interface HttpRequestConfig;
export interface HttpResponse<T>;
export interface RequestContext;
export interface RouteDefinition;
export interface HttpServerOptions;
export interface ServerStatus;

export type RouteHandler;
export type Middleware;
export type ErrorHandler;

// 类
export class HttpClient;
export class HttpHandler;
export class HttpServer;

// 工厂函数
export function createHttpClient(baseURL?, options?): HttpClient;
export function createHttpHandler(): HttpHandler;
export function createHttpServer(handler, options?): HttpServer;

// 单例获取函数
export function getHttpClient(baseURL?, options?): HttpClient;
export function getHttpHandler(): HttpHandler;
export function getHttpServer(handler?, options?): HttpServer;

// 快速启动
export function quickStart(port?, handler?): Promise<HttpServer>;
```

---

## 注意事项

1. **请求体大小限制**: 默认最大 10MB，可通过 `maxBodySize` 配置
2. **HTTPS 配置**: 启用 SSL 时必须提供 `certPath` 和 `keyPath`
3. **端口占用**: 如果端口已被使用，启动时会抛出错误
4. **中间件顺序**: 中间件按添加顺序执行，注意 `next()` 的调用时机
5. **错误处理**: 建议始终设置错误处理器，避免未捕获异常
6. **类型安全**: 响应数据默认类型为 `unknown`，建议使用泛型指定类型