// SSE 连接的流式写端（fiber SetBodyStreamWriter 回调内运行）。
package sse

import (
	"bufio"
	"log"
	"net"
	"time"
)

const (
	// heartbeatInterval 是心跳间隔（注释帧，EventSource 忽略；兼作死连接探测）。
	heartbeatInterval = 15 * time.Second
	// writeDeadline 是单次写操作的超时（滚动刷新——fasthttp 的 WriteTimeout
	// 是全流总时长，长连接必须在流内自行 SetWriteDeadline）。
	writeDeadline = 30 * time.Second
)

// Stream 运行连接的写循环：开场先写一帧注释（让响应头立即下达到客户端），
// 之后消息帧即时写出，心跳保活，断线/注销/关停退出。
// raw 用于滚动刷新写 deadline；每次写前重置，慢客户端写阻塞至超时也由它掐断。
// panic 兜底：写循环运行在 fasthttp 连接协程中，recover 记日志后正常退出（连接关闭），
// 不依赖 fasthttp 的连接级兜底。
func (c *Conn) Stream(w *bufio.Writer, raw net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("sse: stream %s recovered from panic: %v", c.Path, r)
		}
	}()
	if raw != nil {
		_ = raw.SetWriteDeadline(time.Now().Add(writeDeadline))
	}
	// 开场帧：fasthttp 在流首次写出前不下发响应头，先写注释帧让连接立刻建立
	if _, err := w.Write(helloFrame); err != nil {
		return
	}
	if err := w.Flush(); err != nil {
		return
	}
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		var frame []byte
		select {
		case <-c.Closed():
			return
		case f := <-c.Messages():
			frame = f
		case <-ticker.C:
			frame = heartbeatFrame
		}
		if raw != nil {
			_ = raw.SetWriteDeadline(time.Now().Add(writeDeadline))
		}
		if _, err := w.Write(frame); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

// helloFrame 是开场注释帧（让响应头立即下发，EventSource 忽略）。
var helloFrame = []byte(": ok\n\n")

// heartbeatFrame 是 SSE 注释帧（以冒号开头，EventSource 不派发事件）。
var heartbeatFrame = []byte(": hb\n\n")
