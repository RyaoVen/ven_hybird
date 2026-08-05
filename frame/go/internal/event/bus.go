// Package event 提供进程内事件总线：DataChange 变更事件的异步入队、
// 静默窗口合批（debounce）、批内"先删后渲"与批间流水再生。
//
// 总线是静态页失效的唯一路径：
//   - 批内：① 删物化文件 + 清内存缓存（快）→ 屏障 → ② 回源再生 + 落盘（慢），
//     ② 必须在整批 ① 完成后开始；
//   - 批间流水：② 渲染第 N 代期间，消费循环可并行执行第 N+1 代的 ①；
//     页面级代际（epoch map）保证老代渲染不得覆盖新代删除；
//   - 去重：同批重叠的变更范围经 map 去重（params 前缀即范围包含），同页一轮只处理一次。
//
// Debounce：事件入队后等待 QuietWindow 静默窗口，无新变更则 flush；
// 持续变更时 MaxWait 强制 flush 防饥饿。原子语义 = 页级原子（temp+rename，
// 由 isr.Store.Materialize 保证），接受秒级一致性窗口。
package event

import (
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// 默认 debounce 参数（config.Load 的默认值与此一致；
// 字面量构造 Config 未设 EventQuietWindow/EventMaxWait 时总线保留这些值）。
const (
	defaultQuietWindow = 5 * time.Second  // 静默窗口：无新变更即 flush
	defaultMaxWait     = 30 * time.Second // 最大等待：持续变更强制 flush 防饥饿
)

// ChangeEvent 是一次数据变更声明（StaticPage 模板 + 左到右填充的动态段参数）。
type ChangeEvent struct {
	Pattern    string    // StaticPage 模板，如 /news/:id
	Params     []string  // 左到右填充的动态段参数：空 = 全局，给满 = 单页，部分 = 子树
	EnqueuedAt time.Time // 入队时间（debounce 计时基准）
}

// InvalidateFunc 执行 ① 删除阶段：删物化文件 + 清内存缓存。
// 返回被删路径与 smartLoad 热门路径（删除前统计，供 ② 再生）。
type InvalidateFunc func(pattern string, params []string) (deleted, hot []string, err error)

// RenderFunc 执行 ② 的回源渲染（不落盘）：ok=false 表示跳过（handler 失败/NotFound/渲染失败）。
type RenderFunc func(template, path string) (html string, ok bool)

// MaterializeFunc 执行 ② 的落盘（含上限治理）。
type MaterializeFunc func(path, html string) error

// NotifyFunc 在批次 ① 完成后回调（联动推送等；同步调用，须快速返回）。
// events 为本批 ① 成功的变更（失败的已剔除并记日志）。
type NotifyFunc func(events []ChangeEvent)

// regenTask 是 ② 再生任务：一个具体页面 + 其 ① 完成时的代际。
type regenTask struct {
	template string
	path     string
	epoch    uint64
}

// Transport 是事件跨实例传输抽象（集群部署用；单实例无 Transport，现状不变）。
// 实现允许丢消息（如 Redis Pub/Sub）：变更事件本质是敦促更新，
// 丢失的实例靠下次变更或重启重载自然收敛。
type Transport interface {
	// Publish 向其他实例广播一次变更。
	Publish(ev ChangeEvent) error
	// Subscribe 订阅其他实例的变更（实现内部起 goroutine，随进程生命周期）。
	Subscribe(handler func(ev ChangeEvent))
}

// Bus 是进程内变更事件总线：单队列 + 消费循环 + 单再生 worker。
type Bus struct {
	// QuietWindow 是静默窗口（入队后无新变更则 flush）。可在启动期调整（测试用）。
	QuietWindow time.Duration
	// MaxWait 是本批首个事件入队后的最大等待（强制 flush 防饥饿）。可在启动期调整。
	MaxWait time.Duration

	invalidate  InvalidateFunc
	render      RenderFunc
	materialize MaterializeFunc

	transport Transport // 跨实例传输（nil = 单实例；启动期 SetTransport 接线，之后只读）
	notifier  NotifyFunc // flush ① 完成后的联动回调（nil = 无；启动期 SetNotifier 接线，之后只读）

	mu      sync.Mutex            // 保护 pending/firstAt/lastAt
	pending map[string]*ChangeEvent // 待处理批次（key = pattern + params，map 去重）
	firstAt time.Time             // 本批首个事件入队时间
	lastAt  time.Time             // 本批最近事件入队时间
	signal  chan struct{}         // 入队信号（buffered 1，多次入队合并）

	phaseMu  sync.Mutex        // 串行化 ① 删除阶段与 ② 落盘检查+写入（防跨代覆盖）
	epochSeq uint64            // 全局代际序号
	epoch    map[string]uint64 // 页面级代际：① 每删一次递增

	regenCh chan []regenTask // ② 再生任务队列（批间流水：消费循环投递后立即回收集态）
	stop    chan struct{}
}

// New 创建并启动事件总线（消费循环与再生 worker 各一个 goroutine）。
func New(invalidate InvalidateFunc, render RenderFunc, materialize MaterializeFunc) *Bus {
	b := &Bus{
		QuietWindow: defaultQuietWindow,
		MaxWait:     defaultMaxWait,
		invalidate:  invalidate,
		render:      render,
		materialize: materialize,
		pending:     make(map[string]*ChangeEvent),
		epoch:       make(map[string]uint64),
		regenCh:     make(chan []regenTask, 64),
		signal:      make(chan struct{}, 1),
		stop:        make(chan struct{}),
	}
	go b.run()
	go b.regenLoop()
	return b
}

// Stop 停止消费循环与再生 worker（已投递的再生任务可能丢弃；测试与关停用）。
func (b *Bus) Stop() {
	close(b.stop)
}

// SetTransport 接入跨实例传输（启动期调用一次，之后只读）：
// 此后 Enqueue 本地入队同时广播；订阅到的事件经 Receive 只本地入队不转播。
func (b *Bus) SetTransport(t Transport) {
	b.transport = t
	t.Subscribe(b.Receive)
}

// SetNotifier 接入 flush 联动回调（启动期调用一次，之后只读）：
// 每批 ① 完成后以本批成功的变更事件回调（SSE 推送等联动用）。
func (b *Bus) SetNotifier(fn NotifyFunc) {
	b.notifier = fn
}

// Enqueue 异步入队一次变更，永不阻塞调用方；接入 Transport 后同步广播到其他实例
// （广播失败仅记日志——允许丢，接收方靠下次变更或重启重载收敛）。
// 同批内范围重叠的事件在此去重：同 pattern 下 params 前缀即范围包含
// （左到右连续填充语义），宽范围吞并窄范围，同页一轮只处理一次。
func (b *Bus) Enqueue(ev ChangeEvent) {
	b.enqueue(ev)
	if b.transport != nil {
		if err := b.transport.Publish(ev); err != nil {
			log.Printf("event: publish %s params=%v failed: %v", ev.Pattern, ev.Params, err)
		}
	}
}

// Receive 是订阅入口：其他实例广播来的事件只本地入队，不再转播。
// 防回声：本实例发出的事件绕一圈回来时，落进同批窗口被 map 去重吞掉；
// 即使漏网，删除与再生都是幂等操作。
func (b *Bus) Receive(ev ChangeEvent) {
	b.enqueue(ev)
}

// enqueue 本地入队（去重 + debounce 计时）。
func (b *Bus) enqueue(ev ChangeEvent) {
	ev.Params = append([]string(nil), ev.Params...)
	key := ev.Pattern + "\x00" + strings.Join(ev.Params, "\x00")

	b.mu.Lock()
	if len(b.pending) == 0 {
		b.firstAt = ev.EnqueuedAt
	}
	b.lastAt = ev.EnqueuedAt
	if _, ok := b.pending[key]; ok {
		b.mu.Unlock()
		log.Printf("event: dedup repeated change %s params=%v", ev.Pattern, ev.Params)
		return
	}
	for k, e := range b.pending {
		if e.Pattern != ev.Pattern {
			continue
		}
		if paramsPrefix(e.Params, ev.Params) {
			// 已有更宽范围（如全局）→ 新事件被吞并
			b.mu.Unlock()
			log.Printf("event: dedup change %s params=%v (covered by params=%v)", ev.Pattern, ev.Params, e.Params)
			return
		}
		if paramsPrefix(ev.Params, e.Params) {
			// 新事件范围更宽 → 吞并旧的窄范围
			log.Printf("event: dedup change %s params=%v (covered by params=%v)", e.Pattern, e.Params, ev.Params)
			delete(b.pending, k)
		}
	}
	b.pending[key] = &ev
	b.mu.Unlock()

	select {
	case b.signal <- struct{}{}:
	default:
	}
}

// paramsPrefix 判断 a 是否为 b 的前缀（含相等）：前缀即更宽的失效范围。
func paramsPrefix(a, b []string) bool {
	if len(a) > len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// run 是消费循环：收信号 → 按静默窗口/最大等待 arm 定时器 → 到期 flush。
// 单次迭代 panic 兜底：记日志后回到"未 arm"状态继续循环，防止协程退出或崩进程。
func (b *Bus) run() {
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	var timerC <-chan time.Time
	for {
		var stopped bool
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("event: consume loop recovered from panic: %v", r)
					timerC = nil // 定时器状态不可信：回到未 arm，等下次信号重新计时
				}
			}()
			select {
			case <-b.stop:
				stopped = true
				return
			case <-b.signal:
			case <-timerC:
				b.flush()
				timerC = nil
			}

			b.mu.Lock()
			if len(b.pending) == 0 {
				timerC = nil
				b.mu.Unlock()
				return
			}
			quietRemain := b.QuietWindow - time.Since(b.lastAt)
			maxRemain := b.MaxWait - time.Since(b.firstAt)
			wait := quietRemain
			if maxRemain < wait {
				wait = maxRemain
			}
			if wait <= 0 {
				// 已达静默窗口或最大等待：立即 flush
				b.mu.Unlock()
				b.flush()
				timerC = nil
				return
			}
			timer.Reset(wait)
			timerC = timer.C
			b.mu.Unlock()
		}()
		if stopped {
			return
		}
	}
}

// flush 处理一个批次：① 整批删除（屏障）→ ② 再生任务投递给 worker 异步执行。
func (b *Bus) flush() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	batch := b.pending
	b.pending = make(map[string]*ChangeEvent)
	b.mu.Unlock()

	events := make([]*ChangeEvent, 0, len(batch))
	for _, ev := range batch {
		events = append(events, ev)
	}
	// 排序仅为日志与测试的确定性，不影响语义
	sort.Slice(events, func(i, j int) bool {
		if events[i].Pattern != events[j].Pattern {
			return events[i].Pattern < events[j].Pattern
		}
		return strings.Join(events[i].Params, "/") < strings.Join(events[j].Params, "/")
	})

	// ① 删除阶段：phaseMu 保证与 ② 的落盘互斥（老代渲染不得跨过删除写入）。
	// ② 热门路径按页面去重后采集为再生任务，并记录其 ① 完成时的代际。
	// defer 解锁：invalidate 是用户回调，panic 时也要释放锁（否则再生 worker 卡死）。
	b.phaseMu.Lock()
	defer b.phaseMu.Unlock()
	var tasks []regenTask
	var succeeded []ChangeEvent // ① 成功的事件（联动回调用）
	seen := make(map[string]bool)
	regenDeduped := 0
	for _, ev := range events {
		deleted, hot, err := b.invalidate(ev.Pattern, ev.Params)
		if err != nil {
			log.Printf("event: invalidate %s params=%v failed: %v", ev.Pattern, ev.Params, err)
			continue
		}
		succeeded = append(succeeded, *ev)
		for _, path := range deleted {
			b.epochSeq++
			b.epoch[path] = b.epochSeq
		}
		for _, path := range hot {
			if seen[path] {
				regenDeduped++
				continue
			}
			seen[path] = true
			tasks = append(tasks, regenTask{template: ev.Pattern, path: path, epoch: b.epoch[path]})
		}
	}

	waited := time.Since(events[0].EnqueuedAt)
	for _, ev := range events[1:] {
		if d := time.Since(ev.EnqueuedAt); d > waited {
			waited = d
		}
	}
	log.Printf("event: flushed %d changes (waited %s), %d regen targets, %d deduped",
		len(events), waited.Truncate(time.Millisecond), len(tasks), regenDeduped)

	// 联动回调（SSE 推送等）：① 已完成，受影响范围确定
	if b.notifier != nil && len(succeeded) > 0 {
		b.notifier(succeeded)
	}

	// ② 再生交给 worker：消费循环立即回到收集态（批间流水）。
	if len(tasks) > 0 {
		select {
		case b.regenCh <- tasks:
		default:
			// 队列积压（渲染长期阻塞）：丢弃本批再生，页面后续走懒回源，正确性不受影响
			log.Printf("event: regen queue full, dropped %d tasks (pages will re-render lazily)", len(tasks))
		}
	}
}

// regenLoop 是 ② 再生 worker：逐批逐页回源渲染并落盘（单 goroutine，批次天然串行）。
// 单页 panic 兜底：记日志后继续处理后续批次，worker 永不退出。
func (b *Bus) regenLoop() {
	for {
		var stopped bool
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("event: regen worker recovered from panic: %v", r)
				}
			}()
			select {
			case <-b.stop:
				stopped = true
				return
			case tasks := <-b.regenCh:
				for _, task := range tasks {
					b.regenOne(task)
				}
			}
		}()
		if stopped {
			return
		}
	}
}

// regenOne 再生单个页面：渲染前与落盘前各做一次代际检查，
// 期间被更新代删除（① 已 bump epoch）的页面跳过/丢弃，实现跨代不覆盖。
func (b *Bus) regenOne(task regenTask) {
	b.phaseMu.Lock()
	stale := b.epoch[task.path] != task.epoch
	b.phaseMu.Unlock()
	if stale {
		log.Printf("event: skip stale regen %s (page re-invalidated)", task.path)
		return
	}

	// 渲染（慢）在锁外执行：不阻塞批间流水中下一代的 ①
	html, ok := b.render(task.template, task.path)
	if !ok {
		return
	}

	// 落盘检查与写入在 phaseMu 内原子完成：渲染期间若页面被更新代删除，丢弃本次渲染
	b.phaseMu.Lock()
	defer b.phaseMu.Unlock()
	if b.epoch[task.path] != task.epoch {
		log.Printf("event: drop stale render %s (page re-invalidated during render)", task.path)
		return
	}
	if err := b.materialize(task.path, html); err != nil {
		log.Printf("event: materialize %s failed: %v", task.path, err)
	}
}
