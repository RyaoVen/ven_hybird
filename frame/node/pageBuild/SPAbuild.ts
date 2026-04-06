import * as esbuild from "esbuild";
import * as fs from "node:fs/promises";
import * as path from "node:path";

/**
 * SPA 客户端配置选项
 */
export interface SPAClientOptions {
    /** 入口文件路径 */
    entryPoint: string;
    /** 是否压缩代码，默认 false */
    minify?: boolean;
    /** source map 配置：false | "inline" | "external"，默认 false */
    sourcemap?: boolean | "inline" | "external";
    /** 外部依赖列表，不参与打包 */
    external?: string[];
    /** 输出格式："esm" 对应 type="module"，"iife" 为普通脚本，默认 "esm" */
    format?: "esm" | "iife";
    /** 是否直接写入磁盘，默认 false（返回代码字符串） */
    write?: boolean;
    /** 输出文件路径，write=true 时必填 */
    outFile?: string;
    /** 浏览器访问路径，用于生成 script 标签 */
    publicPath?: string;
    /** 编译目标，默认 ["es2020"] */
    target?: string[];
    /** 自定义文件 loader，默认支持 .tsx/.jsx/.ts/.js */
    loader?: Record<string, esbuild.Loader>;
}

/**
 * SPA 客户端构建结果
 */
export interface SPAClientBuildResult {
    /** 编译后的客户端代码（write=false 时可用） */
    clientCode?: string;
    /** source map 内容 */
    map?: string;
    /** 入口文件路径 */
    entryPoint: string;
    /** 输出文件路径 */
    outputFile?: string;
    /** 浏览器访问路径 */
    publicPath?: string;
    /** 输出格式 */
    format: "esm" | "iife";
    /** 是否已写入磁盘 */
    writtenToDisk: boolean;
}

/**
 * SPA React 客户端构建器
 * 基于 esbuild 编译 TSX/JSX 为浏览器可执行代码，用于 SSR 水合
 */
export class SPAClient {
    private options: Required<
        Omit<SPAClientOptions, "outFile" | "publicPath">
    > & {
        outFile?: string;
        publicPath?: string;
    };

    private buildResult: SPAClientBuildResult | null = null;

    /**
     * 创建构建器实例
     * @param options - SPA 客户端配置选项
     */
    constructor(options: SPAClientOptions) {
        this.options = {
            minify: false,
            sourcemap: false,
            external: [],
            format: "esm",
            write: false,
            target: ["es2020"],
            loader: {
                ".tsx": "tsx",
                ".jsx": "jsx",
                ".ts": "ts",
                ".js": "js",
            },
            ...options,
        };
    }

    /**
     * 编译入口文件为客户端代码
     * @returns 构建结果对象
     * @throws write=true 但未指定 outFile 时抛出错误
     */
    async build(): Promise<SPAClientBuildResult> {
        if (this.options.write && !this.options.outFile) {
            throw new Error("outFile is required when write=true");
        }

        const result = await esbuild.build({
            entryPoints: [this.options.entryPoint],
            bundle: true,
            minify: this.options.minify,
            sourcemap: this.options.sourcemap,
            external: this.options.external,
            format: this.options.format,
            platform: "browser",
            target: this.options.target,
            write: this.options.write,
            outfile: this.options.outFile,
            jsx: "automatic",
            loader: this.options.loader,
        });

        const clientCode = !this.options.write
            ? result.outputFiles?.find((f) => f.path.endsWith(".js"))?.text
            : undefined;

        const map = !this.options.write
            ? result.outputFiles?.find((f) => f.path.endsWith(".map"))?.text
            : undefined;

        this.buildResult = {
            clientCode,
            map,
            entryPoint: this.options.entryPoint,
            outputFile: this.options.outFile,
            publicPath: this.options.publicPath,
            format: this.options.format,
            writtenToDisk: this.options.write,
        };

        return this.buildResult;
    }

    /**
     * 保存构建结果到磁盘
     * @returns 无返回值
     * @throws 未调用 build()、未指定 outFile、代码为空时抛出错误
     */
    async save(): Promise<void> {
        if (!this.buildResult) {
            throw new Error("build() must be called before save()");
        }

        if (!this.options.outFile) {
            throw new Error("outFile is required");
        }

        if (this.options.write) {
            return;
        }

        if (!this.buildResult.clientCode) {
            throw new Error("clientCode is empty");
        }

        await fs.mkdir(path.dirname(this.options.outFile), { recursive: true });
        await fs.writeFile(this.options.outFile, this.buildResult.clientCode, "utf-8");

        if (this.buildResult.map && this.options.sourcemap === "external") {
            await fs.writeFile(
                `${this.options.outFile}.map`,
                this.buildResult.map,
                "utf-8"
            );
        }
    }

    /**
     * 编译并保存到磁盘
     * @returns 构建结果对象
     */
    async buildAndSave(): Promise<SPAClientBuildResult> {
        const result = await this.build();
        if (!this.options.write) {
            await this.save();
        }
        return result;
    }

    /**
     * 获取当前构建结果
     * @returns 构建结果对象，未构建时返回 null
     */
    getResult(): SPAClientBuildResult | null {
        return this.buildResult;
    }

    /**
     * 获取客户端代码字符串
     * @returns 代码字符串，未构建时返回 null
     */
    getCode(): string | null {
        return this.buildResult?.clientCode ?? null;
    }

    /**
     * 生成 HTML script 标签
     * @returns script 标签字符串，esm 格式为 `<script type="module">`
     * @throws 未配置 publicPath 时抛出错误
     */
    getScriptTag(): string {
        const publicPath = this.options.publicPath ?? this.buildResult?.publicPath;
        if (!publicPath) {
            throw new Error("publicPath is required");
        }

        return this.options.format === "esm"
            ? `<script type="module" src="${publicPath}"></script>`
            : `<script src="${publicPath}"></script>`;
    }

    /**
     * 获取构建产物信息摘要
     * @returns 包含 outputFile、publicPath、format、scriptTag 的对象
     */
    getAssetInfo(): {
        outputFile: string | undefined;
        publicPath: string | undefined;
        format: "esm" | "iife";
        scriptTag: string;
    } {
        return {
            outputFile: this.options.outFile ?? this.buildResult?.outputFile,
            publicPath: this.options.publicPath ?? this.buildResult?.publicPath,
            format: this.options.format,
            scriptTag: this.getScriptTag(),
        };
    }
}

/**
 * 创建 SPA 客户端构建器实例
 * @param options - SPA 客户端配置选项
 * @returns SPAClient 实例
 */
export function createSPAClient(options: SPAClientOptions): SPAClient {
    return new SPAClient(options);
}