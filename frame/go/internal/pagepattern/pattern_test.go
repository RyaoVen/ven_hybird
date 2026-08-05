package pagepattern

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/pages" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("X-Ven-Internal-Token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"patterns": []string{"/", "/blog/:slug"},
		})
	}))
	defer server.Close()

	validator, err := Fetch(context.Background(), server.URL, "test-token", 5*time.Second)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if err := validator.Validate("/blog/:slug"); err != nil {
		t.Fatalf("expected valid pattern, got error: %v", err)
	}
	if err := validator.Validate("/missing"); err == nil {
		t.Fatal("expected error for unknown pattern, got nil")
	}
}

func TestFetch_TokenRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	if _, err := Fetch(context.Background(), server.URL, "wrong-token", 5*time.Second); err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

// Save/Load 往返：持久化后可加载出等价的校验器（顺序无关）。
func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patterns.json")
	v := NewValidator([]string{"/", "/blog/:slug", "/posts/:id"})
	if err := Save(v, path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	for _, pattern := range []string{"/", "/blog/:slug", "/posts/:id"} {
		if err := loaded.Validate(pattern); err != nil {
			t.Errorf("expected %q valid after round-trip, got %v", pattern, err)
		}
	}
	if err := loaded.Validate("/nope"); err == nil {
		t.Error("expected error for unknown pattern, got nil")
	}
}

// Patterns 返回排序后的列表（无重复）。
func TestPatterns(t *testing.T) {
	v := NewValidator([]string{"/b", "/a", "/b"})
	got := v.Patterns()
	want := []string{"/a", "/b"}
	if len(got) != len(want) {
		t.Fatalf("Patterns = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Patterns = %v, want %v", got, want)
		}
	}
}

// Load 缺失文件返回错误（首启无持久化副本时应失败退出）。
func TestLoadMissing(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// Load 损坏文件返回错误（不静默接受半截数据）。
func TestLoadCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for corrupt file, got nil")
	}
}
