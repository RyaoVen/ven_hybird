package isr

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// accessEntry 是单个物化文件的访问统计（内存态，重启清零）。
type accessEntry struct {
	count      uint64
	lastAccess time.Time
}

// Store 是 ISR 文件层：落盘目录、直发中间件、访问统计与淘汰。
type Store struct {
	dir     string
	enabled bool

	mu     sync.RWMutex
	access map[string]*accessEntry // 相对 URL 路径 → 访问统计
}

// NewStore 创建 ISR 文件层。enabled=false 时中间件永不命中（dev 用）。
func NewStore(dir string, enabled bool) *Store {
	return &Store{dir: dir, enabled: enabled, access: make(map[string]*accessEntry)}
}

// Enabled 返回 ISR 是否启用。
func (s *Store) Enabled() bool {
	return s.enabled
}

// filePathFor 把 URL 路径映射为落盘文件路径（防穿越）。
func (s *Store) filePathFor(urlPath string) (string, bool) {
	clean := strings.Trim(urlPath, "/")
	if clean == "" {
		clean = "index"
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == ".." || segment == "." {
			return "", false
		}
	}
	full := filepath.Join(s.dir, filepath.FromSlash(clean)+".html")
	if !strings.HasPrefix(full, filepath.Clean(s.dir)+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}

// dataOnlyHeader 是 SPA router 取数请求头（值为 "true" 时下游应返回 JSON 而非 HTML）。
// 与 hybrid/page.go 的 dataOnlyHeader 同源：分层纪律 internal 不 import hybrid，本地重新定义。
const dataOnlyHeader = "X-Ven-Data-Only"

// Middleware 命中物化文件直接发回，miss 放行后续路由。
// 同时记录访问统计（供热门判定与 LRU）。
// 用 ReadFile 而非 SendFile：避免 fasthttp 句柄池在 Windows 上锁住文件
// （锁住后原子 rename 与删除都会失败）。
func (s *Store) Middleware() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		if !s.enabled || (ctx.Method() != fiber.MethodGet && ctx.Method() != fiber.MethodHead) {
			return ctx.Next()
		}
		// data-only 取数（SPA 跳转）要的是 JSON 而非整页 HTML：
		// 物化文件截胡会让 router 解析失败卡死，直接放行给下游 handler；
		// 此类请求不计页面访问统计
		if ctx.Get(dataOnlyHeader) == "true" {
			return ctx.Next()
		}
		full, ok := s.filePathFor(ctx.Path())
		if !ok {
			return ctx.Next()
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return ctx.Next()
		}
		// ctx.Path() 是 fasthttp 零拷贝字符串（底层是池化缓冲区），
		// 跨请求留存前必须克隆，否则缓冲区复用后 map key 内容会被篡改
		s.RecordAccess(strings.Clone(ctx.Path()))
		ctx.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return ctx.SendString(string(data))
	}
}

// RecordAccess 记录一次访问（计数 + 最后访问时间）。
func (s *Store) RecordAccess(urlPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.access[urlPath]
	if !ok {
		entry = &accessEntry{}
		s.access[urlPath] = entry
	}
	entry.count++
	entry.lastAccess = time.Now()
}

// AccessCount 返回某路径的访问计数（测试与调试用）。
func (s *Store) AccessCount(urlPath string) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry, ok := s.access[urlPath]; ok {
		return entry.count
	}
	return 0
}

// Exists 判断某路径是否已有物化文件。
func (s *Store) Exists(urlPath string) bool {
	full, ok := s.filePathFor(urlPath)
	if !ok {
		return false
	}
	info, err := os.Stat(full)
	return err == nil && !info.IsDir()
}

// Materialize 原子写入物化文件（临时文件 + rename），并记录访问。
func (s *Store) Materialize(urlPath string, html string) error {
	full, ok := s.filePathFor(urlPath)
	if !ok {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, []byte(html), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	s.RecordAccess(urlPath)
	return nil
}

// Delete 删除指定路径的物化文件与统计，返回删除数（幂等）。
func (s *Store) Delete(urlPaths ...string) int {
	deleted := 0
	for _, urlPath := range urlPaths {
		full, ok := s.filePathFor(urlPath)
		if !ok {
			continue
		}
		if err := os.Remove(full); err == nil {
			deleted++
		}
		s.mu.Lock()
		delete(s.access, urlPath)
		s.mu.Unlock()
	}
	return deleted
}

// Invalidate 删除匹配器范围内的全部物化文件，返回被删路径列表。
func (s *Store) Invalidate(matcher *Matcher) ([]string, error) {
	var matched []string
	root := filepath.Clean(s.dir)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		urlPath := "/" + strings.TrimSuffix(filepath.ToSlash(relative), ".html")
		if urlPath == "/index" {
			urlPath = "/"
		}
		if matcher.Match(urlPath) {
			matched = append(matched, urlPath)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	s.Delete(matched...)
	return matched, nil
}

// ClearAll 清空全部物化文件与访问统计，返回清空数量。
// 启动重载用：变更事件不做持久化，停机期间漏收的失效没有补偿通道，
// 因此实例重启不沿用上次运行的产物，清空后懒回源重新物化（目录本身保留）。
func (s *Store) ClearAll() int {
	if !s.enabled {
		return 0
	}
	removed := 0
	root := filepath.Clean(s.dir)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		if err := os.Remove(path); err == nil {
			removed++
		}
		return nil
	})
	s.mu.Lock()
	s.access = make(map[string]*accessEntry)
	s.mu.Unlock()
	return removed
}

// HotPaths 返回匹配器范围内按访问计数降序的前 limit 个路径。
func (s *Store) HotPaths(matcher *Matcher, limit int) []string {
	s.mu.RLock()
	type scored struct {
		path  string
		count uint64
	}
	var candidates []scored
	for path, entry := range s.access {
		if matcher.Match(path) {
			candidates = append(candidates, scored{path, entry.count})
		}
	}
	s.mu.RUnlock()
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].count != candidates[j].count {
			return candidates[i].count > candidates[j].count
		}
		return candidates[i].path < candidates[j].path
	})
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, candidate.path)
	}
	return paths
}

// CountFiles 返回匹配器范围内的物化文件数。
func (s *Store) CountFiles(matcher *Matcher) int {
	count := 0
	root := filepath.Clean(s.dir)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		urlPath := "/" + strings.TrimSuffix(filepath.ToSlash(relative), ".html")
		if urlPath == "/index" {
			urlPath = "/"
		}
		if matcher.Match(urlPath) {
			count++
		}
		return nil
	})
	return count
}

// EvictLRU 删除匹配器范围内最久未访问的文件，使文件数降到 max。
// 返回淘汰的路径列表。max <= 0 时不做任何事。
func (s *Store) EvictLRU(matcher *Matcher, max int) []string {
	if max <= 0 {
		return nil
	}
	total := s.CountFiles(matcher)
	if total <= max {
		return nil
	}
	s.mu.RLock()
	type scored struct {
		path string
		last time.Time
	}
	var candidates []scored
	for path, entry := range s.access {
		if matcher.Match(path) && s.Exists(path) {
			candidates = append(candidates, scored{path, entry.lastAccess})
		}
	}
	s.mu.RUnlock()
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].last.Equal(candidates[j].last) {
			return candidates[i].last.Before(candidates[j].last)
		}
		return candidates[i].path < candidates[j].path
	})
	evictCount := total - max
	if evictCount > len(candidates) {
		evictCount = len(candidates)
	}
	var evicted []string
	for _, candidate := range candidates[:evictCount] {
		evicted = append(evicted, candidate.path)
	}
	if len(evicted) > 0 {
		s.Delete(evicted...)
		log.Printf("isr: lru evicted %d pages: %v", len(evicted), evicted)
	}
	return evicted
}
