package hybrid

import (
	"io"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ven_hybird/internal/httpserver"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

// mustStaticPage 注册静态页并在失败时终止测试。
func mustStaticPage(t *testing.T, app *App, pattern string, maxPages int, smartLoad bool, h PageHandler) {
	t.Helper()
	if err := app.StaticPage(pattern, maxPages, smartLoad, h); err != nil {
		t.Fatalf("register static page %q failed: %v", pattern, err)
	}
}

// countingResolver 持续消费渲染任务并回调固定 HTML，返回任务计数与停止函数。
func countingResolver(client *fakeSSRClient, pending *ssr.PendingRegistry, html string) (*atomic.Int64, func()) {
	count := &atomic.Int64{}
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case task := <-client.submitted:
				count.Add(1)
				pending.Resolve(ssr.RenderCallback{
					HookID:       task.HookID,
					RequestRoute: task.RequestRoute,
					HTML:         html,
				})
			case <-stop:
				return
			}
		}
	}()
	return count, func() { close(stop) }
}

// waitTaskCount 在期限内等待任务计数达到目标值。
func waitTaskCount(t *testing.T, count *atomic.Int64, target int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if count.Load() >= target {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d render tasks within %s, got %d", target, timeout, count.Load())
}

// shortDebounce 把事件总线的静默窗口缩到测试量级（DataChange 失效是异步的）。
func shortDebounce(app *App) {
	app.bus.QuietWindow = 30 * time.Millisecond
	app.bus.MaxWait = 200 * time.Millisecond
}

// waitFileGone 在期限内等待物化文件被删除（事件总线 ① 生效）。
func waitFileGone(t *testing.T, server *httpserver.Server, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !server.StaticFileExists(path) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %s deleted within %s", path, timeout)
}

func getPage(t *testing.T, app *App, path string) (int, string) {
	t.Helper()
	resp, err := app.Server().App().Test(httptest.NewRequest("GET", path, nil))
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestStaticPage_ServeFromFile(t *testing.T) {
	app, client, pending, _ := setupTestApp(t)
	mustStaticPage(t, app, "/news/:id", 10, false, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})
	count, stop := countingResolver(client, pending, "<html>news</html>")
	defer stop()

	if code, _ := getPage(t, app, "/news/1"); code != 200 {
		t.Fatalf("first visit expected 200, got %d", code)
	}
	waitTaskCount(t, count, 1, 2*time.Second)
	// 二次访问应由 ISR 中间件直发，不再回源
	if code, body := getPage(t, app, "/news/1"); code != 200 || body != "<html>news</html>" {
		t.Fatalf("second visit expected 200 served html, got %d %q", code, body)
	}
	if got := count.Load(); got != 1 {
		t.Fatalf("expected 1 render task after two visits, got %d", got)
	}
}

func TestStaticPage_DataChangeLocal(t *testing.T) {
	app, client, pending, server := setupTestApp(t)
	shortDebounce(app)
	mustStaticPage(t, app, "/news/:id", 10, false, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})
	count, stop := countingResolver(client, pending, "<html>news</html>")
	defer stop()

	getPage(t, app, "/news/1")
	getPage(t, app, "/news/2")
	waitTaskCount(t, count, 2, 2*time.Second)

	// 局部失效：仅 /news/1；失效是异步的，等静默窗口后文件被删
	if err := app.DataChange("/news/:id", "1"); err != nil {
		t.Fatalf("datachange failed: %v", err)
	}
	waitFileGone(t, server, "/news/1", 2*time.Second)
	getPage(t, app, "/news/1") // 重新回源
	waitTaskCount(t, count, 3, 2*time.Second)
	// /news/2 文件未动，直发不回源
	getPage(t, app, "/news/2")
	if got := count.Load(); got != 3 {
		t.Fatalf("expected /news/2 served from file (3 tasks), got %d", got)
	}
}

func TestStaticPage_LRU(t *testing.T) {
	app, client, pending, server := setupTestApp(t)
	mustStaticPage(t, app, "/news/:id", 2, false, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})
	count, stop := countingResolver(client, pending, "<html>news</html>")
	defer stop()

	getPage(t, app, "/news/1")
	time.Sleep(5 * time.Millisecond)
	getPage(t, app, "/news/2")
	time.Sleep(5 * time.Millisecond)
	getPage(t, app, "/news/3") // 触发 LRU：淘汰最久未访问的 /news/1
	waitTaskCount(t, count, 3, 2*time.Second)

	if server.StaticFileExists("/news/1") {
		t.Fatal("expected /news/1 evicted by LRU")
	}
	if !server.StaticFileExists("/news/2") || !server.StaticFileExists("/news/3") {
		t.Fatal("expected /news/2 and /news/3 to survive LRU")
	}
}

func TestStaticPage_SmartPrerender(t *testing.T) {
	app, client, pending, _ := setupTestApp(t)
	shortDebounce(app)
	mustStaticPage(t, app, "/news/:id", 1, true, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})
	count, stop := countingResolver(client, pending, "<html>news</html>")
	defer stop()

	// /news/1 访问两次（最热），/news/2 一次
	getPage(t, app, "/news/1")
	getPage(t, app, "/news/1")
	getPage(t, app, "/news/2")
	waitTaskCount(t, count, 2, 2*time.Second)

	// 全局更新：静默窗口后失效，总线 ② 按热门预渲染 Top-1（/news/1）
	if err := app.DataChange("/news/:id"); err != nil {
		t.Fatalf("datachange failed: %v", err)
	}
	// 预渲染在总线后台执行：/news/1 重渲染一次
	waitTaskCount(t, count, 3, 3*time.Second)

	// /news/1 已被预渲染落盘，直发不回源
	getPage(t, app, "/news/1")
	// /news/2 未预渲染，懒惰回源
	getPage(t, app, "/news/2")
	waitTaskCount(t, count, 4, 2*time.Second)
}

func TestStaticPage_DataChangeUndeclared(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	if err := app.DataChange("/not-declared"); err == nil {
		t.Fatal("expected error for undeclared pattern")
	}
}

func TestStaticPage_DataChangeAsync(t *testing.T) {
	app, client, pending, server := setupTestApp(t)
	mustStaticPage(t, app, "/news/:id", 10, false, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})
	count, stop := countingResolver(client, pending, "<html>news</html>")
	defer stop()
	app.bus.QuietWindow = 200 * time.Millisecond // 拉长窗口以观察"即时返回、稍后生效"

	getPage(t, app, "/news/1")
	waitTaskCount(t, count, 1, 2*time.Second)

	// DataChange 即时返回：校验后仅入队，不做删除
	start := time.Now()
	if err := app.DataChange("/news/:id", "1"); err != nil {
		t.Fatalf("datachange failed: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("DataChange should return immediately, took %s", elapsed)
	}
	// 静默窗口未过，物化文件仍在（秒级一致性窗口）
	if !server.StaticFileExists("/news/1") {
		t.Fatal("file should survive until quiet window elapses")
	}
	// 静默窗口后失效生效
	waitFileGone(t, server, "/news/1", 2*time.Second)
}
