# SSRBuild 构建器

> 文件路径：`frame/node/pageBuild/SSRbuild.ts`

## 概述

`SSRBuild` 是一个基于 esbuild 的 SSR（服务端渲染）构建器，用于将服务端入口文件编译为 Node.js 可执行代码。除了编译功能外，还提供**页面路由管理**和**预加载控制**能力。

## 核心能力

- 基于 esbuild 的高速打包，目标平台为 Node.js
- 原生支持 TypeScript / TSX / JSX
- 页面注册与路由管理，支持动态路由参数
- 预加载控制（全部/白名单/黑名单/禁用）
- 内存构建或直接写入磁盘

## API 文档

### SSRPage 页面定义

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | `string` | 是 | 页面名称，用于预加载白名单/黑名单匹配 |
| `route` | `string` | 是 | 路由路径，支持动态参数如 `/post/:id` |
| `filePath` | `string` | 是 | 页面文件路径 |
| `enabled` | `boolean` | 否 | 是否启用，默认 `true`，`false` 时跳过注册 |

### SSRPreloadMode 预加载模式

| 模式 | 行为 |
|------|------|
| `"all"` | 预加载所有已注册页面（默认） |
| `"whitelist"` | 仅预加载白名单中的页面 |
| `"blacklist"` | 预加载除黑名单外的所有页面 |
| `"none"` | 不预加载任何页面 |

### SSRBuildOptions 配置选项

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `entryPoint` | `string` | 是 | - | 服务端入口文件路径 |
| `pages` | `SSRPage[]` | 否 | `[]` | 页面列表 |
| `minify` | `boolean` | 否 | `false` | 是否压缩代码 |
| `sourcemap` | `boolean \| "inline" \| "external"` | 否 | `false` | source map 生成方式 |
| `external` | `string[]` | 否 | `[]` | 外部依赖，不参与打包 |
| `format` | `"cjs" \| "esm"` | 否 | `"cjs"` | 输出格式 |
| `write` | `boolean` | 否 | `false` | 是否直接写入磁盘 |
| `outFile` | `string` | 否 | - | 输出文件路径，`write=true` 时必填 |
| `target` | `string[]` | 否 | `["node18"]` | 编译目标环境 |
| `loader` | `Record<string, esbuild.Loader>` | 否 | 见下方 | 自定义文件 loader |
| `preloadPages` | `boolean` | 否 | `true` | 是否在构建时预加载页面 |
| `preloadMode` | `SSRPreloadMode` | 否 | `"all"` | 预加载模式 |
| `preloadWhitelist` | `string[]` | 否 | `[]` | 预加载白名单（按 name 或 route 匹配） |
| `preloadBlacklist` | `string[]` | 否 | `[]` | 预加载黑名单（按 name 或 route 匹配） |

**默认 loader 配置：**
```typescript
{
  ".tsx": "tsx",
  ".jsx": "jsx",
  ".ts": "ts",
  ".js": "js",
  ".json": "json"
}
```

### SSRBuildResult 构建结果

| 字段 | 类型 | 说明 |
|------|------|------|
| `serverCode` | `string \| undefined` | 编译后的服务端代码（内存模式可用） |
| `map` | `string \| undefined` | source map 内容 |
| `entryPoint` | `string` | 入口文件路径 |
| `outputFile` | `string \| undefined` | 输出文件路径 |
| `format` | `"cjs" \| "esm"` | 输出格式 |
| `writtenToDisk` | `boolean` | 是否已写入磁盘 |
| `pages` | `SSRPage[]` | 所有已注册页面 |
| `preloadedPages` | `SSRPage[]` | 已预加载的页面 |

### SSRBuild 类方法

#### `constructor(options: SSRBuildOptions)`

创建构建器实例，合并用户配置与默认值，并注册初始页面列表。

#### 页面管理方法

```typescript
// 批量注册页面
registerPages(pages: SSRPage[]): void

// 注册单个页面（enabled=false 会跳过）
registerPage(page: SSRPage): void

// 获取所有已注册页面
getPages(): SSRPage[]

// 根据路由获取页面（支持动态路由）
getPageByRoute(route: string): SSRPage | null
```

#### 预加载方法

```typescript
// 执行预加载，返回已预加载的页面列表
preloadAllPages(): SSRPage[]

// 获取已预加载页面
getPreloadedPages(): SSRPage[]
```

#### 构建方法

```typescript
// 执行构建
async build(): Promise<SSRBuildResult>

// 将构建结果写入磁盘（需先调用 build）
async save(): Promise<void>

// 便捷方法：构建并保存
async buildAndSave(): Promise<SSRBuildResult>

// 获取当前构建结果
getResult(): SSRBuildResult | null

// 获取服务端代码字符串
getCode(): string | null
```

#### 路由解析

```typescript
// 请求路由时返回对应页面
resolvePage(route: string): SSRPage | null
```

### 工厂函数

```typescript
function createSSRBuild(options: SSRBuildOptions): SSRBuild
```

创建 SSRBuild 实例的便捷函数。

## 动态路由匹配规则

支持简单的动态路由参数，格式为 `:paramName`：

| 定义路由 | 请求路由 | 匹配结果 |
|----------|----------|----------|
| `/post/:id` | `/post/123` | ✅ 匹配 |
| `/post/:id` | `/post/abc` | ✅ 匹配 |
| `/post/:id` | `/post` | ❌ 段数不等 |
| `/post/:id` | `/post/123/comments` | ❌ 段数不等 |
| `/user/:id/post/:postId` | `/user/42/post/100` | ✅ 匹配 |

**注意：** 当前实现仅判断是否匹配，不提取参数值。如需提取参数，需自行扩展。

## 使用示例

### 基础用法

```typescript
import { SSRBuild } from "./pageBuild/SSRbuild";

const ssr = new SSRBuild({
    entryPoint: "./src/entry-server.tsx",
    outFile: "./dist/server.js",
    format: "cjs",
    pages: [
        { name: "home", route: "/", filePath: "./pages/Home.tsx" },
        { name: "about", route: "/about", filePath: "./pages/About.tsx" },
        { name: "post", route: "/post/:id", filePath: "./pages/Post.tsx" },
    ]
});

const result = await ssr.buildAndSave();
console.log("已注册页面:", result.pages.map(p => p.route));
```

### 预加载控制

```typescript
// 白名单模式：只预加载首页和关于页
const ssr = new SSRBuild({
    entryPoint: "./src/server.ts",
    pages: [...],
    preloadPages: true,
    preloadMode: "whitelist",
    preloadWhitelist: ["home", "about"]
});

// 黑名单模式：预加载除文章页外的所有页面
const ssr = new SSRBuild({
    entryPoint: "./src/server.ts",
    pages: [...],
    preloadMode: "blacklist",
    preloadBlacklist: ["/post/:id"]
});

// 禁用预加载
const ssr = new SSRBuild({
    entryPoint: "./src/server.ts",
    pages: [...],
    preloadMode: "none"
});
```

### 运行时路由解析

```typescript
import http from "node:http";

const ssr = new SSRBuild({
    entryPoint: "./src/entry-server.tsx",
    pages: [
        { name: "home", route: "/", filePath: "./pages/Home.tsx" },
        { name: "post", route: "/post/:id", filePath: "./pages/Post.tsx" },
    ]
});

await ssr.buildAndSave();

// 模拟请求处理
const server = http.createServer((req, res) => {
    const page = ssr.resolvePage(req.url || "/");

    if (page) {
        console.log(`匹配到页面: ${page.name}, 文件: ${page.filePath}`);
        // 渲染页面...
    } else {
        res.statusCode = 404;
        res.end("Not Found");
    }
});
```

### 动态注册页面

```typescript
const ssr = new SSRBuild({
    entryPoint: "./src/server.ts"
});

// 后续动态注册
ssr.registerPage({ name: "home", route: "/", filePath: "./pages/Home.tsx" });
ssr.registerPages([
    { name: "about", route: "/about", filePath: "./pages/About.tsx" },
    { name: "contact", route: "/contact", filePath: "./pages/Contact.tsx" },
]);

// 禁用某个页面
ssr.registerPage({
    name: "disabled",
    route: "/disabled",
    filePath: "./pages/Disabled.tsx",
    enabled: false  // 不会被注册
});

console.log(ssr.getPages()); // 不包含 disabled 页面
```

### 与 SPAClient 配合使用

```typescript
import { SPAClient } from "./pageBuild/SPAbuild";
import { SSRBuild } from "./pageBuild/SSRbuild";

// 服务端构建
const ssr = new SSRBuild({
    entryPoint: "./src/entry-server.tsx",
    outFile: "./dist/server.js",
    format: "cjs",
    pages: [...]
});

// 客户端构建
const client = new SPAClient({
    entryPoint: "./src/entry-client.tsx",
    outFile: "./dist/client.js",
    publicPath: "/assets/client.js",
    format: "esm"
});

await Promise.all([
    ssr.buildAndSave(),
    client.buildAndSave()
]);
```

## 与 SPAClient 对比

| 特性 | SPAClient | SSRBuild |
|------|-----------|----------|
| 目标平台 | `browser` | `node` |
| 默认 format | `esm` | `cjs` |
| 默认 target | `es2020` | `node18` |
| 页面管理 | ❌ | ✅ |
| 动态路由 | ❌ | ✅ |
| 预加载控制 | ❌ | ✅ |
| script 标签生成 | ✅ | ❌ |
| publicPath | ✅ | ❌ |
| json loader | ❌ | ✅ |

## 注意事项

1. **`write=true` 时 `outFile` 必填**：esbuild 需要知道输出路径
2. **`save()` 前必须先 `build()`**：否则会抛出错误
3. **动态路由仅支持简单参数**：不支持正则或复杂约束
4. **预加载在 `build()` 时触发**：调用 `build()` 会自动执行预加载逻辑
5. **`enabled=false` 的页面不会被注册**：在 `registerPage` 时直接跳过

## 设计思路

### 为什么默认 CommonJS

Node.js 生态仍有大量 CommonJS 模块，`cjs` 格式兼容性更好。如需 ESM，可手动设置 `format: "esm"`。

### 预加载的意义

SSR 场景下，服务启动时预加载页面可以：
- 提前发现模块错误
- 减少「首次请求延迟」（模块已在内存）
- 配合白名单/黑名单实现按需加载策略

### SSR 完整流程

```
┌─────────────────────────────────────────────────────────────────┐
│                         构建阶段                                 │
├─────────────────────────────────────────────────────────────────┤
│  SSRBuild          SPAClient                                    │
│      ↓                  ↓                                       │
│  entry-server.tsx  entry-client.tsx                             │
│      ↓                  ↓                                       │
│  server.js (cjs)   client.js (esm)                              │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                         运行阶段                                 │
├─────────────────────────────────────────────────────────────────┤
│  1. Node.js 加载 server.js                                      │
│  2. 请求到达 → resolvePage() 获取页面信息                        │
│  3. 服务端渲染 HTML                                              │
│  4. 注入 <script type="module" src="/client.js">               │
│  5. 浏览器加载 client.js 进行水合                                │
└─────────────────────────────────────────────────────────────────┘
```