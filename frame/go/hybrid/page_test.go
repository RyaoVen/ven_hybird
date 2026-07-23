package hybrid

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func setupTestApp() (*App, *fakeSSRClient, *ssr.PendingRegistry, *httpserver.Server) {
	cfg := config.Config{
		NodeSubmitTimeout: 5 * time.Second,
		RenderTimeout:     10 * time.Second,
	}
	client := &fakeSSRClient{submitted: make(chan ssr.RenderTask, 1)}
	pending := ssr.NewPendingRegistry(10)
	patterns := pagepattern.NewValidator([]string{"/test/:id", "/ssr/:name", "/admin/:id"})
	server := httpserver.New(cfg, client, pending, fakeHookIDs{}, patterns)
	return New(server), client, pending, server
}

func TestPage_DataOnly(t *testing.T) {
	app, _, _, _ := setupTestApp()
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
	// 无 cookie 时公开页面仍应放行
	app, _, _, _ := setupTestApp()
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
	// 无 cookie 访问有 role 要求的页面应返回 401
	app, _, _, _ := setupTestApp()
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

func TestPage_GrantAuthFlow(t *testing.T) {
	app, _, _, server := setupTestApp()
	if err := app.RegisterRole("admin", nil); err != nil {
		t.Fatalf("register role failed: %v", err)
	}
	app.Page("/admin/:id", []string{"admin"}, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})

	// 模拟业务登录端点：校验通过后调用放行函数
	server.App().Post("/test-login", func(ctx *fiber.Ctx) error {
		return server.GrantAuth(ctx, "admin")
	})

	// 步骤 1: 登录放行，应同时下发 ven_auth 与 ven_role 两个 cookie
	loginResp, err := app.Server().App().Test(httptest.NewRequest("POST", "/test-login", nil))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	if loginResp.StatusCode != 200 {
		t.Fatalf("expected 200 from login, got %d", loginResp.StatusCode)
	}
	var authCookie, roleCookie *http.Cookie
	for _, cookie := range loginResp.Cookies() {
		switch cookie.Name {
		case auth.AuthCookieName:
			authCookie = cookie
		case auth.RoleCookieName:
			roleCookie = cookie
		}
	}
	if authCookie == nil || authCookie.Value == "" {
		t.Fatal("expected ven_auth cookie from login")
	}
	if roleCookie == nil || roleCookie.Value != "admin" {
		t.Fatalf("expected ven_role=admin cookie, got %+v", roleCookie)
	}

	// 步骤 2: 带合法 cookie 访问受保护页面，应放行
	req := httptest.NewRequest("GET", "/admin/42", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	req.AddCookie(authCookie)
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with valid cookie, got %d", resp.StatusCode)
	}

	// 步骤 3: 伪造令牌 cookie，应返回 401
	forged := httptest.NewRequest("GET", "/admin/42", nil)
	forged.Header.Set("X-Ven-Data-Only", "true")
	forged.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: "forged-token"})
	forgedResp, err := app.Server().App().Test(forged)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if forgedResp.StatusCode != 401 {
		t.Fatalf("expected 401 with forged cookie, got %d", forgedResp.StatusCode)
	}

	// 步骤 4: 未注册角色不能放行
	server.App().Post("/test-login-unknown", func(ctx *fiber.Ctx) error {
		return server.GrantAuth(ctx, "no-such-role")
	})
	unknownResp, err := app.Server().App().Test(httptest.NewRequest("POST", "/test-login-unknown", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if unknownResp.StatusCode == 200 {
		t.Fatal("expected grant for unregistered role to fail")
	}
}

func TestPage_SSR(t *testing.T) {
	app, client, pending, _ := setupTestApp()
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
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected text/html, got %s", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<html>hello</html>" {
		t.Fatalf("unexpected body: %s", body)
	}
}
