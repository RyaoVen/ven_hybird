package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	// 节流：10s 内再次校验未知名 pattern，不应重拉
	if err := s.ValidatePagePattern("/missing"); err == nil {
		t.Fatal("expected error for unknown pattern")
	}
	if fetches != 1 {
		t.Fatalf("expected throttled refetch (1 fetch), got %d", fetches)
	}
}
