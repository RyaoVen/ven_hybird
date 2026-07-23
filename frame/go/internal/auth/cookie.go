package auth

import "github.com/gofiber/fiber/v2"

// CookieAuth 从请求 cookie 中解析用户角色。
// 当前为 stub：未接入真实鉴权，统一返回 guest。
func CookieAuth(ctx *fiber.Ctx) (role string, ok bool) {
	_ = ctx
	return "guest", true
}
