import {HttpClient, HttpHandler, HttpServer} from './httpClient';
import {HTTPClientConfig, HttpServerConfig, ResponseConfig} from "../config";
import {request, response} from "./type";

const RenderRoute = '/render';

/**
 * HTTP 控制器类
 * 用于处理 HTTP 请求和响应
 */
export class httpController {
    /**
     * HTTP 客户端实例
     */
    private httpClient: HttpClient = new HttpClient(
        HTTPClientConfig.responseURL,
        {
            headers: HTTPClientConfig.headers,
            timeout: HTTPClientConfig.timeout
        }
    )

    /**
     * HTTP 处理器实例
     */
    private httpHandler: HttpHandler = new HttpHandler();
    private server: HttpServer | null = null;

    /**
     * 处理请求
     * 注册路由并启动 HTTP 服务器
     * @returns {Promise<void>}
     */
    async requestDeal(
        renderHandler?: (task: request) => Promise<{
            html: string;
            router: string;
            pagename: string;
            error?: string;
        }>
    ) {
        this.httpHandler.get('/health', () => {
            return {
                status: 'ok',
                uptime: process.uptime(),
            };
        });

        this.httpHandler.post(RenderRoute, async (ctx) => {
            if (!renderHandler) {
                return {
                    status: 503,
                    data: { error: 'Render handler is not initialized' }
                };
            }

            const task = ctx.body as request;
            if (!task || !task.router || !task.pagename || task.hookId === undefined || task.hookId === null) {
                return {
                    status: 400,
                    data: { error: "Invalid task body: hookId/router/pagename are required" },
                };
            }

            void this.processRenderTask(task, renderHandler);

            return {
                status: 202,
                data: {
                    status: "accepted",
                    hookId: task.hookId,
                    router: task.router,
                    pagename: task.pagename,
                },
            };
        });

        this.server = new HttpServer(this.httpHandler, HttpServerConfig);
        await this.server.start();
    }

    /**
     * 发起 POST 请求
     * 使用 HttpClient 发送 HTTP POST 请求到指定地址
     * @param {response} res - 响应对象
     * @returns {Promise<unknown>} 返回响应数据
     * @throws {Error} 请求失败时抛出错误
     * @example
     * ```typescript
     * const controller = new httpController();
     * const result = await controller.requestPost(responseData);
     * console.log(result);
     * ```
     */
    public async requestPost(res:response) {
        const targetUrl = `${ResponseConfig.path}${ResponseConfig.url}` || "/";
        try {
            const response = await this.httpClient.post(
                targetUrl,
                res,
                {
                    headers: {
                        'Cookie': ResponseConfig.Cookie,
                        'Content-Type': ResponseConfig.Content_Type
                    }
                }
            );
            console.log('POST 请求成功:', response);
            return response;
        } catch (error) {
            console.error('POST 请求失败:', error);
            throw error;
        }
    }

    private async processRenderTask(
        task: request,
        renderHandler: (task: request) => Promise<{ html: string; router: string; pagename: string; error?: string }>
    ): Promise<void> {
        const startAt = Date.now();
        try {
            const result = await renderHandler(task);
            const callbackBody: response = {
                hookId: task.hookId,
                html: result.html,
                router: result.router,
                pagename: result.pagename,
                error: result.error,
                duration: Date.now() - startAt,
            };
            await this.requestPost(callbackBody);
        } catch (error) {
            const callbackBody: response = {
                hookId: task.hookId,
                html: "",
                router: task.router,
                pagename: task.pagename,
                error: (error as Error).message,
                duration: Date.now() - startAt,
            };
            try {
                await this.requestPost(callbackBody);
            } catch (postError) {
                console.error('任务回发失败:', (postError as Error).message);
            }
        }
    }

    getServer() {
        return this.server;
    }
}
