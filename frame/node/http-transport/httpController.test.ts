/**
 * @file HttpController 内部 token 校验测试
 * @description 覆盖 timing-safe 比较：正确/错误/长度不同/未配置放行
 */
import { afterEach, describe, expect, it } from "vitest";
import { HttpController, HttpControllerOptions } from "./httpController";
import { RenderTask } from "./types";

/** 起一个真实 HttpController 并返回请求辅助函数 */
async function startController(
    internalToken: string | undefined,
): Promise<{ post: (headers: Record<string, string>) => Promise<number>; close: () => Promise<void> }> {
    const options: HttpControllerOptions = {
        server: { port: 0, host: "127.0.0.1" },
        callbackURL: "http://127.0.0.1:1", // 不会真正回调（测试不发渲染任务）
        internalToken,
        callbackTimeout: 1000,
        maxConcurrentRenders: 4,
    };
    const controller = new HttpController(options);
    await controller.requestDeal(async () => {
        throw new Error("should not render in token tests");
    });

    const server = controller.getServer()!;
    const raw = server.getRawServer()!;
    const addr = raw.address();
    const port = typeof addr === "object" && addr ? addr.port : 3000;

    const post = async (headers: Record<string, string>): Promise<number> => {
        const res = await fetch(`http://127.0.0.1:${port}/render`, {
            method: "POST",
            headers: { "Content-Type": "application/json", ...headers },
            body: JSON.stringify({ hookId: "h1", requestRoute: "/x", payload: { route: "/x", params: {}, query: {}, initialState: null } } satisfies RenderTask),
        });
        return res.status;
    };

    return {
        post,
        close: async () => {
            await server.stop();
        },
    };
}

describe("HttpController internal token timing-safe", () => {
    const controllers: Array<{ close: () => Promise<void> }> = [];
    afterEach(async () => {
        for (const c of controllers) {
            await c.close();
        }
        controllers.length = 0;
    });

    it("正确 token 通过（202）", async () => {
        const c = await startController("secret-token-1");
        controllers.push(c);
        expect(await c.post({ "X-Ven-Internal-Token": "secret-token-1" })).toBe(202);
    });

    it("错误 token 拒绝（401）", async () => {
        const c = await startController("secret-token-1");
        controllers.push(c);
        expect(await c.post({ "X-Ven-Internal-Token": "wrong-token!" })).toBe(401);
    });

    it("长度不同拒绝（401）", async () => {
        const c = await startController("secret-token-1");
        controllers.push(c);
        expect(await c.post({ "X-Ven-Internal-Token": "short" })).toBe(401);
        expect(await c.post({ "X-Ven-Internal-Token": "secret-token-1-longer-than-expected" })).toBe(401);
    });

    it("缺失 token 拒绝（401）", async () => {
        const c = await startController("secret-token-1");
        controllers.push(c);
        expect(await c.post({})).toBe(401);
    });

    it("internalToken 未配置时放行（202）", async () => {
        const c = await startController(undefined);
        controllers.push(c);
        expect(await c.post({})).toBe(202);
    });
});
