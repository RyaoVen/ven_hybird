import { createPageBuild } from "./pageBuild/pageBuild";
import { httpController } from "./httpClient/controller";
import { HttpServerConfig } from "./config";

async function main(): Promise<void> {
    const pageBuild = createPageBuild();
    const buildResult = await pageBuild.build();

    if (!buildResult.success) {
        throw new Error(
            `构建失败: ${buildResult.spaError?.message ?? ""} ${buildResult.ssrError?.message ?? ""}`.trim()
        );
    }

    const controller = new httpController();
    await controller.requestDeal(async (route, payload) => {
        const page = pageBuild.getPageByRoute(route);
        if (!page) {
            return {
                html: "",
                route,
                error: `Route "${route}" not found`,
            };
        }
        return pageBuild.render(route, payload);
    });

    const pages = pageBuild.getPages().map((page) => page.route);
    console.log("页面路由:", pages);
    console.log(`HTTP 服务已启动: http://${HttpServerConfig.host}:${HttpServerConfig.port}`);
}

main().catch((error: Error) => {
    console.error("启动失败:", error.message);
    process.exit(1);
});
