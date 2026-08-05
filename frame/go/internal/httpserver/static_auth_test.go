// StaticPage 角色鉴权测试：ISR 物化直发路径与 miss 回源路径都受 roles 保护。
package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ven_hybird/internal/auth"
	"ven_hybird/internal/config"
	"ven_hybird/internal/isr"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

// staticAuthTestServer 起最小 Server：注册 editor 角色、挂登录端点、
// 登记一个带 roles 的 StaticPage 声明并注册内部路由（staticAuthMiddleware + ISR 直发中间件）。
func staticAuthTestServer(t *testing.T) (*Server, func(t *testing.T) *http.Cookie) {
	t.Helper()
	cfg := config.Config{NodeSubmitTimeout: time.Second, RenderTimeout: 2 * time.Second}
	cfg.IsrDir = t.TempDir()
	cfg.IsrEnabled = true
	s := New(cfg, stubClient{}, ssr.NewPendingRegistry(4), stubHookIDs{}, pagepattern.NewValidator([]string{"/news/:id"}))
	if err := s.RegisterRole("editor", nil); err != nil {
		t.Fatalf("register editor failed: %v", err)
	}
	decl, err := isr.ParseDeclaration("/news/:id", 10, false)
	if err != nil {
		t.Fatalf("parse declaration failed: %v", err)
	}
	if err := s.RegisterStaticPage(decl); err != nil {
		t.Fatalf("register static page failed: %v", err)
	}
	levels, err := s.ResolveRoles([]string{"editor"})
	if err != nil {
		t.Fatalf("resolve editor roles failed: %v", err)
	}
	s.RegisterStaticPageAuth("/news/:id", levels)
	s.RegisterInternalRoutes()

	// 登录端点：GrantAuthWithUser 下发 ven_auth cookie
	s.App().Post("/login", func(ctx *fiber.Ctx) error { return s.GrantAuthWithUser(ctx, "editor", "u1") })
	return s, func(t *testing.T) *http.Cookie {
		t.Helper()
		resp, err := s.App().Test(httptest.NewRequest("POST", "/login", nil))
		if err != nil {
			t.Fatalf("login failed: %v", err)
		}
		for _, c := range resp.Cookies() {
			if c.Name == auth.AuthCookieName {
				return c
			}
		}
		t.Fatal("no ven_auth cookie from login")
		return nil
	}
}

func TestStaticAuth_UnauthenticatedHTMLRedirect(t *testing.T) {
	s, _ := staticAuthTestServer(t)
	// 未登录 HTML 导航：302 跳登录
	req := httptest.NewRequest("GET", "/news/1", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); len(loc) < 6 || loc[:6] != "/login" {
		t.Fatalf("expected redirect to /login, got %q", loc)
	}
}

func TestStaticAuth_UnauthenticatedDataOnlyJSON(t *testing.T) {
	s, _ := staticAuthTestServer(t)
	// 未登录 data-only 取数：JSON 401（SPA 路由据此跳登录）
	req := httptest.NewRequest("GET", "/news/1", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("expected json response, got %q", resp.Header.Get("Content-Type"))
	}
}

func TestStaticAuth_AuthenticatedPasses(t *testing.T) {
	s, login := staticAuthTestServer(t)
	cookie := login(t)

	// 登录后 HTML 导航：通过鉴权中间件（无物化文件时放行到下游，此处无路由返回 404 但非 302/401/403）
	req := httptest.NewRequest("GET", "/news/1", nil)
	req.AddCookie(cookie)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode == fiber.StatusFound || resp.StatusCode == fiber.StatusUnauthorized || resp.StatusCode == fiber.StatusForbidden {
		t.Fatalf("authenticated request should pass auth, got %d", resp.StatusCode)
	}

	// 登录后 data-only：同样放行
	req = httptest.NewRequest("GET", "/news/1", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	req.AddCookie(cookie)
	resp, err = s.App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode == fiber.StatusUnauthorized || resp.StatusCode == fiber.StatusForbidden {
		t.Fatalf("authenticated data-only should pass auth, got %d", resp.StatusCode)
	}
}

func TestStaticAuth_WrongRoleForbidden(t *testing.T) {
	s, _ := staticAuthTestServer(t)
	// 注册 guest 角色并登录为 guest（不满足 editor 要求）
	if err := s.RegisterRole("guest", nil); err != nil {
		t.Fatalf("register guest failed: %v", err)
	}
	s.App().Post("/login-guest", func(ctx *fiber.Ctx) error { return s.GrantAuthWithUser(ctx, "guest", "u2") })
	loginResp, err := s.App().Test(httptest.NewRequest("POST", "/login-guest", nil))
	if err != nil {
		t.Fatalf("guest login failed: %v", err)
	}
	var cookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == auth.AuthCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no ven_auth cookie from guest login")
	}

	req := httptest.NewRequest("GET", "/news/1", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	req.AddCookie(cookie)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected 403 for wrong role, got %d", resp.StatusCode)
	}
}
