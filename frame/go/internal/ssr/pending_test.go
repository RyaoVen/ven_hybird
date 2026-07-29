package ssr

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPendingRegister_EmptyHookID(t *testing.T) {
	r := NewPendingRegistry(4)
	if _, _, err := r.Register(""); err == nil {
		t.Fatal("expected error for empty hook id")
	}
}

func TestPendingRegister_Duplicate(t *testing.T) {
	r := NewPendingRegistry(4)
	if _, _, err := r.Register("hook-1"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if _, _, err := r.Register("hook-1"); err == nil {
		t.Fatal("expected error for duplicate hook id")
	}
}

func TestPendingRegister_Capacity(t *testing.T) {
	r := NewPendingRegistry(2)
	cleanups := make([]func(), 0, 2)
	for i := 0; i < 2; i++ {
		_, cleanup, err := r.Register(fmt.Sprintf("hook-%d", i))
		if err != nil {
			t.Fatalf("register %d failed: %v", i, err)
		}
		cleanups = append(cleanups, cleanup)
	}
	if _, _, err := r.Register("hook-2"); err == nil {
		t.Fatal("expected capacity error")
	}
	// 清理一个后容量释放，可再注册
	cleanups[0]()
	if _, _, err := r.Register("hook-2"); err != nil {
		t.Fatalf("register after cleanup failed: %v", err)
	}
}

func TestPendingResolve_NotFound(t *testing.T) {
	r := NewPendingRegistry(4)
	if r.Resolve(RenderCallback{HookID: "ghost", RequestRoute: "/x"}) {
		t.Fatal("expected false for unknown hook id")
	}
}

func TestPendingResolve_DeliversAndRemoves(t *testing.T) {
	r := NewPendingRegistry(4)
	waiter, _, err := r.Register("hook-1")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	callback := RenderCallback{HookID: "hook-1", RequestRoute: "/news/1", HTML: "<html/>"}
	if !r.Resolve(callback) {
		t.Fatal("expected resolve to match")
	}
	select {
	case got := <-waiter:
		if got.HTML != callback.HTML {
			t.Fatalf("unexpected callback payload: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("callback not delivered")
	}
	// Resolve 即删：二次 Resolve 返回 false，计数归零
	if r.Resolve(callback) {
		t.Fatal("expected false on second resolve")
	}
	if r.Count() != 0 {
		t.Fatalf("expected 0 pending, got %d", r.Count())
	}
}

func TestPendingCleanup_IdempotentAndSafe(t *testing.T) {
	r := NewPendingRegistry(4)
	_, cleanup1, err := r.Register("hook-1")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	cleanup1()
	cleanup1() // 幂等：重复调用不 panic
	if r.Count() != 0 {
		t.Fatalf("expected 0 pending after cleanup, got %d", r.Count())
	}

	// ABA 防护：同名 hook 重新注册后，旧 cleanup 不得误删新条目
	waiter2, _, err := r.Register("hook-1")
	_ = waiter2
	if err != nil {
		t.Fatalf("re-register failed: %v", err)
	}
	cleanup1()
	if r.Count() != 1 {
		t.Fatalf("stale cleanup removed new entry, count = %d", r.Count())
	}
}

func TestPending_Concurrent(t *testing.T) {
	const n = 50
	r := NewPendingRegistry(n)

	var wg sync.WaitGroup
	errs := make(chan error, n)
	registered := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hookID := fmt.Sprintf("hook-%d", i)
			waiter, cleanup, err := r.Register(hookID)
			if err != nil {
				errs <- fmt.Errorf("register %s: %w", hookID, err)
				return
			}
			defer cleanup()
			// 先告知已注册，主 goroutine 再投递，避免 Resolve 早于 Register 的竞态
			registered <- hookID
			select {
			case callback := <-waiter:
				if callback.HookID != hookID {
					errs <- fmt.Errorf("wrong callback for %s: %s", hookID, callback.HookID)
				}
			case <-time.After(2 * time.Second):
				errs <- fmt.Errorf("timeout waiting callback %s", hookID)
			}
		}(i)
	}

	// 按注册就绪顺序并发投递全部回调（Resolve 即删，等待方各自收到自己的那份）
	for i := 0; i < n; i++ {
		hookID := <-registered
		if !r.Resolve(RenderCallback{HookID: hookID, RequestRoute: "/x", HTML: hookID}) {
			errs <- fmt.Errorf("resolve not matched: %s", hookID)
		}
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if r.Count() != 0 {
		t.Fatalf("expected 0 pending after drain, got %d", r.Count())
	}
}
