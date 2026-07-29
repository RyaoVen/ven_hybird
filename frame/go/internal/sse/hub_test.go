package sse

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ven_hybird/internal/event"
	"ven_hybird/internal/ssr"
)

// countDataFn 返回带计数的数据函数（数据里带回 params/query 便于断言）。
func countDataFn(count *atomic.Int64) DataFunc {
	return func(pattern string, params, query map[string]string) (any, bool) {
		count.Add(1)
		return map[string]any{"pattern": pattern, "params": params, "query": query}, true
	}
}

// recvFrame 在期限内从连接读一帧。
func recvFrame(t *testing.T, conn *Conn, timeout time.Duration) []byte {
	t.Helper()
	select {
	case frame := <-conn.Messages():
		return frame
	case <-time.After(timeout):
		t.Fatal("timed out waiting for frame")
		return nil
	}
}

// expectNoFrame 断言窗口期内无帧。
func expectNoFrame(t *testing.T, conn *Conn, window time.Duration) {
	t.Helper()
	select {
	case frame := <-conn.Messages():
		t.Fatalf("expected no frame, got %q", frame)
	case <-time.After(window):
	}
}

func parsePayload(t *testing.T, frame []byte) ssr.PageBootstrap {
	t.Helper()
	text := string(frame)
	if !strings.HasPrefix(text, "event: page-data\ndata: ") || !strings.HasSuffix(text, "\n\n") {
		t.Fatalf("bad frame shape: %q", text)
	}
	data := strings.TrimSuffix(strings.TrimPrefix(text, "event: page-data\ndata: "), "\n\n")
	var payload ssr.PageBootstrap
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	return payload
}

func TestHub_NotifyEventsScope(t *testing.T) {
	var count atomic.Int64
	hub := New(countDataFn(&count))
	defer hub.Close()

	news1 := hub.Subscribe("/news/:id", "/news/1", map[string]string{"id": "1"}, map[string]string{})
	news2 := hub.Subscribe("/news/:id", "/news/2", map[string]string{"id": "2"}, map[string]string{})
	other := hub.Subscribe("/about", "/about", map[string]string{}, map[string]string{})

	// 局部变更：只有 /news/1 受影响
	hub.NotifyEvents([]event.ChangeEvent{{Pattern: "/news/:id", Params: []string{"1"}, EnqueuedAt: time.Now()}})

	payload := parsePayload(t, recvFrame(t, news1, time.Second))
	if payload.Route != "/news/1" || payload.Params["id"] != "1" {
		t.Fatalf("payload mismatch: %+v", payload)
	}
	expectNoFrame(t, news2, 100*time.Millisecond)
	expectNoFrame(t, other, 100*time.Millisecond)
	if got := count.Load(); got != 1 {
		t.Fatalf("expected 1 recompute, got %d", got)
	}
}

func TestHub_NotifyEventsDedup(t *testing.T) {
	var count atomic.Int64
	hub := New(countDataFn(&count))
	defer hub.Close()

	// 同 path 同 query 的两条连接：一轮只重算一次，都收到推送
	connA := hub.Subscribe("/news/:id", "/news/1", map[string]string{"id": "1"}, map[string]string{})
	connB := hub.Subscribe("/news/:id", "/news/1", map[string]string{"id": "1"}, map[string]string{})

	hub.NotifyEvents([]event.ChangeEvent{{Pattern: "/news/:id", EnqueuedAt: time.Now()}})

	recvFrame(t, connA, time.Second)
	recvFrame(t, connB, time.Second)
	if got := count.Load(); got != 1 {
		t.Fatalf("expected single recompute for identical page, got %d", got)
	}
}

func TestHub_NotifyEventsQueryDistinct(t *testing.T) {
	var count atomic.Int64
	hub := New(countDataFn(&count))
	defer hub.Close()

	// 同 path 不同 query：数据可能不同，分别重算
	connA := hub.Subscribe("/news/:id", "/news/1", map[string]string{"id": "1"}, map[string]string{"tab": "a"})
	connB := hub.Subscribe("/news/:id", "/news/1", map[string]string{"id": "1"}, map[string]string{"tab": "b"})

	hub.NotifyEvents([]event.ChangeEvent{{Pattern: "/news/:id", EnqueuedAt: time.Now()}})

	payloadA := parsePayload(t, recvFrame(t, connA, time.Second))
	payloadB := parsePayload(t, recvFrame(t, connB, time.Second))
	if payloadA.Query["tab"] != "a" || payloadB.Query["tab"] != "b" {
		t.Fatalf("query-aware push mismatch: %+v %+v", payloadA, payloadB)
	}
	if got := count.Load(); got != 2 {
		t.Fatalf("expected 2 recomputes for distinct queries, got %d", got)
	}
}

func TestHub_NotifyPath(t *testing.T) {
	var count atomic.Int64
	hub := New(countDataFn(&count))
	defer hub.Close()

	blog := hub.Subscribe("/blog/:id", "/blog/7", map[string]string{"id": "7"}, map[string]string{})
	other := hub.Subscribe("/blog/:id", "/blog/8", map[string]string{"id": "8"}, map[string]string{})

	// InvalidatePage 联动：精确路径推送（动态页）
	hub.NotifyPath("/blog/7")

	recvFrame(t, blog, time.Second)
	expectNoFrame(t, other, 100*time.Millisecond)
}

func TestHub_BackpressureDrops(t *testing.T) {
	var count atomic.Int64
	hub := New(countDataFn(&count))
	defer hub.Close()

	conn := hub.Subscribe("/news/:id", "/news/1", map[string]string{"id": "1"}, map[string]string{})
	// 不消费：连续推送超过缓冲也不应阻塞
	done := make(chan struct{})
	go func() {
		for i := 0; i < sendBuffer+4; i++ {
			hub.NotifyPath("/news/1")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("push blocked on slow client (should drop instead)")
	}
	_ = conn
}

func TestHub_DataFuncSkip(t *testing.T) {
	hub := New(func(pattern string, params, query map[string]string) (any, bool) {
		return nil, false // handler 失败/NotFound：不推送
	})
	defer hub.Close()
	conn := hub.Subscribe("/news/:id", "/news/1", map[string]string{"id": "1"}, map[string]string{})
	hub.NotifyPath("/news/1")
	expectNoFrame(t, conn, 100*time.Millisecond)
}

func TestHub_Close(t *testing.T) {
	var count atomic.Int64
	hub := New(countDataFn(&count))
	conn := hub.Subscribe("/news/:id", "/news/1", map[string]string{"id": "1"}, map[string]string{})
	hub.Close()
	select {
	case <-conn.Closed():
	case <-time.After(time.Second):
		t.Fatal("expected conn closed on hub close")
	}
	if hub.ConnCount() != 0 {
		t.Fatal("expected empty conn table after close")
	}
	// 关停后订阅返回预关闭连接
	late := hub.Subscribe("/news/:id", "/news/2", map[string]string{"id": "2"}, map[string]string{})
	select {
	case <-late.Closed():
	case <-time.After(time.Second):
		t.Fatal("expected late conn pre-closed")
	}
}
