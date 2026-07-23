package hybrid

import "github.com/gofiber/fiber/v2"

// PageCtx 是页面处理器的请求上下文。
// 它截流 handler 设置的 JSON 数据，由 Page 注册层统一决定返回 JSON 还是 SSR 页面。
type PageCtx struct {
	ctx       *fiber.Ctx
	app       *App
	data      any
	render    bool
	responded bool
}

func newPageCtx(ctx *fiber.Ctx, app *App) *PageCtx {
	return &PageCtx{ctx: ctx, app: app}
}

// Param 读取路径参数，如 /blog/:slug 中的 slug。
func (c *PageCtx) Param(key string) string {
	return c.ctx.Params(key)
}

// Query 读取 URL 查询参数。
func (c *PageCtx) Query(key string) string {
	return c.ctx.Query(key)
}

// JSON 设置页面要返回的 JSON 数据。
// 数据不会立即写回响应，最后由框架根据请求头决定输出 JSON 还是 SSR 页面。
func (c *PageCtx) JSON(data any) error {
	c.data = data
	return nil
}

// Render 标记当前请求需要走 SSR 页面渲染。
func (c *PageCtx) Render() error {
	c.render = true
	return nil
}

// NotFound 直接向客户端返回 404，并标记响应已完成，框架不再做后续处理。
func (c *PageCtx) NotFound() error {
	c.responded = true
	return c.ctx.Status(fiber.StatusNotFound).SendString("Not Found")
}
