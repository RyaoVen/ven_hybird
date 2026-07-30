package hybrid

import (
	"ven_hybird/internal/httpserver"

	"github.com/gofiber/fiber/v2"
)

// ApiCtx 是 API 处理器的请求上下文，对 fiber.Ctx 的封装截流。
// 与 PageCtx 不同：响应直接写出（JSON/Error），不做截流，没有 Render。
type ApiCtx struct {
	ctx    *fiber.Ctx
	server *httpserver.Server
}

func newApiCtx(ctx *fiber.Ctx, server *httpserver.Server) *ApiCtx {
	return &ApiCtx{ctx: ctx, server: server}
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

// User 从 ven_auth cookie 解析当前会话身份（用户主键与角色）；
// 未登录、过期或令牌无效时 ok=false。
func (c *ApiCtx) User() (userID, role string, ok bool) {
	return c.server.CurrentUser(c.ctx)
}

// JSON 以指定状态码返回 JSON 数据。
func (c *ApiCtx) JSON(status int, data any) error {
	return c.ctx.Status(status).JSON(data)
}

// Error 以指定状态码返回统一错误格式 {"error": message}。
func (c *ApiCtx) Error(status int, message string) error {
	return c.ctx.Status(status).JSON(fiber.Map{"error": message})
}
