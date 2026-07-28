package httpserver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ven_hybird/internal/config"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"
)

// 启动重载：ISR 启用时 New 清空上次运行的物化文件。
func TestServer_StartupReloadISR(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "news")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "1.html"), []byte("<html>stale</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		NodeSubmitTimeout: time.Second,
		RenderTimeout:     2 * time.Second,
		IsrDir:            dir,
		IsrEnabled:        true,
	}
	s := New(cfg, stubClient{}, ssr.NewPendingRegistry(4), stubHookIDs{}, pagepattern.NewValidator(nil))
	if s.StaticFileExists("/news/1") {
		t.Fatal("expected stale materialized file cleared on startup")
	}
}

// ISR 禁用（dev）时不动目录。
func TestServer_StartupReloadISRDisabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>keep</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		NodeSubmitTimeout: time.Second,
		RenderTimeout:     2 * time.Second,
		IsrDir:            dir,
		IsrEnabled:        false,
	}
	s := New(cfg, stubClient{}, ssr.NewPendingRegistry(4), stubHookIDs{}, pagepattern.NewValidator(nil))
	if !s.StaticFileExists("/") {
		t.Fatal("expected file kept when ISR disabled")
	}
}
