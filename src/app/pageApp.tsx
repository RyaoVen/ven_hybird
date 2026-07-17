/** 页面应用根组件，根据路由动态加载并渲染对应页面 */

import React from "react";
import { getPageModuleForRoute } from "../../frame/node/.generated/pageRegistry";
import { PageBootstrap } from "./types";

/** PageApp 组件属性 */
export interface PageAppProps {
    /** 页面引导数据，包含路由信息和初始状态 */
    bootstrap: PageBootstrap;
}

/** 页面模块结构 */
interface PageModule {
    /** 页面默认导出的 React 组件 */
    default?: React.ComponentType<PageAppProps>;
}

/**
 * 页面应用根组件
 * @param props.bootstrap - 页面引导数据
 * @returns 渲染后的 React 元素，未匹配路由时返回 404
 */
export function PageApp({ bootstrap }: PageAppProps) {
    const pageModule = getPageModuleForRoute(bootstrap.route) as PageModule | null;
    const Page = pageModule?.default;

    if (!Page) {
        return <main>Page not found</main>;
    }

    return <Page bootstrap={bootstrap} />;
}
