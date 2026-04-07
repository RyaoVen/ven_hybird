import type { SPAClientOptions } from "./pageBuild/SPAbuild";
import type { SSRBuildOptions } from "./pageBuild/SSRbuild";

/**
 * SPA 客户端构建配置
 */
export const SPAClientConfig: SPAClientOptions = {
    entryPoint: "../../../build/app/entry-client.tsx",
    minify: true,
    sourcemap: "external",
    external: [],
    format: "esm",
    write: true,
    outFile: "../../build/entry-client.js",
    publicPath: "/",
    target: ["esnext"],
    loader: {
        ".tsx": "tsx",
        ".ts": "ts",
        ".jsx": "jsx",
        ".js": "js",
        ".css": "css",
        ".json": "json",
        ".png": "file",
        ".jpg": "file",
        ".jpeg": "file",
        ".gif": "file",
        ".svg": "file",
        ".ico": "file",
        ".webp": "file",
        ".mp4": "file",
        ".mp3": "file",
    },
};

/**
 * SSR 服务端构建配置
 */
export const SSRBuildConfig: SSRBuildOptions = {
    entryPoint: "../../../build/app/entry-server.tsx",
    minify: true,
    sourcemap: "external",
    external: [],
    format: "cjs",
    write: true,
    outFile: "../../build/entry-server.js",
    target: ["esnext"],
    loader: {
        ".tsx": "tsx",
        ".ts": "ts",
        ".jsx": "jsx",
        ".js": "js",
        ".css": "css",
        ".json": "json",
    },
};

/**
 * PageBuild 总体配置
 */
export interface PageBuildConfig {
    /** 是否开发模式，开发模式支持热更新 */
    isDev: boolean;
    /** 页面源码目录，用于扫描页面文件 */
    pagesDir: string;
    /** 构建输出目录 */
    outputDir: string;
    /** SSR 模块输出目录（编译后的页面模块） */
    ssrPagesDir: string;
    /** SPA 客户端构建配置 */
    spa: SPAClientOptions;
    /** SSR 服务端构建配置 */
    ssr: SSRBuildOptions;
}

/**
 * 默认 PageBuild 配置
 */
export const PageBuildDefaultConfig: PageBuildConfig = {
    isDev: false,
    pagesDir: "../../../build/app/pages",
    outputDir: "../../build",
    ssrPagesDir: "../../build/pages",
    spa: SPAClientConfig,
    ssr: SSRBuildConfig,
};