# SPAClient 构建器

> 文件路径：`frame/node/pageBuild/SPAbuild.ts`

## 概述

`SPAClient` 是一个基于 esbuild 的 SPA（单页应用）客户端构建器，用于将 TSX/JSX 组件编译为浏览器可执行的客户端代码。主要用于 SSR（服务端渲染）场景下的客户端水合（hydration）。

## 核心能力

- 基于 esbuild 的高速打包
- 原生支持 TypeScript / TSX / JSX
- 支持内存构建或直接写入磁盘
- 自动生成 script 标签供 SSR 注入

## API 文档

### SPAClientOptions 配置选项

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `entryPoint` | `string` | 是 | - | 入口文件路径 |
| `minify` | `boolean` | 否 | `false` | 是否压缩代码 |
| `sourcemap` | `boolean \| "inline" \| "external"` | 否 | `false` | source map 生成方式 |
| `external` | `string[]` | 否 | `[]` | 外部依赖，不参与打包 |
| `format` | `"esm" \| "iife"` | 否 | `"esm"` | 输出格式，esm 对应 `type="module"` |
| `write` | `boolean` | 否 | `false` | 是否直接写入磁盘 |
| `outFile` | `string` | 否 | - | 输出文件路径，`write=true` 时必填 |
| `publicPath` | `string` | 否 | - | 浏览器访问路径，用于生成 script 标签 |
| `target` | `string[]` | 否 | `["es2020"]` | 编译目标环境 |
| `loader` | `Record<string, esbuild.Loader>` | 否 | 见下方 | 自定义文件 loader |

**默认 loader 配置：**
```typescript
{
  ".tsx": "tsx",
  ".jsx": "jsx",
  ".ts": "ts",
  ".js": "js"
}
```

### SPAClientBuildResult 构建结果

| 字段 | 类型 | 说明 |
|------|------|------|
| `clientCode` | `string \| undefined` | 编译后的客户端代码（内存模式可用） |
| `map` | `string \| undefined` | source map 内容 |
| `entryPoint` | `string` | 入口文件路径 |
| `outputFile` | `string \| undefined` | 输出文件路径 |
| `publicPath` | `string \| undefined` | 浏览器访问路径 |
| `format` | `"esm" \| "iife"` | 输出格式 |
| `writtenToDisk` | `boolean` | 是否已写入磁盘 |

### SPAClient 类方法

#### `constructor(options: SPAClientOptions)`

创建构建器实例，合并用户配置与默认值。

#### `async build(): Promise<SPAClientBuildResult>`

执行构建，返回构建结果。核心逻辑：

```typescript
esbuild.build({
    entryPoints: [entryPoint],
    bundle: true,        // 打包所有依赖
    platform: "browser", // 目标平台：浏览器
    jsx: "automatic",    // React 17+ 自动 JSX 运行时
    // ...其他配置
})
```

#### `async save(): Promise<void>`

将构建结果写入磁盘。仅在 `write: false` 时有效。

**注意：** 调用前必须先执行 `build()`。

#### `async buildAndSave(): Promise<SPAClientBuildResult>`

便捷方法，等价于 `build()` + `save()`。

#### `getResult(): SPAClientBuildResult | null`

获取当前构建结果。

#### `getCode(): string | null`

获取编译后的代码字符串。

#### `getScriptTag(): string`

生成 HTML script 标签，用于 SSR 注入。

**输出示例：**
```html
<!-- format: "esm" -->
<script type="module" src="/assets/client.js"></script>

<!-- format: "iife" -->
<script src="/assets/client.js"></script>
```

#### `getAssetInfo(): object`

获取构建产物信息摘要。

### 工厂函数

```typescript
function createSPAClient(options: SPAClientOptions): SPAClient
```

创建 SPAClient 实例的便捷函数。

## 使用示例

### 基础用法

```typescript
import { SPAClient } from "./pageBuild/SPAbuild";

const client = new SPAClient({
    entryPoint: "./src/entry-client.tsx",
    outFile: "./dist/client.js",
    publicPath: "/assets/client.js",
    format: "esm",
    minify: true,
    sourcemap: true
});

const result = await client.buildAndSave();
console.log("构建完成:", result.outputFile);
```

### 内存构建模式

适用于需要进一步处理代码的场景：

```typescript
const client = new SPAClient({
    entryPoint: "./src/entry-client.tsx",
    format: "esm",
    write: false  // 不写入磁盘
});

const result = await client.build();
const code = result.clientCode;  // 获取代码字符串

// 可以对 code 进行后续处理
// ...
```

### SSR 场景集成

```typescript
// 服务端渲染时注入 script 标签
const client = createSPAClient({
    entryPoint: "./src/entry-client.tsx",
    outFile: "./dist/assets/client.js",
    publicPath: "/assets/client.js"
});

await client.buildAndSave();

// 渲染 HTML 模板
const html = `
<!DOCTYPE html>
<html>
<head>
    <title>My App</title>
</head>
<body>
    <div id="root">${serverRenderedHtml}</div>
    ${client.getScriptTag()}
</body>
</html>
`;
```

### 外部依赖排除

```typescript
const client = new SPAClient({
    entryPoint: "./src/app.tsx",
    external: ["react", "react-dom"],  // 不打包这些依赖
    format: "esm"
});

// 输出代码中会保留 import 语句
// import React from "react";
// import ReactDOM from "react-dom";
```

## 注意事项

1. **`write=true` 时 `outFile` 必填**：esbuild 需要知道输出路径
2. **`getScriptTag()` 需要 `publicPath`**：没有设置会抛出错误
3. **`save()` 前必须先 `build()`**：否则会抛出错误
4. **目录自动创建**：输出目录不存在时会自动创建

## 设计思路

### 为什么选择 esbuild

- 编译速度极快（比 Webpack 快 10-100 倍）
- 原生支持 TypeScript 和 JSX
- API 简洁，适合程序化调用

### SSR 水合流程

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  entry-client   │     │   SPAClient     │     │   浏览器加载    │
│  (TSX 入口)     │ ──> │   编译打包      │ ──> │   水合执行      │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

1. 服务端渲染 HTML（包含初始数据）
2. 客户端加载 JS（由 SPAClient 生成）
3. React 调用 `hydrateRoot` 接管 DOM