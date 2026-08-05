package pagecache

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestKey(t *testing.T) {
	k1, err := Key("/blog/1", map[string]string{"a": "1", "b": "2"}, map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("key failed: %v", err)
	}
	k2, _ := Key("/blog/1", map[string]string{"b": "2", "a": "1"}, map[string]any{"x": 1})
	if k1 != k2 {
		t.Fatal("query order should not affect key")
	}
	k3, _ := Key("/blog/1", map[string]string{"a": "1", "b": "2"}, map[string]any{"x": 2})
	if k1 == k3 {
		t.Fatal("data change should change key")
	}
	k4, _ := Key("/blog/2", map[string]string{"a": "1", "b": "2"}, map[string]any{"x": 1})
	if k1 == k4 {
		t.Fatal("path change should change key")
	}
	if _, err := Key("/x", nil, make(chan int)); err == nil {
		t.Fatal("expected error for unserializable data")
	}
}

func TestMemoryBackend(t *testing.T) {
	entry := &Entry{HTML: "h1"}

	b := NewMemoryBackend(4)
	_ = b.Set("k1", entry, time.Hour)
	if got, ok := b.Get("k1"); !ok || got.HTML != "h1" {
		t.Fatal("get failed")
	}
	b.Delete("k1")
	if _, ok := b.Get("k1"); ok {
		t.Fatal("delete failed")
	}

	// DeletePrefix
	_ = b.Set("/a|x|1", entry, time.Hour)
	_ = b.Set("/a|y|2", entry, time.Hour)
	_ = b.Set("/b|x|1", entry, time.Hour)
	b.DeletePrefix("/a|")
	if _, ok := b.Get("/a|x|1"); ok {
		t.Fatal("prefix delete failed")
	}
	if _, ok := b.Get("/a|y|2"); ok {
		t.Fatal("prefix delete failed")
	}
	if _, ok := b.Get("/b|x|1"); !ok {
		t.Fatal("prefix delete overmatched")
	}

	// 惰性过期
	_ = b.Set("exp", entry, 20*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	if _, ok := b.Get("exp"); ok {
		t.Fatal("expected expired entry")
	}

	// 容量淘汰
	b2 := NewMemoryBackend(1)
	_ = b2.Set("a", entry, time.Hour)
	_ = b2.Set("b", entry, time.Hour)
	if _, ok := b2.Get("a"); ok {
		t.Fatal("expected eviction at capacity")
	}
	if _, ok := b2.Get("b"); !ok {
		t.Fatal("expected newest entry present")
	}
}

func TestStoreDoSingleFlight(t *testing.T) {
	store := NewStore(NewMemoryBackend(10), time.Hour, time.Hour)
	var mu sync.Mutex
	calls := 0
	fn := func() (*Entry, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		return &Entry{HTML: "hello"}, nil
	}

	const n = 5
	var wg sync.WaitGroup
	results := make([]*Entry, n)
	shareds := make([]bool, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], shareds[i], errs[i] = store.Do("k", fn)
		}(i)
	}
	wg.Wait()

	if calls != 1 {
		t.Fatalf("expected 1 fn call, got %d", calls)
	}
	for i := 0; i < n; i++ {
		if errs[i] != nil || results[i] == nil || results[i].HTML != "hello" {
			t.Fatalf("caller %d got wrong result: entry=%+v err=%v", i, results[i], errs[i])
		}
	}
	// 成功后已回填缓存
	if got, ok := store.Get("k"); !ok || got.HTML != "hello" {
		t.Fatal("expected cached entry after Do")
	}
	// 计数：回源 1 次，无错误
	hits, misses, _ := store.Stats()
	if misses != 1 {
		t.Fatalf("expected misses=1, got %d", misses)
	}
	if hits == 0 {
		t.Fatal("expected hits>0 after cached Get")
	}
}

func TestStoreDoErrorNotCached(t *testing.T) {
	store := NewStore(NewMemoryBackend(10), time.Hour, time.Hour)
	boom := errors.New("boom")
	if _, _, err := store.Do("k", func() (*Entry, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	if _, ok := store.Get("k"); ok {
		t.Fatal("error result must not be cached")
	}
	// 失败后后续 Do 可以重试
	calls := 0
	entry, _, err := store.Do("k", func() (*Entry, error) {
		calls++
		return &Entry{HTML: "ok"}, nil
	})
	if err != nil || calls != 1 || entry.HTML != "ok" {
		t.Fatalf("expected retry success, got entry=%+v err=%v calls=%d", entry, err, calls)
	}
	// 计数：两次回源（失败那次也算回源）
	if _, misses, _ := store.Stats(); misses != 2 {
		t.Fatalf("expected misses=2, got %d", misses)
	}
}

// stale-while-revalidate：ttl 过后 Get 不命中但条目仍保留（GetStale 可取回），
// Do 对过期条目仍会回源刷新；物理保留期（ttl+stale）过后条目被丢弃。
func TestStoreStaleWindow(t *testing.T) {
	store := NewStore(NewMemoryBackend(10), 30*time.Millisecond, 100*time.Millisecond)
	_, _, err := store.Do("k", func() (*Entry, error) {
		return &Entry{HTML: "v1", RenderedAt: time.Now()}, nil
	})
	if err != nil {
		t.Fatalf("first render failed: %v", err)
	}
	// 新鲜期内：Get 命中
	if got, ok := store.Get("k"); !ok || got.HTML != "v1" {
		t.Fatal("expected fresh hit")
	}

	// 过期但保留期内：Get 不命中，GetStale 可取回过期条目
	time.Sleep(50 * time.Millisecond)
	if _, ok := store.Get("k"); ok {
		t.Fatal("stale entry must not count as fresh hit")
	}
	stale, ok := store.GetStale("k")
	if !ok || stale.HTML != "v1" {
		t.Fatalf("expected stale entry retained, got ok=%v entry=%+v", ok, stale)
	}

	// Do 对过期条目仍回源刷新（stale 不阻断刷新）
	refreshed := false
	entry, shared, err := store.Do("k", func() (*Entry, error) {
		refreshed = true
		return &Entry{HTML: "v2", RenderedAt: time.Now()}, nil
	})
	if err != nil || !refreshed || shared || entry.HTML != "v2" {
		t.Fatalf("expected refresh on stale, got entry=%+v shared=%v err=%v", entry, shared, err)
	}

	// 物理保留期（30+100ms）过后：GetStale 返回 false
	time.Sleep(140 * time.Millisecond)
	if _, ok := store.GetStale("k"); ok {
		t.Fatal("entry should be dropped after physical retention")
	}
}

// stale 窗口为 0 时（未配置/关闭）：行为与无 stale 支持一致——过期条目直接不可见。
func TestStoreStaleWindowZeroDisabled(t *testing.T) {
	store := NewStore(NewMemoryBackend(10), 20*time.Millisecond, 0)
	_, _, err := store.Do("k", func() (*Entry, error) {
		return &Entry{HTML: "v1", RenderedAt: time.Now()}, nil
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, ok := store.Get("k"); ok {
		t.Fatal("expected miss after ttl")
	}
	if _, ok := store.GetStale("k"); ok {
		t.Fatal("stale disabled: entry should be gone after ttl")
	}
}

// TestFlightFollowerTimeout leader 不 complete 时 follower 超时返回，不永久挂起。
// 观测窗口取 flightTimeout 的合理比例（1s）：follower 应在此窗口内返回超时错误而非无限等待。
func TestFlightFollowerTimeout(t *testing.T) {
	g := newFlightGroup()

	// leader 成为 leader 后永不 complete（模拟 panic/代码路径遗漏）
	_, _, shared := g.acquire("k")
	if shared {
		t.Fatal("expected first acquire to be leader")
	}

	// 临时把 flightTimeout 改小（生产默认 30s），避免测试等满超时时长
	orig := flightTimeout
	flightTimeout = 100 * time.Millisecond
	defer func() { flightTimeout = orig }()

	// 用独立 goroutine 等待 acquire，主 goroutine 在观测窗口内判断是否返回
	type res struct {
		err error
	}
	ch := make(chan res, 1)
	go func() {
		_, err, shared := g.acquire("k")
		if !shared {
			ch <- res{err: errors.New("expected follower")}
			return
		}
		ch <- res{err: err}
	}()

	select {
	case r := <-ch:
		if !errors.Is(r.err, ErrFlightTimeout) {
			t.Fatalf("expected ErrFlightTimeout, got %v", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follower hung forever after leader never completed")
	}
}

// TestFlightLeaderPanicDoesNotHangFollower leader fn panic 时 Do 仍 complete（defer 兜底），
// follower 拿到结果而非永久挂起；panic 向上传播由调用方处理。
func TestFlightLeaderPanicDoesNotHangFollower(t *testing.T) {
	store := NewStore(NewMemoryBackend(10), time.Hour, time.Hour)

	type result struct {
		entry  *Entry
		shared bool
		err    error
	}
	leaderDone := make(chan struct{})
	panicCh := make(chan any, 1)
	var wg sync.WaitGroup
	var followerRes result

	// leader：fn panic
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { panicCh <- recover() }()
		_, _, _ = store.Do("k", func() (*Entry, error) {
			close(leaderDone)
			panic("boom")
		})
	}()

	// follower：等 leader 开始后并发 Do（等待 leader 完成，不应被 panic 挂起）
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-leaderDone
		followerRes.entry, followerRes.shared, followerRes.err = store.Do("k", func() (*Entry, error) {
			return &Entry{HTML: "fallback"}, nil
		})
	}()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("follower hung after leader panic (defer complete not executed)")
	}

	if p := <-panicCh; p == nil {
		t.Fatal("expected leader panic to propagate")
	}
	// follower 超时后降级为自身回源拿到 fallback（或共享到错误结果，两者都不挂起）
	if followerRes.entry == nil && followerRes.err == nil {
		t.Fatal("follower got neither result nor error")
	}
}
