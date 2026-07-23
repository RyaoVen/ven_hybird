package build

import (
	"ven_hybird/hybrid"

	"github.com/gofiber/fiber/v2"
)

// registerAuthRoutes 注册鉴权端点。
// 注意：login 当前为 demo 放行，不校验任何凭据，仅用于打通鉴权链路；
// 接入真实登录体系（用户/密码校验）后替换这里的放行逻辑。
func registerAuthRoutes(a *hybrid.App) {
	server := a.Server()

	// 登录：校验通过后调用放行函数下发鉴权 cookie
	server.App().Post("/auth/login", func(ctx *fiber.Ctx) error {
		var body struct {
			Role string `json:"role"`
		}
		if err := ctx.BodyParser(&body); err != nil || body.Role == "" {
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role is required"})
		}
		if err := server.GrantAuth(ctx, body.Role); err != nil {
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return ctx.JSON(fiber.Map{"role": body.Role})
	})

	// 登出：注销会话并清除鉴权 cookie
	server.App().Post("/auth/logout", func(ctx *fiber.Ctx) error {
		server.RevokeAuth(ctx)
		return ctx.SendStatus(fiber.StatusNoContent)
	})
}
