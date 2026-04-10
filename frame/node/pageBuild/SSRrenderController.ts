import { httpController } from "../httpClient/controller";
import { HybridRenderer, HybridRenderResult, HybridConfig } from "./Hybrid";
import { SSRRenderer, SSRRenderConfig } from "./SSRrender";
import { SSRLoader } from "./SSRload";
import { PageRouter, SSRPage } from "./routerGenerate";

/**
 * 过滤器模式
 * - none: 不过滤，允许所有路由
 * - whitelist: 白名单模式，只允许列表中的路由
 * - blacklist: 黑名单模式，禁止列表中的路由
 */
export type FilterMode = "none" | "whitelist" | "blacklist";

/**
 * 加载配置接口
 */
export interface LoadConfig {
    /** 过滤器模式，默认 none */
    filterMode: FilterMode;
    /** 过滤器路由列表 */
    filterRoutes: string[];
    /** 是否启用预加载 */
    preload: boolean;
    /** 预加载路由列表 */
    preloadRoutes: string[];
    /** 是否启用慢加载 */
    slowLoad: boolean;
    /** 慢加载路由列表 */
    slowLoadRoutes: string[];
}

/**
 * SSRrenderController 配置接口
 */
export interface SSRrenderControllerConfig {
    /** 页面输出目录 */
    pagesDir?: string;
    /** 文件扩展名 */
    ext?: string;
    /** 是否开发模式 */
    isDev?: boolean;
    /** SSR 渲染配置 */
    ssrConfig?: Partial<SSRRenderConfig>;
    /** Hybrid 渲染配置 */
    hybridConfig?: Partial<HybridConfig>;
    /** 加载配置 */
    loadConfig?: LoadConfig;
    /** 页面路由管理器 */
    router?: PageRouter;
}

/**
 * 默认加载配置
 */
export const defaultLoadConfig: LoadConfig = {
    filterMode: "none",
    filterRoutes: [],
    preload: false,
    preloadRoutes: [],
    slowLoad: false,
    slowLoadRoutes: [],
};

/**
 * 默认 SSRrenderController 配置
 */
export const defaultControllerConfig: SSRrenderControllerConfig = {
    pagesDir: "dist/pages",
    ext: ".js",
    isDev: false,
    loadConfig: defaultLoadConfig,
};

/**
 * 渲染请求结果
 */
export interface RenderRequestResult {
    /** HTML 内容 */
    html: string;
    /** 路由路径 */
    route: string;
    /** 是否来自缓存 */
    fromCache: boolean;
    /** 渲染耗时（毫秒） */
    duration?: number;
    /** 错误信息 */
    error?: string;
}

/**
 * SSRrenderController - SSR 渲染控制器
 * 整合路由系统、预加载、过滤、慢加载、渲染等业务
 */
export class SSRrenderController {
    /** HTTP 控制器 */
    private controller: httpController;
    /** SSR 加载器 */
    private loader: SSRLoader;
    /** SSR 渲染器 */
    private renderer: SSRRenderer;
    /** Hybrid 渲染器 */
    private hybridRenderer: HybridRenderer;
    /** 页面路由管理器 */
    private router: PageRouter;
    /** 预加载的 HTML 缓存 Map */
    private preloadMap: Map<string, string> = new Map();
    /** 慢加载队列 */
    private slowLoadQueue: Map<string, Promise<HybridRenderResult>> = new Map();
    /** 是否开发模式 */
    private isDev: boolean;
    /** 加载配置 */
    private loadConfig: LoadConfig;

    /**
     * 创建 SSRrenderController 实例
     * @param config - 配置项
     */
    constructor(config: SSRrenderControllerConfig = defaultControllerConfig) {
        this.isDev = config.isDev ?? false;
        this.loadConfig = config.loadConfig ?? defaultLoadConfig;

        // 1. 创建 HTTP 控制器
        this.controller = new httpController();

        // 2. 创建页面路由管理器
        this.router = config.router ?? new PageRouter();

        // 3. 创建 SSR 加载器
        this.loader = new SSRLoader(config.pagesDir ?? "dist/pages", config.ext ?? ".js");

        // 4. 创建 SSR 渲染器配置
        const ssrConfig: SSRRenderConfig = {
            loader: this.loader,
            isDev: this.isDev,
            ...config.ssrConfig,
        };

        // 5. 创建 SSR 渲染器
        this.renderer = new SSRRenderer(ssrConfig);

        // 6. 创建 Hybrid 渲染器并配置
        const hybridConfig: HybridConfig = {
            renderer: this.renderer,
            loader: this.loader,
            ...config.hybridConfig,
        };
        this.hybridRenderer = new HybridRenderer(hybridConfig);
    }

    /**
     * 初始化路由系统
     * 注册所有页面路由到加载器
     */
    initRoutes(): void {
        const pages = this.router.getPages();
        this.loader.registerPages(pages);
    }

    /**
     * 注册单个页面
     * @param page - 页面定义
     */
    registerPage(page: SSRPage): void {
        this.router.registerPage(page);
        this.loader.registerPage(page);
    }

    /**
     * 批量注册页面
     * @param pages - 页面定义数组
     */
    registerPages(pages: SSRPage[]): void {
        this.router.registerPages(pages);
        this.loader.registerPages(pages);
    }

    /**
     * 设置页面路由管理器
     * @param router - PageRouter 实例
     */
    setRouter(router: PageRouter): void {
        this.router = router;
        this.initRoutes();
    }

    /**
     * 过滤路由检查
     * @param route - 请求路由
     * @returns 是否允许访问
     */
    private checkRouteFilter(route: string): boolean {
        const { filterMode, filterRoutes } = this.loadConfig;

        switch (filterMode) {
            case "whitelist":
                // 白名单模式：只允许列表中的路由
                return filterRoutes.some((allowed) => this.matchRoute(allowed, route));
            case "blacklist":
                // 黑名单模式：禁止列表中的路由
                return !filterRoutes.some((blocked) => this.matchRoute(blocked, route));
            case "none":
            default:
                // 无过滤：允许所有路由
                return true;
        }
    }

    /**
     * 路径匹配（支持动态路由）
     * @param pattern - 路由模式（如 /post/:id）
     * @param route - 实际路由（如 /post/123）
     * @returns 是否匹配
     */
    private matchRoute(pattern: string, route: string): boolean {
        const patternParts = pattern.split("/").filter(Boolean);
        const routeParts = route.split("/").filter(Boolean);

        if (patternParts.length !== routeParts.length) {
            return false;
        }

        for (let i = 0; i < patternParts.length; i++) {
            const p = patternParts[i];
            const r = routeParts[i];

            // 动态参数匹配
            if (p.startsWith(":")) continue;
            // 精确匹配
            if (p !== r) return false;
        }

        return true;
    }

    /**
     * 检查是否为慢加载路由
     * @param route - 路由路径
     * @returns 是否需要慢加载
     */
    private isSlowLoadRoute(route: string): boolean {
        if (!this.loadConfig.slowLoad) return false;
        return this.loadConfig.slowLoadRoutes.some((sl) => this.matchRoute(sl, route));
    }

    /**
     * 执行预加载业务
     * 预渲染指定路由的页面并缓存结果
     * @param routes - 需要预加载的路由列表，默认使用配置中的列表
     */
    async preloadPages(routes?: string[]): Promise<void> {
        const targetRoutes = routes ?? this.loadConfig.preloadRoutes;

        if (targetRoutes.length === 0) return;

        console.log(`[Preload] Starting preload for ${targetRoutes.length} routes...`);

        // 并行预加载所有路由
        const preloadPromises = targetRoutes.map(async (route) => {
            try {
                const startTime = Date.now();
                const result = await this.hybridRenderer.hybridRender(route, {});
                this.preloadMap.set(route, result.html);
                console.log(`[Preload] Route "${route}" loaded in ${Date.now() - startTime}ms`);
            } catch (error) {
                console.warn(`[Preload] Failed for route "${route}": ${(error as Error).message}`);
            }
        });

        await Promise.all(preloadPromises);
        console.log("[Preload] Preload completed.");
    }

    /**
     * 执行自动预加载（如果配置中启用）
     */
    async autoPreload(): Promise<void> {
        if (this.loadConfig.preload) {
            await this.preloadPages();
        }
    }

    /**
     * 慢加载处理
     * 对慢加载路由进行后台渲染，返回占位或等待结果
     * @param route - 路由路径
     * @param json - 要注入的 JSON 数据
     * @returns 渲染结果
     */
    private async handleSlowLoad(route: string, json: unknown): Promise<HybridRenderResult> {
        // 检查是否已有正在进行的慢加载任务
        if (this.slowLoadQueue.has(route)) {
            return this.slowLoadQueue.get(route)!;
        }

        // 创建新的慢加载任务
        const loadPromise = this.hybridRenderer.hybridRender(route, json).finally(() => {
            // 完成后从队列移除
            this.slowLoadQueue.delete(route);
        });

        this.slowLoadQueue.set(route, loadPromise);
        return loadPromise;
    }

    /**
     * 清除预加载缓存
     * @param route - 可选的路由路径，不传则清除所有缓存
     */
    clearPreloadCache(route?: string): void {
        if (route) {
            this.preloadMap.delete(route);
            this.loader.clearCache(route);
            this.hybridRenderer.clearCache(route);
        } else {
            this.preloadMap.clear();
            this.loader.clearAllCache();
            this.hybridRenderer.clearCache();
        }
    }

    /**
     * 清除慢加载队列
     * @param route - 可选的路由路径，不传则清除所有队列
     */
    clearSlowLoadQueue(route?: string): void {
        if (route) {
            this.slowLoadQueue.delete(route);
        } else {
            this.slowLoadQueue.clear();
        }
    }

    /**
     * 清除所有缓存和队列
     */
    clearAll(): void {
        this.clearPreloadCache();
        this.clearSlowLoadQueue();
    }

    /**
     * 设置开发模式
     * @param isDev - 是否开发模式
     */
    setDevMode(isDev: boolean): void {
        this.isDev = isDev;
        this.renderer.setDevMode(isDev);
    }

    /**
     * 更新加载配置
     * @param config - 新的加载配置（部分）
     */
    updateLoadConfig(config: Partial<LoadConfig>): void {
        this.loadConfig = { ...this.loadConfig, ...config };
    }

    /**
     * 设置 SPA 客户端脚本路径
     * @param path - 脚本路径
     */
    setSpaClientScriptPath(path: string): void {
        this.hybridRenderer.setSpaClientScriptPath(path);
    }

    /**
     * 设置 SPA 客户端脚本类型
     * @param type - 脚本类型
     */
    setSpaClientScriptType(type: "module" | "text/javascript"): void {
        this.hybridRenderer.setSpaClientScriptType(type);
    }

    /**
     * 获取页面定义
     * @param route - 路由路径
     * @returns 页面定义，未找到返回 null
     */
    getPage(route: string): SSRPage | null {
        return this.router.getPageByRoute(route);
    }

    /**
     * 获取所有页面列表
     * @returns 页面定义数组
     */
    getPages(): SSRPage[] {
        return this.router.getPages();
    }

    /**
     * 请求处理
     * @param route - 路由路径
     * @param json - 要注入的 JSON 数据
     * @returns 渲染结果
     */
    async requestDeal(route: string, json: unknown): Promise<RenderRequestResult> {
        const startTime = Date.now();

        // 1. 路由过滤检查
        if (!this.checkRouteFilter(route)) {
            return {
                html: "",
                route,
                fromCache: false,
                error: `Route "${route}" is blocked by filter`,
            };
        }

        // 2. 查找预加载缓存
        const cachedHtml = this.preloadMap.get(route);
        if (cachedHtml) {
            return {
                html: cachedHtml,
                route,
                fromCache: true,
                duration: Date.now() - startTime,
            };
        }

        // 3. 检查是否为慢加载路由
        if (this.isSlowLoadRoute(route)) {
            try {
                const result = await this.handleSlowLoad(route, json);
                return {
                    html: result.html,
                    route: result.route,
                    fromCache: false,
                    duration: Date.now() - startTime,
                };
            } catch (error) {
                return {
                    html: "",
                    route,
                    fromCache: false,
                    error: `Slow load failed: ${(error as Error).message}`,
                    duration: Date.now() - startTime,
                };
            }
        }

        // 4. 正常渲染流程
        try {
            const result = await this.hybridRenderer.hybridRender(route, json);
            return {
                html: result.html,
                route: result.route,
                fromCache: false,
                duration: Date.now() - startTime,
            };
        } catch (error) {
            return {
                html: "",
                route,
                fromCache: false,
                error: `Render failed: ${(error as Error).message}`,
                duration: Date.now() - startTime,
            };
        }
    }

    /**
     * 批量渲染请求
     * @param requests - 渲染请求列表
     * @returns 渲染结果列表
     */
    async batchRequestDeal(
        requests: Array<{ route: string; json: unknown }>
    ): Promise<RenderRequestResult[]> {
        const results = await Promise.all(
            requests.map((req) => this.requestDeal(req.route, req.json))
        );
        return results;
    }

    /**
     * 获取 HTTP 控制器
     * @returns httpController 实例
     */
    getHttpController(): httpController {
        return this.controller;
    }

    /**
     * 获取 SSR 渲染器
     * @returns SSRRenderer 实例
     */
    getRenderer(): SSRRenderer {
        return this.renderer;
    }

    /**
     * 获取 Hybrid 渲染器
     * @returns HybridRenderer 实例
     */
    getHybridRenderer(): HybridRenderer {
        return this.hybridRenderer;
    }

    /**
     * 获取 SSR 加载器
     * @returns SSRLoader 实例
     */
    getLoader(): SSRLoader {
        return this.loader;
    }

    /**
     * 获取页面路由管理器
     * @returns PageRouter 实例
     */
    getRouter(): PageRouter {
        return this.router;
    }

    /**
     * 获取预加载缓存状态
     * @returns 已缓存的路由列表
     */
    getPreloadedRoutes(): string[] {
        return Array.from(this.preloadMap.keys());
    }

    /**
     * 获取慢加载队列状态
     * @returns 正在加载的路由列表
     */
    getSlowLoadingRoutes(): string[] {
        return Array.from(this.slowLoadQueue.keys());
    }
}

/**
 * 创建 SSRrenderController 实例
 * @param config - 配置项
 * @returns SSRrenderController 实例
 */
export function createSSRrenderController(
    config?: SSRrenderControllerConfig
): SSRrenderController {
    return new SSRrenderController(config ?? defaultControllerConfig);
}

// 默认实例
let defaultController: SSRrenderController | null = null;

/**
 * 获取默认 SSRrenderController 实例
 * @param config - 可选配置，用于初始化或重新配置
 * @returns SSRrenderController 实例
 */
export function getSSRrenderController(
    config?: SSRrenderControllerConfig
): SSRrenderController {
    if (!defaultController || config) {
        defaultController = new SSRrenderController(config ?? defaultControllerConfig);
    }
    return defaultController;
}