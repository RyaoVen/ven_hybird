package pagecache

import (
	"errors"
	"sync"
	"time"
)

// flightTimeout 是 follower 等待 leader 回源的最长时限。
// leader 异常（panic 未 complete、代码路径遗漏）时 follower 不再无限挂起，
// 超时后返回 ErrFlightTimeout 由调用方走自身回源或降级。
// 包级变量便于测试覆盖（生产默认 30s）。
var flightTimeout = 30 * time.Second

// ErrFlightTimeout follower 等待 leader 回源超时。
var ErrFlightTimeout = errors.New("pagecache: flight leader timed out")

// flightCall 是一次进行中的回源调用，follower 等待 done 共享结果。
type flightCall struct {
	done  chan struct{}
	entry *Entry
	err   error
}

// flightGroup 防击穿：同 key 并发仅一个 leader 执行，follower 共享结果。
type flightGroup struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

func newFlightGroup() *flightGroup {
	return &flightGroup{calls: make(map[string]*flightCall)}
}

// acquire 尝试成为 key 的 leader。
// 返回 shared=true 表示已有 leader 在跑，当前调用作为 follower 等到了共享结果；
// shared=false 表示当前调用是 leader，应自行执行并在结束后调用 complete。
// follower 等待带超时（flightTimeout）：leader 异常未 complete 时返回 ErrFlightTimeout，
// 由调用方决定自身回源或降级，绝不无限等待。
func (g *flightGroup) acquire(key string) (entry *Entry, err error, shared bool) {
	g.mu.Lock()
	call, ok := g.calls[key]
	if !ok {
		call = &flightCall{done: make(chan struct{})}
		g.calls[key] = call
		g.mu.Unlock()
		return nil, nil, false
	}
	g.mu.Unlock()

	// follower 等待 leader 完成；done 关闭 happens-before 结果读取。
	// 超时后不再等待（leader 可能 panic 未 complete），返回超时错误。
	select {
	case <-call.done:
		return call.entry, call.err, true
	case <-time.After(flightTimeout):
		return nil, ErrFlightTimeout, true
	}
}

// complete 由 leader 调用：写入结果、唤醒 follower、清理 call。
// 幂等：call 已被删除（如重复 complete）时静默返回。
func (g *flightGroup) complete(key string, entry *Entry, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	call, ok := g.calls[key]
	if !ok {
		return
	}
	call.entry = entry
	call.err = err
	close(call.done)
	delete(g.calls, key)
}
