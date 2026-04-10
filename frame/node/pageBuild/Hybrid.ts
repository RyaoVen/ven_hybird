import { SSRRenderer, SSRRenderContext, SSRRenderResult } from "./SSRrender";
import { SSRLoader } from "./SSRload";

/**
 * Hybrid 渲染结果
 */
export interface HybridRenderResult {
    /** HTML 内容 */
    html: string;
    /** 路由路径 */
    route: string;
}

/**
 * Hybrid 渲染配置
 */
export interface HybridConfig {
    /** SSR 渲染器 */
    renderer?: SSRRenderer;
    /** SSR 加载器 */
    loader?: SSRLoader;
    /** SPA 客户端脚本路径 */
    spaClientScriptPath?: string;
    /** SPA 客户端脚本类型 */
    spaClientScriptType?: "module" | "text/javascript";
}

/**
 * Hybrid 渲染器
 * 结合 SSR 渲染和 SPA 客户端注入
 */
export class HybridRenderer {
    /** SSR 渲染器实例 */
    private renderer: SSRRenderer | null = null;
    /** SSR 加载器实例 */
    private loader: SSRLoader | null = null;
    /** 页面模块缓存 */
    private pageModules: Map<string, unknown> = new Map();
    /** 渲染结果缓存 */
    private renderCache: Map<string, string> = new Map();
    /** SPA 客户端脚本路径 */
    private spaClientScriptPath: string = "";
    /** SPA 客户端脚本类型 */
    private spaClientScriptType: "module" | "text/javascript" = "module";

    /**
     * 创建 Hybrid 渲染器实例
     * @param config - 渲染配置
     */
    constructor(config?: HybridConfig) {
        if (config?.renderer) {
            this.renderer = config.renderer;
        }
        if (config?.loader) {
            this.loader = config.loader;
        }
        if (config?.spaClientScriptPath) {
            this.spaClientScriptPath = config.spaClientScriptPath;
        }
        if (config?.spaClientScriptType) {
            this.spaClientScriptType = config.spaClientScriptType;
        }
    }

    /**
     * 设置 SSR 渲染器
     * @param renderer - SSR 渲染器实例
     */
    setRenderer(renderer: SSRRenderer): void {
        this.renderer = renderer;
    }

    /**
     * 设置 SSR 加载器
     * @param loader - SSR 加载器实例
     */
    setLoader(loader: SSRLoader): void {
        this.loader = loader;
    }

    /**
     * 设置 SPA 客户端脚本路径
     * @param path - 脚本路径
     */
    setSpaClientScriptPath(path: string): void {
        this.spaClientScriptPath = path;
    }

    /**
     * 设置 SPA 客户端脚本类型
     * @param type - 脚本类型
     */
    setSpaClientScriptType(type: "module" | "text/javascript"): void {
        this.spaClientScriptType = type;
    }

    /**
     * 生成 JSON 数据注入脚本
     * @param json - 要注入的 JSON 数据
     * @returns script 标签字符串
     */
    private generateJsonScript(json: unknown): string {
        const jsonStr = JSON.stringify(json)
            .replace(/</g, "\\u003c")
            .replace(/>/g, "\\u003e");
        return `<script>window.__SPA_DATA__=${jsonStr}</script>`;
    }

    /**
     * 生成 SPA 客户端脚本标签
     * @returns script 标签字符串
     */
    private generateSpaClientScript(): string {
        if (!this.spaClientScriptPath) {
            return "";
        }
        return `<script type="${this.spaClientScriptType}" src="${this.spaClientScriptPath}"></script>`;
    }

    /**
     * 在 HTML 中注入 JSON 数据和 SPA 客户端标签
     * @param html - 原始 HTML
     * @param json - 要注入的 JSON 数据
     * @returns 注入后的 HTML
     */
    private injectScripts(html: string, json: unknown): string {
        const jsonScript = this.generateJsonScript(json);
        const spaScript = this.generateSpaClientScript();

        // 在 </body> 前注入脚本
        const bodyEndIndex = html.lastIndexOf("</body>");
        if (bodyEndIndex !== -1) {
            const beforeBodyEnd = html.slice(0, bodyEndIndex);
            const afterBodyEnd = html.slice(bodyEndIndex);
            return `${beforeBodyEnd}${jsonScript}${spaScript}${afterBodyEnd}`;
        }

        // 如果没有 </body> 标签，在末尾追加
        return `${html}${jsonScript}${spaScript}`;
    }

    /**
     * Hybrid 渲染
     * 使用 SSRrender 渲染页面，并在返回的 HTML 中注入 JSON 和 SPA 客户端标签
     * @param router - 路由路径
     * @param json - 要注入的 JSON 数据
     * @returns 渲染结果
     */
    async hybridRender(router: string, json: unknown): Promise<HybridRenderResult> {
        // 1. 构建 SSR 渲染上下文
        const context: SSRRenderContext = {
            route: router,
        };

        // 2. 使用 SSR 渲染器进行渲染
        if (!this.renderer) {
            throw new Error("SSR Renderer not initialized");
        }

        const ssrResult: SSRRenderResult = await this.renderer.render(context);

        // 3. 在 HTML 中注入 JSON 数据和 SPA 客户端标签
        const injectedHtml = this.injectScripts(ssrResult.html, json);

        return {
            html: injectedHtml,
            route: router,
        };
    }

    /**
     * 清除渲染缓存
     * @param route - 可选的路由路径，不传则清除所有缓存
     */
    clearCache(route?: string): void {
        if (route) {
            this.renderCache.delete(route);
            this.pageModules.delete(route);
        } else {
            this.renderCache.clear();
            this.pageModules.clear();
        }
    }
}

// 默认实例
let defaultHybridRenderer: HybridRenderer | null = null;

/**
 * 获取默认 Hybrid 渲染器
 * @param config - 可选配置
 * @returns HybridRenderer 实例
 */
export function getHybridRenderer(config?: HybridConfig): HybridRenderer {
    if (!defaultHybridRenderer) {
        defaultHybridRenderer = new HybridRenderer(config);
    }
    return defaultHybridRenderer;
}

/**
 * 创建 Hybrid 渲染器
 * @param config - 渲染配置
 * @returns HybridRenderer 实例
 */
export function createHybridRenderer(config?: HybridConfig): HybridRenderer {
    return new HybridRenderer(config);
}