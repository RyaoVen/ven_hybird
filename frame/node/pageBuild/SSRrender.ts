import * as React from "react";
import * as ReactDOMServer from "react-dom/server";
import { SSRLoader, PageModule } from "./SSRload";

/**
 * SSR 渲染上下文（请求信息）
 */
export interface SSRRenderContext {
    /** 请求路由 */
    route: string;
    /** 请求 URL */
    url?: string;
    /** 请求方法 */
    method?: string;
    /** 请求头 */
    headers?: Record<string, string>;
    /** 查询参数 */
    query?: Record<string, string>;
    /** 请求体 */
    body?: unknown;
    /** 自定义数据 */
    [key: string]: unknown;
}

/**
 * SSR 渲染配置（静态配置，不随请求变化）
 */
export interface SSRRenderConfig {
    /** 模块加载器 */
    loader: SSRLoader;
    /** 客户端脚本路径（用于 hydration） */
    clientScriptPath?: string;
    /** 客户端脚本类型，默认 "module" */
    clientScriptType?: "module" | "text/javascript";
    /** 根元素 ID，默认 "root" */
    rootId?: string;
    /** 默认页面标题 */
    defaultTitle?: string;
    /** 默认页面元数据 */
    defaultMeta?: Array<{ name?: string; property?: string; content: string }>;
    /** HTML 模板 */
    template?: HTMLTemplate;
    /** 是否开发模式 */
    isDev?: boolean;
}

/**
 * HTML 模板配置
 */
export interface HTMLTemplate {
    /** 文档类型声明，默认 "<!DOCTYPE html>" */
    doctype?: string;
    /** html 元素属性 */
    htmlAttrs?: Record<string, string>;
    /** head 基础内容 */
    headBase?: string;
    /** body 基础内容 */
    bodyBase?: string;
}

/**
 * SSR 渲染结果
 */
export interface SSRRenderResult {
    /** 完整 HTML 字符串 */
    html: string;
    /** 渲染后的组件 HTML（不含文档结构） */
    componentHtml: string;
    /** 页面路由 */
    route: string;
    /** 页面标题 */
    title?: string;
    /** 初始数据（用于客户端 hydration） */
    initialData?: unknown;
}

/**
 * 默认 HTML 模板
 */
const defaultTemplate: HTMLTemplate = {
    doctype: "<!DOCTYPE html>",
    htmlAttrs: { lang: "zh-CN" },
    headBase:
        '<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">',
    bodyBase: "",
};

/**
 * SSR 渲染器
 * 根据路由自动加载模块、获取数据、渲染 HTML
 */
export class SSRRenderer {
    private config: SSRRenderConfig;
    private template: HTMLTemplate;

    /**
     * 创建渲染器实例
     * @param config - 渲染配置
     */
    constructor(config: SSRRenderConfig) {
        this.config = config;
        this.template = config.template
            ? { ...defaultTemplate, ...config.template }
            : defaultTemplate;
    }

    /**
     * 加载页面模块
     * @param route - 页面路由
     * @returns 页面模块
     */
    private loadModule(route: string): PageModule {
        return this.config.loader.load(route, this.config.isDev ?? false);
    }

    /**
     * 获取页面初始数据
     * @param module - 页面模块
     * @param context - 渲染上下文
     * @returns 初始数据
     */
    private async fetchInitialProps(
        module: PageModule,
        context: SSRRenderContext
    ): Promise<unknown> {
        if (module.getInitialProps) {
            return await module.getInitialProps(context);
        }
        return undefined;
    }

    /**
     * 渲染 React 组件为 HTML
     * @param module - 页面模块
     * @param props - 组件 props
     * @returns HTML 字符串
     */
    private renderComponent(module: PageModule, props: Record<string, unknown>): string {
        const component = module.default ?? module.render;

        if (!component) {
            throw new Error(`Page module has no component: route=${module.metadata?.route}`);
        }

        if (typeof component === "function") {
            const element = React.createElement(
                component as React.ComponentType,
                props as React.Attributes
            );
            return ReactDOMServer.renderToString(element);
        }

        if (React.isValidElement(component)) {
            return ReactDOMServer.renderToString(component);
        }

        throw new Error("Invalid component: must be a React component or element");
    }

    /**
     * 生成属性字符串
     */
    private attrsToString(attrs?: Record<string, string>): string {
        if (!attrs) return "";
        return Object.entries(attrs)
            .map(([key, value]) => `${key}="${value}"`)
            .join(" ");
    }

    /**
     * 生成 meta 标签
     */
    private metaToString(
        meta?: Array<{ name?: string; property?: string; content: string }>
    ): string {
        if (!meta) return "";
        return meta
            .map((m) => {
                const attrs: string[] = [];
                if (m.name) attrs.push(`name="${m.name}"`);
                if (m.property) attrs.push(`property="${m.property}"`);
                attrs.push(`content="${m.content}"`);
                return `<meta ${attrs.join(" ")}>`;
            })
            .join("");
    }

    /**
     * 生成初始数据注入脚本
     */
    private generateDataScript(data: unknown): string {
        const json = JSON.stringify(data)
            .replace(/</g, "\\u003c")
            .replace(/>/g, "\\u003e");
        return `<script>window.__INITIAL_DATA__=${json}</script>`;
    }

    /**
     * 生成完整 HTML 文档
     */
    private generateDocument(
        componentHtml: string,
        initialData: unknown,
        route: string
    ): string {
        const { template, config } = this;
        const rootId = config.rootId ?? "root";

        // 从模块元数据获取标题和 meta，否则使用默认值
        const title = (initialData as Record<string, unknown>)?.title ?? config.defaultTitle ?? "";
        const meta = config.defaultMeta ?? [];

        // 构建 head
        const headParts: string[] = [
            template.headBase ?? "",
            title ? `<title>${title}</title>` : "",
            this.metaToString(meta),
        ];

        // 构建 body
        const bodyParts: string[] = [
            `<div id="${rootId}">${componentHtml}</div>`,
            this.generateDataScript({ route, data: initialData }),
            config.clientScriptPath
                ? `<script type="${config.clientScriptType ?? "module"}" src="${config.clientScriptPath}"></script>`
                : "",
            template.bodyBase ?? "",
        ];

        // 构建完整文档
        const htmlAttrs = this.attrsToString(template.htmlAttrs);

        return [
            template.doctype ?? "<!DOCTYPE html>",
            `<html ${htmlAttrs}>`,
            `<head>${headParts.join("")}</head>`,
            `<body>${bodyParts.join("")}</body>`,
            "</html>",
        ].join("");
    }

    /**
     * 渲染页面
     * @param context - 渲染上下文（包含路由等信息）
     * @returns 渲染结果
     */
    async render(context: SSRRenderContext): Promise<SSRRenderResult> {
        const route = context.route;

        // 1. 加载页面模块
        const module = this.loadModule(route);

        // 2. 获取初始数据
        const initialData = await this.fetchInitialProps(module, context);

        // 3. 合并 props
        const props: Record<string, unknown> = {
            route,
            ...(typeof initialData === "object" && initialData !== null ? initialData : {}),
        };

        // 4. 渲染组件
        const componentHtml = this.renderComponent(module, props);

        // 5. 生成完整 HTML
        const html = this.generateDocument(componentHtml, initialData, route);

        return {
            html,
            componentHtml,
            route,
            title: (initialData as Record<string, unknown>)?.title as string | undefined,
            initialData,
        };
    }

    /**
     * 更新配置
     */
    updateConfig(config: Partial<SSRRenderConfig>): void {
        this.config = { ...this.config, ...config };
        if (config.template) {
            this.template = { ...defaultTemplate, ...config.template };
        }
    }

    /**
     * 更新模板
     */
    setTemplate(template: Partial<HTMLTemplate>): void {
        this.template = { ...this.template, ...template };
    }

    /**
     * 设置开发模式
     */
    setDevMode(isDev: boolean): void {
        this.config.isDev = isDev;
    }
}

/**
 * 创建 SSR 渲染器
 * @param config - 渲染配置
 */
export function createSSRRenderer(config: SSRRenderConfig): SSRRenderer {
    return new SSRRenderer(config);
}

// 默认实例
let defaultRenderer: SSRRenderer | null = null;

/**
 * 获取默认渲染器
 * @param config - 可选配置，用于初始化或更新
 */
export function getRenderer(config?: SSRRenderConfig): SSRRenderer {
    if (!defaultRenderer && config) {
        defaultRenderer = new SSRRenderer(config);
    }
    if (config) {
        defaultRenderer?.updateConfig(config);
    }
    if (!defaultRenderer) {
        throw new Error("SSRRenderer not initialized, please call getRenderer with config first");
    }
    return defaultRenderer;
}

/**
 * 渲染页面（使用默认渲染器）
 * @param context - 渲染上下文
 */
export async function renderPage(context: SSRRenderContext): Promise<SSRRenderResult> {
    return getRenderer().render(context);
}