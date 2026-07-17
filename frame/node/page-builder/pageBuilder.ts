/**
 * @file 页面构建主模块
 * @description 整合路由管理、SPA/SSR 构建和 SSR 渲染的顶层入口
 */
import * as path from "node:path";
import { PageBuildConfig, PageBuildDefaultConfig } from "../config";
import { SPAClient, SPAClientBuildResult, createSPAClient } from "./spaBuilder";
import { SSRBuild, SSRBuildResult, createSSRBuild } from "./ssrBuilder";
import { generatePageRegistry } from "./pageRegistryGenerator";
import { MatchedPage, PageRouter, generateRoutes } from "./pageRouter";
import { PageNotFoundError, SSRLoader, createSSRLoader } from "./ssrLoader";
import { PageBootstrap, SSRRenderer } from "./ssrRenderer";

/** 页面构建结果 */
export interface PageBuildResult {
    spa: SPAClientBuildResult | null;   /** SPA 构建结果 */
    ssr: SSRBuildResult | null;         /** SSR 构建结果 */
    spaError?: Error;                   /** SPA 构建错误 */
    ssrError?: Error;                   /** SSR 构建错误 */
    router: PageRouter;                 /** 页面路由器 */
    duration: number;                   /** 构建耗时（毫秒） */
    success: boolean;                   /** 是否全部成功 */
}

/** 渲染结果 */
export interface RenderResult {
    html: string;           /** 完整 HTML */
    requestRoute: string;   /** 原始请求路由 */
    matchedRoute: string;   /** 匹配到的路由模式 */
    pageName: string;       /** 页面名称 */
}

/**
 * 页面构建器
 * @description 管理路由扫描、SPA/SSR 并行构建和 SSR 渲染
 */
export class PageBuild {
    private router = new PageRouter();
    private readonly spaBuilder: SPAClient;
    private readonly ssrBuilder: SSRBuild;
    private readonly loader: SSRLoader;
    private readonly renderer: SSRRenderer;

    /** @param config - 构建配置，默认使用 PageBuildDefaultConfig */
    constructor(private readonly config: PageBuildConfig = PageBuildDefaultConfig) {
        this.spaBuilder = createSPAClient(config.spa);
        this.ssrBuilder = createSSRBuild(config.ssr);
        const bundlePath = path.resolve(process.cwd(), config.ssr.outFile ?? "./build/entry-server.js");
        this.loader = createSSRLoader(bundlePath);
        this.renderer = new SSRRenderer({
            loader: this.loader,
            clientScriptPath: config.spa.publicPath ?? "/assets/entry-client.js",
            isDev: config.isDev,
        });
    }

    /**
     * 初始化页面路由：扫描目录 + 生成注册表文件
     * @returns PageRouter 实例
     */
    async initRouter(): Promise<PageRouter> {
        const pagesDir = path.resolve(process.cwd(), this.config.pagesDir);
        this.router = await generateRoutes(pagesDir);
        await generatePageRegistry(this.router.getPages(), {
            outputPath: path.resolve(process.cwd(), ".generated/pageRegistry.ts"),
        });
        return this.router;
    }

    /**
     * 执行完整构建（路由初始化 + SPA/SSR 并行构建）
     * @returns 构建结果
     */
    async build(): Promise<PageBuildResult> {
        const startTime = Date.now();
        await this.initRouter();
        const [spa, ssr] = await Promise.allSettled([
            this.spaBuilder.buildAndSave(),
            this.ssrBuilder.buildAndSave(),
        ]);
        const spaError = spa.status === "rejected" ? toError(spa.reason) : undefined;
        const ssrError = ssr.status === "rejected" ? toError(ssr.reason) : undefined;
        return {
            spa: spa.status === "fulfilled" ? spa.value : null,
            ssr: ssr.status === "fulfilled" ? ssr.value : null,
            spaError,
            ssrError,
            router: this.router,
            duration: Date.now() - startTime,
            success: !spaError && !ssrError,
        };
    }

    /**
     * 执行 SSR 渲染
     * @param requestRoute - 请求路由
     * @param bootstrap - 启动数据
     * @returns 渲染结果
     * @throws 路由未匹配时抛出 PageNotFoundError
     */
    async render(requestRoute: string, bootstrap: PageBootstrap): Promise<RenderResult> {
        const matchedPage = this.router.getPageByRoute(requestRoute);
        if (!matchedPage) throw new PageNotFoundError(requestRoute);

        const finalBootstrap: PageBootstrap = {
            ...bootstrap,
            route: requestRoute,
            params: { ...bootstrap.params, ...matchedPage.params },
        };
        const result = await this.renderer.render({ bootstrap: finalBootstrap, matchedPage });
        return {
            html: result.html,
            requestRoute,
            matchedRoute: matchedPage.page.route,
            pageName: matchedPage.page.name,
        };
    }

    /** 获取路由器实例 */
    getRouter(): PageRouter { return this.router; }

    /** 根据路由获取匹配的页面信息 */
    getMatchedPage(route: string): MatchedPage | null { return this.router.getPageByRoute(route); }

    /** 清除 SSR bundle 缓存 */
    clearCache(): void { this.renderer.clearCache(); }
}

/**
 * 创建页面构建器实例
 * @param config - 构建配置
 * @returns PageBuild 实例
 */
export function createPageBuild(config?: PageBuildConfig): PageBuild {
    return new PageBuild(config ?? PageBuildDefaultConfig);
}

/** 将未知值转换为 Error 实例 */
function toError(value: unknown): Error {
    return value instanceof Error ? value : new Error(String(value));
}
