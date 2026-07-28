package build

import (
	"ven_hybird/hybrid"
)

// registerApiRoutes 注册业务 API（框架自动加 /api 前缀）。
func registerApiRoutes(a *hybrid.App) error {
	// 连通性检查
	if err := a.Get("/ping", nil, func(c *hybrid.ApiCtx) error {
		return c.JSON(200, map[string]any{"pong": true})
	}); err != nil {
		return err
	}

	// 演示：数据变更显式声明（联动静态页 ISR 失效）
	if err := a.Post("/news/:id/refresh", nil, func(c *hybrid.ApiCtx) error {
		id := c.Param("id")
		if err := a.DataChange("/news/:id", id); err != nil {
			return c.Error(500, err.Error())
		}
		return c.JSON(200, map[string]any{"refreshed": id})
	}); err != nil {
		return err
	}

	return nil
}
