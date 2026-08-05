// 页面代理和渲染回调处理。
package httpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"ven_hybird/internal/pagecache"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

// HandlePage 处理兜底页面渲染请求，以空 InitialState 提交 SSR。
func (s *Server) HandlePage(ctx *fiber.Ctx) error {
	return s.RenderPage(ctx, map[string]any{})
}

// RenderPage 渲染当前路径对应的页面并写回响应：先查页面缓存，命中直接返回 HTML（不回源 Node）；
// 未命中防击穿回源，成功后回填缓存。404/502/504 等失败结果不缓存。
// 渲染事件（缓存 hit/miss/shared、Node 耗时）写日志。
func (s *Server) RenderPage(ctx *fiber.Ctx, data any) error {
	// ctx.Path() 是 fasthttp 零拷贝字符串（底层是池化缓冲区），
	// route 会被 ISR 跨请求留存，必须先克隆
	return s.renderPage(ctx, strings.Clone(ctx.Path()), data)
}

// RenderPageAs 以指定路由渲染页面并在当前 URL 写回（用于 /403 等错误页原地渲染）。
// 缓存 key 使用指定路由（错误页被正确缓存，不污染当前路径的缓存）；
// 响应状态码由调用方提前通过 ctx.Status 设置。
func (s *Server) RenderPageAs(ctx *fiber.Ctx, route string, data any) error {
	return s.renderPage(ctx, route, data)
}

// renderPage 是 RenderPage/RenderPageAs 的共用实现：route 参与缓存 key 与页面匹配。
func (s *Server) renderPage(ctx *fiber.Ctx, route string, data any) error {
	key, keyErr := pagecache.Key(route, ctx.Queries(), data)
	if keyErr == nil {
		if entry, ok := s.pageCache.Get(key); ok {
			log.Printf("render: hit %s %s", ctx.Method(), route)
			// 自愈：缓存命中但物化文件缺失（外部删除）时补写
			if !s.isrStore.Exists(route) {
				s.materializeQuiet(route, entry.HTML)
			}
			ctx.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
			// SSR 页面内容每次渲染可变：no-cache 防止浏览器/中间层缓存部署前的旧页面（与 SSE 同策略）
			ctx.Set(fiber.HeaderCacheControl, "no-cache")
			return ctx.SendString(entry.HTML)
		}
	}

	render := func() (*pagecache.Entry, error) { return s.render(ctx, route, data) }
	var entry *pagecache.Entry
	var shared bool
	var err error
	if keyErr == nil {
		entry, shared, err = s.pageCache.Do(key, render)
	} else {
		// data 无法序列化：跳过缓存直接回源
		log.Printf("render: nocache %s %s (cache key: %v)", ctx.Method(), route, keyErr)
		entry, err = render()
	}
	if err != nil {
		var renderErr *renderError
		if errors.As(err, &renderErr) && renderErr.status >= fiber.StatusInternalServerError && keyErr == nil {
			// stale 兜底：Node 侧失败（502/503/504）且缓存有过期条目 → 发 stale 而非 502，
			// 后台异步回源刷新（防 Node 抖动期全站白屏）。
			// 4xx（如 PAGE_NOT_FOUND）不兜底：Node 是路由权威，说没了就是没了。
			if stale, ok := s.pageCache.GetStale(key); ok && stale.HTML != "" {
				log.Printf("render: stale %s %s", ctx.Method(), route)
				s.refreshStaleAsync(key, route, ctx.Queries(), data)
				ctx.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
				ctx.Set(fiber.HeaderCacheControl, "no-cache")
				return ctx.SendString(stale.HTML)
			}
		}
		if errors.As(err, &renderErr) {
			if renderErr.json {
				return ctx.Status(renderErr.status).JSON(fiber.Map{"error": renderErr.message})
			}
			return ctx.Status(renderErr.status).SendString(renderErr.message)
		}
		return err
	}

	if keyErr == nil {
		if shared {
			log.Printf("render: shared %s %s", ctx.Method(), route)
		} else {
			log.Printf("render: miss %s %s node=%dms", ctx.Method(), route, entry.Duration)
			// ISR 物化：声明为静态页的路径在回源渲染后落盘（仅 leader，避免 follower 重复写）
			s.materializeQuiet(route, entry.HTML)
		}
	}
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	// SSR 页面内容每次渲染可变：no-cache 防止浏览器/中间层缓存部署前的旧页面（与 SSE 同策略）
	ctx.Set(fiber.HeaderCacheControl, "no-cache")
	return ctx.SendString(entry.HTML)
}

// renderError 携带 HTTP 状态码与响应格式的回源失败，用于共享给 flight follower。
type renderError struct {
	status  int
	message string
	json    bool // true 用 JSON 响应，false 用纯文本
}

func (e *renderError) Error() string { return e.message }

// render 回源渲染：提交 SSR 任务给 Node.js 并等待回调结果，
// 成功返回可缓存的 Entry，失败返回 renderError（不缓存）。
// route 为页面匹配路由（兜底/常规渲染为请求路径，错误页渲染为错误页路由）。
func (s *Server) render(ctx *fiber.Ctx, route string, data any) (*pagecache.Entry, error) {
	return s.renderWithQuery(route, ctx.Queries(), data)
}

// refreshStaleAsync 在后台回源刷新过期缓存（stale-while-revalidate 的 revalidate 阶段）。
// 复用 pageCache.Do 的防击穿：同 key 并发仅一次刷新；刷新失败仅记日志（不影响已发出的 stale 响应）。
// query 先拷贝再进 goroutine——ctx 在响应后会被 fiber 回收复用，不能跨请求引用。
func (s *Server) refreshStaleAsync(key, route string, query map[string]string, data any) {
	queryCopy := make(map[string]string, len(query))
	for k, v := range query {
		queryCopy[k] = v
	}
	go func() {
		entry, _, err := s.pageCache.Do(key, func() (*pagecache.Entry, error) {
			return s.renderWithQuery(route, queryCopy, data)
		})
		if err != nil {
			log.Printf("render: stale refresh %s failed: %v", route, err)
			return
		}
		log.Printf("render: stale refresh %s ok node=%dms", route, entry.Duration)
	}()
}

// renderWithQuery 是 render 的核心（显式 query 版本，供后台预渲染复用）。
func (s *Server) renderWithQuery(route string, query map[string]string, data any) (*pagecache.Entry, error) {
	// 步骤 0: Node 熔断——连续失败达阈值后快速失败（503），不再等待渲染超时；
	// 半开间隔后放行一个试探请求，成功即恢复
	if !s.breaker.Allow() {
		return nil, &renderError{fiber.StatusServiceUnavailable, "render worker is unavailable (circuit open)", true}
	}

	// 步骤 1: 生成唯一的 HookID
	hookID, err := s.hookIDs.New()
	if err != nil {
		return nil, &renderError{fiber.StatusInternalServerError, "create render request failed", true}
	}

	// 步骤 2: 在 PendingRegistry 中注册等待通道（记录归属路由，回调时校验）
	// remove 函数用于在请求结束时清理注册，防止内存泄漏
	waiter, remove, err := s.pending.Register(hookID, route)
	if err != nil {
		return nil, &renderError{fiber.StatusServiceUnavailable, err.Error(), true}
	}
	defer remove()

	// 步骤 3: 构造页面启动数据和渲染任务
	bootstrap := ssr.PageBootstrap{
		Route:        route,
		Params:       map[string]string{},
		Query:        query,
		InitialState: data,
	}
	task := ssr.RenderTask{
		HookID:       hookID,
		RequestRoute: route,
		Payload:      bootstrap,
	}

	// 步骤 4: 提交渲染任务至 Node.js 工作节点
	// 使用独立的 context 控制提交超时，与渲染超时分离
	submitContext, cancelSubmit := context.WithTimeout(context.Background(), s.config.NodeSubmitTimeout)
	err = s.ssr.Submit(submitContext, task)
	cancelSubmit()
	if err != nil {
		// 传输层失败（连接拒绝/提交被拒/提交超时）：计入熔断失败
		s.breaker.RecordFailure()
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &renderError{fiber.StatusGatewayTimeout, "render worker submit timed out", true}
		}
		return nil, &renderError{fiber.StatusBadGateway, "render worker rejected request", true}
	}

	// 步骤 5: 等待渲染结果回调或超时
	// 使用 select 同时监听回调通道和超时定时器
	select {
	case callback := <-waiter:
		// Node 有响应即视为传输健康（含回调错误分支）：不计入熔断失败
		s.breaker.RecordSuccess()
		// 收到渲染回调
		if callback.Error != nil {
			// 渲染失败：根据错误码返回不同的 HTTP 状态
			if callback.Error.Code == "PAGE_NOT_FOUND" {
				return nil, &renderError{fiber.StatusNotFound, callback.Error.Message, false}
			}
			return nil, &renderError{fiber.StatusBadGateway, callback.Error.Message, false}
		}
		// 渲染成功：返回可缓存的渲染结果
		return &pagecache.Entry{
			HTML:         callback.HTML,
			MatchedRoute: callback.MatchedRoute,
			PageName:     callback.PageName,
			RenderedAt:   time.Now(),
			Duration:     callback.Duration,
		}, nil
	case <-time.After(s.config.RenderTimeout):
		// 渲染超时：计入熔断失败
		s.breaker.RecordFailure()
		return nil, &renderError{fiber.StatusGatewayTimeout, "render worker timed out", true}
	}
}

// HandleRenderCallback 处理 Node.js 工作节点的渲染结果回调。
func (s *Server) HandleRenderCallback(ctx *fiber.Ctx) error {
	// 步骤 1: 验证内部令牌，防止外部请求伪造渲染回调
	if !s.validInternalToken(ctx.Get("X-Ven-Internal-Token")) {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid internal token"})
	}

	// 步骤 2: 解析回调数据
	var callback ssr.RenderCallback
	if err := json.Unmarshal(ctx.Body(), &callback); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid render callback"})
	}

	// 校验必填字段：HookID 和 RequestRoute 不能为空，RequestRoute 必须以 "/" 开头
	if callback.HookID == "" || callback.RequestRoute == "" || !strings.HasPrefix(callback.RequestRoute, "/") {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid render callback"})
	}
	// 校验业务逻辑：如果没有错误信息，则 HTML 内容不能为空
	if callback.Error == nil && callback.HTML == "" {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "render callback html is required"})
	}

	// 步骤 3: 将回调结果投递给等待中的请求
	// 如果 Resolve 返回 false，说明对应的 pending 请求已超时或被清理
	if !s.pending.Resolve(callback) {
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "render request not found"})
	}
	return ctx.SendStatus(fiber.StatusNoContent)
}

// validInternalToken 验证内部认证令牌，使用常量时间比较防止时序攻击。
// 配置缺失时一律拒绝（fail-open 已移除：内部通道无令牌不得放行，
// 且 config.Load 启动校验已保证运行期令牌非空非默认）。
func (s *Server) validInternalToken(token string) bool {
	// 长度不同直接拒绝，避免不必要的比较操作
	if len(token) != len(s.config.InternalToken) {
		return false
	}
	// 使用常量时间比较，防止通过响应时间差异推测令牌内容
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.InternalToken)) == 1
}
