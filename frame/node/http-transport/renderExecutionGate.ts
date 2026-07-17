/**
 * @file 渲染执行门控模块
 * @description 基于信号量模式控制并发渲染任务数量，防止过载和重复提交
 */

/** 获取执行许可的结果：成功或失败（附带原因） */
export type AcquireResult =
    | { ok: true }
    | { ok: false; reason: "duplicate" | "capacity" };

/**
 * 渲染执行门控
 * @description 通过活跃任务集合控制并发上限和 hookId 去重
 */
export class RenderExecutionGate {
    /** 当前活跃的 hookId 集合 */
    private readonly activeHooks = new Set<string>();

    /**
     * @param maxConcurrent - 最大并发数
     * @throws maxConcurrent < 1 时抛出 Error
     */
    constructor(private readonly maxConcurrent: number) {
        if (maxConcurrent < 1) throw new Error("maxConcurrent must be greater than zero");
    }

    /**
     * 尝试获取执行许可
     * @param hookId - 任务标识
     * @returns 获取结果
     */
    acquire(hookId: string): AcquireResult {
        if (this.activeHooks.has(hookId)) return { ok: false, reason: "duplicate" };
        if (this.activeHooks.size >= this.maxConcurrent) return { ok: false, reason: "capacity" };
        this.activeHooks.add(hookId);
        return { ok: true };
    }

    /** 释放执行许可 @param hookId - 任务标识 */
    release(hookId: string): void { this.activeHooks.delete(hookId); }

    /** 获取当前活跃任务数 */
    size(): number { return this.activeHooks.size; }
}
