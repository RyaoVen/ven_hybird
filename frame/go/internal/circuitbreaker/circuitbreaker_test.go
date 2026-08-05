package circuitbreaker

import (
	"testing"
	"time"
)

// 连续失败达到阈值 → open，open 期间 Allow 拒绝。
func TestClosedToOpen(t *testing.T) {
	b := New(3, 10*time.Second)
	for i := 0; i < 3; i++ {
		if !b.Allow() {
			t.Fatalf("allow #%d should be true in closed", i+1)
		}
		b.RecordFailure()
	}
	if b.State() != StateOpen {
		t.Fatalf("expected open after threshold, got %v", b.State())
	}
	if b.Allow() {
		t.Fatal("allow should be false while open")
	}
	if b.Allow() {
		t.Fatal("allow should stay false before half-open interval")
	}
}

// closed 下成功清零失败计数，不触发熔断。
func TestSuccessResetsFailures(t *testing.T) {
	b := New(3, time.Second)
	b.Allow()
	b.RecordFailure()
	b.Allow()
	b.RecordFailure()
	b.Allow()
	b.RecordSuccess() // 清零
	b.Allow()
	b.RecordFailure()
	b.Allow()
	b.RecordFailure()
	if b.State() != StateClosed {
		t.Fatalf("expected closed after reset, got %v", b.State())
	}
	if b.Failures() != 2 {
		t.Fatalf("expected failures=2, got %d", b.Failures())
	}
}

// 半开间隔后放行试探请求，成功即恢复 closed。
func TestHalfOpenProbeSuccessCloses(t *testing.T) {
	b := New(1, 30*time.Millisecond)
	b.Allow()
	b.RecordFailure() // open
	if b.Allow() {
		t.Fatal("allow should be false right after open")
	}

	time.Sleep(40 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("half-open probe should be allowed after interval")
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("expected half-open, got %v", b.State())
	}
	b.RecordSuccess()
	if b.State() != StateClosed {
		t.Fatalf("expected closed after probe success, got %v", b.State())
	}
	if !b.Allow() {
		t.Fatal("closed should allow again")
	}
}

// 半开期间只放行一个试探请求，其余快速失败。
func TestHalfOpenSingleProbe(t *testing.T) {
	b := New(1, 30*time.Millisecond)
	b.Allow()
	b.RecordFailure()
	time.Sleep(40 * time.Millisecond)

	if !b.Allow() {
		t.Fatal("first probe should be allowed")
	}
	if b.Allow() {
		t.Fatal("concurrent request must be rejected while probe in flight")
	}
	b.RecordSuccess()
	if b.State() != StateClosed {
		t.Fatalf("expected closed, got %v", b.State())
	}
}

// 试探失败回到 open，且探测间隔重新计时。
func TestHalfOpenProbeFailureReopens(t *testing.T) {
	b := New(1, 30*time.Millisecond)
	b.Allow()
	b.RecordFailure()
	time.Sleep(40 * time.Millisecond)

	if !b.Allow() {
		t.Fatal("probe should be allowed")
	}
	b.RecordFailure() // 试探失败
	if b.State() != StateOpen {
		t.Fatalf("expected open after probe failure, got %v", b.State())
	}
	if b.Allow() {
		t.Fatal("should stay open, interval restarted")
	}
}

// 参数归一：threshold=0 → 1，interval=0 → 默认 10s。
func TestNewNormalizesParams(t *testing.T) {
	b := New(0, 0)
	b.Allow()
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Fatalf("threshold 0 should normalize to 1, got %v", b.State())
	}
}
