# Node 模块业务流文档

## 目录
- [项目概述](#项目概述)
- [目录结构](#目录结构)
- [模块详解](#模块详解)
- [业务流程](#业务流程)
- [当前缺失项](#当前缺失项)
- [启动检查清单](#启动检查清单)

---

## 项目概述

本项目是一个 **SSR + SPA Hybrid 渲染框架** 的 Node.js 实现模块，位于 `frame/node` 目录下。

**核心功能**：
1. SPA 客户端构建（基于 esbuild）
2. SSR 服务端构建（基于 esbuild）
3. 页面路由管理
4. SSR 模块加载与缓存
5. SSR 页面渲染
6. Hybrid 渲染（SSR + SPA 注入）
7. HTTP 服务器与请求处理
8. 预加载、慢加载、路由过滤策略

---

## 目录结构

```
frame/node/
├── build/                  # 构建输出目录（当前为空）
├── httpClient/             # HTTP 客户端/服务器模块
│   ├── httpClient.ts       # HTTP 客户端、处理器、服务器实现
│   ├── controller.ts       # HTTP 控制器（请求处理）
│   └── type.ts             # 类型定义
├── pageBuild/              # 页面构建模块
│   ├── pageBuild.ts        # 页面构建入口（整合所有构建业务）
│   ├── SPAbuild.ts         # SPA 客户端构建
│   ├── SSRbuild.ts         # SSR 服务端构建
│   ├── SSRload.ts          # SSR 模块加载器
│   ├── SSRrender.ts        # SSR 渲染器
│   ├── SSRrenderController.ts  # SSR 渲染控制器
│   ├── Hybrid.ts           # Hybrid 渲染器
│   └── routerGenerate.ts   # 路由生成与管理
├── config.ts               # 配置定义
├── configLoad.ts           # 配置加载器（YAML/TOML）
├── main.ts                 # 入口文件（当前仅演示）
├── package.json            # 包配置
└── tsconfig.json           # TypeScript 配置
```

---

## 模块详解

### 1. httpClient 模块

#### httpClient.ts

**HttpClient 类** - HTTP 客户端，发送 HTTP 请求

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor(baseURL, config)` | baseURL: string, config: {headers, timeout} | - | 创建客户端实例 |
| `get(url, config)` | url: string, config?: HttpRequestConfig | Promise<HttpResponse> | 发送 GET 请求 |
| `post(url, data, config)` | url: string, data: unknown, config?: HttpRequestConfig | Promise<HttpResponse> | 发送 POST 请求 |
| `put(url, data, config)` | url: string, data: unknown, config?: HttpRequestConfig | Promise<HttpResponse> | 发送 PUT 请求 |
| `delete(url, config)` | url: string, config?: HttpRequestConfig | Promise<HttpResponse> | 发送 DELETE 请求 |
| `request(config)` | config: HttpRequestConfig | Promise<HttpResponse> | 通用请求方法 |
| `setDefaultHeaders(headers)` | headers: Record<string, string> | void | 设置默认请求头 |
| `setDefaultTimeout(timeout)` | timeout: number | void | 设置默认超时时间 |

**HttpHandler 类** - HTTP 请求处理器，管理路由和中间件

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor()` | - | - | 创建处理器实例 |
| `get(path, handler)` | path: string, handler: RouteHandler | void | 注册 GET 路由 |
| `post(path, handler)` | path: string, handler: RouteHandler | void | 注册 POST 路由 |
| `put(path, handler)` | path: string, handler: RouteHandler | void | 注册 PUT 路由 |
| `delete(path, handler)` | path: string, handler: RouteHandler | void | 注册 DELETE 路由 |
| `use(middleware)` | middleware: Middleware | void | 注册中间件 |
| `setErrorhandler(handler)` | handler: ErrorHandler | void | 注册错误处理器 |
| `handleRequest(ctx)` | ctx: RequestContext | Promise<unknown> | 处理请求（内部方法） |
| `matchRoute(method, path)` | method: string, path: string | RouteDefinition \| null | 匹配路由（内部方法） |

**HttpServer 类** - HTTP 服务器，监听端口处理请求

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor(handler, options)` | handler: HttpHandler, options: HttpServerOptions | - | 创建服务器实例 |
| `start()` | - | Promise<void> | 启动服务器 |
| `stop()` | - | Promise<void> | 停止服务器 |
| `getStatus()` | - | ServerStatus | 获取服务器状态 |
| `getRequestsHandled()` | - | number | 获取已处理请求数 |
| `parseBody(req, maxBodySize)` | req: IncomingMessage, maxBodySize: number | Promise<unknown> | 解析请求体（内部方法） |

#### controller.ts

**httpController 类** - HTTP 控制器，整合 HTTP 业务

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor()` | - | - | 创建控制器实例（自动初始化 HttpClient、HttpHandler） |
| `requestDeal()` | - | Promise<void> | **[待完善]** 注册路由并启动服务器 |
| `requestPost(res)` | res: response | Promise<unknown> | 发送 POST 请求到配置的目标地址 |

#### type.ts

**类型定义**
- `request` - 请求接口 {hookId, router}
- `response` - 响应接口 {hookId, html, router}

---

### 2. pageBuild 模块

#### pageBuild.ts

**PageBuild 类** - 页面构建入口，整合所有构建业务

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor(config)` | config?: PageBuildConfig | - | 创建构建实例，初始化所有子模块 |
| `getContext()` | - | PageBuildContext | 获取运行时上下文 |
| `initRouter()` | - | Promise<PageRouter> | 初始化路由系统，扫描页面目录 |
| `buildSPA()` | - | Promise<{result, error}> | 构建 SPA 客户端 |
| `buildSSR()` | - | Promise<{result, error}> | 构建 SSR 服务端 |
| `build()` | - | Promise<PageBuildResult> | 执行完整构建流程（路由→SPA→SSR→预加载） |
| `preload(routes?)` | routes?: string[] | Promise<void> | 执行预加载 |
| `render(route, json)` | route: string, json?: unknown | Promise<RenderRequestResult> | 处理渲染请求 |
| `batchRender(requests)` | requests: Array<{route, json}> | Promise<RenderRequestResult[]> | 批量渲染请求 |
| `loadPage(route)` | route: string | PageModule | 加载页面模块 |
| `loadPageByRoute(route)` | route: string | PageModule | 根据路由加载页面模块 |
| `getPageByRoute(route)` | route: string | SSRPage \| null | 获取页面定义 |
| `getPages()` | - | SSRPage[] | 获取所有页面列表 |
| `clearCache()` | - | void | 清除所有缓存 |
| `clearPreloadCache(route?)` | route?: string | void | 清除预加载缓存 |
| `updateConfig(config)` | config: Partial<PageBuildConfig> | void | 更新配置 |
| `setDevMode(isDev)` | isDev: boolean | void | 设置开发模式 |
| `getRenderController()` | - | SSRrenderController | 获取渲染控制器 |
| `getPreloadedRoutes()` | - | string[] | 获取预加载状态 |
| `getConfig()` | - | PageBuildConfig | 获取当前配置 |

#### SPAbuild.ts

**SPAClient 类** - SPA 客户端构建器，基于 esbuild 编译浏览器代码

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor(options)` | options: SPAClientOptions | - | 创建构建器实例 |
| `build()` | - | Promise<SPAClientBuildResult> | 编译入口文件 |
| `save()` | - | Promise<void> | 保存构建结果到磁盘 |
| `buildAndSave()` | - | Promise<SPAClientBuildResult> | 编译并保存 |
| `getResult()` | - | SPAClientBuildResult \| null | 获取构建结果 |
| `getCode()` | - | string \| null | 获取客户端代码 |
| `getScriptTag()` | - | string | 生成 HTML script 标签 |
| `getAssetInfo()` | - | {outputFile, publicPath, format, scriptTag} | 获取构建产物信息 |

#### SSRbuild.ts

**SSRBuild 类** - SSR 服务端构建器，基于 esbuild 编译 Node.js 代码

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor(options)` | options: SSRBuildOptions | - | 创建构建器实例 |
| `build()` | - | Promise<SSRBuildResult> | 编译服务端入口 |
| `save()` | - | Promise<void> | 保存构建结果到磁盘 |
| `buildAndSave()` | - | Promise<SSRBuildResult> | 编译并保存 |
| `getResult()` | - | SSRBuildResult \| null | 获取构建结果 |
| `getCode()` | - | string \| null | 获取服务端代码 |

#### SSRload.ts

**SSRLoader 类** - SSR 模块加载器，加载编译后的页面模块

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor(pagesDir, ext)` | pagesDir?: string, ext?: string | - | 创建加载器实例 |
| `registerPage(page)` | page: SSRPage | void | 注册单个页面路由映射 |
| `registerPages(pages)` | pages: SSRPage[] | void | 批量注册页面路由映射 |
| `load(options)` | options: SSRLoadOptions \| (route, isDev) | PageModule | 加载页面模块 |
| `loadPages(pages, isDev)` | pages: SSRPage[], isDev: boolean | Map<string, PageModule> | 批量加载页面模块 |
| `clearCache(route)` | route: string | void | 清除指定路由缓存 |
| `clearAllCache()` | - | void | 清除所有缓存 |
| `exists(route)` | route: string | boolean | 检查模块是否存在 |
| `getCachedRoutes()` | - | string[] | 获取已缓存路由列表 |
| `setPagesDir(pagesDir)` | pagesDir: string | void | 更新页面输出目录 |
| `setExt(ext)` | ext: string | void | 更新文件扩展名 |

#### SSRrender.ts

**SSRRenderer 类** - SSR 渲染器，渲染 React 组件为 HTML

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor(config)` | config: SSRRenderConfig | - | 创建渲染器实例 |
| `render(context)` | context: SSRRenderContext | Promise<SSRRenderResult> | 渲染页面 |
| `loadModule(route)` | route: string | PageModule | 加载页面模块（内部方法） |
| `fetchInitialProps(module, context)` | module: PageModule, context: SSRRenderContext | Promise<unknown> | 获取初始数据（内部方法） |
| `renderComponent(module, props)` | module: PageModule, props: Record<string, unknown> | string | 渲染 React 组件（内部方法） |
| `generateDocument(componentHtml, initialData, route)` | ... | string | 生成完整 HTML（内部方法） |
| `updateConfig(config)` | config: Partial<SSRRenderConfig> | void | 更新配置 |
| `setTemplate(template)` | template: Partial<HTMLTemplate> | void | 更新模板 |
| `setDevMode(isDev)` | isDev: boolean | void | 设置开发模式 |

#### SSRrenderController.ts

**SSRrenderController 类** - SSR 渲染控制器，整合路由、预加载、慢加载、过滤、渲染

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor(config)` | config?: SSRrenderControllerConfig | - | 创建控制器实例 |
| `initRoutes()` | - | void | 初始化路由系统，注册所有页面 |
| `registerPage(page)` | page: SSRPage | void | 注册单个页面 |
| `registerPages(pages)` | pages: SSRPage[] | void | 批量注册页面 |
| `setRouter(router)` | router: PageRouter | void | 设置页面路由管理器 |
| `preloadPages(routes?)` | routes?: string[] | Promise<void> | 执行预加载业务 |
| `autoPreload()` | - | Promise<void> | 执行自动预加载（如果配置启用） |
| `clearPreloadCache(route?)` | route?: string | void | 清除预加载缓存 |
| `clearSlowLoadQueue(route?)` | route?: string | void | 清除慢加载队列 |
| `clearAll()` | - | void | 清除所有缓存和队列 |
| `setDevMode(isDev)` | isDev: boolean | void | 设置开发模式 |
| `updateLoadConfig(config)` | config: Partial<LoadConfig> | void | 更新加载配置 |
| `setSpaClientScriptPath(path)` | path: string | void | 设置 SPA 客户端脚本路径 |
| `setSpaClientScriptType(type)` | type: "module" \| "text/javascript" | void | 设置 SPA 客户端脚本类型 |
| `requestDeal(route, json)` | route: string, json: unknown | Promise<RenderRequestResult> | 处理渲染请求（核心方法） |
| `batchRequestDeal(requests)` | requests: Array<{route, json}> | Promise<RenderRequestResult[]> | 批量渲染请求 |
| `checkRouteFilter(route)` | route: string | boolean | 路由过滤检查（内部方法） |
| `matchRoute(pattern, route)` | pattern: string, route: string | boolean | 路径匹配（内部方法） |
| `isSlowLoadRoute(route)` | route: string | boolean | 检查是否为慢加载路由（内部方法） |
| `handleSlowLoad(route, json)` | route: string, json: unknown | Promise<HybridRenderResult> | 慢加载处理（内部方法） |
| `getHttpController()` | - | httpController | 获取 HTTP 控制器 |
| `getRenderer()` | - | SSRRenderer | 获取 SSR 渲染器 |
| `getHybridRenderer()` | - | HybridRenderer | 获取 Hybrid 渲染器 |
| `getLoader()` | - | SSRLoader | 获取 SSR 加载器 |
| `getRouter()` | - | PageRouter | 获取页面路由管理器 |
| `getPreloadedRoutes()` | - | string[] | 获取预加载缓存状态 |
| `getSlowLoadingRoutes()` | - | string[] | 获取慢加载队列状态 |
| `getPage(route)` | route: string | SSRPage \| null | 获取页面定义 |
| `getPages()` | - | SSRPage[] | 获取所有页面列表 |

#### Hybrid.ts

**HybridRenderer 类** - Hybrid 渲染器，SSR 渲染 + SPA 注入

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor(config?)` | config?: HybridConfig | - | 创建渲染器实例 |
| `setRenderer(renderer)` | renderer: SSRRenderer | void | 设置 SSR 渲染器 |
| `setLoader(loader)` | loader: SSRLoader | void | 设置 SSR 加载器 |
| `setSpaClientScriptPath(path)` | path: string | void | 设置 SPA 客户端脚本路径 |
| `setSpaClientScriptType(type)` | type: "module" \| "text/javascript" | void | 设置 SPA 客户端脚本类型 |
| `hybridRender(router, json)` | router: string, json: unknown | Promise<HybridRenderResult> | Hybrid 渲染（核心方法） |
| `generateJsonScript(json)` | json: unknown | string | 生成 JSON 数据注入脚本（内部方法） |
| `generateSpaClientScript()` | - | string | 生成 SPA 客户端脚本标签（内部方法） |
| `injectScripts(html, json)` | html: string, json: unknown | string | 注入脚本到 HTML（内部方法） |
| `clearCache(route?)` | route?: string | void | 清除渲染缓存 |

#### routerGenerate.ts

**PageRouter 类** - 页面路由管理器

| 方法 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `constructor()` | - | - | 创建路由管理器实例 |
| `registerPage(page)` | page: SSRPage | void | 注册单个页面 |
| `registerPages(pages)` | pages: SSRPage[] | void | 批量注册页面 |
| `getPages()` | - | SSRPage[] | 获取所有已注册页面 |
| `getPageByRoute(route)` | route: string | SSRPage \| null | 根据路由获取页面（支持动态路由） |
| `getPageByName(name)` | name: string | SSRPage \| null | 根据名称获取页面 |
| `removePage(route)` | route: string | void | 移除页面 |
| `clearPages()` | - | void | 清空所有页面 |
| `matchRoute(definedRoute, requestRoute)` | definedRoute: string, requestRoute: string | boolean | 动态路由匹配（内部方法） |

**辅助函数**
| 函数 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `scanPageFiles(dirPath)` | dirPath: string | Promise<string[]> | 扫描目录查找以 "page" 开头的文件 |
| `generatePageFromPath(filePath)` | filePath: string | SSRPage | 从文件路径生成页面定义 |
| `generateRoutes(dirPath)` | dirPath: string | Promise<PageRouter> | 扫描目录生成路由 |
| `createPageRouter()` | - | PageRouter | 创建路由管理器实例 |

---

### 3. config.ts

**配置定义**
| 配置项 | 类型 | 描述 |
|--------|------|------|
| `SPAClientConfig` | SPAClientOptions | SPA 客户端构建配置 |
| `SSRBuildConfig` | SSRBuildOptions | SSR 服务端构建配置 |
| `LoadConfigDefault` | LoadConfig | 默认加载配置（预加载、慢加载、过滤） |
| `PageBuildDefaultConfig` | PageBuildConfig | 默认 PageBuild 配置 |
| `HTTPClientConfig` | object | HTTP 客户端配置 |
| `HttpServerConfig` | HttpServerOptions | HTTP 服务器配置 |
| `ResponseConfig` | object | 响应配置 |

---

### 4. configLoad.ts

**配置加载器函数**
| 函数 | 参数 | 返回值 | 描述 |
|------|------|--------|------|
| `getConfig(key?, defaultValue?)` | key?: string, defaultValue?: T | T | 获取配置值（支持点分隔路径） |
| `getConfigAll()` | - | Record<string, unknown> | 获取整个配置对象 |
| `reloadConfig(options?)` | options?: ConfigLoadOptions | Record<string, unknown> | 重新加载配置文件 |
| `getConfigFileInfo()` | - | {path, format} | 获取已加载的配置文件信息 |
| `initConfig(options?)` | options?: ConfigLoadOptions | void | 初始化配置加载器 |

---

## 业务流程

### 整体构建流程

```
┌─────────────────────────────────────────────────────────────────┐
│                        PageBuild.build()                         │
├─────────────────────────────────────────────────────────────────┤
│  1. initRouter()          扫描页面目录，生成路由表                │
│     └── generateRoutes(pagesDir)                                 │
│         └── scanPageFiles() → generatePageFromPath()             │
│         └── PageRouter.registerPages()                           │
│                                                                   │
│  2. buildSPA() + buildSSR()  并行构建                            │
│     └── SPAClient.buildAndSave()                                 │
│         └── esbuild.build({platform: browser})                   │
│     └── SSRBuild.buildAndSave()                                  │
│         └── esbuild.build({platform: node})                      │
│                                                                   │
│  3. autoPreload()          自动预加载                            │
│     └── SSRrenderController.preloadPages()                       │
│         └── HybridRenderer.hybridRender() × N                    │
│             └── SSRRenderer.render()                             │
│                 └── SSRLoader.load()                             │
│                 └── ReactDOMServer.renderToString()               │
│             └── injectScripts()                                  │
│         └── preloadMap.set(route, html)                          │
│                                                                   │
│  返回: PageBuildResult {spa, ssr, router, duration, success}     │
└─────────────────────────────────────────────────────────────────┘
```

### 渲染请求流程

```
┌─────────────────────────────────────────────────────────────────┐
│                SSRrenderController.requestDeal()                 │
├─────────────────────────────────────────────────────────────────┤
│  输入: route (路由路径), json (注入数据)                          │
│                                                                   │
│  1. checkRouteFilter(route)  路由过滤检查                        │
│     └── whitelist/blacklist/none 模式判断                        │
│     └── 失败返回 {error: "Route blocked by filter"}              │
│                                                                   │
│  2. preloadMap.get(route)   查找预加载缓存                       │
│     └── 存在缓存 → 返回 {html, route, fromCache: true}           │
│                                                                   │
│  3. isSlowLoadRoute(route)  检查是否慢加载路由                    │
│     └── 是 → handleSlowLoad(route, json)                         │
│         └── 查找 slowLoadQueue 是否已有任务                       │
│         └── 无任务 → 创建 Promise 加入队列                        │
│         └── HybridRenderer.hybridRender()                        │
│                                                                   │
│  4. HybridRenderer.hybridRender(router, json)  正常渲染          │
│     ┌───────────────────────────────────────────────────────┐   │
│     │               HybridRenderer.hybridRender              │   │
│     ├───────────────────────────────────────────────────────┤   │
│     │  a. SSRRenderer.render(context)                        │   │
│     │     └── SSRLoader.load(route)                          │   │
│     │         └── require(modulePath)                        │   │
│     │         └── 规范化 PageModule                           │   │
│     │     └── module.getInitialProps(context)                │   │
│     │     └── ReactDOMServer.renderToString(element)         │   │
│     │     └── generateDocument()                             │   │
│     │                                                         │   │
│     │  b. injectScripts(ssrHtml, json)                        │   │
│     │     └── generateJsonScript(json)                        │   │
│     │         └── window.__SPA_DATA__ = {...}                │   │
│     │     └── generateSpaClientScript()                       │   │
│     │         └── <script src="{SPA_CLIENT_SCRIPT_PATH}">     │   │
│     │     └── 在 </body> 前插入                               │   │
│     └───────────────────────────────────────────────────────┘   │
│                                                                   │
│  返回: RenderRequestResult {html, route, fromCache, duration}    │
└─────────────────────────────────────────────────────────────────┘
```

### HTTP 服务器请求流程

```
┌─────────────────────────────────────────────────────────────────┐
│                     HttpServer.start()                           │
├─────────────────────────────────────────────────────────────────┤
│  1. 创建 http.Server / https.Server                              │
│  2. 监听端口 (port, host)                                        │
│  3. 请求到达 → HttpServer.requestListener                        │
│     ┌───────────────────────────────────────────────────────┐   │
│     │              请求处理流程                               │   │
│     ├───────────────────────────────────────────────────────┤   │
│     │  a. parseBody(req, maxBodySize)                        │   │
│     │     └── 解析 JSON/text/form/binary                     │   │
│     │                                                         │   │
│     │  b. 构建 RequestContext                                 │   │
│     │     └── {path, method, headers, query, body, ip}       │   │
│     │                                                         │   │
│     │  c. HttpHandler.handleRequest(ctx)                      │   │
│     │     ┌─────────────────────────────────────────────┐    │   │
│     │     │          中间件链执行                         │    │   │
│     │     ├─────────────────────────────────────────────┤    │   │
│     │     │  for middleware in middlewares:              │    │   │
│     │     │      await middleware(ctx, next)             │    │   │
│     │     │  matchRoute(method, path)                    │    │   │
│     │     │  await handler(ctx)                          │    │   │
│     │     └─────────────────────────────────────────────┘    │   │
│     │                                                         │   │
│     │  d. 返回响应                                            │   │
│     │     └── ctx.rawResponse.writeHead() + write()          │   │
│     │     └── 或 ErrorHandler 处理异常                        │   │
│     └───────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 当前缺失项

### 1. 依赖缺失

| 缺失依赖 | 影响文件 | 解决方案 |
|----------|----------|----------|
| `yaml` | configLoad.ts | `npm install yaml` |
| `@iarna/toml` | configLoad.ts | `npm install @iarna/toml` |

### 2. 源文件缺失

| 缺失文件 | 配置路径 | 描述 |
|----------|----------|------|
| `entry-client.tsx` | ../../../build/app/entry-client.tsx | SPA 客户端入口文件 |
| `entry-server.tsx` | ../../../build/app/entry-server.tsx | SSR 服务端入口文件 |
| `pages/page*.tsx` | ../../../build/app/pages/ | 页面组件文件（如 pageHome.tsx） |
| `app/` 目录 | ../build/page/app/ | 页面应用目录 |

### 3. 业务逻辑缺失

| 缺失业务 | 文件位置 | 描述 |
|----------|----------|------|
| HTTP 路由处理 | httpClient/controller.ts:34 | `requestDeal()` 中路由处理器为空 |
| HTTP 服务器入口 | main.ts | main.ts 只演示 SPA 构建，未启动 HTTP 服务 |
| 路由与渲染联动 | httpClient/controller.ts | httpController 未与 SSRrenderController 整合 |

### 4. package.json 问题

| 问题 | 当前值 | 应修改为 |
|------|--------|----------|
| main 入口 | index.js | main.ts 或 dist/main.js |
| scripts.start | 无 | 需添加启动脚本 |

---

## 启动检查清单

### 必须完成项（才能启动）

1. **安装缺失依赖**
   ```bash
   npm install yaml @iarna/toml
   ```

2. **创建页面源文件**
   - 创建 `../build/page/app/` 目录
   - 创建 `entry-client.tsx`（SPA 客户端入口）
   - 创建 `entry-server.tsx`（SSR 服务端入口）
   - 创建至少一个页面组件（如 `pages/pageHome.tsx`）

3. **完善 httpController.requestDeal()**
   - 注册路由处理器
   - 整合 SSRrenderController
   - 启动 HTTP 服务器

4. **完善 main.ts**
   - 整合完整业务流程
   - 启动 PageBuild.build()
   - 启动 HTTP 服务器

### 示例入口文件模板

**entry-client.tsx**
```tsx
import React from 'react';
import { hydrateRoot } from 'react-dom/client';

// 获取 SSR 注入的初始数据
const initialData = window.__SPA_DATA__ || {};
const route = initialData.route || '/';

// 渲染页面
hydrateRoot(document.getElementById('root'), <App route={route} data={initialData.data} />);
```

**entry-server.tsx**
```tsx
import React from 'react';

export default function App({ route, data }) {
    return (
        <div>
            <h1>Page: {route}</h1>
            <pre>{JSON.stringify(data, null, 2)}</pre>
        </div>
    );
}

export async function getInitialProps(ctx) {
    return { title: 'Home Page', data: ctx };
}
```

**pageHome.tsx**
```tsx
import React from 'react';

export default function Home() {
    return <div>Home Page</div>;
}

export const metadata = { route: '/home' };
```

### 完整启动流程

```typescript
// main.ts 完整版
import { PageBuild, createPageBuild } from './pageBuild/pageBuild';
import { HttpServer, HttpHandler } from './httpClient/httpClient';
import { HttpServerConfig } from './config';

async function main() {
    // 1. 创建 PageBuild 并执行构建
    const pageBuild = createPageBuild();
    const result = await pageBuild.build();
    
    if (!result.success) {
        console.error('Build failed:', result.spaError, result.ssrError);
        process.exit(1);
    }
    
    // 2. 获取渲染控制器
    const renderController = pageBuild.getRenderController();
    
    // 3. 创建 HTTP 处理器并注册路由
    const handler = new HttpHandler();
    handler.get('*', async (ctx) => {
        const renderResult = await renderController.requestDeal(ctx.path, ctx.query);
        if (renderResult.error) {
            return { status: 404, body: renderResult.error };
        }
        return { status: 200, body: renderResult.html, headers: { 'Content-Type': 'text/html' } };
    });
    
    // 4. 启动 HTTP 服务器
    const server = new HttpServer(handler, HttpServerConfig);
    await server.start();
    
    console.log('Server started on port', HttpServerConfig.port);
}

main().catch(console.error);
```

---

## 附录：类型定义汇总

详见各模块文件的接口定义。