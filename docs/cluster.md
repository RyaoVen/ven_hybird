# 集群部署

单实例功能全集 = 集群功能全集。水平扩展只引入两类共享：**KV 共享**（会话、页面缓存，Redis）与**事件广播**（DataChange 变更，Redis Pub/Sub）。未配置 Redis 时行为与单实例开发版完全一致（内存实现、无广播）。

## 拓扑

```text
                    LB（只架在用户流量侧）
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
     Go #1       Go #2       Go #3        ← 无状态，无需 sticky session
        │1:1         │1:1         │1:1      配对（不过 LB）
        ▼           ▼           ▼
    Node #1     Node #2     Node #3       ← 每 Go 一个专属 SSR worker

        └─────┬─────┴─────┬─────┘
              ▼           ▼
        Redis（KV 共享 + 事件广播）   ISR 目录（各实例本地自持，不共享）
```

### Go ↔ Node 必须 1:1 配对

渲染回调协议要求回调回到**提交任务的那个 Go 实例**（pending registry 实例局部：hookID 实例内生成、实例内等待）。因此：

- Go 的 `VEN_NODE_WORKER_URL` 固定指向**自己的** Node worker
- 该 Node 的 `VEN_RENDER_CALLBACK_URL` 固定指回**自己的** Go
- Go↔Node 之间不过 LB；负载均衡天然发生在 LB→Go 这一层

多 Node 时不需要 Go 侧提交负载策略（轮询/最少占用），配对即均衡。

### 无需 sticky session

会话存 Redis（`ven:session:*`），任意 Go 实例都能识别任意用户；LB 用普通轮询即可。

## 两类共享的行为语义

### 页面缓存与会话（Redis KV）

- 页面缓存 `ven:page:*`：Entry JSON，1min TTL，跨实例共享渲染结果；实例 B 命中实例 A 渲染的缓存后还会**本地自愈物化**（补写自己的 ISR 文件）
- 会话 `ven:session:*`：token→role，24h TTL
- **fail-open**：Redis 故障时缓存 Get 按 miss、session Get 按未登录，错误只记日志；连接失败启动时回退内存实现
- flight 防击穿保持实例局部：跨实例同 key 并发可能各回源一次，可接受

### DataChange 事件（Redis Pub/Sub）

- 频道 `ven:events:change`；`DataChange` 本地入队的同时广播，每个实例收到后走自己的完整总线（debounce → 先删后渲 → 按本地热度再生）
- **允许丢消息**：事件本质是敦促更新，不做持久化、不做对账。丢失的实例由下次变更或重启收敛
- 防回声不加实例 ID：本实例发出的事件绕一圈回来时，落进同批静默窗口被 map 去重吞掉；删除/再生本身幂等

## ISR 目录策略：各实例自持（方案 B）

- 每个实例物化到自己的 `VEN_ISR_DIR`，事件广播天然就是失效同步机制（传播到即删除）
- 各实例按**本地**访问统计再生自己的 smartLoad Top-N（热度反映 LB 分给它的流量，更准）
- 共享存储（NFS/对象存储挂载）是 **non-goal**：跨 NFS 原子 rename、N 实例并发物化同一页都是运维坑，而省下的重复渲染已被 Redis 页面缓存覆盖

### 重启重载

实例启动时**清空自己的 ISR 目录**（仅删框架物化的 `.html`），不沿用上次运行的产物——停机期间漏收的失效没有补偿通道，重启即全量重载，之后懒回源重新物化。

## 配置对照表

| 配置 | 全集群一致 | 说明 |
|---|---|---|
| `VEN_REDIS_ADDR/PASSWORD/DB` | ✅ | 空 ADDR = 关闭（单实例内存模式） |
| `VEN_INTERNAL_TOKEN` | ✅ | Go↔Node 内部令牌必须一致 |
| `VEN_LISTEN_ADDR` | 每实例可不同 | Go 监听地址 |
| `VEN_NODE_WORKER_URL` | **每实例不同** | 指向配对的 Node |
| `VEN_RENDER_CALLBACK_URL`（Node 侧） | **每实例不同** | 指回配对的 Go |
| `VEN_ISR_DIR` | 每实例本地 | 不要指向共享存储 |
| `VEN_ISR_ENABLED` | ✅ 建议 true | 生产开启；dev 可 false |
| `VEN_MAX_PENDING_RENDERS` / `VEN_RENDER_TIMEOUT` / `VEN_NODE_SUBMIT_TIMEOUT` | ✅ | 按实例容量调整亦可 |
| `VEN_SESSION_TTL` / `VEN_PAGE_CACHE_TTL` | ✅ | 会话 / 页面缓存有效期 |
| `VEN_EVENT_QUIET_WINDOW` / `VEN_EVENT_MAX_WAIT` | ✅ | 事件总线 debounce 窗口；各实例独立计时，无需对齐 |
