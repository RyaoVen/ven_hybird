/** 新闻详情页组件，对应路由 "/news/:id"，静态页 ISR demo */

import React from "react";
import type { PageAppProps } from "../../app/pageApp";

/**
 * 新闻详情页组件
 * @param props.bootstrap - 页面引导数据
 * @returns 新闻详情页 React 元素
 */
export default function NewsPost({ bootstrap }: PageAppProps) {
    return (
        <main>
            <h1>News Post</h1>
            {/* 展示动态路由参数与服务端预取的初始状态，用于调试 */}
            <pre>{JSON.stringify(bootstrap.params, null, 2)}</pre>
            <pre>{JSON.stringify(bootstrap.initialState, null, 2)}</pre>
        </main>
    );
}

/** 页面元数据，用于构建时自动生成页面注册表 */
export const metadata = {
    /** 页面路由路径 */
    route: "/news/:id"
};
