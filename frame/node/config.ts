/**
 * @file 全局配置模块
 * @description 集中定义 SPA/SSR 构建配置、HTTP 服务器配置和渲染工作进程配置
 */
import type { SPAClientOptions } from "./page-builder/spaBuilder";
import type { SSRBuildOptions } from "./page-builder/ssrBuilder";
import type { HttpServerOptions } from "./http-transport/httpClient";

/** SPA 客户端构建配置：ESM 格式，压缩，外部 source map */
export const SPAClientConfig: SPAClientOptions = {
    entryPoint: "../../src/entry-client.tsx",
    minify: true,
    sourcemap: "external",
    external: [],
    format: "esm",
    write: true,
    outFile: "./build/entry-client.js",
    publicPath: "/assets/entry-client.js",
    target: ["esnext"],
    loader: {
        ".tsx": "tsx", ".ts": "ts", ".jsx": "jsx", ".js": "js",
        ".css": "css", ".json": "json",
        ".png": "file", ".jpg": "file", ".jpeg": "file", ".gif": "file",
        ".svg": "file", ".ico": "file", ".webp": "file", ".mp4": "file", ".mp3": "file",
    },
};

/** SSR 服务端构建配置：CJS 格式，压缩，外部 source map */
export const SSRBuildConfig: SSRBuildOptions = {
    entryPoint: "../../src/entry-server.tsx",
    minify: true,
    sourcemap: "external",
    // react/react-dom 不打入 bundle，运行时从 node_modules 加载，
    // 与 ssrRenderer 的 renderToString 保持同一份 React（否则 hooks 报 Invalid hook call）
    external: ["react", "react-dom"],
    format: "cjs",
    write: true,
    outFile: "./build/entry-server.js",
    target: ["esnext"],
    loader: {
        ".tsx": "tsx", ".ts": "ts", ".jsx": "jsx", ".js": "js",
        ".css": "css", ".json": "json",
    },
};

/** 页面构建配置 */
export interface PageBuildConfig {
    isDev: boolean;             /** 是否开发模式 */
    pagesDir: string;           /** 页面目录路径（相对 cwd） */
    spa: SPAClientOptions;      /** SPA 构建配置 */
    ssr: SSRBuildOptions;       /** SSR 构建配置 */
}

/** 页面构建默认配置 */
export const PageBuildDefaultConfig: PageBuildConfig = {
    isDev: false,
    pagesDir: "../../src",
    spa: SPAClientConfig,
    ssr: SSRBuildConfig,
};

/** HTTP 服务器配置：监听 127.0.0.1:3000，无 SSL，body 上限 10MB */
export const HttpServerConfig: HttpServerOptions = {
    port: 3000,
    host: "127.0.0.1",
    ssl: false,
    certPath: "",
    keyPath: "",
    maxBodySize: 10 * 1024 * 1024,
    timeout: 120000,
    keepAlive: true,
    keepAliveTimeout: 5000,
};

/** 渲染工作进程配置 */
export interface RenderWorkerConfig {
    callbackURL: string;            /** 回调 URL */
    internalToken?: string;         /** 内部通信 token */
    callbackTimeout: number;        /** 回调超时（毫秒） */
    maxConcurrentRenders: number;   /** 最大并发渲染数 */
}

/** 渲染工作进程默认配置，从环境变量读取回调地址和 token */
export const WorkerConfig: RenderWorkerConfig = {
    callbackURL: process.env.VEN_RENDER_CALLBACK_URL ?? "http://127.0.0.1:8080/_internal/render-callback",
    internalToken: process.env.VEN_INTERNAL_TOKEN ?? "development-token",
    callbackTimeout: 5000,
    maxConcurrentRenders: 4,
};
