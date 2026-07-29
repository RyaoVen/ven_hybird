/**
 * @file RenderExecutionGate 单元测试
 * @description 覆盖并发门控的获取/释放/去重/容量语义
 */
import { describe, expect, it } from "vitest";
import { RenderExecutionGate } from "./renderExecutionGate";

describe("RenderExecutionGate", () => {
    it("构造参数小于 1 抛错", () => {
        expect(() => new RenderExecutionGate(0)).toThrow(/greater than zero/);
    });

    it("获取许可后重复 hookId 被拒绝", () => {
        const gate = new RenderExecutionGate(4);
        expect(gate.acquire("hook-1")).toEqual({ ok: true });
        expect(gate.acquire("hook-1")).toEqual({ ok: false, reason: "duplicate" });
    });

    it("达到并发上限后新任务被拒绝", () => {
        const gate = new RenderExecutionGate(2);
        gate.acquire("hook-1");
        gate.acquire("hook-2");
        expect(gate.acquire("hook-3")).toEqual({ ok: false, reason: "capacity" });
        expect(gate.size()).toBe(2);
    });

    it("释放后容量恢复，同 hookId 可重新获取", () => {
        const gate = new RenderExecutionGate(1);
        gate.acquire("hook-1");
        gate.release("hook-1");
        expect(gate.size()).toBe(0);
        expect(gate.acquire("hook-1")).toEqual({ ok: true });
    });

    it("释放不存在的 hookId 无副作用", () => {
        const gate = new RenderExecutionGate(2);
        gate.acquire("hook-1");
        gate.release("ghost");
        expect(gate.size()).toBe(1);
    });
});
