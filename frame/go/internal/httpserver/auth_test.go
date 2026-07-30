package httpserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ven_hybird/internal/auth"
	"ven_hybird/internal/config"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

// newAuthTestServer 起最小 Server 并注册 guest 角色，挂载登录/身份端点。
func newAuthTestServer(t *testing.T, grant func(s *Server, ctx *fiber.Ctx) error) *Server {
	t.Helper()
	cfg := config.Config{NodeSubmitTimeout: time.Second, RenderTimeout: time.Second}
	s := New(cfg, stubClient{}, ssr.NewPendingRegistry(4), stubHookIDs{}, pagepattern.NewValidator(nil))
	if err := s.RegisterRole("guest", nil); err != nil {
		t.Fatalf("register guest failed: %v", err)
	}
	s.App().Post("/login", func(ctx *fiber.Ctx) error { return grant(s, ctx) })
	s.App().Get("/whoami", func(ctx *fiber.Ctx) error {
		userID, role, ok := s.CurrentUser(ctx)
		return ctx.JSON(fiber.Map{"userID": userID, "role": role, "ok": ok})
	})
	return s
}

// loginAndWhoami 登录后带 ven_auth cookie 请求 /whoami，返回响应体。
func loginAndWhoami(t *testing.T, s *Server) string {
	t.Helper()
	loginResp, err := s.App().Test(httptest.NewRequest("POST", "/login", nil))
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	var cookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == auth.AuthCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no ven_auth cookie from login")
	}
	req := httptest.NewRequest("GET", "/whoami", nil)
	req.AddCookie(cookie)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("whoami failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func TestServer_GrantAuthWithUser_CurrentUser(t *testing.T) {
	s := newAuthTestServer(t, func(s *Server, ctx *fiber.Ctx) error {
		return s.GrantAuthWithUser(ctx, "guest", "u-100")
	})
	body := loginAndWhoami(t, s)
	for _, want := range []string{`"userID":"u-100"`, `"role":"guest"`, `"ok":true`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in body, got %s", want, body)
		}
	}
}

// 旧 GrantAuth 路径：会话不带用户身份，CurrentUser 的 userID 为空。
func TestServer_GrantAuth_LegacyEmptyUser(t *testing.T) {
	s := newAuthTestServer(t, func(s *Server, ctx *fiber.Ctx) error {
		return s.GrantAuth(ctx, "guest")
	})
	body := loginAndWhoami(t, s)
	for _, want := range []string{`"userID":""`, `"role":"guest"`, `"ok":true`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in body, got %s", want, body)
		}
	}
}

func TestServer_CurrentUser_Invalid(t *testing.T) {
	s := newAuthTestServer(t, func(s *Server, ctx *fiber.Ctx) error {
		return s.GrantAuthWithUser(ctx, "guest", "u-100")
	})

	// 无 cookie → ok=false
	resp, err := s.App().Test(httptest.NewRequest("GET", "/whoami", nil))
	if err != nil {
		t.Fatalf("whoami failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"ok":false`) {
		t.Fatalf("expected ok=false without cookie, got %s", body)
	}

	// 伪造令牌 → ok=false
	req := httptest.NewRequest("GET", "/whoami", nil)
	req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: "forged-token"})
	resp2, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("whoami failed: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body2), `"ok":false`) {
		t.Fatalf("expected ok=false for forged token, got %s", body2)
	}
}
