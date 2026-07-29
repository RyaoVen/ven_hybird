package event

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

// callLog 线程安全地记录各阶段调用，用于断言 ①② 顺序与次数。
type callLog struct {
	mu          sync.Mutex
	invalidates []string
	renders     []string
	materialized []string
}

func (c *callLog) addInvalidate(pattern string, params []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalidates = append(c.invalidates, pattern+strings.Join(params, "/"))
}

func (c *callLog) addRender(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.renders = append(c.renders, path)
}

func (c *callLog) addMaterialize(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.materialized = append(c.materialized, path)
}

func (c *callLog) invalidateCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.invalidates)
}

func (c *callLog) materializeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.materialized)
}

// newTestBus 用记录调用的假阶段函数构造总线（① 固定返回删除并再生单页 /news/1）。
func newTestBus(calls *callLog) *Bus {
	b := New(
		func(pattern string, params []string) ([]string, []string, error) {
			calls.addInvalidate(pattern, params)
			return []string{"/news/1"}, []string{"/news/1"}, nil
		},
		func(template, path string) (string, bool) {
			calls.addRender(path)
			return "<html></html>", true
		},
		func(path, html string) error {
			calls.addMaterialize(path)
			return nil
		},
	)
	return b
}

func ev(pattern string, params ...string) ChangeEvent {
	return ChangeEvent{Pattern: pattern, Params: params, EnqueuedAt: time.Now()}
}

// waitFor 轮询条件直至满足或超时失败。
func waitFor(t *testing.T, what string, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestBus_EnqueueDoesNotBlock(t *testing.T) {
	calls := &callLog{}
	b := newTestBus(calls)
	defer b.Stop()
	b.QuietWindow = 300 * time.Millisecond

	start := time.Now()
	b.Enqueue(ev("/news/:id", "1"))
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("enqueue should return immediately, took %s", elapsed)
	}
	// 静默窗口未过：① 不应已执行
	if calls.invalidateCount() != 0 {
		t.Fatal("invalidate should not run before quiet window")
	}
}

func TestBus_QuietWindowFlush(t *testing.T) {
	calls := &callLog{}
	b := newTestBus(calls)
	defer b.Stop()
	b.QuietWindow = 50 * time.Millisecond
	b.MaxWait = 10 * time.Second

	b.Enqueue(ev("/news/:id", "1"))
	waitFor(t, "flush after quiet window", func() bool { return calls.invalidateCount() == 1 }, 2*time.Second)
	waitFor(t, "regen materialized", func() bool { return calls.materializeCount() == 1 }, 2*time.Second)
}

func TestBus_MaxWaitForcedFlush(t *testing.T) {
	calls := &callLog{}
	b := newTestBus(calls)
	defer b.Stop()
	b.QuietWindow = 200 * time.Millisecond // 静默窗口远大于入队间隔：永不满足
	b.MaxWait = 150 * time.Millisecond

	start := time.Now()
	// 持续变更：每 30ms 一次，静默窗口永远等不到
	for calls.invalidateCount() == 0 && time.Since(start) < 2*time.Second {
		b.Enqueue(ev("/news/:id", "1"))
		time.Sleep(30 * time.Millisecond)
	}
	elapsed := time.Since(start)
	if calls.invalidateCount() != 1 {
		t.Fatalf("expected 1 forced flush, got %d", calls.invalidateCount())
	}
	if elapsed < b.MaxWait {
		t.Fatalf("flush happened before max wait: %s", elapsed)
	}
}

func TestBus_Dedup(t *testing.T) {
	calls := &callLog{}
	b := newTestBus(calls)
	defer b.Stop()
	b.QuietWindow = 50 * time.Millisecond

	var logBuf bytes.Buffer
	restore := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(restore)

	// 同页重复 + 范围重叠（全局吞并局部）：一轮只处理一次
	b.Enqueue(ev("/news/:id", "1"))
	b.Enqueue(ev("/news/:id", "1"))
	b.Enqueue(ev("/news/:id"))
	waitFor(t, "flush", func() bool { return calls.invalidateCount() > 0 }, 2*time.Second)

	if got := calls.invalidateCount(); got != 1 {
		t.Fatalf("expected 1 invalidate after dedup, got %d", got)
	}
	calls.mu.Lock()
	got := calls.invalidates[0]
	calls.mu.Unlock()
	if got != "/news/:id" { // 全局（无 params）吞并了局部
		t.Fatalf("expected global scope to win, got %q", got)
	}
	if !strings.Contains(logBuf.String(), "dedup") {
		t.Fatal("expected dedup log to be visible")
	}
}

func TestBus_SubtreeSubsumption(t *testing.T) {
	calls := &callLog{}
	b := newTestBus(calls)
	defer b.Stop()
	b.QuietWindow = 50 * time.Millisecond

	// 子树 /:user/blog/:id params=[u1] 吞并 params=[u1 b1]（前缀即范围包含）
	b.Enqueue(ev("/:user/blog/:id", "u1", "b1"))
	b.Enqueue(ev("/:user/blog/:id", "u1"))
	waitFor(t, "flush", func() bool { return calls.invalidateCount() > 0 }, 2*time.Second)
	if got := calls.invalidateCount(); got != 1 {
		t.Fatalf("expected subtree to subsume single page, got %d invalidates", got)
	}
}

func TestBus_StaleRenderDropped(t *testing.T) {
	var mu sync.Mutex
	var invalidates, materialized []string
	render1Started := make(chan struct{})
	allowRender1 := make(chan struct{})
	renderCount := 0

	b := New(
		func(pattern string, params []string) ([]string, []string, error) {
			mu.Lock()
			invalidates = append(invalidates, pattern)
			mu.Unlock()
			return []string{"/news/1"}, []string{"/news/1"}, nil
		},
		func(template, path string) (string, bool) {
			mu.Lock()
			renderCount++
			first := renderCount == 1
			mu.Unlock()
			if first {
				close(render1Started)
				<-allowRender1 // 第 N 代渲染期间制造窗口，等待 N+1 代 ① 完成
			}
			return "<html>new</html>", true
		},
		func(path, html string) error {
			mu.Lock()
			materialized = append(materialized, path)
			mu.Unlock()
			return nil
		},
	)
	defer b.Stop()
	b.QuietWindow = 30 * time.Millisecond
	b.MaxWait = 10 * time.Second

	b.Enqueue(ev("/news/:id")) // 第 N 代
	<-render1Started           // N 代 ① 已完成，② 渲染中

	b.Enqueue(ev("/news/:id")) // 第 N+1 代（批间流水：N 渲染期间入队）
	// 等 N+1 代 ① 完成（删除 + bump epoch）
	waitFor(t, "N+1 invalidate", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(invalidates) == 2
	}, 2*time.Second)

	close(allowRender1) // N 代渲染完成：落盘检查发现已过时，必须丢弃
	// N+1 代的 ② 正常再生落盘
	waitFor(t, "N+1 materialize", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(materialized) == 1
	}, 2*time.Second)

	// 给老代可能误写留出观察窗口
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(materialized) != 1 {
		t.Fatalf("stale render must not overwrite: materialized %d times", len(materialized))
	}
	if len(invalidates) != 2 {
		t.Fatalf("expected 2 invalidates (N and N+1), got %d", len(invalidates))
	}
}

// fakeTransport 记录广播事件，订阅 handler 由测试手动触发（模拟回声）。
type fakeTransport struct {
	mu        sync.Mutex
	published []ChangeEvent
	handler   func(ChangeEvent)
}

func (f *fakeTransport) Publish(ev ChangeEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, ev)
	return nil
}

func (f *fakeTransport) Subscribe(h func(ChangeEvent)) {
	f.handler = h
}

func (f *fakeTransport) publishCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.published)
}

func TestBus_EnqueuePublishes(t *testing.T) {
	calls := &callLog{}
	b := newTestBus(calls)
	defer b.Stop()
	b.QuietWindow = 50 * time.Millisecond
	transport := &fakeTransport{}
	b.SetTransport(transport)

	b.Enqueue(ev("/news/:id", "1"))
	if got := transport.publishCount(); got != 1 {
		t.Fatalf("expected 1 published event, got %d", got)
	}
	// 本地照常处理
	waitFor(t, "local flush", func() bool { return calls.invalidateCount() == 1 }, 2*time.Second)
}

func TestBus_ReceiveDoesNotRepublish(t *testing.T) {
	calls := &callLog{}
	b := newTestBus(calls)
	defer b.Stop()
	b.QuietWindow = 50 * time.Millisecond
	transport := &fakeTransport{}
	b.SetTransport(transport)

	// 其他实例广播来的事件：只本地入队，不转播（防回声循环）
	transport.handler(ev("/news/:id", "1"))
	waitFor(t, "flush from received event", func() bool { return calls.invalidateCount() == 1 }, 2*time.Second)
	if got := transport.publishCount(); got != 0 {
		t.Fatalf("received event must not be republished, got %d publishes", got)
	}
}

func TestBus_EchoDedup(t *testing.T) {
	calls := &callLog{}
	b := newTestBus(calls)
	defer b.Stop()
	b.QuietWindow = 100 * time.Millisecond
	transport := &fakeTransport{}
	b.SetTransport(transport)

	b.Enqueue(ev("/news/:id", "1"))
	transport.handler(ev("/news/:id", "1")) // 自己发出的事件绕一圈回来
	waitFor(t, "flush", func() bool { return calls.invalidateCount() > 0 }, 2*time.Second)
	time.Sleep(100 * time.Millisecond) // 观察窗口内不得有第二次处理
	if got := calls.invalidateCount(); got != 1 {
		t.Fatalf("echo should be deduped into one processing round, got %d", got)
	}
}

func TestBus_NotifyAfterDelete(t *testing.T) {
	calls := &callLog{}
	b := newTestBus(calls)
	defer b.Stop()
	b.QuietWindow = 50 * time.Millisecond

	var mu sync.Mutex
	var notified []ChangeEvent
	b.SetNotifier(func(events []ChangeEvent) {
		mu.Lock()
		defer mu.Unlock()
		// 回调时 ① 必须已完成（先删后通知）
		if calls.invalidateCount() == 0 {
			t.Error("notifier fired before delete phase")
		}
		notified = append(notified, events...)
	})

	b.Enqueue(ev("/news/:id", "1"))
	waitFor(t, "notify", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(notified) == 1
	}, 2*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if notified[0].Pattern != "/news/:id" || len(notified[0].Params) != 1 {
		t.Fatalf("unexpected notified events: %+v", notified)
	}
}
