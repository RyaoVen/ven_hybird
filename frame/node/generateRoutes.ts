/**
 * @file 页面路由注册表生成脚本
 * @description 扫描页面目录生成 .generated/pageRegistry.ts，供 SSR bundle 引用
 */
import * as path from "node:path";
import { PageBuildDefaultConfig } from "./config";
import { generatePageRegistry } from "./page-builder/pageRegistryGenerator";
import { generateRoutes } from "./page-builder/pageRouter";

/** 脚本主函数：扫描页面目录并生成注册表文件 */
async function main(): Promise<void> {
    const pagesDir = path.resolve(process.cwd(), PageBuildDefaultConfig.pagesDir);
    const router = await generateRoutes(pagesDir);
    await generatePageRegistry(router.getPages(), {
        outputPath: path.resolve(process.cwd(), ".generated/pageRegistry.ts"),
    });
}

main().catch((error: Error) => {
    console.error("生成页面 registry 失败:", error.message);
    process.exit(1);
});
