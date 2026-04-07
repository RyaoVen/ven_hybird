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
    /** 页面名称 */
    pageName: string;
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
 */
export class SSRLoader {
    private pagesDir: string;
    private ext: string;
    private moduleCache: Map<string, PageModule> = new Map();

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
     * 获取页面模块文件路径
     * @param pageName - 页面名称
     * @returns 完整文件路径
     */
    private getModulePath(pageName: string): string {
        return path.resolve(this.pagesDir, `${pageName}${this.ext}`);
    }

    /**
     * 清除指定页面的模块缓存
     * @param pageName - 页面名称
     */
    clearCache(pageName: string): void {
        const modulePath = this.getModulePath(pageName);
        this.moduleCache.delete(pageName);

        // 清除 require 缓存
        const cached = require.cache[modulePath];
        if (cached) {
            delete require.cache[modulePath];
            // 清除关联的父子依赖缓存
            this.clearModuleDependencies(cached);
        }
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
        for (const pageName of this.moduleCache.keys()) {
            this.clearCache(pageName);
        }
        this.moduleCache.clear();
    }

    /**
     * 加载页面模块
     * @param options - 加载选项
     * @returns 页面模块对象
     * @throws 模块不存在或加载失败时抛出错误
     */
    load(options: SSRLoadOptions): PageModule;
    load(pageName: string, isDev?: boolean): PageModule;
    load(pageNameOrOptions: string | SSRLoadOptions, isDev: boolean = false): PageModule {
        const options: SSRLoadOptions =
            typeof pageNameOrOptions === "string"
                ? { pageName: pageNameOrOptions, isDev }
                : pageNameOrOptions;

        const { pageName, isDev: dev, pagesDir, ext } = options;

        // 允许临时覆盖默认配置
        const targetDir = pagesDir ?? this.pagesDir;
        const targetExt = ext ?? this.ext;
        const modulePath = path.resolve(targetDir, `${pageName}${targetExt}`);

        // 开发环境下清除缓存
        if (dev) {
            this.clearCache(pageName);
        }

        // 非开发环境且已缓存则直接返回
        if (!dev && this.moduleCache.has(pageName)) {
            return this.moduleCache.get(pageName)!;
        }

        try {
            // 使用 require 加载模块
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
                this.moduleCache.set(pageName, pageModule);
            }

            return pageModule;
        } catch (error) {
            if ((error as NodeJS.ErrnoException).code === "MODULE_NOT_FOUND") {
                throw new Error(`Page module not found: ${pageName} (path: ${modulePath})`);
            }
            throw new Error(
                `Failed to load page module "${pageName}": ${(error as Error).message}`
            );
        }
    }

    /**
     * 批量加载页面模块
     * @param pages - 页面定义数组
     * @param isDev - 是否开发环境
     * @returns 页面名称到模块的映射
     */
    loadPages(pages: SSRPage[], isDev: boolean = false): Map<string, PageModule> {
        const result = new Map<string, PageModule>();

        for (const page of pages) {
            if (page.enabled === false) continue;

            try {
                const module = this.load(page.name, isDev);
                result.set(page.name, module);
            } catch {
                // 跳过加载失败的页面
                console.warn(`Failed to load page: ${page.name}`);
            }
        }

        return result;
    }

    /**
     * 检查模块是否存在
     * @param pageName - 页面名称
     * @returns 模块是否存在
     */
    exists(pageName: string): boolean {
        const modulePath = this.getModulePath(pageName);

        try {
            require.resolve(modulePath);
            return true;
        } catch {
            return false;
        }
    }

    /**
     * 获取当前缓存的页面列表
     * @returns 已缓存的页面名称数组
     */
    getCachedPages(): string[] {
        return Array.from(this.moduleCache.keys());
    }

    /**
     * 更新页面输出目录
     * @param pagesDir - 新的页面目录
     */
    setPagesDir(pagesDir: string): void {
        this.pagesDir = pagesDir;
    }

    /**
     * 更新文件扩展名
     * @param ext - 新的扩展名
     */
    setExt(ext: string): void {
        this.ext = ext;
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
 * @param pageName - 页面名称
 * @param isDev - 是否开发环境
 * @returns 页面模块对象
 */
export function loadPageModule(pageName: string, isDev: boolean = false): PageModule {
    return getLoader().load(pageName, isDev);
}

/**
 * 清除页面缓存（使用默认加载器）
 * @param pageName - 页面名称
 */
export function clearPageCache(pageName: string): void {
    return getLoader().clearCache(pageName);
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