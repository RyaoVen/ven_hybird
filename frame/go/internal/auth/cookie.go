package auth

import "github.com/gofiber/fiber/v2"

// CookieAuth 从请求 cookie 中解析用户角色。
// 当前为 stub：未接入真实鉴权，统一返回 guest。
// 声明为包级变量以便测试替换；接入真实鉴权时替换整个实现。
var CookieAuth = func(ctx *fiber.Ctx) (role string, ok bool) {
	_ = ctx
	return "guest", true
}
