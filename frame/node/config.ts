import type { SPAClientOptions } from "./pageBuild/SPAbuild";
import type { SSRBuildOptions } from "./pageBuild/SSRbuild";
import type { LoadConfig, FilterMode } from "./pageBuild/SSRrenderController";
import {HttpServerOptions} from "./httpClient/httpClient";
import {response} from "./httpClient/type";

/**
 * SPA 客户端构建配置
 */
export const SPAClientConfig: SPAClientOptions = {
    entryPoint: "./build/app/entry-client.tsx",
    minify: true,
    sourcemap: "external",
    external: [],
    format: "esm",
    write: true,
    outFile: "./build/entry-client.js",
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
    entryPoint: "./build/app/entry-server.tsx",
    minify: true,
    sourcemap: "external",
    external: [],
    format: "cjs",
    write: true,
    outFile: "./build/entry-server.js",
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
 * 加载配置
 */
export const LoadConfigDefault: LoadConfig = {
    filterMode: "none" as FilterMode,
    filterRoutes: [],
    preload: false,
    preloadRoutes: [],
    slowLoad: false,
    slowLoadRoutes: [],
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
    /** 加载配置（预加载、慢加载、过滤） */
    loadConfig?: LoadConfig;
}

/**
 * 默认 PageBuild 配置
 */
export const PageBuildDefaultConfig: PageBuildConfig = {
    isDev: false,
    pagesDir: "./build/app/pages",
    outputDir: "./build",
    ssrPagesDir: "./build",
    spa: SPAClientConfig,
    ssr: SSRBuildConfig,
    loadConfig: LoadConfigDefault,
};

export const HTTPClientConfig = {
    responseURL: "http://localhost:3000",
    headers: {
        "Content-Type": "application/json",
    },
    timeout: 5000,

};

export const HttpServerConfig:HttpServerOptions = {
    port: 3000,
    host: "0.0.0.0",
    ssl: false,
    certPath: "",
    keyPath: "",
    maxBodySize: 10 * 1024 * 1024,
    timeout: 120000,
    keepAlive: true,
    keepAliveTimeout: 5000
}
export const ResponseConfig = {
    Cookie: '',
    Content_Type:'application/json',
    path:'/',
    url:''
}
