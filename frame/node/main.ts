/**
 * @file SSR 渲染服务入口
 * @description 构建页面产物后启动 HTTP 服务器，接收渲染请求并通过回调返回结果
 */
import { HttpController } from "./http-transport/httpController";
import { RenderTask } from "./http-transport/types";
import { HttpServerConfig, WorkerConfig } from "./config";
import { createPageBuild } from "./page-builder/pageBuilder";

/**
 * 服务主函数：构建 -> 启动 HTTP 服务 -> 注册渲染处理
 * @throws 构建失败时抛出 Error
 */
async function main(): Promise<void> {
    const pageBuild = createPageBuild();
    const buildResult = await pageBuild.build();
    if (!buildResult.success) {
        throw new Error(
            `构建失败: ${buildResult.spaError?.message ?? ""} ${buildResult.ssrError?.message ?? ""}`.trim(),
        );
    }

    const controller = new HttpController({
        server: HttpServerConfig,
        callbackURL: WorkerConfig.callbackURL,
        internalToken: WorkerConfig.internalToken,
        callbackTimeout: WorkerConfig.callbackTimeout,
        maxConcurrentRenders: WorkerConfig.maxConcurrentRenders,
    });

    await controller.requestDeal(
        async (task: RenderTask) => {
            return pageBuild.render(task.requestRoute, task.payload);
        },
        () => pageBuild.getRouter().getPages().map((page) => page.route),
    );

    // 优雅关停：收到退出信号时停止接收新请求并排空中断连接
    const shutdown = async (): Promise<void> => {
        console.log("shutdown signal received, stopping...");
        try {
            await controller.getServer()?.stop();
        } catch (error) {
            console.error("graceful stop failed:", (error as Error).message);
        }
        process.exit(0);
    };
    process.on("SIGINT", shutdown);
    process.on("SIGTERM", shutdown);

    console.log("页面路由:", pageBuild.getRouter().getPages().map((page) => page.route));
    console.log("任务入口: POST /render");
    console.log("页面模式: GET /pages");
    console.log(`回调地址: ${WorkerConfig.callbackURL}`);
    console.log(`HTTP 服务已启动: http://${HttpServerConfig.host}:${HttpServerConfig.port}`);
}

main().catch((error: Error) => {
    console.error("启动失败:", error.message);
    process.exit(1);
});
