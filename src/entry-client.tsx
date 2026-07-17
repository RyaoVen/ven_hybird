/** 客户端入口，从 window.__VEN_BOOTSTRAP__ 读取数据并执行水合 */

import React from "react";
import { hydrateRoot } from "react-dom/client";
import { PageApp } from "./app/pageApp";
import { PageBootstrap } from "./app/types";

/** 扩展 Window 接口，声明服务端注入的引导数据 */
declare global {
    interface Window {
        /** 服务端注入的页面引导数据 */
        __VEN_BOOTSTRAP__?: PageBootstrap;
    }
}

/** 读取服务端注入的引导数据 */
const bootstrap = window.__VEN_BOOTSTRAP__;

/** 获取页面根 DOM 节点 */
const root = document.getElementById("root");

/** 根节点和引导数据均存在时执行水合 */
if (root && bootstrap) {
    hydrateRoot(root, <PageApp bootstrap={bootstrap} />);
}
