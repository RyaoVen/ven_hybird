package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ven_hybird/internal/config"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"
)

func TestValidatePagePatternRefetch(t *testing.T) {
	fetches := 0
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pages" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fetches++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"patterns": []string{"/new-page"}})
	}))
	defer worker.Close()

	cfg := config.Config{
		NodeWorkerURL:     worker.URL,
		NodeSubmitTimeout: time.Second,
		RenderTimeout:     2 * time.Second,
		PatternsFile:      filepath.Join(t.TempDir(), "patterns.json"),
	}
	// 初始校验器为空（模拟 Node 新增页面后 Go 列表过期）
	s := New(cfg, stubClient{}, ssr.NewPendingRegistry(4), stubHookIDs{}, pagepattern.NewValidator(nil))

	// 校验失败 → 触发重拉 → 重校验成功
	if err := s.ValidatePagePattern("/new-page"); err != nil {
		t.Fatalf("expected refetch+validate success, got %v", err)
	}
	if fetches != 1 {
		t.Fatalf("expected 1 fetch, got %d", fetches)
	}
	// 重拉成功后持久化最近一次成功拉取的 pattern（下次启动 Node 不可达时回退）
	persisted, err := pagepattern.Load(cfg.PatternsFile)
	if err != nil {
		t.Fatalf("refetched patterns not persisted: %v", err)
	}
	if err := persisted.Validate("/new-page"); err != nil {
		t.Fatalf("persisted patterns missing refetched page: %v", err)
	}

	// 节流：10s 内再次校验未知名 pattern，不应重拉
	if err := s.ValidatePagePattern("/missing"); err == nil {
		t.Fatal("expected error for unknown pattern")
	}
	if fetches != 1 {
		t.Fatalf("expected throttled refetch (1 fetch), got %d", fetches)
	}
}

// TestPatternsActiveRefresh 主动刷新：启动刷新 goroutine 后，Node 路由表变化在间隔内被感知。
// 直接检查内部校验器（不调用 ValidatePagePattern，避免被动重拉路径干扰，验证的是主动刷新路径）。
func TestPatternsActiveRefresh(t *testing.T) {
	fetches := 0
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pages" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fetches++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"patterns": []string{"/active-new"}})
	}))
	defer worker.Close()

	cfg := config.Config{
		NodeWorkerURL:     worker.URL,
		NodeSubmitTimeout: time.Second,
		RenderTimeout:     2 * time.Second,
		PatternsFile:      filepath.Join(t.TempDir(), "patterns.json"),
		PatternRefresh:    100 * time.Millisecond,
	}
	s := New(cfg, stubClient{}, ssr.NewPendingRegistry(4), stubHookIDs{}, pagepattern.NewValidator(nil))
	s.StartPatternRefresher()

	// 主动刷新在间隔内拉取并换入新校验器；期间不产生任何被动校验请求
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.patternMu.RLock()
		err := s.patterns.Validate("/active-new")
		s.patternMu.RUnlock()
		if err == nil {
			return // 主动刷新已感知新页面
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("active refresh did not pick up new pattern within 3s (fetches=%d)", fetches)
}

