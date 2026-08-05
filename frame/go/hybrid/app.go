// Package hybrid 是用户引用的胶水层，把 internal/httpserver 的页面渲染能力
// 反包成固定格式的注册函数。
package hybrid

import (
	"ven_hybird/internal/event"
	"ven_hybird/internal/httpserver"
	"ven_hybird/internal/isr"
	"ven_hybird/internal/sse"
)

// App 是 hybrid 框架应用，底层基于 *httpserver.Server。
type App struct {
	server        *httpserver.Server
	pages         []page
	loginRedirect string // 401 时的登录跳转目标

	pageHandlers   map[string]PageHandler      // Page 数据函数（按模板字符串；启动期注册，运行期只读）
	staticHandlers map[string]PageHandler      // StaticPage 数据函数（同上）
	pageDecls      map[string]*isr.Declaration // 页面 pattern 解析缓存（SSE 路由解析用）
	bus            *event.Bus                  // 变更事件总线（DataChange 唯一失效路径）
	hub            *sse.Hub                    // SSE 推送连接表
}

// defaultLoginRedirect 是默认的登录跳转路径。
const defaultLoginRedirect = "/login"

// New 创建 hybrid 应用，注入已构建好的 *httpserver.Server。
func New(server *httpserver.Server) *App {
	a := &App{
		server:         server,
		loginRedirect:  defaultLoginRedirect,
		pageHandlers:   make(map[string]PageHandler),
		staticHandlers: make(map[string]PageHandler),
		pageDecls:      make(map[string]*isr.Declaration),
	}
	a.hub = sse.New(a.pageData)
	// 事件总线接线：① 删除走 httpserver 的 ISR 失效，② 再生走本层数据函数 + 回源渲染/落盘；
	// 配置了 Redis 时接入跨实例传输（DataChange 广播到全部实例，各实例独立走本地总线）；
	// flush ① 完成后联动 SSE 推送（在线用户无感更新）
	a.bus = event.New(server.InvalidateStatic, a.renderStatic, server.MaterializeStatic)
	// debounce/容量参数配置化：字面量构造 Config（测试）未设的字段保留各组件默认值
	if cfg := server.Config(); cfg.EventQuietWindow > 0 {
		a.bus.QuietWindow = cfg.EventQuietWindow
		a.bus.MaxWait = cfg.EventMaxWait
	}
	if cfg := server.Config(); cfg.EventMaxPending > 0 {
		a.bus.MaxPending = cfg.EventMaxPending
	}
	if cfg := server.Config(); cfg.SSEMaxConns > 0 {
		a.hub.MaxConns = cfg.SSEMaxConns
	}
	a.bus.SetNotifier(a.hub.NotifyEvents)
	if transport := server.EventTransport(); transport != nil {
		a.bus.SetTransport(transport)
	}
	a.registerSSE()
	return a
}

// Server 返回底层的 *httpserver.Server。
func (a *App) Server() *httpserver.Server {
	return a.server
}

// SetLoginRedirect 配置 401（未登录）时的登录跳转目标，默认 /login。
// 同步到 httpserver，StaticPage 鉴权中间件的 302 跳转同样生效。
func (a *App) SetLoginRedirect(path string) {
	a.loginRedirect = path
	a.server.SetLoginRedirect(path)
}

// SetVisitRecorder 设置页面访问统计埋点回调（nil = 关闭埋点）。
// 回调在页面请求的 goroutine 中同步调用，必须快速返回；panic 由框架兜底，不影响页面响应。
// 计数规则：仅 GET 页面请求（跳过 data-only 取数与非页面前缀），ISR 直发同样计数。
func (a *App) SetVisitRecorder(fn func(path string)) {
	a.server.SetVisitRecorder(fn)
}

// RegisterRole 注册一个角色（权限等级），可指定继承的父角色。
// 需在 Page() 调用前完成角色注册，否则 Page() 解析 role 会失败。
func (a *App) RegisterRole(role string, parents []string) error {
	return a.server.RegisterRole(role, parents)
}

// InvalidatePage 使指定路径的页面缓存失效（业务数据变更后手动调用），
// 并联动向正在浏览该路径的 SSE 连接推送最新数据（动态页实时更新）。
func (a *App) InvalidatePage(path string) {
	a.server.InvalidatePage(path)
	a.hub.NotifyPath(path)
}

// Close 关停实时推送（drain 全部 SSE 连接；优雅关停时在 HTTP 关闭前调用）。
func (a *App) Close() {
	a.hub.Close()
}

// Listen 注册页面兜底路由并启动监听。
// 兜底路由必须最后注册（fiber 按注册顺序匹配），统一在这里强制顺序。
func (a *App) Listen(addr string) error {
	a.server.RegisterPageFallback()
	return a.server.App().Listen(addr)
}
