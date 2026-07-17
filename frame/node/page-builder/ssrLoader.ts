/**
 * @file SSR Bundle 加载器
 * @description 加载和缓存 SSR 构建产物，提供按路由查找页面模块和获取根 App 组件的能力
 */
import * as path from "node:path";

/** 页面模块接口 */
export interface PageModule {
    default?: unknown;                                                           /** 页面组件 */
    getInitialProps?: (bootstrap: unknown) => Promise<unknown> | unknown;        /** 服务端数据预取函数 */
    metadata?: Record<string, unknown>;                                          /** 页面元数据 */
    [key: string]: unknown;
}

/** SSR Bundle 模块接口 */
export interface SSRBundleModule {
    default?: unknown;                                              /** 根 App 组件 */
    getPageModule?: (route: string) => PageModule | null;          /** 根据路由获取页面模块 */
}

/** 页面未找到错误 */
export class PageNotFoundError extends Error {
    /** @param route - 未匹配的请求路由 */
    constructor(route: string) {
        super(`Page route not found: ${route}`);
        this.name = "PageNotFoundError";
    }
}

/**
 * SSR Bundle 加载器
 * @description 生产模式缓存 bundle，开发模式每次重新加载以支持热更新
 */
export class SSRLoader {
    private bundlePath: string;
    private bundleCache: SSRBundleModule | null = null;

    /** @param bundlePath - SSR bundle 文件路径 */
    constructor(bundlePath: string) {
        this.bundlePath = path.resolve(bundlePath);
    }

    /**
     * 加载 SSR bundle
     * @param isDev - 是否为开发模式
     * @returns bundle 模块对象
     * @throws 加载失败时抛出 Error
     */
    loadBundle(isDev: boolean): SSRBundleModule {
        if (isDev) this.clearCache();
        if (this.bundleCache) return this.bundleCache;
        try {
            const loaded = require(this.bundlePath) as SSRBundleModule;
            this.bundleCache = loaded;
            return loaded;
        } catch (error) {
            throw new Error(`Failed to load SSR bundle: ${(error as Error).message}`);
        }
    }

    /**
     * 根据路由加载页面模块
     * @param route - 请求路由
     * @param isDev - 是否为开发模式
     * @returns 页面模块
     * @throws 路由未找到时抛出 PageNotFoundError
     */
    loadPage(route: string, isDev: boolean): PageModule {
        const bundle = this.loadBundle(isDev);
        const page = bundle.getPageModule?.(route);
        if (!page) throw new PageNotFoundError(route);
        return page;
    }

    /**
     * 获取根 App 组件
     * @param isDev - 是否为开发模式
     * @returns App 组件
     * @throws bundle 无 default 导出时抛出 Error
     */
    getApp(isDev: boolean): unknown {
        const bundle = this.loadBundle(isDev);
        if (!bundle.default) throw new Error("SSR bundle has no default App export");
        return bundle.default;
    }

    /** 清除 bundle 缓存（含 Node.js require 缓存） */
    clearCache(): void {
        this.bundleCache = null;
        const resolved = require.resolve(this.bundlePath);
        delete require.cache[resolved];
    }
}

/**
 * 创建 SSR 加载器实例
 * @param bundlePath - bundle 文件路径
 * @returns SSRLoader 实例
 */
export function createSSRLoader(bundlePath: string): SSRLoader {
    return new SSRLoader(bundlePath);
}
