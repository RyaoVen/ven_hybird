import { createPageBuild } from "./pageBuild/pageBuild";
import { httpController } from "./httpClient/controller";
import { request } from "./httpClient/type";
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
    await controller.requestDeal(async (task: request) => {
        const page = pageBuild.getPageByRoute(task.router);
        if (!page) {
            return {
                html: "",
                router: task.router,
                pagename: task.pagename,
                error: `Route "${task.router}" not found`,
            };
        }
        if (page.name !== task.pagename) {
            return {
                html: "",
                router: task.router,
                pagename: task.pagename,
                error: `Page name mismatch: expect "${page.name}", got "${task.pagename}"`,
            };
        }
        const renderResult = await pageBuild.render(task.router, task.payload ?? {});
        return {
            html: renderResult.html,
            router: task.router,
            pagename: task.pagename,
            error: renderResult.error,
        };
    });

    const pages = pageBuild.getPages().map((page) => page.route);
    console.log("页面路由:", pages);
    console.log("任务入口: POST /render");
    console.log(`HTTP 服务已启动: http://${HttpServerConfig.host}:${HttpServerConfig.port}`);
}

main().catch((error: Error) => {
    console.error("启动失败:", error.message);
    process.exit(1);
});
