/** 应用核心类型定义 */

/** 页面引导数据，SSR 时生成并注入客户端用于水合 */
export interface PageBootstrap {
    /** 当前路由路径，如 "/home"、"/user/:id" */
    route: string;

    /** 路由路径参数，值始终为字符串 */
    params: Record<string, string>;

    /** URL 查询参数，值始终为字符串 */
    query: Record<string, string>;

    /** 服务端预取的初始状态，类型由各页面自行定义 */
    initialState: unknown;
}
