/**
 * @file PageRouter 单元测试
 * @description 覆盖路由标准化、模板匹配、文件路径生成页面、匹配优先级与目录扫描
 */
import { describe, expect, it, afterEach, beforeEach } from "vitest";
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import {
    PageRouter,
    generatePageFromPath,
    matchRoute,
    normalizeRoute,
    scanPageFiles,
    type SSRPage,
} from "./pageRouter";

/** 构造测试页面描述 */
function page(route: string, enabled = true): SSRPage {
    return { id: route, name: route, route, filePath: route, enabled };
}

describe("normalizeRoute", () => {
    it("去除查询参数", () => {
        expect(normalizeRoute("/news/1?a=1&b=2")).toBe("/news/1");
    });
    it("合并连续斜杠", () => {
        expect(normalizeRoute("//news///1")).toBe("/news/1");
    });
    it("去除尾部斜杠，根路径保留", () => {
        expect(normalizeRoute("/news/")).toBe("/news");
        expect(normalizeRoute("/")).toBe("/");
    });
    it("空路径归一为根", () => {
        expect(normalizeRoute("")).toBe("/");
    });
});

describe("matchRoute", () => {
    it("静态模板全等匹配", () => {
        expect(matchRoute("/about", "/about")).toEqual({});
        expect(matchRoute("/about", "/other")).toBeNull();
    });
    it("动态段提取参数", () => {
        expect(matchRoute("/news/:id", "/news/42")).toEqual({ id: "42" });
    });
    it("多层动态段", () => {
        expect(matchRoute("/blog/:year/:id", "/blog/2026/7")).toEqual({ year: "2026", id: "7" });
    });
    it("段数不等不匹配", () => {
        expect(matchRoute("/news/:id", "/news")).toBeNull();
        expect(matchRoute("/news", "/news/1")).toBeNull();
    });
    it("参数值做 URI 解码", () => {
        expect(matchRoute("/news/:id", "/news/%E4%B8%AD%E6%96%87")).toEqual({ id: "中文" });
    });
    it("模板与路径都先标准化", () => {
        expect(matchRoute("/news/:id/", "/news/1?x=1")).toEqual({ id: "1" });
    });
});

describe("generatePageFromPath", () => {
    const root = path.join(path.sep, "site", "src");

    it("普通目录转路由", () => {
        const page = generatePageFromPath(path.join(root, "about", "page.tsx"), root);
        expect(page.route).toBe("/about");
        expect(page.name).toBe("about");
    });
    it("[id] 目录转动态段", () => {
        const page = generatePageFromPath(path.join(root, "news", "[id]", "page.tsx"), root);
        expect(page.route).toBe("/news/:id");
        expect(page.name).toBe("id");
    });
    it("多层动态段", () => {
        const page = generatePageFromPath(path.join(root, "blog", "[year]", "[id]", "page.tsx"), root);
        expect(page.route).toBe("/blog/:year/:id");
    });
    it("根目录页面映射到 /", () => {
        const page = generatePageFromPath(path.join(root, "page.tsx"), root);
        expect(page.route).toBe("/");
        expect(page.name).toBe("index");
    });
});

describe("PageRouter", () => {
    it("重复路由注册抛错", () => {
        const router = new PageRouter();
        router.registerPage(page("/about"));
        expect(() => router.registerPage(page("/about"))).toThrow(/Duplicate/);
    });
    it("disabled 页面不注册", () => {
        const router = new PageRouter();
        router.registerPage(page("/ghost", false));
        expect(router.getPageByRoute("/ghost")).toBeNull();
    });
    it("精确匹配优先，其次动态匹配并提取参数", () => {
        const router = new PageRouter();
        router.registerPages([page("/news/:id"), page("/news/hot")]);
        const exact = router.getPageByRoute("/news/hot");
        expect(exact?.page.route).toBe("/news/hot");
        expect(exact?.params).toEqual({});
        const dynamic = router.getPageByRoute("/news/42");
        expect(dynamic?.page.route).toBe("/news/:id");
        expect(dynamic?.params).toEqual({ id: "42" });
    });
    it("多个动态模板竞争时静态段多者优先", () => {
        const router = new PageRouter();
        router.registerPages([page("/:area/:id"), page("/news/:id")]);
        expect(router.getPageByRoute("/news/1")?.page.route).toBe("/news/:id");
        expect(router.getPageByRoute("/blog/1")?.page.route).toBe("/:area/:id");
    });
    it("未匹配返回 null", () => {
        const router = new PageRouter();
        router.registerPage(page("/about"));
        expect(router.getPageByRoute("/nope")).toBeNull();
    });
});

describe("scanPageFiles", () => {
    let dir = "";

    beforeEach(async () => {
        dir = await fs.mkdtemp(path.join(os.tmpdir(), "ven-pages-"));
    });
    afterEach(async () => {
        await fs.rm(dir, { recursive: true, force: true });
    });

    it("递归收集 page.tsx / page.jsx，忽略其他文件", async () => {
        await fs.mkdir(path.join(dir, "news", "[id]"), { recursive: true });
        await fs.writeFile(path.join(dir, "page.tsx"), "index");
        await fs.writeFile(path.join(dir, "news", "[id]", "page.tsx"), "news");
        await fs.writeFile(path.join(dir, "news", "[id]", "page.jsx.snap"), "not a page");
        await fs.writeFile(path.join(dir, "news", "[id]", "helper.ts"), "not a page");

        const files = await scanPageFiles(dir);
        expect(files).toHaveLength(2);
        expect(files.every((f) => /page\.(tsx|jsx)$/.test(f))).toBe(true);
        expect(files.some((f) => f.includes(path.join("news", "[id]")))).toBe(true);
    });

    it("空目录返回空数组", async () => {
        expect(await scanPageFiles(dir)).toEqual([]);
    });
});
