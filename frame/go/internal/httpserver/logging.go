// 请求访问日志。
package httpserver

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
)

// requestLogger 记录每个请求的方法、路径、状态码与耗时。
// 处理器返回错误时记录错误（最终状态码由全局 ErrorHandler 统一写为 500）。
func requestLogger() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		start := time.Now()
		err := ctx.Next()
		elapsed := time.Since(start).Round(time.Microsecond)
		if err != nil {
			log.Printf("http: %s %s error=%q %s", ctx.Method(), ctx.Path(), err, elapsed)
			return err
		}
		log.Printf("http: %s %s %d %s", ctx.Method(), ctx.Path(), ctx.Response().StatusCode(), elapsed)
		return nil
	}
}
