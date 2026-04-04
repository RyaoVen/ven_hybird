import * as esbuild from "esbuild";
import * as fs from "node:fs/promises";
import * as path from "node:path";

/** SPA 客户端配置选项 */
export interface SPAClientOptions {
    /** 入口文件路径 */
    entryPoint: string;
    /** 是否压缩代码 */
    minify?: boolean;
    /** 是否生成 source map */
    sourcemap?: boolean | "inline" | "external";
    /** 外部依赖列表 */
    external?: string[];
    /** 输出格式，esm 对应 type="module" */
    format?: "esm" | "iife";
    /** 是否直接写入磁盘 */
    write?: boolean;
    /** 输出文件路径 */
    outFile?: string;
    /** 浏览器访问路径 */
    publicPath?: string;
    /** 编译目标 */
    target?: string[];
    /** 自定义 loader */
    loader?: Record<string, esbuild.Loader>;
}

/** SPA 客户端构建结果 */
export interface SPAClientBuildResult {
    /** 编译后的客户端代码 */
    clientCode?: string;
    /** source map */
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
 * 用于将 TSX/JSX 组件编译为客户端代码，支持 SSR 水合
 */
export class SPAClient {
    private options: Required<
        Omit<SPAClientOptions, "outFile" | "publicPath">
    > & {
        outFile?: string;
        publicPath?: string;
    };

    private buildResult: SPAClientBuildResult | null = null;

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

    /** 编译入口文件为客户端代码 */
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

    /** 保存当前构建结果到磁盘 */
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

    /** 编译并保存 */
    async buildAndSave(): Promise<SPAClientBuildResult> {
        const result = await this.build();
        if (!this.options.write) {
            await this.save();
        }
        return result;
    }

    /** 获取当前构建结果 */
    getResult(): SPAClientBuildResult | null {
        return this.buildResult;
    }

    /** 获取客户端代码字符串 */
    getCode(): string | null {
        return this.buildResult?.clientCode ?? null;
    }

    /** 获取脚本标签 */
    getScriptTag(): string {
        const publicPath = this.options.publicPath ?? this.buildResult?.publicPath;
        if (!publicPath) {
            throw new Error("publicPath is required");
        }

        return this.options.format === "esm"
            ? `<script type="module" src="${publicPath}"></script>`
            : `<script src="${publicPath}"></script>`;
    }

    /** 获取构建产物信息 */
    getAssetInfo() {
        return {
            outputFile: this.options.outFile ?? this.buildResult?.outputFile,
            publicPath: this.options.publicPath ?? this.buildResult?.publicPath,
            format: this.options.format,
            scriptTag: this.getScriptTag(),
        };
    }
}

/** 创建 SPA 客户端实例 */
export function createSPAClient(options: SPAClientOptions): SPAClient {
    return new SPAClient(options);
}
