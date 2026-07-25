/** 登录页组件，对应路由 "/login"，demo 登录表单 + next 回跳 */

import React, { useState } from "react";
import type { PageAppProps } from "../app/pageApp";

/**
 * 校验回跳地址，防止开放重定向。
 * 仅允许以单个 "/" 开头的站内路径（拒绝 "//evil.com" 与反斜杠）。
 * @param next - 待校验的回跳地址
 * @returns 合法返回 next，否则返回 "/"
 */
function safeNext(next: string | undefined): string {
    if (!next || !next.startsWith("/") || next.startsWith("//") || next.includes("\\")) {
        return "/";
    }
    return next;
}

/**
 * 登录页组件
 * @description demo 阶段只校验角色名（对应 POST /auth/login 的放行逻辑），
 * 接入真实登录体系后替换为用户名/密码表单。
 * @param props.bootstrap - 页面引导数据（query.next 为回跳地址）
 * @returns 登录页 React 元素
 */
export default function Login({ bootstrap }: PageAppProps) {
    const [role, setRole] = useState("guest");
    const [error, setError] = useState("");
    const next = safeNext(bootstrap.query.next);

    async function onSubmit(event: React.FormEvent) {
        event.preventDefault();
        setError("");
        const response = await fetch("/auth/login", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ role }),
        });
        if (!response.ok) {
            const body = await response.json().catch(() => ({}));
            setError(body.error ?? `登录失败（${response.status}）`);
            return;
        }
        window.location.href = next;
    }

    return (
        <main>
            <h1>登录</h1>
            <form onSubmit={onSubmit}>
                <label>
                    角色（demo）：
                    <input value={role} onChange={(event) => setRole(event.target.value)} />
                </label>
                <button type="submit">登录</button>
            </form>
            {error !== "" && <p role="alert">{error}</p>}
            <p>登录后回跳：{next}</p>
        </main>
    );
}

/** 页面元数据，用于构建时自动生成页面注册表 */
export const metadata = {
    /** 页面路由路径 */
    route: "/login"
};
