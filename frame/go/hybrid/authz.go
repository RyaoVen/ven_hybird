// 鉴权公共逻辑（Page 与 API 共用一份实现）与 /api 路由规则。
package hybrid

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// apiPrefix 是后端接口的保留前缀，框架给所有 API 自动加上。
const apiPrefix = "/api"

// authCheck 执行 cookie 鉴权与权限校验，返回用户角色与判定结果。
// levels 为空 = 公开放行（status 200）；status 为 401（unauthenticated）或 403（forbidden）时拒绝。
func (a *App) authCheck(ctx *fiber.Ctx, levels []int64) (userRole string, status int, reason string) {
	if len(levels) == 0 {
		return "", fiber.StatusOK, ""
	}
	userRole, ok := a.server.CookieAuth(ctx)
	if !ok {
		return "", fiber.StatusUnauthorized, "unauthenticated"
	}
	allowed, err := a.server.CheckAuth(userRole, levels)
	if err != nil || !allowed {
		return userRole, fiber.StatusForbidden, "forbidden"
	}
	return userRole, fiber.StatusOK, ""
}

// apiRoute 归一化 API 路由：强制 /api 前缀，pattern 自带 /api 前缀时报错。
func apiRoute(pattern string) (string, error) {
	if pattern == apiPrefix || strings.HasPrefix(pattern, apiPrefix+"/") {
		return "", fmt.Errorf("hybrid: api pattern %q must not include %s prefix (auto-prepended)", pattern, apiPrefix)
	}
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	return apiPrefix + pattern, nil
}

// checkPagePatternAllowed 禁止前端页面占用 /api 前缀（前后端路由物理隔离）。
func checkPagePatternAllowed(pattern string) error {
	if pattern == apiPrefix || strings.HasPrefix(pattern, apiPrefix+"/") {
		return fmt.Errorf("hybrid: page pattern %q is reserved for backend APIs (%s)", pattern, apiPrefix)
	}
	return nil
}
