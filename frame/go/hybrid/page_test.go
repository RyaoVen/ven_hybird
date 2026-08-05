package hybrid

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func setupTestApp(t *testing.T) (*App, *fakeSSRClient, *ssr.PendingRegistry, *httpserver.Server) {
	cfg := config.Config{
		NodeSubmitTimeout: 5 * time.Second,
		RenderTimeout:     10 * time.Second,
	}
	client := &fakeSSRClient{submitted: make(chan ssr.RenderTask, 1)}
	pending := ssr.NewPendingRegistry(10)
	patterns := pagepattern.NewValidator([]string{"/test/:id", "/ssr/:name", "/admin/:id", "/news/:id"})
	cfg.IsrDir = t.TempDir()
	cfg.IsrEnabled = true
	server := httpserver.New(cfg, client, pending, fakeHookIDs{}, patterns)
	return New(server), client, pending, server
}

func TestPage_DataOnly(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	mustPage(t, app, "/test/:id", nil, func(c *PageCtx) error {
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

func TestPageCtx_User(t *testing.T) {
	app, _, _, server := setupTestApp(t)
	if err := app.RegisterRole("guest", nil); err != nil {
		t.Fatalf("register guest failed: %v", err)
	}
	mustPage(t, app, "/test/:id", nil, func(c *PageCtx) error {
		userID, role, ok := c.User()
		return c.JSON(fiber.Map{"id": c.Param("id"), "userID": userID, "role": role, "ok": ok})
	})
	server.App().Post("/test-login-user", func(ctx *fiber.Ctx) error {
		return server.GrantAuthWithUser(ctx, "guest", "u-9")
	})

	// data-only 取数带会话 cookie → JSON 中携身份
	cookie := loginAs(t, app, "/test-login-user")
	req := httptest.NewRequest("GET", "/test/42", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	req.AddCookie(cookie)
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{`"userID":"u-9"`, `"role":"guest"`, `"ok":true`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("expected %s in body, got %s", want, body)
		}
	}

	// 无 cookie → ok=false
	req2 := httptest.NewRequest("GET", "/test/42", nil)
	req2.Header.Set("X-Ven-Data-Only", "true")
	resp2, err := app.Server().App().Test(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body2), `"ok":false`) {
		t.Fatalf("expected ok=false without cookie, got %s", body2)
	}
}

func TestPage_PublicSkipsCookieAuth(t *testing.T) {
	// 无 cookie 时公开页面仍应放行
	app, _, _, _ := setupTestApp(t)
	mustPage(t, app, "/test/:id", nil, func(c *PageCtx) error {
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
	app, _, _, _ := setupTestApp(t)
	if err := app.RegisterRole("admin", nil); err != nil {
		t.Fatalf("register role failed: %v", err)
	}
	mustPage(t, app, "/admin/:id", []string{"admin"}, func(c *PageCtx) error {
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
	if got := resp.Header.Get("X-Ven-Login-Path"); got != "/login" {
		t.Fatalf("expected X-Ven-Login-Path=/login, got %q", got)
	}
}

func TestPage_GrantAuthFlow(t *testing.T) {
	app, _, _, server := setupTestApp(t)
	if err := app.RegisterRole("admin", nil); err != nil {
		t.Fatalf("register role failed: %v", err)
	}
	mustPage(t, app, "/admin/:id", []string{"admin"}, func(c *PageCtx) error {
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
	app, client, pending, _ := setupTestApp(t)
	mustPage(t, app, "/ssr/:name", nil, func(c *PageCtx) error {
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

// resolveTasks 持续消费提交任务并按 HTML/错误回调，返回停止函数。
func resolveTasks(client *fakeSSRClient, pending *ssr.PendingRegistry, html string, renderErr *ssr.RenderError) func() {
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case task := <-client.submitted:
				pending.Resolve(ssr.RenderCallback{
					HookID:       task.HookID,
					RequestRoute: task.RequestRoute,
					HTML:         html,
					Error:        renderErr,
				})
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}

// expectNoTask 断言在窗口期内没有新的渲染任务提交。
func expectNoTask(t *testing.T, client *fakeSSRClient, window time.Duration) {
	t.Helper()
	select {
	case task := <-client.submitted:
		t.Fatalf("unexpected render task submitted: %+v", task)
	case <-time.After(window):
	}
}

func TestPage_CacheHit(t *testing.T) {
	app, client, pending, _ := setupTestApp(t)
	mustPage(t, app, "/ssr/:name", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"name": c.Param("name")})
	})
	stop := resolveTasks(client, pending, "<html>cached</html>", nil)
	defer stop()

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/ssr/world", nil)
		resp, err := app.Server().App().Test(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("request %d expected 200, got %d", i, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "<html>cached</html>" {
			t.Fatalf("request %d unexpected body: %s", i, body)
		}
	}
	// 第二次请求应命中缓存，不再回源
	expectNoTask(t, client, 200*time.Millisecond)
}

func TestPage_CacheDataVariation(t *testing.T) {
	app, client, pending, _ := setupTestApp(t)
	// data 随 query 变化 → 不同 key → 每次都回源
	mustPage(t, app, "/ssr/:name", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"q": c.Query("q")})
	})
	stop := resolveTasks(client, pending, "<html>ok</html>", nil)
	defer stop()

	for _, q := range []string{"a", "b"} {
		req := httptest.NewRequest("GET", "/ssr/world?q="+q, nil)
		resp, err := app.Server().App().Test(req)
		if err != nil {
			t.Fatalf("request q=%s failed: %v", q, err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("request q=%s expected 200, got %d", q, resp.StatusCode)
		}
	}
	// 两个不同 data 的请求各回源一次；第三个与 q=a 相同 key 的请求应命中缓存
	req := httptest.NewRequest("GET", "/ssr/world?q=a", nil)
	if _, err := app.Server().App().Test(req); err != nil {
		t.Fatalf("repeat request failed: %v", err)
	}
	expectNoTask(t, client, 200*time.Millisecond)
}

func TestPage_CacheSkips404(t *testing.T) {
	app, client, pending, _ := setupTestApp(t)
	mustPage(t, app, "/ssr/:name", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"name": c.Param("name")})
	})
	stop := resolveTasks(client, pending, "", &ssr.RenderError{Code: "PAGE_NOT_FOUND", Message: "not found"})
	defer stop()

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/ssr/world", nil)
		resp, err := app.Server().App().Test(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		if resp.StatusCode != 404 {
			t.Fatalf("request %d expected 404, got %d", i, resp.StatusCode)
		}
	}
	// 404 不缓存：两次请求都应回源；resolveTasks 已消费两个任务，此处断言没有第三个
	expectNoTask(t, client, 200*time.Millisecond)
}

func TestPage_InvalidatePage(t *testing.T) {
	app, client, pending, _ := setupTestApp(t)
	mustPage(t, app, "/ssr/:name", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"name": c.Param("name")})
	})
	stop := resolveTasks(client, pending, "<html>v1</html>", nil)
	defer stop()

	get := func() string {
		req := httptest.NewRequest("GET", "/ssr/world", nil)
		resp, err := app.Server().App().Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}

	get() // 回源并回填
	app.InvalidatePage("/ssr/world")
	get() // 失效后应重新回源（resolveTasks 再消费一个任务）
	expectNoTask(t, client, 200*time.Millisecond)
}

func TestPage_CacheSingleFlight(t *testing.T) {
	app, client, pending, _ := setupTestApp(t)
	mustPage(t, app, "/ssr/:name", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"name": c.Param("name")})
	})
	stop := resolveTasks(client, pending, "<html>flight</html>", nil)
	defer stop()

	const n = 5
	var wg sync.WaitGroup
	statuses := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/ssr/world", nil)
			resp, err := app.Server().App().Test(req)
			if err != nil {
				t.Errorf("request %d failed: %v", i, err)
				return
			}
			statuses[i] = resp.StatusCode
		}(i)
	}
	wg.Wait()
	for i, status := range statuses {
		if status != 200 {
			t.Fatalf("request %d expected 200, got %d", i, status)
		}
	}
	// 并发同 key 请求只回源一次
	expectNoTask(t, client, 200*time.Millisecond)
}

// setupGuardTestApp 注册 admin/guest 角色与受保护页面，并提供 guest 登录端点。
func setupGuardTestApp(t *testing.T) (*App, *fakeSSRClient, *ssr.PendingRegistry, *httpserver.Server) {
	t.Helper()
	app, client, pending, server := setupTestApp(t)
	if err := app.RegisterRole("guest", nil); err != nil {
		t.Fatalf("register guest failed: %v", err)
	}
	if err := app.RegisterRole("admin", nil); err != nil {
		t.Fatalf("register admin failed: %v", err)
	}
	mustPage(t, app, "/admin/:id", []string{"admin"}, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})
	server.App().Post("/test-login-guest", func(ctx *fiber.Ctx) error {
		return server.GrantAuth(ctx, "guest")
	})
	return app, client, pending, server
}

// loginAs 通过测试登录端点获取 ven_auth cookie。
func loginAs(t *testing.T, app *App, loginPath string) *http.Cookie {
	t.Helper()
	resp, err := app.Server().App().Test(httptest.NewRequest("POST", loginPath, nil))
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == auth.AuthCookieName {
			return cookie
		}
	}
	t.Fatal("no ven_auth cookie from login")
	return nil
}

func TestPage_UnauthenticatedRedirect(t *testing.T) {
	app, _, _, _ := setupGuardTestApp(t)

	req := httptest.NewRequest("GET", "/admin/42?tab=a", nil)
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 302 {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, "/login?next=") {
		t.Fatalf("expected redirect to /login, got %q", location)
	}
	if !strings.Contains(location, "next=%2Fadmin%2F42%3Ftab%3Da") {
		t.Fatalf("expected next to contain escaped original url, got %q", location)
	}
}

func TestPage_UnauthenticatedRedirectCustomPath(t *testing.T) {
	app, _, _, _ := setupGuardTestApp(t)
	app.SetLoginRedirect("/auth/signin")

	req := httptest.NewRequest("GET", "/admin/42", nil)
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, "/auth/signin?next=") {
		t.Fatalf("expected redirect to custom path, got %q", location)
	}
}

// 回归 #57：author 继承 reader 后，reader 不得访问声明 ["author"] 的页面（守卫穿透）。
func TestPage_InheritedRoleGuard(t *testing.T) {
	app, client, pending, server := setupTestApp(t)
	if err := app.RegisterRole("reader", nil); err != nil {
		t.Fatalf("register reader failed: %v", err)
	}
	if err := app.RegisterRole("author", []string{"reader"}); err != nil {
		t.Fatalf("register author failed: %v", err)
	}
	mustPage(t, app, "/admin/:id", []string{"author"}, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})
	server.App().Post("/test-login-reader", func(ctx *fiber.Ctx) error {
		return server.GrantAuth(ctx, "reader")
	})
	server.App().Post("/test-login-author", func(ctx *fiber.Ctx) error {
		return server.GrantAuth(ctx, "author")
	})

	// reader（父角色）访问 author 页面：403 原地错误页，不得 200 直出
	readerCookie := loginAs(t, app, "/test-login-reader")
	stop := resolveTasks(client, pending, "<html>forbidden</html>", nil)
	req := httptest.NewRequest("GET", "/admin/42", nil)
	req.AddCookie(readerCookie)
	resp, err := app.Server().App().Test(req)
	stop()
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 403 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("reader must be forbidden on author page, got %d %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	// author（声明角色）访问：200 正常渲染
	authorCookie := loginAs(t, app, "/test-login-author")
	stop2 := resolveTasks(client, pending, "<html>write</html>", nil)
	defer stop2()
	req2 := httptest.NewRequest("GET", "/admin/42", nil)
	req2.AddCookie(authorCookie)
	resp2, err := app.Server().App().Test(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("author should access author page, got %d", resp2.StatusCode)
	}
}

func TestPage_ForbiddenRenders403Page(t *testing.T) {
	app, client, pending, _ := setupGuardTestApp(t)
	cookie := loginAs(t, app, "/test-login-guest")

	stop := resolveTasks(client, pending, "<html>forbidden</html>", nil)
	defer stop()

	req := httptest.NewRequest("GET", "/admin/42", nil)
	req.AddCookie(cookie)
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<html>forbidden</html>" {
		t.Fatalf("expected 403 page html, got %s", body)
	}
}

func TestPage_ForbiddenRenderRoute(t *testing.T) {
	app, client, pending, _ := setupGuardTestApp(t)
	cookie := loginAs(t, app, "/test-login-guest")

	go func() {
		task := <-client.submitted
		if task.Payload.Route != "/403" {
			t.Errorf("expected render route /403, got %q", task.Payload.Route)
		}
		pending.Resolve(ssr.RenderCallback{
			HookID:       task.HookID,
			RequestRoute: task.RequestRoute,
			HTML:         "<html>forbidden</html>",
		})
	}()

	req := httptest.NewRequest("GET", "/admin/42", nil)
	req.AddCookie(cookie)
	if _, err := app.Server().App().Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
}

func TestPage_ForbiddenDataOnly(t *testing.T) {
	app, client, _, _ := setupGuardTestApp(t)
	cookie := loginAs(t, app, "/test-login-guest")

	req := httptest.NewRequest("GET", "/admin/42", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	req.AddCookie(cookie)
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("expected JSON body, got %s", ct)
	}
	// data-only 拒绝不触发 SSR
	expectNoTask(t, client, 200*time.Millisecond)
}

// mustPage 注册页面并在失败时终止测试（适配 Page 的 error 返回）。
func mustPage(t *testing.T, app *App, pattern string, roles []string, h PageHandler) {
	t.Helper()
	if err := app.Page(pattern, roles, h); err != nil {
		t.Fatalf("register page %q failed: %v", pattern, err)
	}
}

func TestPage_InvalidPatternReturnsError(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	// "/not-registered" 不在测试 validator 的 pattern 列表里
	if err := app.Page("/not-registered", nil, func(c *PageCtx) error { return nil }); err == nil {
		t.Fatal("expected error for unregistered pattern, got nil")
	}
}

func TestPage_HeadRequest(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	mustPage(t, app, "/test/:id", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})

	req := httptest.NewRequest("HEAD", "/test/42", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("HEAD request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for HEAD, got %d", resp.StatusCode)
	}
}

// TestApp_SetVisitRecorder 验证 hybrid.App 公开埋点注册：GET 页面请求计数，
// data-only 取数跳过（避免 SPA 导航双埋点）。
func TestApp_SetVisitRecorder(t *testing.T) {
	app, client, pending, _ := setupTestApp(t)
	var got []string
	app.SetVisitRecorder(func(path string) { got = append(got, path) })
	mustPage(t, app, "/test/:id", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})

	// 正常页面请求（SSR 导航）：计数
	go func() {
		task := <-client.submitted
		pending.Resolve(ssr.RenderCallback{
			HookID:       task.HookID,
			RequestRoute: task.RequestRoute,
			MatchedRoute: "/test/:id",
			HTML:         "<html>test</html>",
		})
	}()
	req := httptest.NewRequest("GET", "/test/42", nil)
	resp, err := app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if len(got) != 1 || got[0] != "/test/42" {
		t.Fatalf("expected recorded [/test/42], got %v", got)
	}

	// data-only 取数（SPA 导航）：不重复计数（不提交 SSR 任务，直接 JSON 返回）
	req = httptest.NewRequest("GET", "/test/43", nil)
	req.Header.Set("X-Ven-Data-Only", "true")
	resp, err = app.Server().App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if len(got) != 1 {
		t.Fatalf("data-only should not be counted, got %v", got)
	}
}
