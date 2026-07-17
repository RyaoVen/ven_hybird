/**
 * @file SSR 渲染器模块
 * @description 将 React 组件渲染为完整 HTML 文档，注入 bootstrap 数据供客户端水合
 */
import * as React from "react";
import * as ReactDOMServer from "react-dom/server";
import { MatchedPage } from "./pageRouter";
import { PageModule, SSRLoader } from "./ssrLoader";

/** 页面启动数据，在 SSR 与客户端之间传递 */
export interface PageBootstrap {
    route: string;                          /** 请求路由 */
    params: Record<string, string>;         /** 动态路由参数 */
    query: Record<string, string>;          /** 查询参数 */
    initialState: unknown;                  /** 页面初始状态 */
}

/** SSR 渲染器配置 */
export interface SSRRenderConfig {
    loader: SSRLoader;                      /** SSR 加载器 */
    clientScriptPath: string;              /** 客户端脚本访问路径 */
    isDev?: boolean;                        /** 是否开发模式 */
}

/** SSR 渲染上下文 */
export interface SSRRenderContext {
    bootstrap: PageBootstrap;               /** 启动数据 */
    matchedPage: MatchedPage;               /** 路由匹配结果 */
}

/** SSR 渲染结果 */
export interface SSRRenderResult {
    html: string;                           /** 完整 HTML 文档 */
    bootstrap: PageBootstrap;               /** 最终启动数据 */
}

/**
 * SSR 渲染器
 * @description 加载页面模块、执行数据预取、渲染 React 组件并生成 HTML 文档
 */
export class SSRRenderer {
    constructor(private readonly config: SSRRenderConfig) {}

    /**
     * 执行 SSR 渲染
     * @param context - 渲染上下文
     * @returns 渲染结果
     */
    async render(context: SSRRenderContext): Promise<SSRRenderResult> {
        const pageModule = this.config.loader.loadPage(
            context.bootstrap.route,
            this.config.isDev ?? false,
        );
        const initialState = await this.loadInitialState(pageModule, context.bootstrap);
        const bootstrap: PageBootstrap = { ...context.bootstrap, initialState };
        const App = this.config.loader.getApp(this.config.isDev ?? false) as React.ComponentType<{
            bootstrap: PageBootstrap;
        }>;
        const componentHtml = ReactDOMServer.renderToString(
            React.createElement(App, { bootstrap }),
        );
        return {
            html: this.generateDocument(componentHtml, bootstrap),
            bootstrap,
        };
    }

    /** 清除 SSR bundle 缓存 */
    clearCache(): void {
        this.config.loader.clearCache();
    }

    /**
     * 加载页面初始状态：调用 getInitialProps 并与已有 initialState 合并
     * @param pageModule - 页面模块
     * @param bootstrap - 启动数据
     * @returns 合并后的初始状态
     */
    private async loadInitialState(
        pageModule: PageModule,
        bootstrap: PageBootstrap,
    ): Promise<unknown> {
        if (!pageModule.getInitialProps) return bootstrap.initialState;
        const pageState = await pageModule.getInitialProps(bootstrap);
        if (isRecord(bootstrap.initialState) && isRecord(pageState)) {
            return { ...bootstrap.initialState, ...pageState };
        }
        return pageState;
    }

    /**
     * 生成完整 HTML 文档
     * @description 包含 SSR HTML、bootstrap 数据（防 XSS 转义）和客户端脚本
     * @param componentHtml - SSR 渲染的组件 HTML
     * @param bootstrap - 启动数据
     * @returns HTML 文档字符串
     */
    private generateDocument(componentHtml: string, bootstrap: PageBootstrap): string {
        // 对 JSON 进行转义防止 XSS 注入（<、>、& 及 Unicode 行/段分隔符）
        const serializedBootstrap = JSON.stringify(bootstrap)
            .replace(/</g, "\\u003c")
            .replace(/>/g, "\\u003e")
            .replace(/&/g, "\\u0026")
            .replace(/\u2028/g, "\\u2028")
            .replace(/\u2029/g, "\\u2029");

        return [
            "<!DOCTYPE html>",
            '<html lang="zh-CN">',
            '<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>',
            "<body>",
            `<div id="root">${componentHtml}</div>`,
            `<script>window.__VEN_BOOTSTRAP__=${serializedBootstrap}</script>`,
            `<script type="module" src="${this.config.clientScriptPath}"></script>`,
            "</body>",
            "</html>",
        ].join("");
    }
}

/** 类型守卫：判断值是否为非 null 纯对象 */
function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}
