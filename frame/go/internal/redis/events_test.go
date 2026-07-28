package redis

import (
	"sync"
	"testing"
	"time"

	"ven_hybird/internal/event"
)

// TestEventTransport_Roundtrip 验证发布/订阅回环，且接收时间被覆盖为本地时刻。
func TestEventTransport_Roundtrip(t *testing.T) {
	_, client := newMiniredis(t)
	transport := NewEventTransport(client)

	received := make(chan event.ChangeEvent, 1)
	transport.Subscribe(func(ev event.ChangeEvent) { received <- ev })
	// 订阅建立需要一点时间
	time.Sleep(50 * time.Millisecond)

	old := time.Now().Add(-time.Hour)
	if err := transport.Publish(event.ChangeEvent{Pattern: "/news/:id", Params: []string{"1"}, EnqueuedAt: old}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	select {
	case ev := <-received:
		if ev.Pattern != "/news/:id" || len(ev.Params) != 1 || ev.Params[0] != "1" {
			t.Fatalf("event mismatch: %+v", ev)
		}
		if ev.EnqueuedAt.Before(time.Now().Add(-time.Minute)) {
			t.Fatal("EnqueuedAt should be overwritten with local receive time")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for subscribed event")
	}
}

// TestEventTransport_CrossBus 集成验证：总线 A 的 DataChange 经 Redis 传播到总线 B，
// B 在静默窗口后走自己的失效；回声被 A 的去重吞掉（各处理一次）。
func TestEventTransport_CrossBus(t *testing.T) {
	_, client := newMiniredis(t)

	// newBus 构造接同一 Redis 的总线，返回失效计数（mutex 保护）。
	newBus := func(mu *sync.Mutex, count *int) *event.Bus {
		bus := event.New(
			func(pattern string, params []string) ([]string, []string, error) {
				mu.Lock()
				*count++
				mu.Unlock()
				return nil, nil, nil
			},
			func(template, path string) (string, bool) { return "", false },
			func(path, html string) error { return nil },
		)
		bus.QuietWindow = 50 * time.Millisecond
		bus.SetTransport(NewEventTransport(client))
		t.Cleanup(bus.Stop)
		return bus
	}

	var muA, muB sync.Mutex
	countA, countB := 0, 0
	busA := newBus(&muA, &countA)
	newBus(&muB, &countB) // 总线 B 只需存在并消费

	time.Sleep(50 * time.Millisecond) // 等双方订阅建立
	busA.Enqueue(event.ChangeEvent{Pattern: "/news/:id", Params: []string{"1"}, EnqueuedAt: time.Now()})

	wait := func(get func() int, target int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if get() >= target {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for count %d", target)
	}
	wait(func() int { muA.Lock(); defer muA.Unlock(); return countA }, 1)
	wait(func() int { muB.Lock(); defer muB.Unlock(); return countB }, 1)

	// 观察窗口：回声不得引起第二次处理
	time.Sleep(150 * time.Millisecond)
	muA.Lock()
	defer muA.Unlock()
	muB.Lock()
	defer muB.Unlock()
	if countA != 1 || countB != 1 {
		t.Fatalf("expected A=1 B=1 (echo deduped), got A=%d B=%d", countA, countB)
	}
}
