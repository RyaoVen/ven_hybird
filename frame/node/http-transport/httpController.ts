/**
 * @file HTTP 渲染控制器模块
 * @description SSR 渲染服务的 HTTP 接入层，接收渲染请求、控制并发、异步执行并通过回调返回结果
 */
import { HttpClient, HttpHandler, HttpServer, HttpServerOptions } from "./httpClient";
import { RenderExecutionGate } from "./renderExecutionGate";
import { PagePatternList, RenderCallback, RenderError, RenderTask } from "./types";

/** 渲染任务接收路由 */
const renderRoute = "/render";

/** 页面路由模式列表查询路由（nodePagesPattern） */
const pagesRoute = "/pages";

/** HTTP 控制器配置 */
export interface HttpControllerOptions {
    server: HttpServerOptions;          /** HTTP 服务器配置 */
    callbackURL: string;                /** 渲染结果回调 URL */
    internalToken?: string;             /** 内部通信 token */
    callbackTimeout: number;            /** 回调超时（毫秒） */
    maxConcurrentRenders: number;       /** 最大并发渲染数 */
}

/** 渲染处理函数类型 */
export type RenderHandler = (task: RenderTask) => Promise<{
    html: string;
    requestRoute: string;
    matchedRoute: string;
    pageName: string;
}>;

/** 页面路由模式提供函数，返回全部页面路由模板（nodePagesPattern） */
export type PagePatternProvider = () => string[];

/**
 * HTTP 渲染控制器
 * @description 管理渲染请求的接收、验证、并发控制和异步回调
 */
export class HttpController {
    private readonly httpHandler = new HttpHandler();
    private readonly callbackClient: HttpClient;
    private readonly gate: RenderExecutionGate;
    private server: HttpServer | null = null;

    /** @param options - 控制器配置 */
    constructor(private readonly options: HttpControllerOptions) {
        this.callbackClient = new HttpClient(options.callbackURL, {
            timeout: options.callbackTimeout,
            headers: { "Content-Type": "application/json" },
        });
        this.gate = new RenderExecutionGate(options.maxConcurrentRenders);
    }

    /**
     * 注册路由并启动 HTTP 服务器
     * @description GET /health 健康检查；POST /render 接收渲染任务（异步执行，立即返回 202）；
     * 传入 pagePatterns 时额外注册 GET /pages 返回全部页面路由模式
     * @param renderHandler - 渲染处理函数
     * @param pagePatterns - 页面路由模式提供函数（可选）
     */
    async requestDeal(renderHandler: RenderHandler, pagePatterns?: PagePatternProvider): Promise<void> {
        this.httpHandler.get("/health", () => ({
            status: "ok",
            activeRenders: this.gate.size(),
        }));

        if (pagePatterns) {
            this.httpHandler.get(pagesRoute, (ctx) => {
                if (!this.isInternalRequest(ctx.headers)) {
                    return { status: 401, data: { error: "Invalid internal token" } };
                }
                const data: PagePatternList = { patterns: pagePatterns() };
                return { status: 200, data };
            });
        }

        this.httpHandler.post(renderRoute, async (ctx) => {
            if (!this.isInternalRequest(ctx.headers)) {
                return { status: 401, data: { error: "Invalid internal token" } };
            }

            const task = ctx.body as Partial<RenderTask> | null;
            if (!task || !isRenderTask(task)) {
                return { status: 400, data: { error: "hookId, requestRoute and payload are required" } };
            }

            const acquired = this.gate.acquire(task.hookId);
            if (!acquired.ok) {
                return {
                    status: acquired.reason === "duplicate" ? 409 : 503,
                    data: { error: acquired.reason === "duplicate" ? "Render task already exists" : "Render worker is busy" },
                };
            }

            void this.processRenderTask(task, renderHandler)
                .catch((error) => console.error("Unexpected render task failure:", (error as Error).message))
                .finally(() => this.gate.release(task.hookId));

            return { status: 202, data: { status: "accepted", hookId: task.hookId } };
        });

        this.server = new HttpServer(this.httpHandler, this.options.server);
        await this.server.start();
    }

    /** 获取 HTTP 服务器实例 */
    getServer(): HttpServer | null { return this.server; }

    /**
     * 异步处理渲染任务并通过回调返回结果
     * @param task - 渲染任务
     * @param renderHandler - 渲染处理函数
     */
    private async processRenderTask(task: RenderTask, renderHandler: RenderHandler): Promise<void> {
        const startAt = Date.now();
        try {
            const result = await renderHandler(task);
            await this.postCallback({
                hookId: task.hookId,
                requestRoute: result.requestRoute,
                matchedRoute: result.matchedRoute,
                pageName: result.pageName,
                html: result.html,
                duration: Date.now() - startAt,
            });
        } catch (error) {
            const renderError = toRenderError(error);
            try {
                await this.postCallback({
                    hookId: task.hookId, requestRoute: task.requestRoute,
                    html: "", error: renderError, duration: Date.now() - startAt,
                });
            } catch (callbackError) {
                console.error("Render callback delivery failed:", (callbackError as Error).message);
            }
        }
    }

    /**
     * 发送渲染结果回调
     * @param callback - 回调数据
     * @throws 非 2xx 响应时抛出 Error
     */
    private async postCallback(callback: RenderCallback): Promise<void> {
        const headers: Record<string, string> = { "Content-Type": "application/json" };
        if (this.options.internalToken) headers["X-Ven-Internal-Token"] = this.options.internalToken;
        const response = await this.callbackClient.post("", callback, { headers });
        if (response.status < 200 || response.status >= 300) {
            throw new Error(`Render callback rejected: HTTP ${response.status}`);
        }
    }

    /** 验证内部请求 token */
    private isInternalRequest(headers: Record<string, string>): boolean {
        if (!this.options.internalToken) return true;
        return headers["x-ven-internal-token"] === this.options.internalToken;
    }
}

/** 类型守卫：验证请求体是否为合法 RenderTask */
function isRenderTask(task: Partial<RenderTask>): task is RenderTask {
    return typeof task.hookId === "string" && task.hookId.length > 0 &&
        typeof task.requestRoute === "string" && task.requestRoute.startsWith("/") &&
        isBootstrap(task.payload);
}

/** 类型守卫：验证值是否为合法 PageBootstrap */
function isBootstrap(payload: unknown): payload is RenderTask["payload"] {
    if (!payload || typeof payload !== "object") return false;
    const value = payload as RenderTask["payload"];
    return typeof value.route === "string" &&
        typeof value.params === "object" && value.params !== null &&
        typeof value.query === "object" && value.query !== null;
}

/** 将未知错误转换为 RenderError */
function toRenderError(error: unknown): RenderError {
    if (error instanceof Error && error.name === "PageNotFoundError") {
        return { code: "PAGE_NOT_FOUND", message: error.message };
    }
    return {
        code: "RENDER_FAILED",
        message: error instanceof Error ? error.message : String(error),
    };
}
