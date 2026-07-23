/**
 * @file HTTP 传输层核心类型定义
 * @description 定义 SSR 渲染任务调度中的数据接口：启动数据、任务、错误、回调
 */

/** 页面启动数据，由外部调用方传入 */
export interface PageBootstrap {
    route: string;                          /** 请求路由 */
    params: Record<string, string>;         /** 动态路由参数 */
    query: Record<string, string>;          /** 查询参数 */
    initialState: unknown;                  /** 初始状态 */
}

/** 渲染任务，通过 POST /render 提交 */
export interface RenderTask {
    hookId: string;                         /** 任务唯一标识，用于回调关联和去重 */
    requestRoute: string;                   /** 请求路由 */
    payload: PageBootstrap;                 /** 页面启动数据 */
}

/** 渲染错误 */
export interface RenderError {
    code: "INVALID_REQUEST" | "PAGE_NOT_FOUND" | "RENDER_FAILED";  /** 错误码 */
    message: string;                                                 /** 错误描述 */
}

/** 渲染回调数据，渲染完成后 POST 回外部服务 */
export interface RenderCallback {
    hookId: string;             /** 关联的任务标识 */
    requestRoute: string;       /** 原始请求路由 */
    matchedRoute?: string;      /** 匹配到的路由模式 */
    pageName?: string;          /** 页面名称 */
    html: string;               /** 渲染后的 HTML，失败时为空 */
    error?: RenderError;        /** 错误信息，成功时为 undefined */
    duration?: number;          /** 渲染耗时（毫秒） */
}

/** 页面路由模式列表，通过 GET /pages 返回给外部服务（nodePagesPattern） */
export interface PagePatternList {
    patterns: string[];         /** 全部页面路由模板，如 /blog/:slug */
}
