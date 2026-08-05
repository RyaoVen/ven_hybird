// Package sse 提供 SSE 实时推送：连接表 + 变更事件驱动的 initialState 推送。
//
// 定位：ISR/DataChange 管"后来的访客看到新内容"，本包管"正在浏览的用户页面自己更新"。
// 推送与 bootstrap 同形的 PageBootstrap JSON（SSE 事件名 page-data），
// 前端复用 SPA router 的状态通道重渲染，页面组件零改动。
//
// 背压语义：慢客户端宁可丢更新也不阻塞推送（每连接独立缓冲，满即丢）；
// 心跳保活 + 滚动写 deadline（fasthttp WriteTimeout 是全流总时长，见 Conn.Stream）。
package sse

import (
	"encoding/json"
	"log"
	"net/url"
	"strings"
	"sync"

	"ven_hybird/internal/event"
	"ven_hybird/internal/isr"
	"ven_hybird/internal/ssr"
)

// sendBuffer 是单连接的消息缓冲（满即丢，见 Conn.Send）。
const sendBuffer = 8

// defaultMaxConns 是 SSE 连接表容量上限（config.Load 默认值与此一致；
// 字面量构造 Hub 未设 MaxConns 时保留此值）。
const defaultMaxConns = 1000

// pushEventName 是 SSE 事件名（前端 addEventListener 同名订阅）。
const pushEventName = "page-data"

// DataFunc 重新取数：以无请求静态源执行页面数据函数。
// ok=false 表示跳过（handler 失败或 NotFound）。
type DataFunc func(pattern string, params, query map[string]string) (data any, ok bool)

// Conn 是一条 SSE 连接：正在浏览某页面的一个客户端。
type Conn struct {
	Pattern string            // 匹配的页面 pattern，如 /news/:id
	Path    string            // 具体路径，如 /news/1
	Params  map[string]string // 路径参数
	Query   map[string]string // 查询参数（数据可能随 query 变化）

	send   chan []byte
	closed chan struct{}
	once   sync.Once
}

// Send 非阻塞投递一帧；缓冲满返回 false（调用方记日志丢弃）。
func (c *Conn) Send(frame []byte) bool {
	select {
	case c.send <- frame:
		return true
	default:
		return false
	}
}

// Messages 是流式写端消费的消息通道。
func (c *Conn) Messages() <-chan []byte { return c.send }

// Closed 在连接注销或 Hub 关停时关闭（流式写端退出信号）。
func (c *Conn) Closed() <-chan struct{} { return c.closed }

func (c *Conn) close() { c.once.Do(func() { close(c.closed) }) }

// Hub 是 SSE 连接表 + 推送调度。
type Hub struct {
	mu     sync.Mutex
	conns  map[*Conn]struct{}
	closed bool

	// MaxConns 是连接数上限：超出时新订阅被拒绝（预关闭连接，写端立即退出），
	// 不影响已有连接；防 conns map 无界增长。<= 0 = 不设上限。
	MaxConns int

	dataFn DataFunc
	decls  map[string]*isr.Declaration // pattern → 声明缓存（事件范围匹配用）
}

// New 创建推送 Hub。dataFn 由 hybrid 接线（复用页面数据函数）。
func New(dataFn DataFunc) *Hub {
	return &Hub{
		conns:    make(map[*Conn]struct{}),
		MaxConns: defaultMaxConns,
		dataFn:   dataFn,
		decls:    make(map[string]*isr.Declaration),
	}
}

// Subscribe 注册一条连接（Hub 已关停或连接数达上限时返回预关闭连接，写端立即退出）。
func (h *Hub) Subscribe(pattern, path string, params, query map[string]string) *Conn {
	conn := &Conn{
		Pattern: pattern,
		Path:    path,
		Params:  params,
		Query:   query,
		send:    make(chan []byte, sendBuffer),
		closed:  make(chan struct{}),
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		conn.close()
		return conn
	}
	// 连接数上限：拒绝新订阅（预关闭），已有连接不受影响——防 conns map 无界增长
	if h.MaxConns > 0 && len(h.conns) >= h.MaxConns {
		h.mu.Unlock()
		log.Printf("sse: conn limit reached (%d), rejected subscribe %s", h.MaxConns, path)
		conn.close()
		return conn
	}
	h.conns[conn] = struct{}{}
	h.mu.Unlock()
	return conn
}

// Unsubscribe 注销连接（写端随之退出）。
func (h *Hub) Unsubscribe(conn *Conn) {
	h.mu.Lock()
	delete(h.conns, conn)
	h.mu.Unlock()
	conn.close()
}

// Close 关闭全部连接（优雅关停 drain；EventSource 客户端自动重连到存活实例）。
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	conns := make([]*Conn, 0, len(h.conns))
	for conn := range h.conns {
		conns = append(conns, conn)
	}
	h.conns = make(map[*Conn]struct{})
	h.mu.Unlock()
	for _, conn := range conns {
		conn.close()
	}
}

// ConnCount 返回当前连接数（测试与诊断用）。
func (h *Hub) ConnCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.conns)
}

// NotifyEvents 是事件总线 flush 完成 ① 后的回调（静态页范围推送）：
// 受影响路径 = 变更匹配器范围内的已连接页面。
func (h *Hub) NotifyEvents(events []event.ChangeEvent) {
	for _, ev := range events {
		matcher, err := h.matcherFor(ev.Pattern, ev.Params)
		if err != nil {
			log.Printf("sse: build matcher for %s failed: %v", ev.Pattern, err)
			continue
		}
		h.pushIf(func(c *Conn) bool { return matcher.Match(c.Path) })
	}
}

// NotifyPath 是 InvalidatePage 的联动（动态页精确路径推送）。
func (h *Hub) NotifyPath(path string) {
	h.pushIf(func(c *Conn) bool { return c.Path == path })
}

// matcherFor 构造事件匹配器（声明按 pattern 缓存复用）。
func (h *Hub) matcherFor(pattern string, params []string) (*isr.Matcher, error) {
	h.mu.Lock()
	decl, ok := h.decls[pattern]
	h.mu.Unlock()
	if !ok {
		var err error
		decl, err = isr.ParseDeclaration(pattern, 0, false)
		if err != nil {
			return nil, err
		}
		h.mu.Lock()
		h.decls[pattern] = decl
		h.mu.Unlock()
	}
	return decl.BuildMatcher(params)
}

// pushIf 快照匹配连接，按 path+规范化 query 分组去重重算后逐连接投递。
// 重算在锁外执行（数据函数可能慢），投递非阻塞。
func (h *Hub) pushIf(match func(*Conn) bool) {
	h.mu.Lock()
	var matched []*Conn
	for conn := range h.conns {
		if match(conn) {
			matched = append(matched, conn)
		}
	}
	h.mu.Unlock()
	if len(matched) == 0 {
		return
	}

	type group struct {
		pattern string
		path    string
		params  map[string]string
		query   map[string]string
		conns   []*Conn
	}
	groups := make(map[string]*group)
	for _, conn := range matched {
		key := conn.Path + "|" + canonicalQuery(conn.Query)
		g, ok := groups[key]
		if !ok {
			g = &group{pattern: conn.Pattern, path: conn.Path, params: conn.Params, query: conn.Query}
			groups[key] = g
		}
		g.conns = append(g.conns, conn)
	}

	for _, g := range groups {
		data, ok := h.dataFn(g.pattern, g.params, g.query)
		if !ok {
			continue
		}
		payload, err := json.Marshal(ssr.PageBootstrap{
			Route:        g.path,
			Params:       g.params,
			Query:        g.query,
			InitialState: data,
		})
		if err != nil {
			log.Printf("sse: encode payload for %s failed: %v", g.path, err)
			continue
		}
		var frame strings.Builder
		frame.WriteString("event: " + pushEventName + "\ndata: ")
		frame.Write(payload)
		frame.WriteString("\n\n")
		dropped := 0
		for _, conn := range g.conns {
			if !conn.Send([]byte(frame.String())) {
				dropped++
			}
		}
		log.Printf("sse: pushed %s to %d conns (%d dropped)", g.path, len(g.conns)-dropped, dropped)
	}
}

// canonicalQuery 规范化 query（key 排序），同 path 同 query 的连接共享一次重算。
func canonicalQuery(query map[string]string) string {
	values := make(url.Values, len(query))
	for k, v := range query {
		values.Set(k, v)
	}
	return values.Encode()
}
