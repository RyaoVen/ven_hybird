// 业务 API 注册：fiber 风格的 Get/Post/Put/Delete，自动 /api 前缀，鉴权与 Page 共享。
package hybrid

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
)

// ApiHandler 是 API 处理器签名。
type ApiHandler func(c *ApiCtx) error

// Get 注册 GET /api<pattern> 接口。roles 鉴权；响应全 JSON。
func (a *App) Get(pattern string, roles []string, h ApiHandler) error {
	return a.registerAPI(fiber.MethodGet, pattern, roles, h)
}

// Post 注册 POST /api<pattern> 接口。roles 鉴权；响应全 JSON。
func (a *App) Post(pattern string, roles []string, h ApiHandler) error {
	return a.registerAPI(fiber.MethodPost, pattern, roles, h)
}

// Put 注册 PUT /api<pattern> 接口。roles 鉴权；响应全 JSON。
func (a *App) Put(pattern string, roles []string, h ApiHandler) error {
	return a.registerAPI(fiber.MethodPut, pattern, roles, h)
}

// Delete 注册 DELETE /api<pattern> 接口。roles 鉴权；响应全 JSON。
func (a *App) Delete(pattern string, roles []string, h ApiHandler) error {
	return a.registerAPI(fiber.MethodDelete, pattern, roles, h)
}

// registerAPI 归一化路由、解析角色并注册 fiber 路由。
// 请求流程：cookie 鉴权 + 权限校验（401/403 永远裸 JSON，401 带 X-Ven-Login-Path 头）→ ApiCtx 执行 handler。
func (a *App) registerAPI(method, pattern string, roles []string, h ApiHandler) error {
	route, err := apiRoute(pattern)
	if err != nil {
		return err
	}
	levels, err := a.server.ResolveRoles(roles)
	if err != nil {
		return fmt.Errorf("hybrid: resolve roles for api %q failed: %w", route, err)
	}

	a.server.App().Add(method, route, func(ctx *fiber.Ctx) error {
		userRole, status, reason := a.authCheck(ctx, levels)
		if status != fiber.StatusOK {
			log.Printf("auth: denied %s %s reason=%s role=%s api=%s", ctx.Method(), ctx.Path(), reason, userRole, route)
			if status == fiber.StatusUnauthorized {
				ctx.Set(loginPathHeader, a.loginRedirect)
			}
			return ctx.Status(status).JSON(fiber.Map{"error": reason})
		}
		return h(newApiCtx(ctx, a.server))
	})
	return nil
}
