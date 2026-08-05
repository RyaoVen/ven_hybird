// Package pagecache 提供 SSR 渲染结果的 Go 端缓存。
// 命中缓存直接返回 HTML，不回源 Node；未命中防击穿回源并回填。
package pagecache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sync/atomic"
	"time"
)

// Entry 是缓存的页面渲染结果。
type Entry struct {
	HTML         string    // 渲染出的 HTML
	MatchedRoute string    // Node 实际匹配的路由模板（诊断 + 未来按路由失效）
	PageName     string    // 页面组件名（诊断）
	RenderedAt   time.Time // 渲染完成时间（新鲜度判断，未来 SWR 挂载点）
	Duration     int64     // Node 渲染耗时 ms（诊断）
}

// Backend 页面缓存后端接口。
// 预留 Redis 等外部存储的切换空间：实现此接口即可替换，缓存逻辑不动。
type Backend interface {
	Get(key string) (*Entry, bool)
	Set(key string, entry *Entry, ttl time.Duration) error
	Delete(key string)
	// DeletePrefix 删除所有以 prefix 开头的条目（按路径手动失效用）。
	DeletePrefix(prefix string)
}

// Store 页面缓存：TTL 与防击穿语义在这里，后端只做 KV 存取。
// hits/misses/shared 为运行计数，供日志与监控观测。
type Store struct {
	backend Backend
	ttl     time.Duration // 新鲜窗口：ttl 内算缓存命中
	stale   time.Duration // stale 保留窗口：过期后仍保留（渲染失败时 stale 兜底；0 = 不保留）
	flight  *flightGroup
	hits    atomic.Uint64 // 缓存命中次数
	misses  atomic.Uint64 // 回源次数（无论结果成败）
	shared  atomic.Uint64 // flight 共享次数
}

// NewStore 创建页面缓存，backend 为底层 KV 实现。
// ttl 为条目新鲜期；stale 为过期后的物理保留期（stale-while-revalidate 用，
// 0 = 不保留，行为与无 stale 支持时一致）。
// 物理保留期 = ttl + stale：新鲜判定在 Store 层（依据 Entry.RenderedAt），
// 后端只负责存多久——这样内存与 Redis 后端无需感知新鲜语义。
func NewStore(backend Backend, ttl time.Duration, stale time.Duration) *Store {
	if stale < 0 {
		stale = 0
	}
	return &Store{backend: backend, ttl: ttl, stale: stale, flight: newFlightGroup()}
}

// Stats 返回缓存运行计数：命中、回源、共享次数。
func (s *Store) Stats() (hits, misses, shared uint64) {
	return s.hits.Load(), s.misses.Load(), s.shared.Load()
}

// fresh 判断条目是否仍在新鲜窗口内。
// RenderedAt 为零（旧格式/测试构造）视为新鲜，避免无谓的降级。
func (s *Store) fresh(entry *Entry) bool {
	return entry.RenderedAt.IsZero() || time.Since(entry.RenderedAt) < s.ttl
}

// Get 查找缓存条目，仅新鲜条目算命中；过期但仍保留的条目返回 false
// （调用方渲染失败时可经 GetStale 取回作 stale 兜底）。
func (s *Store) Get(key string) (*Entry, bool) {
	entry, ok := s.backend.Get(key)
	if !ok || !s.fresh(entry) {
		return nil, false
	}
	s.hits.Add(1)
	return entry, true
}

// GetStale 返回后端中仍保留的条目（可能已过期），供渲染失败时的 stale 兜底。
// 条目不存在或已过物理保留期（ttl+stale）返回 false。
func (s *Store) GetStale(key string) (*Entry, bool) {
	return s.backend.Get(key)
}

// Invalidate 删除指定 key 的缓存条目。
func (s *Store) Invalidate(key string) {
	s.backend.Delete(key)
}

// InvalidatePrefix 删除所有以 prefix 开头的缓存条目。
func (s *Store) InvalidatePrefix(prefix string) {
	s.backend.DeletePrefix(prefix)
}

// Do 执行带回填的回源：先查缓存，新鲜命中直接返回；
// 未命中或仅剩 stale 条目走防击穿——同 key 并发仅 leader 执行 fn，follower 等待并共享结果。
// fn 成功则先把结果回填缓存，再共享给 follower；fn 失败共享错误且不写缓存。
// 返回 shared=true 表示结果来自缓存或 flight 共享（本次未回源）。
func (s *Store) Do(key string, fn func() (*Entry, error)) (entry *Entry, shared bool, err error) {
	if item, ok := s.backend.Get(key); ok && s.fresh(item) {
		s.hits.Add(1)
		return item, true, nil
	}
	if entry, err, shared := s.flight.acquire(key); shared {
		s.shared.Add(1)
		return entry, true, err
	}
	entry, err = fn()
	s.misses.Add(1)
	if err == nil && entry != nil && entry.HTML != "" {
		// 先回填再唤醒，保证 follower 与后续请求立即可用；
		// 物理保留期 = ttl + stale，过期后仍可被 GetStale 取回
		_ = s.backend.Set(key, entry, s.ttl+s.stale)
	}
	s.flight.complete(key, entry, err)
	return entry, false, err
}

// Key 构造缓存键：path + 规范化 query + data 指纹。
// data 经 json.Marshal（map key 有序，输出确定）后取 SHA-256；
// data 无法序列化时返回 error，调用方应跳过缓存直接回源。
func Key(path string, query map[string]string, data any) (string, error) {
	canonical := make(url.Values, len(query))
	for k, v := range query {
		canonical.Set(k, v)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return path + "|" + canonical.Encode() + "|" + hex.EncodeToString(sum[:]), nil
}
