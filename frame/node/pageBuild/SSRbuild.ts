import * as esbuild from "esbuild";
import * as fs from "node:fs/promises";
import * as path from "node:path";

/**
 * 页面定义
 */
export interface SSRPage {
    /** 页面名称，用于预加载白名单/黑名单匹配 */
    name: string;
    /** 路由路径，支持动态参数如 /about 或 /post/:id */
    route: string;
    /** 页面文件路径 */
    filePath: string;
    /** 是否启用，false 时跳过注册，默认 true */
    enabled?: boolean;
}

/**
 * 预加载模式
 * - "all": 预加载所有页面
 * - "whitelist": 仅预加载白名单中的页面
 * - "blacklist": 预加载除黑名单外的所有页面
 * - "none": 不预加载
 */
export type SSRPreloadMode = "all" | "whitelist" | "blacklist" | "none";

/**
 * SSR 构建配置选项
 */
export interface SSRBuildOptions {
    /** 服务端入口文件路径 */
    entryPoint: string;
    /** 页面列表 */
    pages?: SSRPage[];
    /** 是否压缩代码，默认 false */
    minify?: boolean;
    /** source map 配置：false | "inline" | "external"，默认 false */
    sourcemap?: boolean | "inline" | "external";
    /** 外部依赖列表，不参与打包 */
    external?: string[];
    /** 输出格式："cjs" 或 "esm"，默认 "cjs" */
    format?: "cjs" | "esm";
    /** 是否直接写入磁盘，默认 false（返回代码字符串） */
    write?: boolean;
    /** 输出文件路径，write=true 时必填 */
    outFile?: string;
    /** 编译目标，默认 ["node18"] */
    target?: string[];
    /** 自定义文件 loader，默认支持 .tsx/.jsx/.ts/.js/.json */
    loader?: Record<string, esbuild.Loader>;
    /** 是否在构建时预加载页面，默认 true */
    preloadPages?: boolean;
    /** 预加载模式，默认 "all" */
    preloadMode?: SSRPreloadMode;
    /** 预加载白名单，按 name 或 route 匹配 */
    preloadWhitelist?: string[];
    /** 预加载黑名单，按 name 或 route 匹配 */
    preloadBlacklist?: string[];
}

/**
 * SSR 构建结果
 */
export interface SSRBuildResult {
    /** 编译后的服务端代码（write=false 时可用） */
    serverCode?: string;
    /** source map 内容 */
    map?: string;
    /** 入口文件路径 */
    entryPoint: string;
    /** 输出文件路径 */
    outputFile?: string;
    /** 输出格式 */
    format: "cjs" | "esm";
    /** 是否已写入磁盘 */
    writtenToDisk: boolean;
    /** 所有已注册页面 */
    pages: SSRPage[];
    /** 已预加载的页面 */
    preloadedPages: SSRPage[];
}

/**
 * SSR 构建器
 * 基于 esbuild 编译服务端入口为 Node.js 可执行代码
 * 支持页面路由管理、动态路由匹配、预加载控制
 */
export class SSRBuild {
    private options: Required<
        Omit<SSRBuildOptions, "outFile" | "pages">
    > & {
        outFile?: string;
        pages: SSRPage[];
    };

    private buildResult: SSRBuildResult | null = null;
    private pageMap: Map<string, SSRPage> = new Map();
    private preloadedPages: Map<string, SSRPage> = new Map();

    /**
     * 创建构建器实例
     * @param options - SSR 构建配置选项
     */
    constructor(options: SSRBuildOptions) {
        this.options = {
            minify: false,
            sourcemap: false,
            external: [],
            format: "cjs",
            write: false,
            target: ["node18"],
            loader: {
                ".tsx": "tsx",
                ".jsx": "jsx",
                ".ts": "ts",
                ".js": "js",
                ".json": "json",
            },
            pages: [],
            preloadPages: true,
            preloadMode: "all",
            preloadWhitelist: [],
            preloadBlacklist: [],
            ...options,
        };

        this.registerPages(this.options.pages);
    }

    /**
     * 批量注册页面
     * @param pages - 页面定义数组
     */
    registerPages(pages: SSRPage[]): void {
        for (const page of pages) {
            this.registerPage(page);
        }
    }

    /**
     * 注册单个页面
     * @param page - 页面定义对象
     * @description enabled=false 时跳过注册
     */
    registerPage(page: SSRPage): void {
        if (page.enabled === false) return;
        this.pageMap.set(page.route, page);
    }

    /**
     * 获取所有已注册页面
     * @returns 页面定义数组
     */
    getPages(): SSRPage[] {
        return Array.from(this.pageMap.values());
    }

    /**
     * 根据路由获取页面，支持动态路由匹配
     * @param route - 请求路由路径
     * @returns 匹配的页面定义，未找到返回 null
     */
    getPageByRoute(route: string): SSRPage | null {
        if (this.pageMap.has(route)) {
            return this.pageMap.get(route)!;
        }

        for (const page of this.pageMap.values()) {
            if (this.matchRoute(page.route, route)) {
                return page;
            }
        }

        return null;
    }

    /**
     * 动态路由匹配
     * @param definedRoute - 定义的路由模式，如 /post/:id
     * @param requestRoute - 实际请求路由，如 /post/123
     * @returns 是否匹配
     * @private
     */
    private matchRoute(definedRoute: string, requestRoute: string): boolean {
        const definedParts = definedRoute.split("/").filter(Boolean);
        const requestParts = requestRoute.split("/").filter(Boolean);

        if (definedParts.length !== requestParts.length) {
            return false;
        }

        for (let i = 0; i < definedParts.length; i++) {
            const d = definedParts[i];
            const r = requestParts[i];

            if (d.startsWith(":")) continue;
            if (d !== r) return false;
        }

        return true;
    }

    /**
     * 根据预加载配置判断是否应预加载该页面
     * @param page - 页面定义对象
     * @returns 是否应预加载
     * @private
     */
    private shouldPreload(page: SSRPage): boolean {
        const { preloadMode, preloadWhitelist, preloadBlacklist } = this.options;

        if (preloadMode === "none") return false;
        if (preloadMode === "all") return true;
        if (preloadMode === "whitelist") {
            return (
                preloadWhitelist.includes(page.name) ||
                preloadWhitelist.includes(page.route)
            );
        }
        if (preloadMode === "blacklist") {
            return !(
                preloadBlacklist.includes(page.name) ||
                preloadBlacklist.includes(page.route)
            );
        }

        return false;
    }

    /**
     * 执行预加载
     * @returns 已预加载的页面列表
     * @description 根据 preloadMode 和 preloadPages 配置筛选页面
     */
    preloadAllPages(): SSRPage[] {
        this.preloadedPages.clear();

        if (!this.options.preloadPages) {
            return [];
        }

        for (const page of this.pageMap.values()) {
            if (this.shouldPreload(page)) {
                this.preloadedPages.set(page.route, page);
            }
        }

        return Array.from(this.preloadedPages.values());
    }

    /**
     * 获取已预加载页面
     * @returns 已预加载的页面列表
     */
    getPreloadedPages(): SSRPage[] {
        return Array.from(this.preloadedPages.values());
    }

    /**
     * 编译服务端入口文件
     * @returns 构建结果对象
     * @throws write=true 但未指定 outFile 时抛出错误
     */
    async build(): Promise<SSRBuildResult> {
        if (this.options.write && !this.options.outFile) {
            throw new Error("outFile is required when write=true");
        }

        const result = await esbuild.build({
            entryPoints: [this.options.entryPoint],
            bundle: true,
            minify: this.options.minify,
            sourcemap: this.options.sourcemap,
            external: this.options.external,
            format: this.options.format,
            platform: "node",
            target: this.options.target,
            write: this.options.write,
            outfile: this.options.outFile,
            jsx: "automatic",
            loader: this.options.loader,
        });

        const serverCode = !this.options.write
            ? result.outputFiles?.find((f) => f.path.endsWith(".js"))?.text
            : undefined;

        const map = !this.options.write
            ? result.outputFiles?.find((f) => f.path.endsWith(".map"))?.text
            : undefined;

        const preloadedPages = this.preloadAllPages();

        this.buildResult = {
            serverCode,
            map,
            entryPoint: this.options.entryPoint,
            outputFile: this.options.outFile,
            format: this.options.format,
            writtenToDisk: this.options.write,
            pages: this.getPages(),
            preloadedPages,
        };

        return this.buildResult;
    }

    /**
     * 保存构建结果到磁盘
     * @returns 无返回值
     * @throws 未调用 build()、未指定 outFile、代码为空时抛出错误
     */
    async save(): Promise<void> {
        if (!this.buildResult) {
            throw new Error("build() must be called before save()");
        }

        if (!this.options.outFile) {
            throw new Error("outFile is required");
        }

        if (this.options.write) {
            return;
        }

        if (!this.buildResult.serverCode) {
            throw new Error("serverCode is empty");
        }

        await fs.mkdir(path.dirname(this.options.outFile), { recursive: true });
        await fs.writeFile(this.options.outFile, this.buildResult.serverCode, "utf-8");

        if (this.buildResult.map && this.options.sourcemap === "external") {
            await fs.writeFile(`${this.options.outFile}.map`, this.buildResult.map, "utf-8");
        }
    }

    /**
     * 编译并保存到磁盘
     * @returns 构建结果对象
     */
    async buildAndSave(): Promise<SSRBuildResult> {
        const result = await this.build();
        if (!this.options.write) {
            await this.save();
        }
        return result;
    }

    /**
     * 获取当前构建结果
     * @returns 构建结果对象，未构建时返回 null
     */
    getResult(): SSRBuildResult | null {
        return this.buildResult;
    }

    /**
     * 获取服务端代码字符串
     * @returns 代码字符串，未构建时返回 null
     */
    getCode(): string | null {
        return this.buildResult?.serverCode ?? null;
    }

    /**
     * 根据请求路由解析对应页面
     * @param route - 请求路由路径
     * @returns 匹配的页面定义，未找到返回 null
     */
    resolvePage(route: string): SSRPage | null {
        return this.getPageByRoute(route);
    }
}

/**
 * 创建 SSR 构建器实例
 * @param options - SSR 构建配置选项
 * @returns SSRBuild 实例
 */
export function createSSRBuild(options: SSRBuildOptions): SSRBuild {
    return new SSRBuild(options);
}