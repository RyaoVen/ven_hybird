# VenHybird 设计哲学

VenHybird 是 Go(Fiber) 网关 + Node SSR worker 的混合渲染框架。它把两种成熟心智模型缝在一起：

- **后端 Spring 思想**：声明式注册（`hybrid.Page(...)` 一句话挂路由+角色+数据函数）、AOP 式织入（鉴权/缓存/日志横切在框架层，业务无感）、显式声明（ISR 用 `StaticPage` 声明，失效用 `DataChange` 声明）
- **前端 Next 思想**：文件路由（`src/**/page.tsx` 是唯一真相源）、SSR 直出 + SPA 接管、ISR 物化落盘

目标：**双栈各不到 5 个 API，几乎 0 学习成本**。业务只需要写 page.tsx + 一个数据函数。

## 渲染协议：提交与回调分离

Go 不持有 Node 的连接等结果，而是 `POST /render` 提交任务（带 HookID）→ Node 立即 202 → 渲染完 POST 回 Go 的 `/_internal/render-callback`。Go 侧的 pending 注册中心按 HookID 把回调投递给等待中的请求。提交超时与渲染超时分离，并发由两侧各自的门控（pending 上限 / 渲染执行门）保护。

同一个渲染底座服务两种响应：`X-Ven-Data-Only: true` → 只回 JSON（SPA 接管后取数）；否则 SSR 直出 HTML。

## 变更事件 = 敦促更新，允许丢

`DataChange(pattern, ...params)` 是全部实时语义的源头，它的定位是**敦促系统尽快收敛到新数据**，不是可靠投递的事务消息：

- 永远异步入队、即时返回；事件总线单队列消费，debounce 合批（静默窗口 + 最大等待防饥饿）
- 批内**先删后渲**（删物化文件+清内存缓存 → 屏障 → 回源再生+落盘），批间流水，页面级代际保证老代渲染不覆盖新代，map 去重吃掉重叠范围
- **不做持久化**：进程重启时清空 ISR 物化目录、懒回源重新物化，丢的事件靠重启重载自然收敛
- 集群下经 Redis Pub/Sub 广播（同样允许丢），各实例独立走本地总线

接受秒级一致性窗口，换来失效路径的极简与永不阻塞业务。

## SSE：补 ISR 的更新不及时

ISR 文件再生完成前，用户可能看到"一块老一块新"的页面。SSE 推送补上这个窗口：事件总线每次 flush 后向 `/_internal/sse` 的订阅连接推送与首屏 `PageBootstrap` 同形的 JSON，浏览器端走 SPA router 的 setState 通道无感刷新——页面代码零改动，SSR/SPA/ISR 一视同仁。

推送同样是**敦促而非可靠投递**：慢客户端丢帧不保活，断连靠 EventSource 自动重连。选 SSE 不选 WebSocket：普通内容场景双向通信不高频，SSE 够用且零客户端代码；需要 WS 的业务自己接。

## 集群哲学：单实例行为的水平复制

多实例 = 单实例行为 + Redis 两类共享：会话/页面缓存走 Redis KV，变更事件走 Redis Pub/Sub。约束只有一条：**Go↔Node 必须 1:1 配对**（渲染回调须回到提交者实例），LB 只架在用户流量侧。ISR 目录各实例自持——共享存储（NFS/对象存储）是明确的 non-goal，跨 NFS 原子 rename 和 N 实例并发物化同一页都是运维坑，而省下的重复渲染已被 Redis 页面缓存覆盖。

## 分层纪律

`internal/` 实现一切但绝不暴露；`hybrid/` 是业务唯一引用的胶水层，签名里不出现 internal 类型；`build/` 是业务注册的地方。框架演进的默认姿势是往 internal 加能力、往 hybrid 加一句话声明，而不是让业务写框架代码。
