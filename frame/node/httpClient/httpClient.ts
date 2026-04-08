import * as http from "node:http";
import * as https from "node:https";
import * as url from "node:url";
import * as fs from "node:fs";

// ============================================
// 类型定义
// ============================================
 // 定义发送 HTTP 请求所需的各项配置参数

export interface HttpRequestConfig {
    /** 请求 URL（相对路径或绝对路径） */
    url: string;
    /** HTTP 请求方法，默认 GET */
    method?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH" | "HEAD" | "OPTIONS";
    /** 自定义请求头 */
    headers?: Record<string, string>;
    /** 请求体数据，POST/PUT/PATCH 时使用 */
    body?: unknown;
    /** URL 查询参数对象，会自动序列化为查询字符串 */
    query?: Record<string, string | number | boolean>;
    /** 请求超时时间（毫秒），默认 30000 */
    timeout?: number;
    /** 响应数据解析类型，默认 json */
    responseType?: "json" | "text" | "arraybuffer" | "blob";
}

/**
 * HTTP 响应接口
 * 封装 HTTP 请求的响应结果
 * @template T - 响应数据类型
 */
export interface HttpResponse<T = unknown> {
    /** HTTP 状态码（如 200, 404, 500） */
    status: number;
    /** HTTP 状态文本（如 OK, Not Found） */
    statusText: string;
    /** 响应头对象 */
    headers: Record<string, string>;
    /** 解析后的响应数据 */
    data: T;
    /** 原始请求配置 */
    config: HttpRequestConfig;
}

/**
 * HTTP 请求处理器上下文
 * 封装请求的完整信息，用于服务端请求处理
 */
export interface RequestContext {
    /** 请求路径（不含查询参数） */
    path: string;
    /** HTTP 请求方法 */
    method: string;
    /** 请求头对象 */
    headers: Record<string, string>;
    /** URL 查询参数 */
    query: Record<string, string>;
    /** 请求体数据 */
    body: unknown;
    /** 客户端 IP 地址 */
    ip?: string;
    /** 原始请求对象（如 Express.Request） */
    rawRequest?: unknown;
    /** 原始响应对象（如 Express.Response） */
    rawResponse?: unknown;
}

/**
 * 路由处理器函数类型
 * 处理匹配到的路由请求
 * @param ctx - 请求上下文
 * @returns 处理结果（可返回 Promise）
 */
export type RouteHandler = (ctx: RequestContext) => Promise<unknown> | unknown;

/**
 * 路由定义接口
 * 定义单个路由的配置信息
 */
export interface RouteDefinition {
    /** 路由路径（支持动态参数 :param） */
    path: string;
    /** HTTP 方法 */
    method: string;
    /** 路由处理函数 */
    handler: RouteHandler;
}

/**
 * 中间件函数类型
 * 用于在请求处理前/后执行自定义逻辑
 * @param ctx - 请求上下文
 * @param next - 下一个中间件或处理器的调用函数
 * @returns 可返回 Promise 或 void
 */
export type Middleware = (ctx: RequestContext, next: () => Promise<void>) => Promise<void> | void;

/**
 * 错误处理器函数类型
 * 用于统一处理请求过程中的错误
 * @param error - 捕获的错误对象
 * @param ctx - 请求上下文
 * @returns 错误响应数据
 */
export type ErrorHandler = (error: Error, ctx: RequestContext) => Promise<unknown> | unknown;

// ============================================
// HTTP 客户端 - 用于发送 HTTP 请求
// ============================================

/**
 * HTTP 客户端类
 * 用于发送 HTTP 请求，支持拦截器、超时、自动序列化等功能
 *
 * @example
 * ```typescript
 * const client = new HttpClient("https://api.example.com", {
 *   headers: { "Authorization": "Bearer token" },
 *   timeout: 5000
 * });
 *
 * // GET 请求
 * const response = await client.get("/users");
 * console.log(response.data);
 *
 * // POST 请求
 * const result = await client.post("/users", { name: "John" });
 * ```
 */
export class HttpClient {
    /** 基础 URL，会自动拼接到相对路径请求 */
    private baseURL: string;
    /** 默认请求头，所有请求都会携带 */
    private defaultHeaders: Record<string, string>;
    /** 默认超时时间（毫秒） */
    private defaultTimeout: number;
    /** 拦截器集合 */
    private interceptors: {
        /** 请求拦截器列表 */
        request: Array<(config: HttpRequestConfig) => HttpRequestConfig>;
        /** 响应拦截器列表 */
        response: Array<(response: HttpResponse) => HttpResponse>;
    };

    /**
     * 创建 HTTP 客户端实例
     *
     * @param baseURL - 基础 URL，用于拼接相对路径请求，默认 ""
     * @param options - 配置选项
     * @param options.headers - 默认请求头
     * @param options.timeout - 默认超时时间（毫秒）
     *
     * @example
     * ```typescript
     * const client = new HttpClient("https://api.example.com");
     * const clientWithOptions = new HttpClient("https://api.example.com", {
     *   headers: { "Content-Type": "application/json" },
     *   timeout: 10000
     * });
     * ```
     */
    constructor(
        baseURL: string = "",
        options?: {
            headers?: Record<string, string>;
            timeout?: number;
        }
    ) {
        this.baseURL = baseURL;
        this.defaultHeaders = options?.headers ?? {};
        this.defaultTimeout = options?.timeout ?? 30000;
        this.interceptors = {
            request: [],
            response: [],
        };
    }

    /**
     * 添加请求拦截器
     * 请求拦截器在请求发送前执行，可用于修改请求配置
     *
     * @param interceptor - 拦截器函数，接收请求配置，返回修改后的配置
     *
     * @example
     * ```typescript
     * client.useRequestInterceptor((config) => {
     *   config.headers = { ...config.headers, "X-Token": "my-token" };
     *   return config;
     * });
     * ```
     */
    useRequestInterceptor(interceptor: (config: HttpRequestConfig) => HttpRequestConfig): void {
        this.interceptors.request.push(interceptor);
    }

    /**
     * 添加响应拦截器
     * 响应拦截器在收到响应后执行，可用于统一处理响应数据
     *
     * @param interceptor - 拦截器函数，接收响应对象，返回修改后的响应
     *
     * @example
     * ```typescript
     * client.useResponseInterceptor((response) => {
     *   if (response.status === 401) {
     *     // 处理未授权
     *   }
     *   return response;
     * });
     * ```
     */
    useResponseInterceptor(interceptor: (response: HttpResponse) => HttpResponse): void {
        this.interceptors.response.push(interceptor);
    }

    /**
     * 构建完整请求 URL
     * 将 baseURL 和查询参数拼接成完整 URL
     *
     * @param config - 请求配置
     * @returns 完整的请求 URL
     */
    private buildURL(config: HttpRequestConfig): string {
        let url = config.url;
        if (this.baseURL && !url.startsWith("http")) {
            url = this.baseURL + url;
        }

        if (config.query) {
            const params = new URLSearchParams();
            for (const [key, value] of Object.entries(config.query)) {
                params.append(key, String(value));
            }
            url += (url.includes("?") ? "&" : "?") + params.toString();
        }

        return url;
    }

    /**
     * 发送 HTTP 请求
     * 核心请求方法，支持完整的请求配置和拦截器链
     *
     * @template T - 响应数据类型
     * @param config - 请求配置对象
     * @returns 响应结果 Promise
     * @throws 请求失败或超时时抛出错误
     *
     * @example
     * ```typescript
     * const response = await client.request<{ id: number }>({
     *   url: "/users",
     *   method: "GET",
     *   query: { page: 1 }
     * });
     * ```
     */
    async request<T = unknown>(config: HttpRequestConfig): Promise<HttpResponse<T>> {
        // 合并默认配置
        let finalConfig: HttpRequestConfig = {
            ...config,
            method: config.method ?? "GET",
            headers: { ...this.defaultHeaders, ...config.headers },
            timeout: config.timeout ?? this.defaultTimeout,
        };

        // 执行请求拦截器
        for (const interceptor of this.interceptors.request) {
            finalConfig = interceptor(finalConfig);
        }

        const url = this.buildURL(finalConfig);
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), finalConfig.timeout);

        try {
            const response = await fetch(url, {
                method: finalConfig.method,
                headers: finalConfig.headers as Record<string, string>,
                body: finalConfig.body ? JSON.stringify(finalConfig.body) : undefined,
                signal: controller.signal,
            });

            clearTimeout(timeoutId);

            // 解析响应头
            const headers: Record<string, string> = {};
            response.headers.forEach((value, key) => {
                headers[key] = value;
            });

            // 解析响应体
            let data: unknown;
            const responseType = finalConfig.responseType ?? "json";
            switch (responseType) {
                case "json":
                    data = await response.json();
                    break;
                case "text":
                    data = await response.text();
                    break;
                case "arraybuffer":
                    data = await response.arrayBuffer();
                    break;
                case "blob":
                    data = await response.blob();
                    break;
                default:
                    data = await response.json();
            }

            let httpResponse: HttpResponse<T> = {
                status: response.status,
                statusText: response.statusText,
                headers,
                data: data as T,
                config: finalConfig,
            };

            // 执行响应拦截器
            for (const interceptor of this.interceptors.response) {
                httpResponse = interceptor(httpResponse) as HttpResponse<T>;
            }

            return httpResponse;
        } catch (error) {
            clearTimeout(timeoutId);
            throw new Error(
                `HTTP request failed: ${url} - ${(error as Error).message}`
            );
        }
    }

    /**
     * 发送 GET 请求
     * 快捷方法，用于获取资源
     *
     * @template T - 响应数据类型
     * @param url - 请求 URL
     * @param config - 额外请求配置（可选）
     * @returns 响应结果 Promise
     *
     * @example
     * ```typescript
     * const users = await client.get<User[]>("/users");
     * const user = await client.get<User>("/users/1", { query: { fields: "name,email" } });
     * ```
     */
    async get<T = unknown>(
        url: string,
        config?: Partial<HttpRequestConfig>
    ): Promise<HttpResponse<T>> {
        return this.request<T>({ ...config, url, method: "GET" });
    }

    /**
     * 发送 POST 请求
     * 快捷方法，用于创建资源
     *
     * @template T - 响应数据类型
     * @param url - 请求 URL
     * @param body - 请求体数据
     * @param config - 额外请求配置（可选）
     * @returns 响应结果 Promise
     *
     * @example
     * ```typescript
     * const newUser = await client.post<User>("/users", { name: "John", email: "john@example.com" });
     * ```
     */
    async post<T = unknown>(
        url: string,
        body?: unknown,
        config?: Partial<HttpRequestConfig>
    ): Promise<HttpResponse<T>> {
        return this.request<T>({ ...config, url, method: "POST", body });
    }

    /**
     * 发送 PUT 请求
     * 快捷方法，用于更新资源
     *
     * @template T - 响应数据类型
     * @param url - 请求 URL
     * @param body - 请求体数据
     * @param config - 额外请求配置（可选）
     * @returns 响应结果 Promise
     *
     * @example
     * ```typescript
     * const updatedUser = await client.put<User>("/users/1", { name: "John Updated" });
     * ```
     */
    async put<T = unknown>(
        url: string,
        body?: unknown,
        config?: Partial<HttpRequestConfig>
    ): Promise<HttpResponse<T>> {
        return this.request<T>({ ...config, url, method: "PUT", body });
    }

    /**
     * 发送 DELETE 请求
     * 快捷方法，用于删除资源
     *
     * @template T - 响应数据类型
     * @param url - 请求 URL
     * @param config - 额外请求配置（可选）
     * @returns 响应结果 Promise
     *
     * @example
     * ```typescript
     * await client.delete("/users/1");
     * ```
     */
    async delete<T = unknown>(
        url: string,
        config?: Partial<HttpRequestConfig>
    ): Promise<HttpResponse<T>> {
        return this.request<T>({ ...config, url, method: "DELETE" });
    }

    /**
     * 发送 PATCH 请求
     * 快捷方法，用于部分更新资源
     *
     * @template T - 响应数据类型
     * @param url - 请求 URL
     * @param body - 请求体数据
     * @param config - 额外请求配置（可选）
     * @returns 响应结果 Promise
     *
     * @example
     * ```typescript
     * const patchedUser = await client.patch<User>("/users/1", { email: "new@example.com" });
     * ```
     */
    async patch<T = unknown>(
        url: string,
        body?: unknown,
        config?: Partial<HttpRequestConfig>
    ): Promise<HttpResponse<T>> {
        return this.request<T>({ ...config, url, method: "PATCH", body });
    }

    /**
     * 设置基础 URL
     * 动态更新客户端的基础 URL
     *
     * @param baseURL - 新的基础 URL
     *
     * @example
     * ```typescript
     * client.setBaseURL("https://new-api.example.com");
     * ```
     */
    setBaseURL(baseURL: string): void {
        this.baseURL = baseURL;
    }

    /**
     * 设置默认请求头
     * 添加或更新默认请求头，所有后续请求都会携带
     *
     * @param headers - 要添加的请求头对象
     *
     * @example
     * ```typescript
     * client.setDefaultHeaders({ "Authorization": "Bearer new-token" });
     * ```
     */
    setDefaultHeaders(headers: Record<string, string>): void {
        this.defaultHeaders = { ...this.defaultHeaders, ...headers };
    }
}

// ============================================
// HTTP 请求处理器 - 用于接收/处理 HTTP 请求
// ============================================

/**
 * HTTP 请求处理器类
 * 用于接收和处理 HTTP 请求（服务端），支持路由、中间件、错误处理
 *
 * @example
 * ```typescript
 * const handler = new HttpHandler();
 *
 * // 注册路由
 * handler.get("/users", async (ctx) => {
 *   return { users: [{ id: 1, name: "John" }] };
 * });
 *
 * handler.post("/users", async (ctx) => {
 *   const body = ctx.body;
 *   return { created: body };
 * });
 *
 * // 添加中间件
 * handler.use(async (ctx, next) => {
 *   console.log(`${ctx.method} ${ctx.path}`);
 *   await next();
 * });
 *
 * // 处理请求
 * const result = await handler.handle({
 *   path: "/users",
 *   method: "GET",
 *   headers: {},
 *   query: {},
 *   body: null
 * });
 * ```
 */
export class HttpHandler {
    /** 注册的路由列表 */
    private routes: RouteDefinition[];
    /** 中间件列表 */
    private middlewares: Middleware[];
    /** 全局错误处理器 */
    private errorHandler?: ErrorHandler;

    /**
     * 创建 HTTP 请求处理器实例
     */
    constructor() {
        this.routes = [];
        this.middlewares = [];
    }

    /**
     * 注册路由（内部方法）
     * 将路由添加到路由列表
     *
     * @param method - HTTP 方法
     * @param path - 路由路径
     * @param handler - 处理函数
     */
    private registerRoute(method: string, path: string, handler: RouteHandler): void {
        this.routes.push({ method: method.toUpperCase(), path, handler });
    }

    /**
     * 注册 GET 路由
     *
     * @param path - 路由路径，支持动态参数（如 /users/:id）
     * @param handler - 路由处理函数
     *
     * @example
     * ```typescript
     * handler.get("/users", (ctx) => ({ users: [] }));
     * handler.get("/users/:id", (ctx) => ({ id: ctx.params.id }));
     * ```
     */
    get(path: string, handler: RouteHandler): void {
        this.registerRoute("GET", path, handler);
    }

    /**
     * 注册 POST 路由
     *
     * @param path - 路由路径
     * @param handler - 路由处理函数
     *
     * @example
     * ```typescript
     * handler.post("/users", (ctx) => ({ created: ctx.body }));
     * ```
     */
    post(path: string, handler: RouteHandler): void {
        this.registerRoute("POST", path, handler);
    }

    /**
     * 注册 PUT 路由
     *
     * @param path - 路由路径
     * @param handler - 路由处理函数
     *
     * @example
     * ```typescript
     * handler.put("/users/:id", (ctx) => ({ updated: ctx.body }));
     * ```
     */
    put(path: string, handler: RouteHandler): void {
        this.registerRoute("PUT", path, handler);
    }

    /**
     * 注册 DELETE 路由
     *
     * @param path - 路由路径
     * @param handler - 路由处理函数
     *
     * @example
     * ```typescript
     * handler.delete("/users/:id", (ctx) => ({ deleted: true }));
     * ```
     */
    delete(path: string, handler: RouteHandler): void {
        this.registerRoute("DELETE", path, handler);
    }

    /**
     * 注册 PATCH 路由
     *
     * @param path - 路由路径
     * @param handler - 路由处理函数
     *
     * @example
     * ```typescript
     * handler.patch("/users/:id", (ctx) => ({ patched: ctx.body }));
     * ```
     */
    patch(path: string, handler: RouteHandler): void {
        this.registerRoute("PATCH", path, handler);
    }

    /**
     * 添加中间件
     * 中间件按添加顺序依次执行
     *
     * @param middleware - 中间件函数
     *
     * @example
     * ```typescript
     * // 日志中间件
     * handler.use(async (ctx, next) => {
     *   console.log(`Request: ${ctx.method} ${ctx.path}`);
     *   await next();
     *   console.log(`Response: ${ctx.body}`);
     * });
     *
     * // 认证中间件
     * handler.use(async (ctx, next) => {
     *   if (!ctx.headers["Authorization"]) {
     *     ctx.body = { error: "Unauthorized" };
     *     return;
     *   }
     *   await next();
     * });
     * ```
     */
    use(middleware: Middleware): void {
        this.middlewares.push(middleware);
    }

    /**
     * 设置全局错误处理器
     * 当路由处理过程中抛出异常时，会调用此处理器
     *
     * @param handler - 错误处理函数
     *
     * @example
     * ```typescript
     * handler.setErrorHandler((error, ctx) => ({
     *   error: error.message,
     *   path: ctx.path,
     *   timestamp: Date.now()
     * }));
     * ```
     */
    setErrorHandler(handler: ErrorHandler): void {
        this.errorHandler = handler;
    }

    /**
     * 匹配路由
     * 根据请求方法和路径查找匹配的路由定义
     *
     * @param method - HTTP 方法
     * @param path - 请求路径
     * @returns 匹配的路由定义，未找到返回 null
     */
    private matchRoute(method: string, path: string): RouteDefinition | null {
        for (const route of this.routes) {
            if (route.method === method && this.matchPath(route.path, path)) {
                return route;
            }
        }
        return null;
    }

    /**
     * 路径匹配（支持动态参数）
     * 检查请求路径是否匹配路由模式
     *
     * @param pattern - 路由模式（如 /users/:id）
     * @param path - 实际请求路径（如 /users/123）
     * @returns 是否匹配
     */
    private matchPath(pattern: string, path: string): boolean {
        const patternParts = pattern.split("/");
        const pathParts = path.split("/");

        if (patternParts.length !== pathParts.length) {
            return false;
        }

        for (let i = 0; i < patternParts.length; i++) {
            if (!patternParts[i].startsWith(":") && patternParts[i] !== pathParts[i]) {
                return false;
            }
        }

        return true;
    }

    /**
     * 提取路径参数
     * 从请求路径中提取动态参数值
     *
     * @param pattern - 路由模式（如 /users/:id/:name）
     * @param path - 实际请求路径（如 /users/123/john）
     * @returns 参数对象（如 { id: "123", name: "john" }）
     */
    private extractParams(pattern: string, path: string): Record<string, string> {
        const params: Record<string, string> = {};
        const patternParts = pattern.split("/");
        const pathParts = path.split("/");

        for (let i = 0; i < patternParts.length; i++) {
            if (patternParts[i].startsWith(":")) {
                const paramName = patternParts[i].slice(1);
                params[paramName] = pathParts[i];
            }
        }

        return params;
    }

    /**
     * 执行中间件链
     * 按顺序执行所有中间件，最后执行路由处理器
     *
     * @param ctx - 请求上下文
     * @param handler - 最终的路由处理函数
     * @returns 处理结果
     */
    private async executeMiddlewares(ctx: RequestContext, handler: RouteHandler): Promise<unknown> {
        let index = 0;

        const next = async (): Promise<void> => {
            if (index < this.middlewares.length) {
                const middleware = this.middlewares[index++];
                await middleware(ctx, next);
            } else {
                // 所有中间件执行完毕，执行路由处理器
                return handler(ctx) as Promise<void>;
            }
        };

        await next();
        return ctx.body;
    }

    /**
     * 处理 HTTP 请求
     * 核心处理方法，执行路由匹配、中间件链、返回响应
     *
     * @param ctx - 请求上下文对象
     * @returns 处理结果，包含状态码、响应数据、可选响应头
     *
     * @example
     * ```typescript
     * const result = await handler.handle({
     *   path: "/users",
     *   method: "GET",
     *   headers: {},
     *   query: { page: "1" },
     *   body: null
     * });
     * console.log(result.status);  // 200
     * console.log(result.data);    // { users: [] }
     * ```
     */
    async handle(ctx: RequestContext): Promise<{
        status: number;
        data: unknown;
        headers?: Record<string, string>;
    }> {
        try {
            // 匹配路由
            const route = this.matchRoute(ctx.method, ctx.path);

            if (!route) {
                return {
                    status: 404,
                    data: { error: "Not Found", path: ctx.path },
                };
            }

            // 提取路径参数并合并到 context
            const params = this.extractParams(route.path, ctx.path);
            (ctx as RequestContext & { params: Record<string, string> }).params = params;

            // 执行中间件和处理器
            const result = await this.executeMiddlewares(ctx, route.handler);

            return {
                status: 200,
                data: result,
            };
        } catch (error) {
            if (this.errorHandler) {
                const result = await this.errorHandler(error as Error, ctx);
                return {
                    status: 500,
                    data: result,
                };
            }

            return {
                status: 500,
                data: {
                    error: "Internal Server Error",
                    message: (error as Error).message,
                },
            };
        }
    }

    /**
     * 获取所有已注册的路由
     * 用于调试或路由检查
     *
     * @returns 路由定义数组副本
     *
     * @example
     * ```typescript
     * const routes = handler.getRoutes();
     * routes.forEach(r => console.log(`${r.method} ${r.path}`));
     * ```
     */
    getRoutes(): RouteDefinition[] {
        return [...this.routes];
    }
}

// ============================================
// 工厂函数
// ============================================

/**
 * 创建 HTTP 客户端实例
 * 工厂函数，简化客户端创建过程
 *
 * @param baseURL - 基础 URL
 * @param options - 配置选项
 * @param options.headers - 默认请求头
 * @param options.timeout - 默认超时时间
 * @returns HttpClient 实例
 *
 * @example
 * ```typescript
 * const client = createHttpClient("https://api.example.com", {
 *   headers: { "Content-Type": "application/json" }
 * });
 * ```
 */
export function createHttpClient(
    baseURL?: string,
    options?: {
        headers?: Record<string, string>;
        timeout?: number;
    }
): HttpClient {
    return new HttpClient(baseURL, options);
}

/**
 * 创建 HTTP 请求处理器实例
 * 工厂函数，简化处理器创建过程
 *
 * @returns HttpHandler 实例
 *
 * @example
 * ```typescript
 * const handler = createHttpHandler();
 * handler.get("/api", (ctx) => ({ data: "hello" }));
 * ```
 */
export function createHttpHandler(): HttpHandler {
    return new HttpHandler();
}

// 默认实例
let defaultHttpClient: HttpClient | null = null;
let defaultHttpHandler: HttpHandler | null = null;

/**
 * 获取默认 HTTP 客户端实例
 * 单例模式，确保全局共享同一客户端实例
 *
 * @param baseURL - 可选，设置或更新基础 URL
 * @param options - 可选配置选项
 * @returns 默认 HttpClient 实例
 *
 * @example
 * ```typescript
 * // 获取默认客户端
 * const client = getHttpClient();
 *
 * // 初始化或更新
 * const client = getHttpClient("https://api.example.com", { timeout: 5000 });
 * ```
 */
export function getHttpClient(
    baseURL?: string,
    options?: {
        headers?: Record<string, string>;
        timeout?: number;
    }
): HttpClient {
    if (!defaultHttpClient) {
        defaultHttpClient = new HttpClient(baseURL, options);
    } else if (baseURL) {
        defaultHttpClient.setBaseURL(baseURL);
    }
    return defaultHttpClient;
}

/**
 * 获取默认 HTTP 请求处理器实例
 * 单例模式，确保全局共享同一处理器实例
 *
 * @returns 默认 HttpHandler 实例
 *
 * @example
 * ```typescript
 * const handler = getHttpHandler();
 * handler.get("/users", (ctx) => ({ users: [] }));
 * ```
 */
export function getHttpHandler(): HttpHandler {
    if (!defaultHttpHandler) {
        defaultHttpHandler = new HttpHandler();
    }
    return defaultHttpHandler;
}

// ============================================
// HTTP 服务器 - 网络层实现
// ============================================

/**
 * HTTP 服务器配置接口
 */
export interface HttpServerOptions {
    /** 监听端口，默认 3000 */
    port?: number;
    /** 监听主机，默认 "0.0.0.0" */
    host?: string;
    /** 是否启用 HTTPS */
    ssl?: boolean;
    /** SSL 证书路径（启用 HTTPS 时必需） */
    certPath?: string;
    /** SSL 密钥路径（启用 HTTPS 时必需） */
    keyPath?: string;
    /** 请求体最大大小（字节），默认 10MB */
    maxBodySize?: number;
    /** 服务器超时时间（毫秒），默认 120000 */
    timeout?: number;
    /** 是否保持连接 */
    keepAlive?: boolean;
    /** 保持连接超时时间（毫秒） */
    keepAliveTimeout?: number;
}

/**
 * 服务器状态接口
 */
export interface ServerStatus {
    /** 是否正在运行 */
    running: boolean;
    /** 监听端口 */
    port?: number;
    /** 监听主机 */
    host?: string;
    /** 启动时间 */
    startTime?: Date;
    /** 已处理请求总数 */
    requestsHandled?: number;
}

/**
 * HTTP 服务器类
 * 使用 Node.js http/https 模块实现完整的网络层
 *
 * @example
 * ```typescript
 * const handler = new HttpHandler();
 * handler.get("/users", (ctx) => ({ users: [] }));
 *
 * const server = new HttpServer(handler, { port: 3000 });
 * server.start();
 *
 * // 或者使用 HTTPS
 * const sslServer = new HttpServer(handler, {
 *   port: 443,
 *   ssl: true,
 *   certPath: "./cert.pem",
 *   keyPath: "./key.pem"
 * });
 * sslServer.start();
 * ```
 */
export class HttpServer {
    private handler: HttpHandler;
    private options: HttpServerOptions;
    private server: http.Server | https.Server | null = null;
    private status: ServerStatus;
    private requestsHandled: number = 0;

    /**
     * 创建 HTTP 服务器实例
     *
     * @param handler - HTTP 请求处理器实例
     * @param options - 服务器配置选项
     */
    constructor(handler: HttpHandler, options: HttpServerOptions = {}) {
        this.handler = handler;
        this.options = {
            port: options.port ?? 3000,
            host: options.host ?? "0.0.0.0",
            ssl: options.ssl ?? false,
            certPath: options.certPath,
            keyPath: options.keyPath,
            maxBodySize: options.maxBodySize ?? 10 * 1024 * 1024, // 10MB
            timeout: options.timeout ?? 120000,
            keepAlive: options.keepAlive ?? true,
            keepAliveTimeout: options.keepAliveTimeout ?? 5000,
        };
        this.status = { running: false };
    }

    /**
     * 解析请求体
     * 支持 JSON、文本和二进制数据
     *
     * @param req - Node.js 原始请求对象
     * @param maxBodySize - 最大请求体大小
     * @returns 解析后的请求体数据
     */
    private async parseBody(
        req: http.IncomingMessage,
        maxBodySize: number
    ): Promise<unknown> {
        const contentType = req.headers["content-type"] ?? "";
        const chunks: Buffer[] = [];
        let totalSize = 0;

        return new Promise((resolve, reject) => {
            req.on("data", (chunk: Buffer) => {
                totalSize += chunk.length;
                if (totalSize > maxBodySize) {
                    req.destroy();
                    reject(new Error(`Request body too large: ${totalSize} bytes (max: ${maxBodySize})`));
                    return;
                }
                chunks.push(chunk);
            });

            req.on("end", () => {
                if (chunks.length === 0) {
                    resolve(null);
                    return;
                }

                const buffer = Buffer.concat(chunks);

                // 根据 Content-Type 解析
                if (contentType.includes("application/json")) {
                    try {
                        resolve(JSON.parse(buffer.toString()));
                    } catch (e) {
                        reject(new Error(`Invalid JSON body: ${(e as Error).message}`));
                    }
                } else if (contentType.includes("text/")) {
                    resolve(buffer.toString());
                } else if (contentType.includes("application/x-www-form-urlencoded")) {
                    const params = new URLSearchParams(buffer.toString());
                    const result: Record<string, string> = {};
                    params.forEach((value, key) => {
                        result[key] = value;
                    });
                    resolve(result);
                } else {
                    // 默认返回 Buffer（二进制数据）
                    resolve(buffer);
                }
            });

            req.on("error", reject);
        });
    }

    /**
     * 解析查询参数
     * 从 URL 中提取查询字符串参数
     *
     * @param urlString - 完整的请求 URL
     * @returns 查询参数对象
     */
    private parseQuery(urlString: string): Record<string, string> {
        const parsedUrl = url.parse(urlString, true);
        return parsedUrl.query as Record<string, string>;
    }

    /**
     * 解析请求头
     * 将 Node.js 请求头转换为简单对象
     *
     * @param req - Node.js 原始请求对象
     * @returns 请求头对象
     */
    private parseHeaders(req: http.IncomingMessage): Record<string, string> {
        const headers: Record<string, string> = {};
        for (const [key, value] of Object.entries(req.headers)) {
            if (value !== undefined) {
                headers[key] = Array.isArray(value) ? value.join(",") : value;
            }
        }
        return headers;
    }

    /**
     * 获取客户端 IP 地址
     * 支持代理服务器场景
     *
     * @param req - Node.js 原始请求对象
     * @returns 客户端 IP 地址
     */
    private getClientIP(req: http.IncomingMessage): string {
        const forwarded = req.headers["x-forwarded-for"];
        if (forwarded) {
            const ips = Array.isArray(forwarded) ? forwarded[0] : forwarded.split(",")[0];
            return ips.trim();
        }
        const socket = req.socket;
        return socket?.remoteAddress ?? "unknown";
    }

    /**
     * 构建 RequestContext
     * 将 Node.js 原始请求转换为处理器上下文
     *
     * @param req - Node.js 原始请求对象
     * @param res - Node.js 原始响应对象
     * @returns 请求上下文
     */
    private async buildContext(
        req: http.IncomingMessage,
        res: http.ServerResponse
    ): Promise<RequestContext> {
        const urlString = req.url ?? "/";
        const parsedUrl = url.parse(urlString, true);
        const path = parsedUrl.pathname ?? "/";

        const body = await this.parseBody(req, this.options.maxBodySize!);
        const query = this.parseQuery(urlString);
        const headers = this.parseHeaders(req);
        const ip = this.getClientIP(req);

        return {
            path,
            method: req.method ?? "GET",
            headers,
            query,
            body,
            ip,
            rawRequest: req,
            rawResponse: res,
        };
    }

    /**
     * 发送响应
     * 将处理结果写入 Node.js 响应对象
     *
     * @param res - Node.js 原始响应对象
     * @param statusCode - HTTP 状态码
     * @param data - 响应数据
     * @param customHeaders - 自定义响应头
     */
    private sendResponse(
        res: http.ServerResponse,
        statusCode: number,
        data: unknown,
        customHeaders?: Record<string, string>
    ): void {
        const defaultHeaders = {
            "Content-Type": "application/json; charset=utf-8",
            "X-Content-Type-Options": "nosniff",
        };

        const headers = { ...defaultHeaders, ...customHeaders };

        // 设置响应头
        for (const [key, value] of Object.entries(headers)) {
            res.setHeader(key, value);
        }

        // 设置状态码
        res.statusCode = statusCode;

        // 写入响应体
        let body: string;
        if (typeof data === "string") {
            body = data;
        } else if (Buffer.isBuffer(data)) {
            res.setHeader("Content-Type", "application/octet-stream");
            res.end(data);
            return;
        } else {
            body = JSON.stringify(data);
        }

        res.end(body);
    }

    /**
     * 处理单个请求
     * 核心请求处理流程
     *
     * @param req - Node.js 原始请求对象
     * @param res - Node.js 原始响应对象
     */
    private async handleRequest(
        req: http.IncomingMessage,
        res: http.ServerResponse
    ): Promise<void> {
        try {
            // 构建上下文
            const ctx = await this.buildContext(req, res);

            // 调用处理器
            const result = await this.handler.handle(ctx);

            // 发送响应
            this.sendResponse(res, result.status, result.data, result.headers);

            // 统计
            this.requestsHandled++;
        } catch (error) {
            // 错误处理
            const errorMessage = (error as Error).message;
            const statusCode = errorMessage.includes("too large") ? 413 : 500;

            this.sendResponse(res, statusCode, {
                error: statusCode === 413 ? "Payload Too Large" : "Internal Server Error",
                message: errorMessage,
            });
        }
    }

    /**
     * 创建 Node.js 服务器实例
     * 根据 SSL 配置创建 http 或 https 服务器
     *
     * @returns Node.js 服务器实例
     */
    private createServer(): http.Server | https.Server {
        const requestHandler = async (req: http.IncomingMessage, res: http.ServerResponse) => {
            await this.handleRequest(req, res);
        };

        if (this.options.ssl) {
            // HTTPS 服务器
            if (!this.options.certPath || !this.options.keyPath) {
                throw new Error("SSL enabled but certPath or keyPath not provided");
            }

            const cert = fs.readFileSync(this.options.certPath);
            const key = fs.readFileSync(this.options.keyPath);

            return https.createServer({ cert, key }, requestHandler);
        }

        // HTTP 服务器
        return http.createServer(requestHandler);
    }

    /**
     * 启动服务器
     * 开始监听指定端口
     *
     * @returns 服务器实例（可用于事件监听）
     *
     * @example
     * ```typescript
     * const server = new HttpServer(handler, { port: 3000 });
     * await server.start();
     * console.log("Server started on port 3000");
     * ```
     */
    async start(): Promise<http.Server | https.Server> {
        if (this.status.running) {
            throw new Error("Server is already running");
        }

        this.server = this.createServer();

        // 设置服务器选项
        this.server.timeout = this.options.timeout!;
        if (this.options.keepAlive) {
            this.server.keepAliveTimeout = this.options.keepAliveTimeout!;
        }

        return new Promise((resolve, reject) => {
            this.server!.listen(this.options.port!, this.options.host!, () => {
                this.status = {
                    running: true,
                    port: this.options.port,
                    host: this.options.host,
                    startTime: new Date(),
                    requestsHandled: 0,
                };
                console.log(
                    `HTTP Server started at http://${this.options.host}:${this.options.port}`
                );
                resolve(this.server!);
            });

            this.server!.on("error", (error: NodeJS.ErrnoException) => {
                if (error.code === "EADDRINUSE") {
                    reject(new Error(`Port ${this.options.port} is already in use`));
                } else {
                    reject(error);
                }
            });
        });
    }

    /**
     * 停止服务器
     * 关闭端口监听，停止接收新请求
     *
     * @example
     * ```typescript
     * await server.stop();
     * console.log("Server stopped");
     * ```
     */
    async stop(): Promise<void> {
        if (!this.status.running || !this.server) {
            throw new Error("Server is not running");
        }

        return new Promise((resolve, reject) => {
            this.server!.close((error) => {
                if (error) {
                    reject(error);
                } else {
                    this.status = { running: false };
                    this.server = null;
                    console.log("HTTP Server stopped");
                    resolve();
                }
            });

            // 强制关闭超时
            setTimeout(() => {
                this.server = null;
                this.status = { running: false };
                resolve();
            }, 5000);
        });
    }

    /**
     * 重启服务器
     * 先停止再重新启动
     *
     * @example
     * ```typescript
     * await server.restart();
     * ```
     */
    async restart(): Promise<http.Server | https.Server> {
        if (this.status.running) {
            await this.stop();
        }
        return this.start();
    }

    /**
     * 获取服务器状态
     * 返回当前运行状态信息
     *
     * @returns 服务器状态对象
     *
     * @example
     * ```typescript
     * const status = server.getStatus();
     * console.log(`Running: ${status.running}, Port: ${status.port}`);
     * ```
     */
    getStatus(): ServerStatus {
        return {
            ...this.status,
            requestsHandled: this.requestsHandled,
        };
    }

    /**
     * 更新处理器
     * 动态更换请求处理器（无需重启服务器）
     *
     * @param handler - 新的 HTTP 请求处理器
     */
    setHandler(handler: HttpHandler): void {
        this.handler = handler;
    }

    /**
     * 获取底层 Node.js 服务器实例
     * 用于高级配置或事件监听
     *
     * @returns Node.js 服务器实例，未启动时返回 null
     */
    getRawServer(): http.Server | https.Server | null {
        return this.server;
    }
}

// ============================================
// 服务器工厂函数
// ============================================

/**
 * 创建 HTTP 服务器实例
 * 工厂函数，简化服务器创建过程
 *
 * @param handler - HTTP 请求处理器实例
 * @param options - 服务器配置选项
 * @returns HttpServer 实例
 *
 * @example
 * ```typescript
 * const handler = createHttpHandler();
 * handler.get("/api", (ctx) => ({ data: "hello" }));
 *
 * const server = createHttpServer(handler, { port: 8080 });
 * await server.start();
 * ```
 */
export function createHttpServer(
    handler: HttpHandler,
    options?: HttpServerOptions
): HttpServer {
    return new HttpServer(handler, options);
}

// 默认服务器实例
let defaultHttpServer: HttpServer | null = null;

/**
 * 获取默认 HTTP 服务器实例
 * 单例模式，确保全局共享同一服务器实例
 *
 * @param handler - HTTP 请求处理器（可选，用于初始化）
 * @param options - 服务器配置选项（可选）
 * @returns 默认 HttpServer 实例
 *
 * @example
 * ```typescript
 * const handler = getHttpHandler();
 * handler.get("/users", (ctx) => ({ users: [] }));
 *
 * const server = getHttpServer(handler, { port: 3000 });
 * await server.start();
 * ```
 */
export function getHttpServer(
    handler?: HttpHandler,
    options?: HttpServerOptions
): HttpServer {
    if (!defaultHttpServer) {
        if (!handler) {
            handler = new HttpHandler();
        }
        defaultHttpServer = new HttpServer(handler, options);
    } else if (handler) {
        defaultHttpServer.setHandler(handler);
    }
    return defaultHttpServer;
}

/**
 * 快速启动 HTTP 服务器
 * 一行代码启动服务器，适合快速测试
 *
 * @param port - 监听端口，默认 3000
 * @param handler - 请求处理器（可选，不提供则创建默认处理器）
 * @returns 启动后的服务器实例
 *
 * @example
 * ```typescript
 * // 最简单的方式
 * const server = await quickStart(3000);
 *
 * // 带路由配置
 * const handler = createHttpHandler();
 * handler.get("/hello", (ctx) => ({ message: "Hello World" }));
 * const server = await quickStart(3000, handler);
 * ```
 */
export async function quickStart(
    port: number = 3000,
    handler?: HttpHandler
): Promise<HttpServer> {
    const server = getHttpServer(handler, { port });
    await server.start();
    return server;
}