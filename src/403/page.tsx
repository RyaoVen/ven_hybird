/** 403 错误页组件，对应路由 "/403"，鉴权失败（角色不足）时原地渲染 */

import React from "react";
import type { PageAppProps } from "../app/pageApp";

/**
 * 403 错误页组件
 * @param props.bootstrap - 页面引导数据
 * @returns 403 页 React 元素
 */
export default function Forbidden({ bootstrap }: PageAppProps) {
    return (
        <main>
            <h1>403 无权访问</h1>
            <p>当前角色没有访问该页面的权限。</p>
            <pre>{JSON.stringify(bootstrap.query, null, 2)}</pre>
        </main>
    );
}

/** 页面元数据，用于构建时自动生成页面注册表 */
export const metadata = {
    /** 页面路由路径 */
    route: "/403"
};
