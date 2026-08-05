package httpserver

import (
	"bytes"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestPanicRecovery_HandlerPanic500 handler panic 被 Recover 中间件捕获：
// 不崩进程，走全局 ErrorHandler 返回统一 500 并记录错误日志。
func TestPanicRecovery_HandlerPanic500(t *testing.T) {
	var buf bytes.Buffer
	original := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(original)

	s := newTestServer()
	s.App().Get("/boom", func(ctx *fiber.Ctx) error {
		panic("boom")
	})

	resp, err := s.App().Test(httptest.NewRequest("GET", "/boom", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"error":"internal server error"`) {
		t.Fatalf("expected unified error body, got %s", body)
	}
	// panic 经 ErrorHandler 记录（http error 行）
	if !strings.Contains(buf.String(), "http error: GET /boom") {
		t.Fatalf("expected error log line, got %q", buf.String())
	}
}

// TestPanicRecovery_MiddlewarePanic500 中间件链中的 panic（Recover 之后注册的 Use）
// 同样被捕获：后续请求不受影响，仍能正常服务。
func TestPanicRecovery_MiddlewarePanic500(t *testing.T) {
	s := newTestServer()
	s.App().Use("/guarded", func(ctx *fiber.Ctx) error {
		panic("middleware boom")
	})
	s.App().Get("/ok", func(ctx *fiber.Ctx) error { return ctx.SendString("pong") })

	resp, err := s.App().Test(httptest.NewRequest("GET", "/guarded", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("expected 500 for panicking middleware, got %d", resp.StatusCode)
	}

	// panic 不影响后续请求
	resp, err = s.App().Test(httptest.NewRequest("GET", "/ok", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 after recovered panic, got %d", resp.StatusCode)
	}
}
