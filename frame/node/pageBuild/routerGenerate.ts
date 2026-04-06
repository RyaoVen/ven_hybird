import * as fs from "node:fs/promises";

/** 应用构建输出目录 */
const APP_PATH = "../../../build/app/";

/**
 * 递归读取目录，查找所有以 "page" 开头的 .jsx/.tsx 文件
 * @param filePath - 要扫描的目录路径
 * @returns 匹配的文件名数组（仅文件名，不含路径）
 * @example
 * const files = await readFile("./src/pages");
 * // 返回: ["pageHome.tsx", "pageAbout.tsx"]
 */
async function readFile(filePath: string): Promise<string[]> {
    const fileList: string[] = [];
    try {
        const files = await fs.readdir(filePath);
        for (const file of files) {
            const fullPath = `${filePath}/${file}`;
            const stat = await fs.stat(fullPath);
            if (stat.isDirectory()) {
                // 递归读取子目录，合并结果
                const subFiles = await readFile(fullPath);
                fileList.push(...subFiles);
            } else if (
                (file.endsWith(".jsx") || file.endsWith(".tsx")) &&
                file.startsWith("page")
            ) {
                fileList.push(file);
            }
        }
    } catch (err) {
        console.log(err);
    }

    return fileList;
}