package redis

import (
	"context"
	"testing"
	"time"

	"ven_hybird/internal/pagecache"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// newMiniredis 启动内存版 Redis 并返回连接好的客户端。
func newMiniredis(t *testing.T) (*miniredis.Miniredis, *goredis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return mr, client
}

func TestNewClient_PingFailure(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close() // 关闭后 ping 必失败
	if _, err := NewClient(addr, "", 0); err == nil {
		t.Fatal("expected ping failure for unreachable redis")
	}
}

func TestSessionBackend_CRUD(t *testing.T) {
	mr, client := newMiniredis(t)
	backend := NewSessionBackend(client)

	if err := backend.Set("token-1", "admin", "u-100", time.Minute); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	role, userID, ok := backend.Get("token-1")
	if !ok || role != "admin" || userID != "u-100" {
		t.Fatalf("expected (admin, u-100), got (%q, %q) ok=%v", role, userID, ok)
	}
	// TTL 过期
	mr.FastForward(2 * time.Minute)
	if _, _, ok := backend.Get("token-1"); ok {
		t.Fatal("expected session expired after ttl")
	}
	// 删除
	if err := backend.Set("token-2", "guest", "", time.Minute); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	backend.Delete("token-2")
	if _, _, ok := backend.Get("token-2"); ok {
		t.Fatal("expected session deleted")
	}
}

// 兼容性：旧格式会话值（纯 role、无分隔符）按 role/userID="" 读，不丢会话。
func TestSessionBackend_LegacyValueCompatible(t *testing.T) {
	_, client := newMiniredis(t)
	backend := NewSessionBackend(client)

	// 模拟升级前写入的旧格式值
	if err := client.Set(context.Background(), sessionKey("legacy"), "admin", time.Minute).Err(); err != nil {
		t.Fatalf("seed legacy value failed: %v", err)
	}
	role, userID, ok := backend.Get("legacy")
	if !ok || role != "admin" || userID != "" {
		t.Fatalf("expected legacy (admin, \"\"), got (%q, %q) ok=%v", role, userID, ok)
	}
}

func TestSessionBackend_FailOpen(t *testing.T) {
	mr, client := newMiniredis(t)
	backend := NewSessionBackend(client)
	mr.Close() // Redis 宕机

	if _, _, ok := backend.Get("token-1"); ok {
		t.Fatal("get should fail-open to false")
	}
	if err := backend.Set("token-1", "admin", "", time.Minute); err == nil {
		t.Fatal("set should report error (login path sees failure)")
	}
	backend.Delete("token-1") // 不 panic 即可
}

func testEntry() *pagecache.Entry {
	return &pagecache.Entry{
		HTML:         "<html>news</html>",
		MatchedRoute: "/news/:id",
		PageName:     "news/id",
		RenderedAt:   time.Now().Truncate(time.Second),
		Duration:     42,
	}
}

func TestPageBackend_CRUD(t *testing.T) {
	mr, client := newMiniredis(t)
	backend := NewPageBackend(client)
	entry := testEntry()
	key := "/news/1||deadbeef"

	if err := backend.Set(key, entry, time.Minute); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	got, ok := backend.Get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.HTML != entry.HTML || got.MatchedRoute != entry.MatchedRoute ||
		got.PageName != entry.PageName || got.Duration != entry.Duration ||
		!got.RenderedAt.Equal(entry.RenderedAt) {
		t.Fatalf("entry roundtrip mismatch: %+v", got)
	}
	// TTL 过期
	mr.FastForward(2 * time.Minute)
	if _, ok := backend.Get(key); ok {
		t.Fatal("expected entry expired after ttl")
	}
	// 删除
	_ = backend.Set(key, entry, time.Minute)
	backend.Delete(key)
	if _, ok := backend.Get(key); ok {
		t.Fatal("expected entry deleted")
	}
}

func TestPageBackend_DeletePrefix(t *testing.T) {
	_, client := newMiniredis(t)
	backend := NewPageBackend(client)
	entry := testEntry()
	ctx := context.Background()

	keys := []string{
		"/news/1||a", "/news/1||b", // 命中前缀
		"/news/2||a", "/other||c", // 不命中
	}
	for _, key := range keys {
		if err := backend.Set(key, entry, time.Minute); err != nil {
			t.Fatalf("set %s failed: %v", key, err)
		}
	}
	// 其他前缀的框架 key 不受影响
	if err := client.Set(ctx, sessionKey("token-1"), "admin", 0).Err(); err != nil {
		t.Fatalf("set session failed: %v", err)
	}

	backend.DeletePrefix("/news/1|")

	for _, key := range []string{"/news/1||a", "/news/1||b"} {
		if _, ok := backend.Get(key); ok {
			t.Fatalf("expected %s deleted", key)
		}
	}
	for _, key := range []string{"/news/2||a", "/other||c"} {
		if _, ok := backend.Get(key); !ok {
			t.Fatalf("expected %s survive", key)
		}
	}
	if role, _ := client.Get(ctx, sessionKey("token-1")).Result(); role != "admin" {
		t.Fatal("expected session key survive")
	}
}

func TestPageBackend_DeletePrefixGlobChars(t *testing.T) {
	_, client := newMiniredis(t)
	backend := NewPageBackend(client)
	entry := testEntry()

	// 路径含 glob 元字符：前缀必须按字面匹配，不得误删相似路径
	weird := "/weird/[x]||a"
	similar := "/weird/x||a" // 若 [x] 未转义，会被 [x] 字符类误命中
	if err := backend.Set(weird, entry, time.Minute); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if err := backend.Set(similar, entry, time.Minute); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	backend.DeletePrefix("/weird/[x]|")
	if _, ok := backend.Get(weird); ok {
		t.Fatal("expected glob-char key deleted")
	}
	if _, ok := backend.Get(similar); !ok {
		t.Fatal("similar key must survive: prefix matched literally, not as glob")
	}
}

func TestPageBackend_FailOpen(t *testing.T) {
	mr, client := newMiniredis(t)
	backend := NewPageBackend(client)
	mr.Close()

	if _, ok := backend.Get("/news/1||a"); ok {
		t.Fatal("get should fail-open to miss")
	}
	if err := backend.Set("/news/1||a", testEntry(), time.Minute); err == nil {
		t.Fatal("set should report error")
	}
	backend.Delete("/news/1||a")     // 不 panic
	backend.DeletePrefix("/news/1|") // 不 panic，记日志后中断
}
