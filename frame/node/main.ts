import {createSPAClient} from "./pageBuild/SPAbuild";


async function main() {
    // 创建客户端
    const client = createSPAClient({
        entryPoint: "./src/entry-client.tsx",
        outFile: "./public/assets/entry-client.js",
        publicPath: "/assets/entry-client.js",
        format: "esm",
        minify: true,
    });
    // 构建并保存
    await client.buildAndSave();
}

main().catch((error) => {
    console.error("构建失败:", error);
    process.exit(1);
});
