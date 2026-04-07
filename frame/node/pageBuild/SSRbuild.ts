import * as esbuild from "esbuild";
import * as fs from "node:fs/promises";
import * as path from "node:path";

/**
 * SSR 构建配置选项
 */
export interface SSRBuildOptions {
    /** 服务端入口文件路径 */
    entryPoint: string;
    /** 是否压缩代码，默认 false */
    minify?: boolean;
    /** source map 配置：false | "inline" | "external"，默认 false */
    sourcemap?: boolean | "inline" | "external";
    /** 外部依赖列表，不参与打包 */
    external?: string[];
    /** 输出格式："cjs" 或 "esm"，默认 "cjs" */
    format?: "cjs" | "esm";
    /** 是否直接写入磁盘，默认 false（返回代码字符串） */
    write?: boolean;
    /** 输出文件路径，write=true 时必填 */
    outFile?: string;
    /** 编译目标，默认 ["node18"] */
    target?: string[];
    /** 自定义文件 loader，默认支持 .tsx/.jsx/.ts/.js/.json */
    loader?: Record<string, esbuild.Loader>;
}

/**
 * SSR 构建结果
 */
export interface SSRBuildResult {
    /** 编译后的服务端代码（write=false 时可用） */
    serverCode?: string;
    /** source map 内容 */
    map?: string;
    /** 入口文件路径 */
    entryPoint: string;
    /** 输出文件路径 */
    outputFile?: string;
    /** 输出格式 */
    format: "cjs" | "esm";
    /** 是否已写入磁盘 */
    writtenToDisk: boolean;
}

/**
 * SSR 构建器
 * 基于 esbuild 编译服务端入口为 Node.js 可执行代码
 */
export class SSRBuild {
    private options: Required<Omit<SSRBuildOptions, "outFile">> & {
        outFile?: string;
    };

    private buildResult: SSRBuildResult | null = null;

    /**
     * 创建构建器实例
     * @param options - SSR 构建配置选项
     */
    constructor(options: SSRBuildOptions) {
        this.options = {
            minify: false,
            sourcemap: false,
            external: [],
            format: "cjs",
            write: false,
            target: ["node18"],
            loader: {
                ".tsx": "tsx",
                ".jsx": "jsx",
                ".ts": "ts",
                ".js": "js",
                ".json": "json",
            },
            ...options,
        };
    }

    /**
     * 编译服务端入口文件
     * @returns 构建结果对象
     * @throws write=true 但未指定 outFile 时抛出错误
     */
    async build(): Promise<SSRBuildResult> {
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
            platform: "node",
            target: this.options.target,
            write: this.options.write,
            outfile: this.options.outFile,
            jsx: "automatic",
            loader: this.options.loader,
        });

        const serverCode = !this.options.write
            ? result.outputFiles?.find((f) => f.path.endsWith(".js"))?.text
            : undefined;

        const map = !this.options.write
            ? result.outputFiles?.find((f) => f.path.endsWith(".map"))?.text
            : undefined;

        this.buildResult = {
            serverCode,
            map,
            entryPoint: this.options.entryPoint,
            outputFile: this.options.outFile,
            format: this.options.format,
            writtenToDisk: this.options.write,
        };

        return this.buildResult;
    }

    /**
     * 保存构建结果到磁盘
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

        if (!this.buildResult.serverCode) {
            throw new Error("serverCode is empty");
        }

        await fs.mkdir(path.dirname(this.options.outFile), { recursive: true });
        await fs.writeFile(this.options.outFile, this.buildResult.serverCode, "utf-8");

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
    async buildAndSave(): Promise<SSRBuildResult> {
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
    getResult(): SSRBuildResult | null {
        return this.buildResult;
    }

    /**
     * 获取服务端代码字符串
     * @returns 代码字符串，未构建时返回 null
     */
    getCode(): string | null {
        return this.buildResult?.serverCode ?? null;
    }
}

/**
 * 创建 SSR 构建器实例
 * @param options - SSR 构建配置选项
 * @returns SSRBuild 实例
 */
export function createSSRBuild(options: SSRBuildOptions): SSRBuild {
    return new SSRBuild(options);
}