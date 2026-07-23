/** 关于页组件，对应路由 "/about"，展示引导数据 */

import React from "react";
import type { PageAppProps } from "../app/pageApp";

/**
 * 关于页组件
 * @param props.bootstrap - 页面引导数据
 * @returns 关于页 React 元素
 */
export default function About({ bootstrap }: PageAppProps) {
    return (
        <main>
            <h1>About Page</h1>
            {/* 展示服务端预取的初始状态，用于调试 */}
            <pre>{JSON.stringify(bootstrap.initialState, null, 2)}</pre>
        </main>
    );
}

/** 页面元数据，用于构建时自动生成页面注册表 */
export const metadata = {
    /** 页面路由路径 */
    route: "/about"
};
