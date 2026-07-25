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
	ttl     time.Duration
	flight  *flightGroup
	hits    atomic.Uint64 // 缓存命中次数
	misses  atomic.Uint64 // 回源次数（无论结果成败）
	shared  atomic.Uint64 // flight 共享次数
}

// NewStore 创建页面缓存，backend 为底层 KV 实现，ttl 为条目有效期。
func NewStore(backend Backend, ttl time.Duration) *Store {
	return &Store{backend: backend, ttl: ttl, flight: newFlightGroup()}
}

// Stats 返回缓存运行计数：命中、回源、共享次数。
func (s *Store) Stats() (hits, misses, shared uint64) {
	return s.hits.Load(), s.misses.Load(), s.shared.Load()
}

// Get 查找缓存条目，未命中或已过期返回 false。
func (s *Store) Get(key string) (*Entry, bool) {
	entry, ok := s.backend.Get(key)
	if ok {
		s.hits.Add(1)
	}
	return entry, ok
}

// Invalidate 删除指定 key 的缓存条目。
func (s *Store) Invalidate(key string) {
	s.backend.Delete(key)
}

// InvalidatePrefix 删除所有以 prefix 开头的缓存条目。
func (s *Store) InvalidatePrefix(prefix string) {
	s.backend.DeletePrefix(prefix)
}

// Do 执行带回填的回源：先查缓存，命中直接返回；
// 未命中走防击穿——同 key 并发仅 leader 执行 fn，follower 等待并共享结果。
// fn 成功则先把结果回填缓存，再共享给 follower；fn 失败共享错误且不写缓存。
// 返回 shared=true 表示结果来自缓存或 flight 共享（本次未回源）。
func (s *Store) Do(key string, fn func() (*Entry, error)) (entry *Entry, shared bool, err error) {
	if entry, ok := s.backend.Get(key); ok {
		s.hits.Add(1)
		return entry, true, nil
	}
	if entry, err, shared := s.flight.acquire(key); shared {
		s.shared.Add(1)
		return entry, true, err
	}
	entry, err = fn()
	s.misses.Add(1)
	if err == nil && entry != nil && entry.HTML != "" {
		// 先回填再唤醒，保证 follower 与后续请求立即可用
		_ = s.backend.Set(key, entry, s.ttl)
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
