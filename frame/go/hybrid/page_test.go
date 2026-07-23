package hybrid

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

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
	patterns := pagepattern.NewValidator([]string{"/test/:id", "/ssr/:name"})
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
