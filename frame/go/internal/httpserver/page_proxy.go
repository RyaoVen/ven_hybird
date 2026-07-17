// 页面代理和渲染回调处理。
package httpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

// HandlePage 处理页面渲染请求，提交任务给 Node.js 并等待回调结果。
func (s *Server) HandlePage(ctx *fiber.Ctx) error {
	// 步骤 1: 生成唯一的 HookID
	hookID, err := s.hookIDs.New()
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "create render request failed"})
	}

	// 步骤 2: 在 PendingRegistry 中注册等待通道
	// remove 函数用于在请求结束时清理注册，防止内存泄漏
	waiter, remove, err := s.pending.Register(hookID)
	if err != nil {
		return ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	defer remove()

	// 步骤 3: 构造页面启动数据和渲染任务
	requestRoute := ctx.Path()
	bootstrap := ssr.PageBootstrap{
		Route:        requestRoute,
		Params:       map[string]string{},
		Query:        ctx.Queries(),
		InitialState: map[string]any{},
	}
	task := ssr.RenderTask{
		HookID:       hookID,
		RequestRoute: requestRoute,
		Payload:      bootstrap,
	}

	// 步骤 4: 提交渲染任务至 Node.js 工作节点
	// 使用独立的 context 控制提交超时，与渲染超时分离
	submitContext, cancelSubmit := context.WithTimeout(context.Background(), s.config.NodeSubmitTimeout)
	err = s.ssr.Submit(submitContext, task)
	cancelSubmit()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return ctx.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{"error": "render worker submit timed out"})
		}
		return ctx.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "render worker rejected request"})
	}

	// 步骤 5: 等待渲染结果回调或超时
	// 使用 select 同时监听回调通道和超时定时器
	select {
	case callback := <-waiter:
		// 收到渲染回调
		if callback.Error != nil {
			// 渲染失败：根据错误码返回不同的 HTTP 状态
			if callback.Error.Code == "PAGE_NOT_FOUND" {
				return ctx.Status(fiber.StatusNotFound).SendString(callback.Error.Message)
			}
			return ctx.Status(fiber.StatusBadGateway).SendString(callback.Error.Message)
		}
		// 渲染成功：返回 HTML 内容
		ctx.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return ctx.SendString(callback.HTML)
	case <-time.After(s.config.RenderTimeout):
		// 渲染超时
		return ctx.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{"error": "render worker timed out"})
	}
}

// HandleRenderCallback 处理 Node.js 工作节点的渲染结果回调。
func (s *Server) HandleRenderCallback(ctx *fiber.Ctx) error {
	// 步骤 1: 验证内部令牌，防止外部请求伪造渲染回调
	if !s.validInternalToken(ctx.Get("X-Ven-Internal-Token")) {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid internal token"})
	}

	// 步骤 2: 解析回调数据
	var callback ssr.RenderCallback
	if err := json.Unmarshal(ctx.Body(), &callback); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid render callback"})
	}

	// 校验必填字段：HookID 和 RequestRoute 不能为空，RequestRoute 必须以 "/" 开头
	if callback.HookID == "" || callback.RequestRoute == "" || !strings.HasPrefix(callback.RequestRoute, "/") {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid render callback"})
	}
	// 校验业务逻辑：如果没有错误信息，则 HTML 内容不能为空
	if callback.Error == nil && callback.HTML == "" {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "render callback html is required"})
	}

	// 步骤 3: 将回调结果投递给等待中的请求
	// 如果 Resolve 返回 false，说明对应的 pending 请求已超时或被清理
	if !s.pending.Resolve(callback) {
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "render request not found"})
	}
	return ctx.SendStatus(fiber.StatusNoContent)
}

// validInternalToken 验证内部认证令牌，使用常量时间比较防止时序攻击。
func (s *Server) validInternalToken(token string) bool {
	// 未配置令牌时，允许所有请求（开发模式）
	if s.config.InternalToken == "" {
		return true
	}
	// 长度不同直接拒绝，避免不必要的比较操作
	if len(token) != len(s.config.InternalToken) {
		return false
	}
	// 使用常量时间比较，防止通过响应时间差异推测令牌内容
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.InternalToken)) == 1
}
