import {httpController} from "../httpClient/controller";

/**
 * 过滤器模式
 * - none: 不过滤，允许所有路由
 * - whitelist: 白名单模式，只允许列表中的路由
 * - blacklist: 黑名单模式，禁止列表中的路由
 */
export type FilterMode = "none" | "whitelist" | "blacklist";

/**
 * 加载配置接口
 */
export interface LoadConfig {
    /** 过滤器模式，默认 none */
    filterMode: FilterMode;
    /** 过滤器路由列表 */
    filterRoutes: string[];
    /** 是否启用预加载 */
    preload: boolean;
    /** 预加载路由列表 */
    preloadRoutes: string[];
    /** 是否启用慢加载 */
    slowLoad: boolean;
    /** 慢加载路由列表 */
    slowLoadRoutes: string[];
}

/**
 * 默认加载配置
 */
export const defaultLoadConfig: LoadConfig = {
    filterMode: "none",
    filterRoutes: [],
    preload: false,
    preloadRoutes: [],
    slowLoad: false,
    slowLoadRoutes: [],
};
export class SSRrenderController {
    private controller:httpController = new httpController()
    //预加载业务

    //请求处理
    async requestDeal(){

    }

}

