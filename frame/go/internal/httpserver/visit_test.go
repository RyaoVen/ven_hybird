// 访问统计埋点中间件测试。
package httpserver

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestVisitTracking_Middleware 埋点中间件：仅 GET 页面请求计数，白名单/数据取数跳过。
func TestVisitTracking_Middleware(t *testing.T) {
	s := newTestServer()
	var got []string
	s.SetVisitRecorder(func(path string) { got = append(got, path) })
	s.App().Get("/hello", func(ctx *fiber.Ctx) error { return ctx.SendString("hi") })

	cases := []struct {
		name     string
		method   string
		path     string
		dataOnly bool
		wantHit  bool
	}{
		{name: "GET page", method: "GET", path: "/hello", wantHit: true},
		{name: "GET home root", method: "GET", path: "/", wantHit: true},
		{name: "POST not counted", method: "POST", path: "/hello", wantHit: false},
		{name: "api excluded", method: "GET", path: "/api/ping", wantHit: false},
		{name: "assets excluded", method: "GET", path: "/assets/app.js", wantHit: false},
		{name: "auth excluded", method: "GET", path: "/auth/callback", wantHit: false},
		{name: "internal excluded", method: "GET", path: "/_internal/sse", wantHit: false},
		{name: "images excluded", method: "GET", path: "/images/1", wantHit: false},
		{name: "healthz excluded", method: "GET", path: "/healthz", wantHit: false},
		{name: "mcp excluded", method: "GET", path: "/mcp", wantHit: false},
		{name: "data-only excluded", method: "GET", path: "/hello", dataOnly: true, wantHit: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := len(got)
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.dataOnly {
				req.Header.Set(dataOnlyHeader, "true")
			}
			_, err := s.App().Test(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			// 中间件先于路由执行：未注册路径返回 404 也须经过埋点，这里不校验状态码
			hit := len(got) > before
			if hit != tc.wantHit {
				t.Fatalf("expected counted=%v, got %v (recorded: %v)", tc.wantHit, hit, got[before:])
			}
		})
	}
}

// TestVisitTracking_RecorderNil 未注入回调（nil）时中间件不记录也不影响请求。
func TestVisitTracking_RecorderNil(t *testing.T) {
	s := newTestServer()
	s.App().Get("/hello", func(ctx *fiber.Ctx) error { return ctx.SendString("hi") })

	resp, err := s.App().Test(httptest.NewRequest("GET", "/hello", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestVisitTracking_RecorderSetAfterNew 回调可在 New 之后注入（组装根接线场景），注入即生效。
func TestVisitTracking_RecorderSetAfterNew(t *testing.T) {
	s := newTestServer()
	s.App().Get("/hello", func(ctx *fiber.Ctx) error { return ctx.SendString("hi") })

	var got []string
	s.SetVisitRecorder(func(path string) { got = append(got, path) })
	if _, err := s.App().Test(httptest.NewRequest("GET", "/hello", nil)); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if len(got) != 1 || got[0] != "/hello" {
		t.Fatalf("expected recorded [/hello], got %v", got)
	}
}

// TestVisitTracking_RecorderPanic 回调 panic 被中间件兜住，请求不受影响。
func TestVisitTracking_RecorderPanic(t *testing.T) {
	s := newTestServer()
	s.SetVisitRecorder(func(path string) { panic("recorder bug") })
	s.App().Get("/hello", func(ctx *fiber.Ctx) error { return ctx.SendString("hi") })

	resp, err := s.App().Test(httptest.NewRequest("GET", "/hello", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
