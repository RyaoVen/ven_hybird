package router

import (
	"ven_hybird/http"
	"ven_hybird/types"

	"github.com/gofiber/fiber/v2"
)

type ControllerConfig struct {
	FiberConfig fiber.Config
}

type Controller struct {
	router      *PageRouter
	httpHandler *http.HTTPHandler
	app         *fiber.App
}

func NewController(rootPath string, app *fiber.App) *Controller {
	controller := &Controller{}
	controller.app = app
	controller.router = NewPageRouter(rootPath)
	return controller
}

// 生成路由
func (controller *Controller) GenerateRouter() {
	controller.router.generateRouter()
	controller.GenerateStaticRouter(controller.router.GetPages())
	controller.GenerateDynamicRouter(controller.router.dynamicPages)
}

// 静态页面路由生成
func (controller *Controller) GenerateStaticRouter(staticPages []types.StaticPage) {
	for _, page := range staticPages {
		// 确保路由以 / 开头
		route := page.Route
		if len(route) > 0 && route[0] != '/' {
			route = "/" + route
		}

		controller.app.Get(route, func(c *fiber.Ctx) error {
			return c.SendString(controller.httpHandler.GetPageHTML(page))
		})
	}
}

// 动态页面路由生成
// 根据动态页面的 Route 和 Params 生成 fiber 路由
// Route 格式如 "user/:id" 或 "post/:slug/comment/:id"
// Params 格式如 []string{":id"} 或 []string{":slug", ":id"}
func (controller *Controller) GenerateDynamicRouter(dynamicPages []types.DynamicPage) {
	for _, page := range dynamicPages {
		// 确保路由以 / 开头
		route := page.Route
		if len(route) > 0 && route[0] != '/' {
			route = "/" + route
		}

		controller.app.Get(route, func(c *fiber.Ctx) error {
			// 从请求中提取所有参数值
			params := make(map[string]string)
			for _, param := range page.Params {
				// param 格式为 ":id"，去掉冒号前缀获取参数名
				paramName := param[1:]
				params[paramName] = c.Params(paramName)
			}
			return c.SendString(controller.httpHandler.GetDynamicPageHTML(page, params))
		})
	}
}

// SPA 静态资源加载
// TODO 没写完，还得加配置系统
func (controller *Controller) SPAClientLoading() {
	controller.app.Get("/spa/script.js", func(ctx *fiber.Ctx) error {
		return ctx.SendFile("./")
	})
}
