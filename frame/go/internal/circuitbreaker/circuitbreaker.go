// Package circuitbreaker 实现 Node 工作节点的熔断状态机：
// 连续失败达到阈值后进入 open，调用方快速失败（不再等待渲染超时）；
// 半开间隔后放行一个试探请求（probe），成功即恢复 closed，失败则回到 open。
package circuitbreaker

import (
	"sync"
	"time"
)

// State 是熔断器的状态。
type State int

const (
	// StateClosed 关闭：正常放行，累计失败次数。
	StateClosed State = iota
	// StateOpen 开启：快速失败，直到半开间隔过后放行试探请求。
	StateOpen
	// StateHalfOpen 半开：已有试探请求在途，其他请求仍快速失败。
	StateHalfOpen
)

// defaultHalfOpenInterval 是半开探测间隔的兜底默认值（配置缺失时）。
const defaultHalfOpenInterval = 10 * time.Second

// Breaker 是 Node 熔断状态机。并发安全：所有方法内部加锁。
//
// 语义：closed 下每次失败累计计数，达到 threshold 转 open；
// open 下 Allow 一律拒绝，直到距进入 open 超过 halfOpenInterval 后
// 自动转 half-open 并放行恰好一个试探请求；试探成功转 closed（计数清零），
// 试探失败回到 open（间隔重新计时）。
type Breaker struct {
	mu        sync.Mutex
	threshold int           // 连续失败阈值
	halfOpen  time.Duration // 半开探测间隔

	state     State
	failures  int
	openSince time.Time // 进入 open 的时间（半开探测最早时间 = openSince + halfOpen）
	probing   bool      // half-open 期间是否已有试探请求在途
}

// New 创建熔断器。threshold < 1 归一为 1；halfOpenInterval <= 0 时用默认 10s。
func New(threshold int, halfOpenInterval time.Duration) *Breaker {
	if threshold < 1 {
		threshold = 1
	}
	if halfOpenInterval <= 0 {
		halfOpenInterval = defaultHalfOpenInterval
	}
	return &Breaker{threshold: threshold, halfOpen: halfOpenInterval, state: StateClosed}
}

// Allow 询问本次调用是否放行。
// open 且未到半开时间 → false；到达半开时间 → 自动转 half-open 并放行
// 当前调用作为试探请求；half-open 且有试探在途 → false。
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(b.openSince) < b.halfOpen {
			return false
		}
		// 半开间隔已过：转 half-open，当前调用就是试探请求
		b.state = StateHalfOpen
		b.probing = true
		return true
	case StateHalfOpen:
		if b.probing {
			return false
		}
		b.probing = true
		return true
	}
	return false
}

// RecordSuccess 报告一次成功（Node 有响应即算，含渲染回调错误分支）：
// 试探成功恢复 closed 并清零失败计数。
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = StateClosed
	b.failures = 0
	b.probing = false
}

// RecordFailure 报告一次失败（提交失败/提交超时/渲染超时）：
// closed 下累计，达到阈值转 open；half-open 下试探失败回到 open（间隔重新计时）。
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case StateClosed:
		b.failures++
		if b.failures >= b.threshold {
			b.state = StateOpen
			b.openSince = time.Now()
		}
	case StateHalfOpen:
		b.state = StateOpen
		b.probing = false
		b.openSince = time.Now()
	case StateOpen:
		// open 期间的额外失败不改变状态（探测窗口不变）
	}
}

// State 返回当前状态（诊断/测试）。
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Failures 返回当前连续失败计数（诊断/测试）。
func (b *Breaker) Failures() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failures
}
