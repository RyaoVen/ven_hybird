package pagecache

import (
	"strings"
	"sync"
	"time"
)

// memoryEntry 是内存后端里的缓存条目。
type memoryEntry struct {
	entry     *Entry
	expiresAt time.Time
}

// MemoryBackend 是 Backend 的内存实现（RWMutex + map + 容量上限 + 惰性过期）。
type MemoryBackend struct {
	mu       sync.RWMutex
	entries  map[string]memoryEntry
	capacity int
}

// NewMemoryBackend 创建内存缓存后端，capacity 为最大条目数。
func NewMemoryBackend(capacity int) *MemoryBackend {
	if capacity < 1 {
		capacity = 1
	}
	return &MemoryBackend{entries: make(map[string]memoryEntry), capacity: capacity}
}

// Get 查找 key，过期条目惰性删除并返回 false。
func (m *MemoryBackend) Get(key string) (*Entry, bool) {
	m.mu.RLock()
	item, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(item.expiresAt) {
		m.mu.Lock()
		// 双重检查，避免删掉他人刚写入的新条目
		if current, exists := m.entries[key]; exists && current.expiresAt.Equal(item.expiresAt) {
			delete(m.entries, key)
		}
		m.mu.Unlock()
		return nil, false
	}
	return item.entry, true
}

// Set 写入缓存条目，ttl 后过期。
// 容量满时先清理过期条目，仍满则淘汰一个近似随机条目后写入。
func (m *MemoryBackend) Set(key string, entry *Entry, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.entries) >= m.capacity {
		now := time.Now()
		for k, item := range m.entries {
			if now.After(item.expiresAt) {
				delete(m.entries, k)
			}
		}
		if len(m.entries) >= m.capacity {
			// 淘汰 map 遍历首项（近似随机），缓存正确性不依赖淘汰策略
			for k := range m.entries {
				delete(m.entries, k)
				break
			}
		}
	}
	m.entries[key] = memoryEntry{entry: entry, expiresAt: time.Now().Add(ttl)}
	return nil
}

// Delete 删除指定 key。
func (m *MemoryBackend) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
}

// DeletePrefix 删除所有以 prefix 开头的条目。
func (m *MemoryBackend) DeletePrefix(prefix string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.entries {
		if strings.HasPrefix(k, prefix) {
			delete(m.entries, k)
		}
	}
}
