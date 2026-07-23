// VenHybird Go 网关入口，负责 HTTP 请求接收和 SSR 任务调度。
package main

import (
	"context"
	"log"

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
	// Node 是页面路由权威，Go 用它校验页面注册
	patterns, err := pagepattern.Fetch(context.Background(), cfg.NodeWorkerURL, cfg.InternalToken, cfg.NodeSubmitTimeout)
	if err != nil {
		log.Fatal(err)
	}

	// 步骤 4: 创建 HTTP 服务器并注册内部路由（渲染回调、健康检查、静态资源等）
	server := httpserver.New(cfg, client, pending, ssr.CryptoHookIDGenerator{}, patterns)
	server.RegisterInternalRoutes()

	// 步骤 5: 创建 hybrid 应用并注册业务页面
	app := hybrid.New(server)
	if err := build.Register(app); err != nil {
		log.Fatal(err)
	}

	// 步骤 6: 注册页面兜底路由
	// 必须最后注册，fiber 按注册顺序匹配，否则业务页面会被兜底抢先
	server.RegisterPageFallback()

	// 步骤 7: 启动 HTTP 服务器，开始监听指定地址
	log.Printf("VenHybird Go gateway listening on %s", cfg.ListenAddr)
	if err := server.App().Listen(cfg.ListenAddr); err != nil {
		log.Fatal(err)
	}
}
