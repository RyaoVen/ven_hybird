// VenHybird Go 网关入口，负责 HTTP 请求接收和 SSR 任务调度。
package main

import (
	"log"

	"ven_hybird/internal/config"
	"ven_hybird/internal/httpserver"
	"ven_hybird/internal/ssr"
)

// main 初始化各组件并启动 HTTP 服务器。
func main() {
	// 步骤 1: 从环境变量加载配置，包含所有运行时参数
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// 步骤 2: 创建 Node.js SSR 工作节点的 HTTP 客户端
	// 用于向 Node 端提交渲染任务
	client := ssr.NewNodeClient(cfg.NodeWorkerURL, cfg.NodeSubmitTimeout, cfg.InternalToken)

	// 步骤 3: 创建 pending 任务注册中心
	// 管理所有等待 Node 端渲染回调的异步任务
	pending := ssr.NewPendingRegistry(cfg.MaxPendingRenders)

	// 步骤 4: 创建 HTTP 服务器，注入所有依赖组件
	// 使用 CryptoHookIDGenerator 生成加密安全的唯一请求标识
	server := httpserver.New(cfg, client, pending, ssr.CryptoHookIDGenerator{})

	// 步骤 5: 注册所有 HTTP 路由
	server.RegisterRoutes()

	// 步骤 6: 启动 HTTP 服务器，开始监听指定地址
	log.Printf("VenHybird Go gateway listening on %s", cfg.ListenAddr)
	if err := server.App().Listen(cfg.ListenAddr); err != nil {
		log.Fatal(err)
	}
}
