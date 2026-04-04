import * as fs from "node:fs/promises";

/** 应用构建输出目录路径 */
const APP_PATH = "../../../build/app/"

/**
 * 递归读取目录，查找所有以 "page" 开头的 .jsx 或 .tsx 文件
 * @param filePath - 要读取的目录路径
 * @returns 匹配文件的文件名数组
 * @example
 */
async function readFile(filePath: string): Promise<string[]> {
    let fileList: string[] = []
    try {
        const files = await fs.readdir(filePath)
        for (const file of files) {
            const fullPath = `${filePath}/${file}`
            const stat = await fs.stat(fullPath)
            if (stat.isDirectory()) {
                await readFile(fullPath)
            } else if (
                (file.endsWith(".jsx") || file.endsWith(".tsx"))
                && file.startsWith("page")
            ) {
                fileList.push(file)
            }
        }
    } catch (err) {
        console.log(err)
    }

    return fileList
}