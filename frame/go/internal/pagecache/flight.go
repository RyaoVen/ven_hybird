package pagecache

import "sync"

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
	// follower 等待 leader 完成；done 关闭 happens-before 结果读取
	<-call.done
	return call.entry, call.err, true
}

// complete 由 leader 调用：写入结果、唤醒 follower、清理 call。
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
