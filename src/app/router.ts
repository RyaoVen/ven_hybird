/**
 * @file SPA 客户端路由器
 * @description 框架预制：拦截站内链接点击，按 registry 匹配路由后经 data-only 取数，
 * 驱动 PageApp 重渲染；401 统一跳登录页，403 原地渲染错误页，失败回整页跳转（MPA 兜底）
 */
import { matchPage } from "../../frame/node/.generated/pageRegistry";

/** 路由器状态（由 PageApp 持有并渲染） */
export interface VenRouterState {
    /** 当前真实路径（不含 query） */
    route: string;
    /** 当前查询参数 */
    query: Record<string, string>;
    /** 当前页面数据（与首屏 bootstrap.initialState 同形） */
    initialState: unknown;
    /** 是否正在取数 */
    loading: boolean;
    /** 是否命中 403（原地渲染错误页，URL 不变） */
    forbidden: boolean;
}

/** 状态订阅回调 */
export type VenRouterStateHandler = (state: VenRouterState) => void;

/** 数据请求头：与 Go 端双模式约定一致 */
const dataOnlyHeaders = { "X-Ven-Data-Only": "true" };
/** 401 响应携带登录跳转目标的响应头 */
const loginPathHeader = "x-ven-login-path";
/** 响应头缺失时的兜底登录路径（与 Go 端默认值一致） */
const defaultLoginPath = "/login";

let handler: VenRouterStateHandler | null = null;
let installed = false;
/** 导航序号：只应用最新一次取数结果（竞态丢弃） */
let navSeq = 0;
/** 当前 history 条目 key（滚动位置按 key 存档） */
let currentKey = "initial";
const scrollPositions = new Map<string, number>();

let currentState: VenRouterState = {
    route: "/",
    query: {},
    initialState: null,
    loading: false,
    forbidden: false,
};

/** 合并状态并通知订阅方 */
function emit(patch: Partial<VenRouterState>): void {
    currentState = { ...currentState, ...patch };
    handler?.(currentState);
}

/** 取路径部分（不含 query） */
function pathOf(url: string): string {
    return url.split("?", 1)[0] || "/";
}

/** 解析查询参数 */
function queryOf(url: string): Record<string, string> {
    const query: Record<string, string> = {};
    const search = url.includes("?") ? url.slice(url.indexOf("?") + 1) : "";
    for (const [key, value] of new URLSearchParams(search)) {
        query[key] = value;
    }
    return query;
}

/** 设置取数加载态（状态 + #root 透明度，直接操作 DOM 避免 React 重挂载） */
function setLoading(loading: boolean): void {
    const root = document.getElementById("root");
    if (root) {
        root.style.transition = "opacity 120ms";
        root.style.opacity = loading ? "0.6" : "1";
    }
    emit({ loading });
}

/**
 * 客户端导航：匹配不到页面路由时回整页跳转（MPA 兜底）。
 * @param url - 目标地址（可含 query）
 * @param options.replace - 用 replaceState 代替 pushState
 * @param options.key - history 条目 key（popstate 恢复用）
 */
export function navigate(url: string, options?: { replace?: boolean; key?: string }): void {
    if (!matchPage(url)) {
        window.location.href = url;
        return;
    }
    scrollPositions.set(currentKey, window.scrollY);
    const key = options?.key ?? String(Date.now());
    if (options?.replace) {
        history.replaceState({ key }, "", url);
    } else {
        history.pushState({ key }, "", url);
    }
    currentKey = key;
    void loadRoute(url, false);
}

/** 取数并驱动重渲染 */
async function loadRoute(url: string, isPop: boolean): Promise<void> {
    const seq = ++navSeq;
    setLoading(true);

    let response: Response;
    try {
        response = await fetch(url, { headers: dataOnlyHeaders });
    } catch {
        // 网络失败回整页跳转
        if (seq === navSeq) window.location.href = url;
        return;
    }
    if (seq !== navSeq) return; // 竞态丢弃

    if (response.status === 401) {
        const loginPath = response.headers.get(loginPathHeader) ?? defaultLoginPath;
        window.location.href = `${loginPath}?next=${encodeURIComponent(url)}`;
        return;
    }
    if (response.status === 403) {
        // 原地渲染错误页，URL 不变
        setLoading(false);
        emit({ route: pathOf(url), query: queryOf(url), initialState: null, forbidden: true });
        return;
    }
    if (!response.ok) {
        // 404/500 等交给 MPA 兜底
        window.location.href = url;
        return;
    }

    const data: unknown = await response.json();
    setLoading(false);
    emit({ route: pathOf(url), query: queryOf(url), initialState: data, forbidden: false });
    window.scrollTo(0, isPop ? (scrollPositions.get(currentKey) ?? 0) : 0);
}

/** 链接点击拦截：仅处理无修饰键左键点击的站内单斜杠链接 */
function onDocumentClick(event: MouseEvent): void {
    if (event.defaultPrevented || event.button !== 0 ||
        event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
        return;
    }
    const anchor = (event.target as Element | null)?.closest?.("a[href]");
    if (!anchor) return;
    if (anchor.target || anchor.hasAttribute("download") || anchor.hasAttribute("data-no-router")) {
        return;
    }
    const href = anchor.getAttribute("href") ?? "";
    if (!href.startsWith("/") || href.startsWith("//")) return;
    event.preventDefault();
    navigate(href);
}

/** 前进/后退：重取数据并恢复滚动位置 */
function onPopState(event: PopStateEvent): void {
    const url = window.location.pathname + window.location.search;
    if (!matchPage(url)) return;
    scrollPositions.set(currentKey, window.scrollY);
    if (typeof event.state?.key === "string") {
        currentKey = event.state.key;
    }
    void loadRoute(url, true);
}

/**
 * 安装路由器（PageApp 在 useEffect 中调用）。
 * @param initial - 初始状态（来自首屏 bootstrap）
 * @param onState - 状态订阅回调
 * @returns 卸载函数
 */
export function installVenRouter(
    initial: VenRouterState,
    onState: VenRouterStateHandler,
): () => void {
    currentState = initial;
    handler = onState;
    if (installed) {
        return () => { handler = null; };
    }
    installed = true;
    history.scrollRestoration = "manual";
    document.addEventListener("click", onDocumentClick);
    window.addEventListener("popstate", onPopState);
    return () => {
        document.removeEventListener("click", onDocumentClick);
        window.removeEventListener("popstate", onPopState);
        installed = false;
        handler = null;
    };
}
