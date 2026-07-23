/** 博客详情页组件，对应路由 "/blog/:id"，展示路径参数与引导数据 */

import React from "react";
import type { PageAppProps } from "../../app/pageApp";

/**
 * 博客详情页组件
 * @param props.bootstrap - 页面引导数据
 * @returns 博客详情页 React 元素
 */
export default function BlogPost({ bootstrap }: PageAppProps) {
    return (
        <main>
            <h1>Blog Post</h1>
            {/* 展示动态路由参数与服务端预取的初始状态，用于调试 */}
            <pre>{JSON.stringify(bootstrap.params, null, 2)}</pre>
            <pre>{JSON.stringify(bootstrap.initialState, null, 2)}</pre>
        </main>
    );
}

/** 页面元数据，用于构建时自动生成页面注册表 */
export const metadata = {
    /** 页面路由路径 */
    route: "/blog/:id"
};
