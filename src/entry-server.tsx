/** 服务端渲染入口，导出页面模块查找函数和 PageApp 组件 */

export { getPageModuleForRoute as getPageModule } from "../frame/node/.generated/pageRegistry";
export { PageApp as default } from "./app/pageApp";
