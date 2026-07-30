package hybrid

import (
	"ven_hybird/internal/httpserver"

	"github.com/gofiber/fiber/v2"
)

// PageCtx 是页面处理器的请求上下文，对 fiber.Ctx 的封装截流。
// 它截流 handler 设置的 JSON 数据，由 Page 注册层统一决定返回 JSON 还是 SSR 页面。
// 后台预渲染等无请求场景使用静态源（params/query 覆盖），此时 fiber ctx 为 nil。
type PageCtx struct {
	ctx       *fiber.Ctx
	server    *httpserver.Server
	params    map[string]string // 非 nil 时覆盖 ctx.Params（静态源）
	query     map[string]string // 非 nil 时覆盖 ctx.Query（静态源）
	data      any
	render    bool
	responded bool
}

func newPageCtx(ctx *fiber.Ctx, server *httpserver.Server) *PageCtx {
	return &PageCtx{ctx: ctx, server: server}
}

// newStaticPageCtx 构造无请求的静态 PageCtx（后台预渲染执行数据函数用）。
func newStaticPageCtx(params, query map[string]string, server *httpserver.Server) *PageCtx {
	return &PageCtx{params: params, query: query, server: server}
}

// Param 读取路径参数，如 /blog/:slug 中的 slug。
func (c *PageCtx) Param(key string) string {
	if c.params != nil {
		return c.params[key]
	}
	return c.ctx.Params(key)
}

// Query 读取 URL 查询参数。
func (c *PageCtx) Query(key string) string {
	if c.query != nil {
		return c.query[key]
	}
	return c.ctx.Query(key)
}

// JSON 设置页面要返回的 JSON 数据。
// 数据不会立即写回响应，最后由框架根据请求头决定输出 JSON 还是 SSR 页面。
func (c *PageCtx) JSON(data any) error {
	c.data = data
	return nil
}

// User 从 ven_auth cookie 解析当前会话身份（用户主键与角色）；
// 未登录、过期或令牌无效时 ok=false；静态源（后台预渲染）无请求上下文，恒 ok=false。
func (c *PageCtx) User() (userID, role string, ok bool) {
	if c.ctx == nil {
		return "", "", false
	}
	return c.server.CurrentUser(c.ctx)
}

// Render 标记当前请求需要走 SSR 页面渲染。
func (c *PageCtx) Render() error {
	c.render = true
	return nil
}

// NotFound 直接向客户端返回 404，并标记响应已完成，框架不再做后续处理。
// 静态源（无请求）下仅标记完成，由调用方决定跳过。
func (c *PageCtx) NotFound() error {
	c.responded = true
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Status(fiber.StatusNotFound).SendString("Not Found")
}
