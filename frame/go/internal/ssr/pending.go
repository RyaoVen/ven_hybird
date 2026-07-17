// PendingRegistry 管理等待渲染回调的任务。
package ssr

import (
	"fmt"
	"sync"
)

// PendingRegistry 管理等待渲染回调的 pending 任务。
type PendingRegistry struct {
	mu         sync.Mutex                     // 保护 waiters 并发安全
	waiters    map[string]chan RenderCallback // HookID 到回调通道的映射
	maxPending int                            // 最大并发 pending 数
}

// NewPendingRegistry 创建 PendingRegistry 实例。
func NewPendingRegistry(maxPending int) *PendingRegistry {
	return &PendingRegistry{
		waiters:    make(map[string]chan RenderCallback),
		maxPending: maxPending,
	}
}

// Register 注册一个 pending 渲染任务，返回回调通道和清理函数。
func (r *PendingRegistry) Register(hookID string) (<-chan RenderCallback, func(), error) {
	if hookID == "" {
		return nil, nil, fmt.Errorf("hook id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查 HookID 是否已被注册（防止重复提交）
	if _, exists := r.waiters[hookID]; exists {
		return nil, nil, fmt.Errorf("hook id already registered")
	}
	// 检查是否达到系统容量上限
	if len(r.waiters) >= r.maxPending {
		return nil, nil, fmt.Errorf("pending render capacity reached")
	}

	// 创建容量为 1 的带缓冲通道，确保回调方不会因无接收者而阻塞
	waiter := make(chan RenderCallback, 1)
	r.waiters[hookID] = waiter

	// 返回清理函数：安全地从 map 中移除该 HookID
	// 使用指针比较确保只删除自己注册的条目（防止并发场景下的 ABA 问题）
	return waiter, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.waiters[hookID] == waiter {
			delete(r.waiters, hookID)
		}
	}, nil
}

// Resolve 将渲染回调投递到对应的等待通道，返回是否匹配成功。
func (r *PendingRegistry) Resolve(callback RenderCallback) bool {
	r.mu.Lock()
	waiter, exists := r.waiters[callback.HookID]
	if exists {
		// 找到后立即从 map 中移除，防止重复回调
		delete(r.waiters, callback.HookID)
	}
	r.mu.Unlock()

	if !exists {
		return false
	}

	// 将回调结果发送到等待通道（通道容量为 1，不会阻塞）
	waiter <- callback
	return true
}

// Count 返回当前 pending 任务数量。
func (r *PendingRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.waiters)
}
