package hybrid

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ven_hybird/internal/auth"

	"github.com/gofiber/fiber/v2"
)

func TestApi_PrefixEnforced(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	if err := app.Get("/ping", nil, func(c *ApiCtx) error {
		return c.JSON(200, fiber.Map{"pong": true})
	}); err != nil {
		t.Fatalf("register api failed: %v", err)
	}

	// /api/ping 可达
	req := httptest.NewRequest("GET", "/api/ping", nil)
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 at /api/ping, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "pong") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestApi_RejectsDoublePrefix(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	if err := app.Get("/api/ping", nil, func(c *ApiCtx) error { return nil }); err == nil {
		t.Fatal("expected error for pattern with /api prefix")
	}
}

func TestApi_PageRejectsApiPrefix(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	if err := app.Page("/api/foo", nil, func(c *PageCtx) error { return nil }); err == nil {
		t.Fatal("expected error for Page with /api prefix")
	}
	if err := app.StaticPage("/api/foo", 1, false, func(c *PageCtx) error { return nil }); err == nil {
		t.Fatal("expected error for StaticPage with /api prefix")
	}
}

func TestApi_AuthFlow(t *testing.T) {
	app, _, _, server := setupTestApp(t)
	if err := app.RegisterRole("guest", nil); err != nil {
		t.Fatalf("register guest failed: %v", err)
	}
	if err := app.RegisterRole("admin", nil); err != nil {
		t.Fatalf("register role failed: %v", err)
	}
	if err := app.Get("/secure", []string{"admin"}, func(c *ApiCtx) error {
		return c.JSON(200, fiber.Map{"ok": true})
	}); err != nil {
		t.Fatalf("register api failed: %v", err)
	}
	server.App().Post("/test-login-guest", func(ctx *fiber.Ctx) error {
		return server.GrantAuth(ctx, "guest")
	})

	// 无 cookie → 401 JSON + X-Ven-Login-Path
	resp, _ := app.Server().App().Test(httptest.NewRequest("GET", "/api/secure", nil))
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Ven-Login-Path"); got != "/login" {
		t.Fatalf("expected X-Ven-Login-Path=/login, got %q", got)
	}

	// guest 角色 → 403
	loginResp, _ := app.Server().App().Test(httptest.NewRequest("POST", "/test-login-guest", nil))
	var cookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == auth.AuthCookieName {
			cookie = c
		}
	}
	req := httptest.NewRequest("GET", "/api/secure", nil)
	req.AddCookie(cookie)
	resp2, _ := app.Server().App().Test(req)
	if resp2.StatusCode != 403 {
		t.Fatalf("expected 403 for guest, got %d", resp2.StatusCode)
	}
}

func TestApiCtx_User(t *testing.T) {
	app, _, _, server := setupTestApp(t)
	if err := app.RegisterRole("guest", nil); err != nil {
		t.Fatalf("register guest failed: %v", err)
	}
	if err := app.Get("/whoami", nil, func(c *ApiCtx) error {
		userID, role, ok := c.User()
		return c.JSON(200, fiber.Map{"userID": userID, "role": role, "ok": ok})
	}); err != nil {
		t.Fatalf("register api failed: %v", err)
	}
	server.App().Post("/test-login-user", func(ctx *fiber.Ctx) error {
		return server.GrantAuthWithUser(ctx, "guest", "u-7")
	})

	// 未登录 → ok=false
	resp, err := app.Server().App().Test(httptest.NewRequest("GET", "/api/whoami", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"ok":false`) {
		t.Fatalf("expected ok=false without cookie, got %s", body)
	}

	// GrantAuthWithUser 登录 → 取回一致的 userID/role
	cookie := loginAs(t, app, "/test-login-user")
	req := httptest.NewRequest("GET", "/api/whoami", nil)
	req.AddCookie(cookie)
	resp2, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	for _, want := range []string{`"userID":"u-7"`, `"role":"guest"`, `"ok":true`} {
		if !strings.Contains(string(body2), want) {
			t.Fatalf("expected %s in body, got %s", want, body2)
		}
	}
}

func TestApiCtx_BindAndBody(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	if err := app.Post("/echo/:name", nil, func(c *ApiCtx) error {
		var body struct {
			Value int `json:"value"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Error(400, "invalid body")
		}
		if len(c.Body()) == 0 {
			return c.Error(400, "empty raw body")
		}
		return c.JSON(201, fiber.Map{"name": c.Param("name"), "value": body.Value})
	}); err != nil {
		t.Fatalf("register api failed: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/echo/ven", strings.NewReader(`{"value":42}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"name":"ven"`) || !strings.Contains(string(body), `"value":42`) {
		t.Fatalf("unexpected body: %s", body)
	}
}
