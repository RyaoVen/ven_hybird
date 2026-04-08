import * as path from "node:path";
import { PageBuildConfig, PageBuildDefaultConfig } from "../config";
import {
    SSRBuild,
    SSRBuildResult,
    createSSRBuild,
} from "./SSRbuild";
import {
    SPAClient,
    SPAClientBuildResult,
    createSPAClient,
} from "./SPAbuild";
import { PageRouter, generateRoutes, createPageRouter } from "./routerGenerate";
import { SSRLoader, createSSRLoader } from "./SSRload";

/**
 * PageBuild 构建结果
 */
export interface PageBuildResult {
    /** SPA 客户端构建结果 */
    spa: SPAClientBuildResult | null;
    /** SSR 服务端构建结果 */
    ssr: SSRBuildResult | null;
    /** SPA 构建错误 */
    spaError?: Error;
    /** SSR 构建错误 */
    ssrError?: Error;
    /** 页面路由表 */
    router: PageRouter;
    /** 构建耗时（毫秒） */
    duration: number;
    /** 整体是否成功 */
    success: boolean;
}

/**
 * PageBuild 运行时上下文
 */
export interface PageBuildContext {
    /** 配置 */
    config: PageBuildConfig;
    /** SPA 构建器 */
    spaBuilder: SPAClient;
    /** SSR 构建器 */
    ssrBuilder: SSRBuild;
    /** 页面路由管理器 */
    router: PageRouter;
    /** 模块加载器 */
    loader: SSRLoader;
}

/**
 * PageBuild - 页面构建入口
 * 整合 SPA 构建、SSR 构建、路由生成、模块加载
 */
export class PageBuild {
    private config: PageBuildConfig;
    private spaBuilder: SPAClient;
    private ssrBuilder: SSRBuild;
    private router: PageRouter;
    private loader: SSRLoader;

    /**
     * 创建 PageBuild 实例
     * @param config - 构建配置，默认使用 PageBuildDefaultConfig
     */
    constructor(config: PageBuildConfig = PageBuildDefaultConfig) {
        this.config = config;
        this.spaBuilder = createSPAClient(config.spa);
        this.ssrBuilder = createSSRBuild(config.ssr);
        this.router = createPageRouter();
        this.loader = createSSRLoader(config.ssrPagesDir);
    }

    /**
     * 获取运行时上下文
     * @returns 包含所有构建组件的上下文对象
     */
    getContext(): PageBuildContext {
        return {
            config: this.config,
            spaBuilder: this.spaBuilder,
            ssrBuilder: this.ssrBuilder,
            router: this.router,
            loader: this.loader,
        };
    }

    /**
     * 初始化页面路由
     * 扫描页面目录并生成路由表
     */
    async initRouter(): Promise<PageRouter> {
        const pagesDir = path.resolve(__dirname, this.config.pagesDir);
        this.router = await generateRoutes(pagesDir);
        return this.router;
    }

    /**
     * 构建 SPA 客户端
     * @returns 构建结果，失败时返回 null 并设置 error
     */
    async buildSPA(): Promise<{ result: SPAClientBuildResult | null; error?: Error }> {
        try {
            const result = await this.spaBuilder.buildAndSave();
            return { result };
        } catch (error) {
            return { result: null, error: error as Error };
        }
    }

    /**
     * 构建 SSR 服务端
     * @returns 构建结果，失败时返回 null 并设置 error
     */
    async buildSSR(): Promise<{ result: SSRBuildResult | null; error?: Error }> {
        try {
            const result = await this.ssrBuilder.buildAndSave();
            return { result };
        } catch (error) {
            return { result: null, error: error as Error };
        }
    }

    /**
     * 执行完整构建流程
     * 依次执行：路由初始化 -> SPA 构建 -> SSR 构建（并行）
     */
    async build(): Promise<PageBuildResult> {
        const startTime = Date.now();

        // 1. 初始化路由
        await this.initRouter();

        // 2. 并行构建 SPA 和 SSR
        const [spaResult, ssrResult] = await Promise.all([
            this.buildSPA(),
            this.buildSSR(),
        ]);

        const duration = Date.now() - startTime;

        return {
            spa: spaResult.result,
            ssr: ssrResult.result,
            spaError: spaResult.error,
            ssrError: ssrResult.error,
            router: this.router,
            duration,
            success: !spaResult.error && !ssrResult.error,
        };
    }

    /**
     * 加载指定路由的页面模块
     * @param route - 页面路由
     * @returns 页面模块对象
     */
    loadPage(route: string) {
        return this.loader.load(route, this.config.isDev);
    }

    /**
     * 加载页面模块（需先通过路由获取页面信息）
     * @param route - 页面路由
     * @returns 页面模块对象
     * @throws 页面路由不存在时抛出错误
     */
    loadPageByRoute(route: string) {
        const page = this.router.getPageByRoute(route);
        if (!page) {
            throw new Error(`Page route not found: ${route}`);
        }
        // 注册路由映射后加载
        this.loader.registerPage(page);
        return this.loader.load(page.route, this.config.isDev);
    }

    /**
     * 根据路由获取页面定义
     * @param route - 请求路由路径
     * @returns 页面定义，未找到返回 null
     */
    getPageByRoute(route: string) {
        return this.router.getPageByRoute(route);
    }

    /**
     * 获取所有页面列表
     * @returns 页面定义数组
     */
    getPages() {
        return this.router.getPages();
    }

    /**
     * 清除模块缓存（用于开发模式热更新）
     */
    clearCache(): void {
        this.loader.clearAllCache();
    }

    /**
     * 更新配置
     * @param config - 新的配置对象
     */
    updateConfig(config: Partial<PageBuildConfig>): void {
        this.config = { ...this.config, ...config };

        // 根据新配置重建构建器
        this.spaBuilder = createSPAClient(this.config.spa);
        this.ssrBuilder = createSSRBuild(this.config.ssr);
        this.loader = createSSRLoader(this.config.ssrPagesDir);
    }

    /**
     * 设置开发模式
     * @param isDev - 是否开发模式
     */
    setDevMode(isDev: boolean): void {
        this.config.isDev = isDev;
    }
}

/**
 * 创建 PageBuild 实例
 * @param config - 构建配置
 * @returns PageBuild 实例
 */
export function createPageBuild(config?: PageBuildConfig): PageBuild {
    return new PageBuild(config ?? PageBuildDefaultConfig);
}

// 默认实例
let defaultInstance: PageBuild | null = null;

/**
 * 获取默认 PageBuild 实例
 * @param config - 可选配置，用于初始化或重新配置
 * @returns PageBuild 实例
 */
export function getPageBuild(config?: PageBuildConfig): PageBuild {
    if (!defaultInstance || config) {
        defaultInstance = new PageBuild(config ?? PageBuildDefaultConfig);
    }
    return defaultInstance;
}