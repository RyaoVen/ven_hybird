package isr

import (
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestParseDeclarationPrefix(t *testing.T) {
	cases := []struct {
		template string
		prefix   string
	}{
		{"/blog/:id", "/blog"},
		{"/:user/blog/:id", "/"},
		{"/about", "/about"},
		{"/", "/"},
	}
	for _, c := range cases {
		decl, err := ParseDeclaration(c.template, 0, false)
		if err != nil {
			t.Fatalf("parse %s failed: %v", c.template, err)
		}
		if decl.Prefix != c.prefix {
			t.Fatalf("parse %s: expected prefix %s, got %s", c.template, c.prefix, decl.Prefix)
		}
	}
}

func TestDeclarationMatch(t *testing.T) {
	decl, _ := ParseDeclaration("/:user/blog/:id", 0, false)
	params, ok := decl.Match("/alice/blog/42")
	if !ok {
		t.Fatal("expected match")
	}
	if params["user"] != "alice" || params["id"] != "42" {
		t.Fatalf("wrong params: %v", params)
	}
	if _, ok := decl.Match("/alice/blog"); ok {
		t.Fatal("expected no match for shorter path")
	}
	if _, ok := decl.Match("/alice/other/42"); ok {
		t.Fatal("expected no match for different static segment")
	}
}

func TestBuildMatcherGranularity(t *testing.T) {
	decl, _ := ParseDeclaration("/:user/blog/:id", 0, false)

	// 全局
	m, _ := decl.BuildMatcher(nil)
	if !m.Match("/alice/blog/1") || !m.Match("/bob/blog/2") {
		t.Fatal("global matcher should match all")
	}
	// 子树
	m, _ = decl.BuildMatcher([]string{"alice"})
	if !m.Match("/alice/blog/1") {
		t.Fatal("subtree matcher should match alice")
	}
	if m.Match("/bob/blog/1") {
		t.Fatal("subtree matcher should not match bob")
	}
	// 单页
	m, _ = decl.BuildMatcher([]string{"alice", "42"})
	if !m.Match("/alice/blog/42") {
		t.Fatal("exact matcher should match the page")
	}
	if m.Match("/alice/blog/43") {
		t.Fatal("exact matcher should not match other pages")
	}
	// 超个数报错
	if _, err := decl.BuildMatcher([]string{"a", "b", "c"}); err == nil {
		t.Fatal("expected error for too many params")
	}
}

func TestStoreMaterializeAndServe(t *testing.T) {
	store := NewStore(t.TempDir(), true)
	if err := store.Materialize("/news/1", "<html>v1</html>"); err != nil {
		t.Fatalf("materialize failed: %v", err)
	}
	if !store.Exists("/news/1") {
		t.Fatal("expected file exists")
	}
	if store.AccessCount("/news/1") != 1 {
		t.Fatalf("expected access count 1, got %d", store.AccessCount("/news/1"))
	}

	// 中间件命中直发
	app := fiber.New()
	app.Use(store.Middleware())
	resp, err := app.Test(httptest.NewRequest("GET", "/news/1", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	// 中间件 miss 放行
	app2 := fiber.New()
	app2.Use(store.Middleware())
	app2.Get("/news/2", func(ctx *fiber.Ctx) error { return ctx.SendString("fallback") })
	resp2, _ := app2.Test(httptest.NewRequest("GET", "/news/2", nil))
	_, _ = io.Copy(io.Discard, resp2.Body)
	_ = resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200 fallback, got %d", resp2.StatusCode)
	}
}

func TestStoreTraversalRejected(t *testing.T) {
	store := NewStore(t.TempDir(), true)
	if err := store.Materialize("/../evil", "x"); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestStoreInvalidateGranularity(t *testing.T) {
	store := NewStore(t.TempDir(), true)
	decl, _ := ParseDeclaration("/:user/blog/:id", 0, false)
	for _, p := range []string{"/alice/blog/1", "/alice/blog/2", "/bob/blog/1"} {
		if err := store.Materialize(p, "<html>"+p+"</html>"); err != nil {
			t.Fatalf("materialize %s failed: %v", p, err)
		}
	}

	// 子树失效
	m, _ := decl.BuildMatcher([]string{"alice"})
	deleted, err := store.Invalidate(m)
	if err != nil {
		t.Fatalf("invalidate failed: %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("expected 2 deleted, got %d", len(deleted))
	}
	if store.Exists("/bob/blog/1") != true {
		t.Fatal("bob should survive")
	}

	// 全局失效
	m, _ = decl.BuildMatcher(nil)
	deleted, _ = store.Invalidate(m)
	if len(deleted) != 1 || store.Exists("/bob/blog/1") {
		t.Fatal("global invalidate failed")
	}
}

func TestStoreHotPathsAndLRU(t *testing.T) {
	store := NewStore(t.TempDir(), true)
	decl, _ := ParseDeclaration("/news/:id", 0, false)
	matcher, _ := decl.BuildMatcher(nil)

	for _, p := range []string{"/news/1", "/news/2", "/news/3"} {
		_ = store.Materialize(p, "<html>"+p+"</html>")
	}
	// /news/3 最热，/news/2 次之
	store.RecordAccess("/news/2")
	store.RecordAccess("/news/3")
	store.RecordAccess("/news/3")

	hot := store.HotPaths(matcher, 2)
	if len(hot) != 2 || hot[0] != "/news/3" || hot[1] != "/news/2" {
		t.Fatalf("unexpected hot paths: %v", hot)
	}

	// LRU：上限 2，/news/1 最久未访问应被淘汰
	time.Sleep(5 * time.Millisecond)
	_ = store.Materialize("/news/4", "<html>4</html>")
	evicted := store.EvictLRU(matcher, 2)
	if len(evicted) == 0 {
		t.Fatal("expected eviction")
	}
	if store.CountFiles(matcher) > 2 {
		t.Fatalf("expected <= 2 files after eviction, got %d", store.CountFiles(matcher))
	}
}

func TestStoreClearAll(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, true)
	if err := s.Materialize("/news/1", "<html>1</html>"); err != nil {
		t.Fatal(err)
	}
	if err := s.Materialize("/news/2", "<html>2</html>"); err != nil {
		t.Fatal(err)
	}
	if cleared := s.ClearAll(); cleared != 2 {
		t.Fatalf("expected 2 cleared, got %d", cleared)
	}
	if s.Exists("/news/1") || s.Exists("/news/2") {
		t.Fatal("expected all materialized files cleared")
	}
	if s.AccessCount("/news/1") != 0 {
		t.Fatal("expected access stats cleared")
	}
}

func TestStoreClearAllDisabled(t *testing.T) {
	dir := t.TempDir()
	enabled := NewStore(dir, true)
	if err := enabled.Materialize("/news/1", "<html>1</html>"); err != nil {
		t.Fatal(err)
	}
	// 禁用（dev）的 Store 不清空
	s := NewStore(dir, false)
	if cleared := s.ClearAll(); cleared != 0 {
		t.Fatalf("disabled store should clear nothing, got %d", cleared)
	}
	if !enabled.Exists("/news/1") {
		t.Fatal("expected files kept when disabled")
	}
}
