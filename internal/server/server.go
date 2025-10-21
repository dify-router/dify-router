package server

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/dify-router/dify-router/internal/gateway"
	"github.com/dify-router/dify-router/internal/static"
	"github.com/dify-router/dify-router/internal/utils/log"
)

func initConfig() {
	// 初始化配置
	err := static.InitConfig("conf/config.yaml")
	if err != nil {
		log.Panic("failed to init config: %v", err)
	}
	log.Info("config init success")
}

func initGatewayServer() {
	config := static.GetDifySandboxGlobalConfigurations()
	
	// 设置Gin模式
	if !config.App.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建分布式路由器，传入 Redis 地址和密码
	router := gateway.NewDistributedRouter(config.Redis.Addr, config.Redis.Password)
	
	// 设置负载均衡策略
	router.SetLoadBalancerStrategy(config.Gateway.LoadBalancerStrategy)
	
	// 设置端口
	router.SetPorts(config.Gateway.Port, config.App.Port)

	// 启动网关服务器
	// 启动网关服务器
	gatewayAddr := fmt.Sprintf(":%d", config.Gateway.Port)
	adminAddr := fmt.Sprintf(":%d", config.App.Port)
	log.Info("Starting gateway server on " + gatewayAddr)
	log.Info("Starting admin API on " + adminAddr)
	log.Info("Load balancer strategy: %s", config.Gateway.LoadBalancerStrategy)
	log.Info("Health check interval: %d seconds", config.Gateway.HealthCheckInterval)
		// 调试：打印 Redis 配置详情
	log.Info("Redis Config - Addr: %s, Password: '%s', DB: %d", 
		config.Redis.Addr, 
		config.Redis.Password, 
		config.Redis.DB)

	if err := router.Run(gatewayAddr); err != nil {
		log.Panic("Failed to start gateway server: %v", err)
	}
}

func Run() {
	// 初始化配置
	initConfig()
	
	// 启动网关服务器
	initGatewayServer()
}
