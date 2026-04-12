import * as fs from "node:fs/promises";
import * as path from "node:path";

/**
 * 页面定义
 */
export interface SSRPage {
    /** 页面名称，用于识别页面 */
    name: string;
    /** 路由路径，支持动态参数如 /about 或 /post/:id */
    route: string;
    /** 页面文件路径 */
    filePath: string;
    /** 是否启用，false 时跳过注册，默认 true */
    enabled?: boolean;
}

/**
 * 页面路由管理器
 * 负责页面注册、路由匹配、路由生成
 */
export class PageRouter {
    private pages: Map<string, SSRPage> = new Map();

    /**
     * 批量注册页面
     * @param pages - 页面定义数组
     */
    registerPages(pages: SSRPage[]): void {
        for (const page of pages) {
            this.registerPage(page);
        }
    }

    /**
     * 注册单个页面
     * @param page - 页面定义对象
     */
    registerPage(page: SSRPage): void {
        if (page.enabled === false) return;
        this.pages.set(page.route, page);
    }

    /**
     * 获取所有已注册页面
     * @returns 页面定义数组
     */
    getPages(): SSRPage[] {
        return Array.from(this.pages.values());
    }

    /**
     * 根据路由获取页面，支持动态路由匹配
     * @param route - 请求路由路径
     * @returns 匹配的页面定义，未找到返回 null
     */
    getPageByRoute(route: string): SSRPage | null {
        if (this.pages.has(route)) {
            return this.pages.get(route)!;
        }

        for (const page of this.pages.values()) {
            if (this.matchRoute(page.route, route)) {
                return page;
            }
        }

        return null;
    }

    /**
     * 动态路由匹配
     * @param definedRoute - 定义的路由模式，如 /post/:id
     * @param requestRoute - 实际请求路由，如 /post/123
     * @returns 是否匹配
     */
    private matchRoute(definedRoute: string, requestRoute: string): boolean {
        const definedParts = definedRoute.split("/").filter(Boolean);
        const requestParts = requestRoute.split("/").filter(Boolean);

        if (definedParts.length !== requestParts.length) {
            return false;
        }

        for (let i = 0; i < definedParts.length; i++) {
            const d = definedParts[i];
            const r = requestParts[i];

            if (d.startsWith(":")) continue;
            if (d !== r) return false;
        }

        return true;
    }

    /**
     * 根据页面名称获取页面
     * @param name - 页面名称
     * @returns 页面定义，未找到返回 null
     */
    getPageByName(name: string): SSRPage | null {
        for (const page of this.pages.values()) {
            if (page.name === name) {
                return page;
            }
        }
        return null;
    }

    /**
     * 移除页面
     * @param route - 页面路由
     */
    removePage(route: string): void {
        this.pages.delete(route);
    }

    /**
     * 清空所有页面
     */
    clearPages(): void {
        this.pages.clear();
    }
}

/**
 * 扫描目录，查找所有文件名为 "page" 的页面文件
 * @param dirPath - 要扫描的目录路径
 * @returns 匹配的文件路径数组
 * @example
 * const files = await scanPageFiles("./src/app");
 * // 返回: ["src/app/home/page.tsx", "src/app/blog/page.jsx"]
 */
export async function scanPageFiles(dirPath: string): Promise<string[]> {
    const fileList: string[] = [];

    try {
        const files = await fs.readdir(dirPath);
        for (const file of files) {
            const fullPath = path.join(dirPath, file);
            const stat = await fs.stat(fullPath);

            if (stat.isDirectory()) {
                const subFiles = await scanPageFiles(fullPath);
                fileList.push(...subFiles);
            } else if (file === "page.tsx" || file === "page.jsx") {
                fileList.push(fullPath);
            }
        }
    } catch (err) {
        console.error(`Failed to scan directory ${dirPath}:`, err);
    }

    return fileList;
}

/**
 * 从文件路径生成页面定义
 * 规则：
 * - 页面文件必须为目录下的 page.tsx/page.jsx
 * - 页面名 = page 文件所属目录名
 * - 路由 = 所属目录相对 app 根目录的路径
 * @param filePath - 页面文件路径
 * @param rootDir - app 根目录路径
 * @returns 页面定义对象
 */
export function generatePageFromPath(filePath: string, rootDir?: string): SSRPage {
    const pageDir = path.dirname(filePath);
    const appRoot = rootDir ? path.resolve(rootDir) : pageDir;
    const relativeDir = path.relative(appRoot, pageDir);
    const name = path.basename(pageDir);
    const normalizedRelativeDir = relativeDir.split(path.sep).filter(Boolean).join("/");
    const route = normalizedRelativeDir ? `/${normalizedRelativeDir}` : "/";

    return {
        name,
        route,
        filePath,
        enabled: true,
    };
}

/**
 * 扫描目录并自动生成页面路由
 * @param dirPath - 页面目录路径
 * @returns PageRouter 实例，包含所有扫描到的页面
 */
export async function generateRoutes(dirPath: string): Promise<PageRouter> {
    const router = new PageRouter();
    const files = await scanPageFiles(dirPath);

    for (const file of files) {
        const page = generatePageFromPath(file, dirPath);
        router.registerPage(page);
    }

    return router;
}

/**
 * 创建页面路由管理器实例
 * @returns PageRouter 实例
 */
export function createPageRouter(): PageRouter {
    return new PageRouter();
}
