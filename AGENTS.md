# AGENTS.md

VenHybird：Go(Fiber) 网关 + Node SSR worker 的混合渲染框架。设计哲学见 [vision.md](vision.md)，使用文档见 [README.md](README.md)。

## 布局与三层纪律

- `src/**/page.tsx`：React 页面，Node 是页面路由的唯一真相源（`[id]` → `:id`，支持多层动态）
- `frame/node`（:3000）：registry 生成、单 bundle SSR/SPA、渲染执行门、`POST /render`（202+回调）、`GET /pages`
- `frame/go`（:8080）：
  - `internal/`：全部实现（config / ssr / httpserver / auth / pagecache / pagepattern / isr / event / sse / redis）。**internal 的类型绝不暴露到 hybrid 的公开签名**
  - `hybrid/`：胶水层，业务唯一引用的包（`App` / `Page` / `StaticPage` / `Get|Post|Put|Delete` / `PageCtx` / `ApiCtx` / `DataChange` 等）
  - `build/`：业务注册入口（`Register` 空壳），改业务只动这里
- `main.go`：启动编排（配置 → Node client → 拉 `/pages` → server → hybrid.New → build.Register → Listen）

## 构建与检查（提交前必跑）

```bash
cd frame/go && go build ./... && go vet ./... && go test ./...
cd frame/node && npm run typecheck && npm test
```

启动：Node 先（`cd frame/node && npm run build && node dist/main.js`），Go 后（`cd frame/go && go run .`，启动依赖 Node 在线拉路由表）。

## 测试风格

- Go：fake SSR client（channel 收任务）+ `pending.Resolve` 注入回调；`httptest` 起假 Node；ISR 目录用 `t.TempDir()`；配置用字面量 `config.Config{...}`（缺省字段走零值回退）
- Node：vitest，只测纯逻辑（pageRouter / renderExecutionGate 等），不碰 DOM/React
- 本机跑不了 `-race`（0xc0000139 DLL 问题），并发正确性靠设计 + 普通并发用例

## 关键坑（都踩过）

- **fasthttp 零拷贝字符串不能跨请求留存**：`ctx.Path()` 等底层是池化缓冲区，留存前必须 `strings.Clone`
- fiber 按注册顺序匹配：内部路由 → ISR 中间件 → 业务页面 → 兜底（兜底由 `hybrid.App.Listen` 内置，必须最后）
- SSR bundle 必须 external `react`/`react-dom`，否则 hooks 报 Invalid hook call
- Windows 杀进程用 `taskkill //PID //F`（`go run` 会留子进程，按端口 netstat 找 PID）
- github.com:443 本机不通：push 走 `gh-ssh` 远端（ssh://git@ssh.github.com:443），gh CLI 正常；`gh pr merge` 本地 ff 步骤常失败但服务器端已合并，以 `gh pr view <N> --json state` 为准再 `git pull gh-ssh master`
- C 盘空间紧张，编译莫名失败先看磁盘

## Git 工作流红线

- 一单元一 issue → 分支 `<type>/issue-<N>-<slug>` → 小步 conventional commits `<type>(<scope>): 中文描述 (#N)` → 检查通过 → push → PR（body：做了什么/怎么验证/`Closes #N`）→ `gh pr merge --squash --delete-branch` → 回 master 同步
- 不直推 master、不 force-push、不改写已 push 历史、不动他人 PR/issue
