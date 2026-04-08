import * as path from "node:path";
import { SSRPage } from "./routerGenerate";

/**
 * 页面模块接口
 */
export interface PageModule {
    /** 页面组件渲染函数 */
    render?: (props?: unknown) => unknown;
    /** 页面组件默认导出 */
    default?: unknown;
    /** 页面数据获取函数 */
    getInitialProps?: (context?: unknown) => Promise<unknown>;
    /** 页面元数据 */
    metadata?: Record<string, unknown>;
    /** 其他导出 */
    [key: string]: unknown;
}

/**
 * SSR 模块加载选项
 */
export interface SSRLoadOptions {
    /** 页面路由（唯一标识） */
    route: string;
    /** 页面文件名（不含扩展名） */
    pageName?: string;
    /** 是否开发环境，开发环境会清除缓存支持热更新 */
    isDev: boolean;
    /** 页面输出目录，默认 "dist/pages" */
    pagesDir?: string;
    /** 文件扩展名，默认 ".js" */
    ext?: string;
}

/**
 * SSR 模块加载器
 * 负责加载编译后的页面模块，支持开发环境热更新
 * 使用 route 作为唯一标识，避免同名页面冲突
 */
export class SSRLoader {
    private pagesDir: string;
    private ext: string;
    /** 模块缓存，key 为 route */
    private moduleCache: Map<string, PageModule> = new Map();
    /** route 到文件路径的映射 */
    private routePathMap: Map<string, string> = new Map();

    /**
     * 创建模块加载器实例
     * @param pagesDir - 页面输出目录
     * @param ext - 文件扩展名
     */
    constructor(pagesDir: string = "dist/pages", ext: string = ".js") {
        this.pagesDir = pagesDir;
        this.ext = ext;
    }

    /**
     * 注册页面路由与文件路径的映射
     * @param page - 页面定义
     */
    registerPage(page: SSRPage): void {
        if (page.enabled === false) return;
        const filePath = path.resolve(this.pagesDir, `${page.name}${this.ext}`);
        this.routePathMap.set(page.route, filePath);
    }

    /**
     * 批量注册页面路由映射
     * @param pages - 页面定义数组
     */
    registerPages(pages: SSRPage[]): void {
        for (const page of pages) {
            this.registerPage(page);
        }
    }

    /**
     * 根据路由获取模块文件路径
     * @param route - 页面路由
     * @param pagesDir - 可选的页面目录覆盖
     * @param ext - 可选的扩展名覆盖
     * @returns 模块文件路径
     */
    private getModulePath(route: string, pagesDir?: string, ext?: string): string {
        // 优先使用注册的映射
        if (this.routePathMap.has(route)) {
            return this.routePathMap.get(route)!;
        }
        // 未注册时尝试用 route 作为文件名（去掉斜杠和动态参数）
        const targetDir = pagesDir ?? this.pagesDir;
        const targetExt = ext ?? this.ext;
        const pageName = route
            .replace(/^\//, "")
            .replace(/\/:/g, "_")
            .replace(/\//g, "_");
        return path.resolve(targetDir, `${pageName}${targetExt}`);
    }

    /**
     * 清除指定路由的模块缓存
     * @param route - 页面路由
     */
    clearCache(route: string): void {
        this.moduleCache.delete(route);

        // 直接从映射中获取实际加载时使用的路径
        const modulePath = this.routePathMap.get(route);
        if (!modulePath) return;

        // 清除 require 缓存
        const cached = require.cache[modulePath];
        if (cached) {
            delete require.cache[modulePath];
            this.clearModuleDependencies(cached);
        }

        // 同时清除路径映射
        this.routePathMap.delete(route);
    }

    /**
     * 清除模块的依赖缓存
     * @param module - Node.js 模块对象
     */
    private clearModuleDependencies(module: NodeModule): void {
        if (module.children) {
            for (const child of module.children) {
                if (require.cache[child.filename]) {
                    delete require.cache[child.filename];
                    this.clearModuleDependencies(child);
                }
            }
        }
    }

    /**
     * 清除所有模块缓存
     */
    clearAllCache(): void {
        for (const route of this.moduleCache.keys()) {
            this.clearCache(route);
        }
        this.moduleCache.clear();
        this.routePathMap.clear();
    }

    /**
     * 加载页面模块
     * @param options - 加载选项
     * @returns 页面模块对象
     * @throws 模块不存在或加载失败时抛出错误
     */
    load(options: SSRLoadOptions): PageModule;
    load(route: string, isDev?: boolean): PageModule;
    load(routeOrOptions: string | SSRLoadOptions, isDev: boolean = false): PageModule {
        const options: SSRLoadOptions =
            typeof routeOrOptions === "string"
                ? { route: routeOrOptions, isDev }
                : routeOrOptions;

        const { route, pageName, isDev: dev, pagesDir, ext } = options;

        // 计算实际的模块路径，不修改实例默认配置
        let modulePath: string;
        if (pageName) {
            const targetDir = pagesDir ?? this.pagesDir;
            const targetExt = ext ?? this.ext;
            modulePath = path.resolve(targetDir, `${pageName}${targetExt}`);
            // 存入映射供后续 clearCache 使用
            this.routePathMap.set(route, modulePath);
        } else {
            modulePath = this.getModulePath(route, pagesDir, ext);
            // 存入映射供后续 clearCache 使用
            this.routePathMap.set(route, modulePath);
        }

        // 开发环境下清除缓存
        if (dev) {
            this.clearCache(route);
            // clearCache 会删除 routePathMap，需要重新设置
            this.routePathMap.set(route, modulePath);
        }

        // 非开发环境且已缓存则直接返回
        if (!dev && this.moduleCache.has(route)) {
            return this.moduleCache.get(route)!;
        }

        // 先检查模块文件是否存在
        try {
            require.resolve(modulePath);
        } catch (resolveError) {
            throw new Error(`Page module not found: route=${route} (path: ${modulePath})`);
        }

        // 模块文件存在，开始加载
        try {
            // eslint-disable-next-line @typescript-eslint/no-var-requires
            const module = require(modulePath);

            // 规范化模块导出
            const pageModule: PageModule = {
                ...module,
                default: module.default ?? module,
                render: module.render ?? module.default?.render,
                getInitialProps: module.getInitialProps ?? module.default?.getInitialProps,
                metadata: module.metadata ?? module.default?.metadata,
            };

            // 非开发环境缓存模块
            if (!dev) {
                this.moduleCache.set(route, pageModule);
            }

            return pageModule;
        } catch (loadError) {
            throw new Error(
                `Failed to load page module "${route}" (dependency error): ${(loadError as Error).message}`
            );
        }
    }

    /**
     * 批量加载页面模块
     * @param pages - 页面定义数组
     * @param isDev - 是否开发环境
     * @returns route 到模块的映射
     */
    loadPages(pages: SSRPage[], isDev: boolean = false): Map<string, PageModule> {
        // 先注册所有页面映射
        this.registerPages(pages);

        const result = new Map<string, PageModule>();

        for (const page of pages) {
            if (page.enabled === false) continue;

            try {
                const module = this.load(page.route, isDev);
                result.set(page.route, module);
            } catch {
                console.warn(`Failed to load page: route=${page.route}`);
            }
        }

        return result;
    }

    /**
     * 检查模块是否存在
     * @param route - 页面路由
     * @returns 模块是否存在
     */
    exists(route: string): boolean {
        const modulePath = this.getModulePath(route);
        try {
            require.resolve(modulePath);
            return true;
        } catch {
            return false;
        }
    }

    /**
     * 获取当前缓存的页面路由列表
     * @returns 已缓存的页面路由数组
     */
    getCachedRoutes(): string[] {
        return Array.from(this.moduleCache.keys());
    }

    /**
     * 更新页面输出目录
     * @param pagesDir - 新的页面目录
     */
    setPagesDir(pagesDir: string): void {
        this.pagesDir = pagesDir;
        // 清除路径映射，需要重新注册
        this.routePathMap.clear();
    }

    /**
     * 更新文件扩展名
     * @param ext - 新的扩展名
     */
    setExt(ext: string): void {
        this.ext = ext;
        // 清除路径映射，需要重新注册
        this.routePathMap.clear();
    }
}

// 默认单例实例
let defaultLoader: SSRLoader | null = null;

/**
 * 获取默认加载器实例
 * @param pagesDir - 页面目录
 * @param ext - 文件扩展名
 * @returns SSRLoader 实例
 */
export function getLoader(pagesDir?: string, ext?: string): SSRLoader {
    if (!defaultLoader) {
        defaultLoader = new SSRLoader(pagesDir, ext);
    }
    return defaultLoader;
}

/**
 * 加载页面模块（使用默认加载器）
 * @param route - 页面路由
 * @param isDev - 是否开发环境
 * @returns 页面模块对象
 */
export function loadPageModule(route: string, isDev: boolean = false): PageModule {
    return getLoader().load(route, isDev);
}

/**
 * 清除页面缓存（使用默认加载器）
 * @param route - 页面路由
 */
export function clearPageCache(route: string): void {
    return getLoader().clearCache(route);
}

/**
 * 清除所有缓存（使用默认加载器）
 */
export function clearAllPageCache(): void {
    return getLoader().clearAllCache();
}

/**
 * 创建 SSR 加载器实例
 * @param pagesDir - 页面输出目录
 * @param ext - 文件扩展名
 * @returns SSRLoader 实例
 */
export function createSSRLoader(pagesDir?: string, ext?: string): SSRLoader {
    return new SSRLoader(pagesDir, ext);
}