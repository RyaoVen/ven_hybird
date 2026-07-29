/**
 * @file config 单元测试
 * @description VEN_NODE_PORT 端口解析：默认值 / env 覆盖 / 非法值回退
 */
import { afterEach, describe, expect, it, vi } from "vitest";

/** 重新加载 config 模块（模块级常量在 import 时读 env，须重置模块缓存） */
async function loadConfig() {
    vi.resetModules();
    return await import("./config");
}

describe("HttpServerConfig.port（VEN_NODE_PORT）", () => {
    afterEach(() => {
        vi.unstubAllEnvs();
    });

    it("未设置时默认 3000", async () => {
        vi.stubEnv("VEN_NODE_PORT", "");
        const { HttpServerConfig } = await loadConfig();
        expect(HttpServerConfig.port).toBe(3000);
    });

    it("env 覆盖生效", async () => {
        vi.stubEnv("VEN_NODE_PORT", "3100");
        const { HttpServerConfig } = await loadConfig();
        expect(HttpServerConfig.port).toBe(3100);
    });

    it("非法值回退 3000", async () => {
        for (const bad of ["abc", "0", "-1", "70000"]) {
            vi.stubEnv("VEN_NODE_PORT", bad);
            const { HttpServerConfig } = await loadConfig();
            expect(HttpServerConfig.port).toBe(3000);
            vi.unstubAllEnvs();
        }
    });
});
