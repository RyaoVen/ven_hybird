/** 页面应用根组件，根据路由动态加载并渲染对应页面，客户端导航由预制 router 驱动 */

import React, { useEffect, useState } from "react";
import { getPageModule, matchPage } from "../../frame/node/.generated/pageRegistry";
import { installVenRouter, VenRouterState } from "./router";
import { PageBootstrap } from "./types";

/** PageApp 组件属性 */
export interface PageAppProps {
    /** 页面引导数据，包含路由信息和初始状态 */
    bootstrap: PageBootstrap;
}

/** 页面模块结构 */
interface PageModule {
    /** 页面默认导出的 React 组件 */
    default?: React.ComponentType<{ bootstrap: PageBootstrap }>;
}

/** 从 bootstrap 构造初始路由状态 */
function initialStateOf(bootstrap: PageBootstrap): VenRouterState {
    return {
        route: bootstrap.route,
        query: bootstrap.query,
        initialState: bootstrap.initialState,
        loading: false,
        forbidden: false,
    };
}

/**
 * 页面应用根组件
 * @description SSR 时按 bootstrap 静态渲染（与历史行为一致）；
 * 客户端 hydration 后安装预制 router，导航经 data-only 取数重渲染。
 * @param props.bootstrap - 页面引导数据
 * @returns 渲染后的 React 元素
 */
export function PageApp({ bootstrap }: PageAppProps) {
    const [state, setState] = useState<VenRouterState>(() => initialStateOf(bootstrap));

    // 仅客户端执行；SSR 不运行 effect，输出与改造前完全一致，无 hydration mismatch
    useEffect(() => installVenRouter(initialStateOf(bootstrap), setState), []);

    if (state.forbidden) {
        const forbiddenModule = getPageModule("/403") as PageModule | null;
        const Forbidden = forbiddenModule?.default;
        if (Forbidden) {
            return (
                <Forbidden
                    bootstrap={{ route: "/403", params: {}, query: state.query, initialState: null }}
                />
            );
        }
        return <main>403 Forbidden</main>;
    }

    const match = matchPage(state.route);
    const Component = (match?.module as PageModule | null | undefined)?.default;
    if (!Component) {
        return <main>Page not found</main>;
    }
    return (
        <Component
            bootstrap={{
                route: state.route,
                params: match?.params ?? {},
                query: state.query,
                initialState: state.initialState,
            }}
        />
    );
}
