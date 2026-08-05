// VenHybird Go 网关入口，负责 HTTP 请求接收和 SSR 任务调度。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ven_hybird/build"
	"ven_hybird/hybrid"
	"ven_hybird/internal/config"
	"ven_hybird/internal/httpserver"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"
)

// main 初始化各组件并启动 HTTP 服务器。
func main() {
	// 步骤 1: 从环境变量加载配置，包含所有运行时参数
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// 步骤 2: 创建 Node.js SSR 工作节点的 HTTP 客户端与 pending 任务注册中心
	client := ssr.NewNodeClient(cfg.NodeWorkerURL, cfg.NodeSubmitTimeout, cfg.InternalToken)
	pending := ssr.NewPendingRegistry(cfg.MaxPendingRenders)

	// 步骤 3: 从 Node 端拉取全部页面路由模式（nodePagesPattern）
	// Node 是页面路由权威，Go 用它校验页面注册；失败重试 3 次退避。
	// 高可用：拉取失败时回退最近一次持久化的 pattern 启动（Node 恢复后 refetchPatterns 自动换新）；
	// 首启无持久化副本才失败退出——没有路由表无法服务。
	patterns, fallback, err := loadPatterns(cfg, 3)
	if err != nil {
		log.Fatalf("Node 未就绪: %v（首启无持久化页面模式，无法提供服务；Node worker 启动后重试）", err)
	}
	if fallback {
		log.Printf("Node 未就绪，使用持久化页面模式启动（%s），Node 恢复后自动重拉", cfg.PatternsFile)
	}

	// 步骤 4: 创建 HTTP 服务器并注册内部路由（渲染回调、健康检查、静态资源等）
	server := httpserver.New(cfg, client, pending, ssr.CryptoHookIDGenerator{}, patterns)
	server.RegisterInternalRoutes()

	// 步骤 5: 创建 hybrid 应用并注册业务页面
	app := hybrid.New(server)
	if err := build.Register(app); err != nil {
		log.Fatal(err)
	}

	// 步骤 6: 注册退出信号处理，收到 SIGINT/SIGTERM 时优雅关停
	// （排空间进行中的请求，pending 渲染随超时自然结束）
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-shutdown
		log.Printf("shutdown signal received, draining...")
		app.Close() // 先 drain SSE 连接（EventSource 客户端自动重连到存活实例）
		if err := server.App().ShutdownWithTimeout(5 * time.Second); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	// 步骤 7: 启动 HTTP 服务器（app.Listen 内部先注册页面兜底路由）
	log.Printf("VenHybird Go gateway listening on %s", cfg.ListenAddr)
	if err := app.Listen(cfg.ListenAddr); err != nil {
		log.Fatal(err)
	}
}

// fetchPatternsWithRetry 拉取页面路由模式，失败按次数退避重试。
func fetchPatternsWithRetry(cfg config.Config, attempts int) (*pagepattern.Validator, error) {
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		var patterns *pagepattern.Validator
		patterns, err = pagepattern.Fetch(context.Background(), cfg.NodeWorkerURL, cfg.InternalToken, cfg.NodeSubmitTimeout)
		if err == nil {
			return patterns, nil
		}
		log.Printf("fetch page patterns attempt %d/%d failed: %v", attempt, attempts, err)
		if attempt < attempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return nil, err
}

// loadPatterns 拉取 Node 页面模式；失败时回退最近一次成功拉取的持久化副本。
// 返回 (validator, fallbackUsed, error)：error 表示 Node 不可达且无持久化副本
// （首启场景，无法服务，由调用方失败退出）。
func loadPatterns(cfg config.Config, attempts int) (*pagepattern.Validator, bool, error) {
	patterns, err := fetchPatternsWithRetry(cfg, attempts)
	if err == nil {
		// 持久化最近一次成功拉取的 pattern（best-effort，写失败不影响启动）
		if perr := pagepattern.Save(patterns, cfg.PatternsFile); perr != nil {
			log.Printf("persist page patterns failed: %v", perr)
		}
		return patterns, false, nil
	}
	persisted, perr := pagepattern.Load(cfg.PatternsFile)
	if perr != nil {
		return nil, false, fmt.Errorf("node not ready and no persisted page patterns (%s): %v", cfg.PatternsFile, err)
	}
	return persisted, true, nil
}
