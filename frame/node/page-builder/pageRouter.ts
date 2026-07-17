/**
 * @file 页面路由管理模块
 * @description 提供基于文件系统的页面路由扫描、注册、匹配能力，支持静态和动态参数路由
 */
import * as fs from "node:fs/promises";
import * as path from "node:path";

/** SSR 页面描述 */
export interface SSRPage {
    id: string;         /** 页面唯一标识 */
    name: string;       /** 页面名称 */
    route: string;      /** 路由路径模式，支持动态参数如 `/user/:id` */
    filePath: string;   /** 页面组件文件绝对路径 */
    enabled: boolean;   /** 是否启用 */
}

/** 路由匹配结果 */
export interface MatchedPage {
    page: SSRPage;                      /** 匹配到的页面 */
    params: Record<string, string>;     /** 解析出的动态路由参数 */
}

/**
 * 页面路由器
 * @description 管理页面路由注册与匹配，先精确查找再参数化模式匹配
 */
export class PageRouter {
    /** 已注册的页面路由映射表 */
    private pages = new Map<string, SSRPage>();

    /**
     * 注册单个页面路由
     * @param page - 页面描述
     * @throws 路由重复时抛出 Error
     */
    registerPage(page: SSRPage): void {
        if (!page.enabled) return;
        if (this.pages.has(page.route)) {
            throw new Error(`Duplicate page route: ${page.route}`);
        }
        this.pages.set(page.route, page);
    }

    /**
     * 批量注册页面路由
     * @param pages - 页面描述数组
     */
    registerPages(pages: SSRPage[]): void {
        for (const page of pages) {
            this.registerPage(page);
        }
    }

    /**
     * 获取所有已注册页面（按优先级排序：静态段多的在前）
     * @returns 排序后的页面数组
     */
    getPages(): SSRPage[] {
        return [...this.pages.values()].sort(comparePages);
    }

    /**
     * 根据请求路径匹配页面
     * @param route - 请求路径
     * @returns 匹配结果，未匹配返回 null
     */
    getPageByRoute(route: string): MatchedPage | null {
        const normalizedRoute = normalizeRoute(route);
        const exact = this.pages.get(normalizedRoute);
        if (exact) return { page: exact, params: {} };

        for (const page of this.getPages()) {
            const params = matchRoute(page.route, normalizedRoute);
            if (params) return { page, params };
        }
        return null;
    }
}

/**
 * 递归扫描目录下所有 page.tsx / page.jsx 文件
 * @param dirPath - 扫描根目录
 * @returns 页面文件绝对路径数组
 */
export async function scanPageFiles(dirPath: string): Promise<string[]> {
    const entries = await fs.readdir(dirPath, { withFileTypes: true });
    const fileList: string[] = [];
    for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
        const fullPath = path.join(dirPath, entry.name);
        if (entry.isDirectory()) {
            fileList.push(...await scanPageFiles(fullPath));
            continue;
        }
        if (entry.isFile() && (entry.name === "page.tsx" || entry.name === "page.jsx")) {
            fileList.push(fullPath);
        }
    }
    return fileList;
}

/**
 * 根据文件路径生成页面描述
 * @description 目录名 `[xxx]` 转换为动态参数 `:xxx`
 * @param filePath - 页面文件绝对路径
 * @param rootDir - 页面根目录
 * @returns 页面描述对象
 */
export function generatePageFromPath(filePath: string, rootDir: string): SSRPage {
    const pageDir = path.dirname(filePath);
    const relativeDir = path.relative(path.resolve(rootDir), pageDir);
    const segments = relativeDir
        .split(path.sep)
        .filter(Boolean)
        .map((segment) => {
            const dynamic = /^\[([^\]/]+)]$/.exec(segment);
            return dynamic ? `:${dynamic[1]}` : segment;
        });
    const route = segments.length === 0 ? "/" : `/${segments.join("/")}`;
    return {
        id: route,
        name: (segments.length > 0 ? segments[segments.length - 1] : "index").replace(/^:/, ""),
        route,
        filePath: path.resolve(filePath),
        enabled: true,
    };
}

/**
 * 扫描目录并生成页面路由器
 * @param dirPath - 页面目录路径
 * @returns 已注册所有页面的 PageRouter 实例
 */
export async function generateRoutes(dirPath: string): Promise<PageRouter> {
    const rootDir = path.resolve(dirPath);
    const files = await scanPageFiles(rootDir);
    const router = new PageRouter();
    router.registerPages(files.map((file) => generatePageFromPath(file, rootDir)));
    return router;
}

/**
 * 创建空的页面路由器
 * @returns 新的 PageRouter 实例
 */
export function createPageRouter(): PageRouter {
    return new PageRouter();
}

/**
 * 标准化路由路径：去除查询参数、合并斜杠、去除尾部斜杠
 * @param route - 原始路由
 * @returns 标准化后的路由
 */
export function normalizeRoute(route: string): string {
    const pathname = route.split("?", 1)[0] || "/";
    const normalized = `/${pathname}`.replace(/\/{2,}/g, "/");
    return normalized.length > 1 ? normalized.replace(/\/$/, "") : normalized;
}

/**
 * 将请求路径与路由模板匹配，支持 `:param` 动态参数
 * @param template - 路由模板（如 `/user/:id`）
 * @param route - 实际请求路径
 * @returns 解析出的参数对象，不匹配返回 null
 */
export function matchRoute(template: string, route: string): Record<string, string> | null {
    const templateParts = normalizeRoute(template).split("/").filter(Boolean);
    const routeParts = normalizeRoute(route).split("/").filter(Boolean);
    if (templateParts.length !== routeParts.length) return null;

    const params: Record<string, string> = {};
    for (let index = 0; index < templateParts.length; index += 1) {
        const templatePart = templateParts[index];
        const routePart = routeParts[index];
        if (templatePart.startsWith(":")) {
            params[templatePart.slice(1)] = decodeURIComponent(routePart);
            continue;
        }
        if (templatePart !== routePart) return null;
    }
    return params;
}

/**
 * 页面排序比较：静态段多的优先，其次动态段少的优先，最后字典序
 */
function comparePages(left: SSRPage, right: SSRPage): number {
    const leftParts = left.route.split("/").filter(Boolean);
    const rightParts = right.route.split("/").filter(Boolean);
    const leftStatic = leftParts.filter((part) => !part.startsWith(":")).length;
    const rightStatic = rightParts.filter((part) => !part.startsWith(":")).length;
    if (leftStatic !== rightStatic) return rightStatic - leftStatic;

    const leftDynamic = leftParts.length - leftStatic;
    const rightDynamic = rightParts.length - rightStatic;
    if (leftDynamic !== rightDynamic) return leftDynamic - rightDynamic;

    return left.route.localeCompare(right.route);
}
