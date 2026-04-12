import {HttpClient, HttpHandler, HttpServer} from './httpClient';
import {HTTPClientConfig, HttpServerConfig, ResponseConfig} from "../config";
import {response} from "./type";

const PostUrl = '/post';

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
        renderHandler?: (route: string, payload: unknown) => Promise<{
            html: string;
            route: string;
            error?: string;
        }>
    ) {
        this.httpHandler.get('/health', () => {
            return {
                status: 'ok',
                uptime: process.uptime(),
            };
        });

        this.httpHandler.post(PostUrl, async (ctx) => {
            return this.requestPost(ctx.body as response);
        });

        this.httpHandler.get('*', async (ctx) => {
            if (!renderHandler) {
                return {
                    status: 503,
                    data: { error: 'Render handler is not initialized' },
                };
            }
            const renderResult = await renderHandler(ctx.path, {
                query: ctx.query,
                body: ctx.body,
                headers: ctx.headers,
                method: ctx.method,
                ip: ctx.ip,
            });
            if (renderResult.error) {
                return {
                    status: 404,
                    data: renderResult.error,
                    headers: { "Content-Type": "text/plain; charset=utf-8" },
                };
            }
            return {
                status: 200,
                data: renderResult.html,
                headers: { "Content-Type": "text/html; charset=utf-8" },
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
        const targetUrl = `${ResponseConfig.path}${ResponseConfig.url}`;
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

    getServer() {
        return this.server;
    }
}
