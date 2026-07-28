package hybrid

import "github.com/gofiber/fiber/v2"

// ApiCtx 是 API 处理器的请求上下文，对 fiber.Ctx 的封装截流。
// 与 PageCtx 不同：响应直接写出（JSON/Error），不做截流，没有 Render。
type ApiCtx struct {
	ctx *fiber.Ctx
}

func newApiCtx(ctx *fiber.Ctx) *ApiCtx {
	return &ApiCtx{ctx: ctx}
}

// Param 读取路径参数，如 /users/:id 中的 id。
func (c *ApiCtx) Param(key string) string {
	return c.ctx.Params(key)
}

// Query 读取 URL 查询参数。
func (c *ApiCtx) Query(key string) string {
	return c.ctx.Query(key)
}

// Bind 把请求体解析到 v（JSON）。
func (c *ApiCtx) Bind(v any) error {
	return c.ctx.BodyParser(v)
}

// Body 返回原始请求体（逃生口：Bind 之外的自定义解析场景）。
func (c *ApiCtx) Body() []byte {
	return c.ctx.Body()
}

// JSON 以指定状态码返回 JSON 数据。
func (c *ApiCtx) JSON(status int, data any) error {
	return c.ctx.Status(status).JSON(data)
}

// Error 以指定状态码返回统一错误格式 {"error": message}。
func (c *ApiCtx) Error(status int, message string) error {
	return c.ctx.Status(status).JSON(fiber.Map{"error": message})
}
