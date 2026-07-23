package hybrid

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"ven_hybird/internal/auth"
	"ven_hybird/internal/config"
	"ven_hybird/internal/httpserver"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

type fakeSSRClient struct {
	submitted chan ssr.RenderTask
}

func (f *fakeSSRClient) Submit(ctx context.Context, task ssr.RenderTask) error {
	f.submitted <- task
	return nil
}

type fakeHookIDs struct{}

func (fakeHookIDs) New() (string, error) { return "hook-test", nil }

func setupTestApp() (*App, *fakeSSRClient, *ssr.PendingRegistry) {
	cfg := config.Config{
		NodeSubmitTimeout: 5 * time.Second,
		RenderTimeout:     10 * time.Second,
	}
	client := &fakeSSRClient{submitted: make(chan ssr.RenderTask, 1)}
	pending := ssr.NewPendingRegistry(10)
	patterns := pagepattern.NewValidator([]string{"/test/:id", "/ssr/:name", "/admin/:id"})
	server := httpserver.New(cfg, client, pending, fakeHookIDs{}, patterns)
	return New(server), client, pending
}

func TestPage_DataOnly(t *testing.T) {
	app, _, _ := setupTestApp()
	app.Page("/test/:id", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})

	req := httptest.NewRequest("GET", "/test/42", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"id":"42"}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestPage_PublicSkipsCookieAuth(t *testing.T) {
	// 把 CookieAuth 换成"无 cookie 即失败"的版本，公开页面仍应放行
	original := auth.CookieAuth
	auth.CookieAuth = func(ctx *fiber.Ctx) (string, bool) { return "", false }
	defer func() { auth.CookieAuth = original }()

	app, _, _ := setupTestApp()
	app.Page("/test/:id", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})

	req := httptest.NewRequest("GET", "/test/42", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for public page without cookie, got %d", resp.StatusCode)
	}
}

func TestPage_ProtectedRequiresCookieAuth(t *testing.T) {
	// 同样的失败 CookieAuth，有 role 要求的页面应返回 401
	original := auth.CookieAuth
	auth.CookieAuth = func(ctx *fiber.Ctx) (string, bool) { return "", false }
	defer func() { auth.CookieAuth = original }()

	app, _, _ := setupTestApp()
	if err := app.RegisterRole("admin", nil); err != nil {
		t.Fatalf("register role failed: %v", err)
	}
	app.Page("/admin/:id", []string{"admin"}, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})

	req := httptest.NewRequest("GET", "/admin/42", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 for protected page without cookie, got %d", resp.StatusCode)
	}
}

func TestPage_SSR(t *testing.T) {
	app, client, pending := setupTestApp()
	app.Page("/ssr/:name", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"name": c.Param("name")})
	})

	go func() {
		task := <-client.submitted
		pending.Resolve(ssr.RenderCallback{
			HookID:       task.HookID,
			RequestRoute: task.RequestRoute,
			HTML:         "<html>hello</html>",
		})
	}()

	req := httptest.NewRequest("GET", "/ssr/world", nil)
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html, got %s", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<html>hello</html>" {
		t.Fatalf("unexpected body: %s", body)
	}
}
