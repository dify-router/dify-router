package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
	"github.com/dify-router/dify-router/internal/middleware"
	"github.com/dify-router/dify-router/internal/static"
)

// 动态路由器
type DistributedRouter struct {
	redisClient    *redis.Client
	ginRouter      *gin.Engine
	muxRouter      *mux.Router
	routeManager   *RouteManager
	sandboxPool    *SandboxPool
	loadBalancer   *LoadBalancer
	gatewayPort    int
	managementPort int
}

func NewDistributedRouter(redisAddr, redisPassword string) *DistributedRouter {
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       0,
	})

	// 测试 Redis 连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		if err.Error() == "NOAUTH Authentication required." {
			log.Printf("❌ Redis authentication failed. Please check your Redis password in config.yaml")
			log.Printf("💡 You can:")
			log.Printf("   1. Set the correct password in conf/config.yaml")
			log.Printf("   2. Disable Redis authentication: redis-cli -> CONFIG SET requirepass \"\"")
			log.Printf("   3. Or run without Redis (routes will be stored in memory only)")
		} else {
			log.Printf("❌ Failed to connect to Redis at %s: %v", redisAddr, err)
		}
		// 继续运行，但使用内存存储
		log.Printf("⚠️  Running with in-memory storage only. Routes will not be persisted.")
	} else {
		log.Printf("✅ Successfully connected to Redis at %s", redisAddr)
	}

	router := &DistributedRouter{
		redisClient:    rdb,
		ginRouter:      gin.New(),
		muxRouter:      mux.NewRouter(),
		routeManager:   NewRouteManager(rdb),
		sandboxPool:    NewSandboxPool(rdb),
		loadBalancer:   NewLoadBalancer(),
		gatewayPort:    8080,
		managementPort: 8081,
	}

	router.setupRoutes()
	return router
}

func (dr *DistributedRouter) SetLoadBalancerStrategy(strategy string) {
	dr.loadBalancer.SetStrategy(strategy)
}

func (dr *DistributedRouter) SetPorts(gatewayPort, managementPort int) {
	dr.gatewayPort = gatewayPort
	dr.managementPort = managementPort
}

func (dr *DistributedRouter) setupRoutes() {
	// 设置Gin路由（用于管理API）
	dr.setupGinRoutes()
	
	// 设置Mux路由（用于动态路由）
	dr.setupMuxRoutes()
}

func (dr *DistributedRouter) setupGinRoutes() {
	dr.ginRouter.Use(gin.Recovery())
	dr.ginRouter.Use(dr.corsMiddleware())
	dr.ginRouter.Use(gin.Logger())

	// 管理接口 - 添加管理员认证
	adminGroup := dr.ginRouter.Group("/admin")
	adminGroup.Use(middleware.AdminAuth())
	{
		adminGroup.GET("/routes", dr.listRoutesHandler)
		adminGroup.POST("/routes", dr.addRouteHandler)
		adminGroup.PUT("/routes/:id", dr.updateRouteHandler)
		adminGroup.DELETE("/routes/:id", dr.deleteRouteHandler)
		adminGroup.GET("/sandboxes", dr.listSandboxesHandler)
		adminGroup.POST("/sandboxes/register", dr.registerSandboxHandler)
		adminGroup.DELETE("/sandboxes/:id", dr.deleteSandboxHandler)
		adminGroup.GET("/health", dr.healthHandler)

		// 事件流管理接口
		adminGroup.GET("/events/stream-info", dr.getStreamInfoHandler)
		adminGroup.GET("/events/pending", dr.getPendingMessagesHandler)
		adminGroup.POST("/events/test", dr.publishTestEventHandler)
		adminGroup.GET("/events/consumers", dr.getEventConsumersHandler)

		// 其他管理接口
		adminGroup.GET("/config/version", dr.getConfigVersionHandler)
		adminGroup.GET("/events/stats", dr.getEventStatsHandler)
		adminGroup.POST("/sync/trigger", dr.triggerSyncHandler)
		adminGroup.GET("/routes/:routeId/details", dr.getRouteDetailsHandler)
		adminGroup.POST("/events/cleanup", dr.cleanupEventsHandler)
	}
}

func (dr *DistributedRouter) setupMuxRoutes() {
	// 使用Mux处理所有动态路由，添加业务认证
	dr.muxRouter.PathPrefix("/").HandlerFunc(dr.authenticatedRouteHandler)
}

// 认证路由处理器
func (dr *DistributedRouter) authenticatedRouteHandler(w http.ResponseWriter, r *http.Request) {
	// 检查业务网关认证
	if !dr.authenticateGatewayRequest(r) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(gin.H{"error": "invalid gateway api key"})
		return
	}
	
	// 认证通过，继续处理路由
	dr.dynamicRouteHandler(w, r)
}

// 网关认证检查
func (dr *DistributedRouter) authenticateGatewayRequest(r *http.Request) bool {
	apiKey := r.Header.Get("X-Api-Key")
	config := static.GetDifySandboxGlobalConfigurations()
	
	// 使用网关密钥进行认证
	expectedKey := config.App.GatewayKey
	if expectedKey == "" {
		expectedKey = config.App.Key // 兼容旧配置
	}
	
	return expectedKey != "" && expectedKey == apiKey
}

func (dr *DistributedRouter) dynamicRouteHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	method := r.Method

	// 查找匹配的路由
	route := dr.routeManager.matchRoute(path, method)
	if route == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(gin.H{"error": "route not found"})
		return
	}

	// 根据处理器类型路由
	switch route.Handler {
	case "sandbox":
		dr.handleSandboxRequest(route, w, r)
	case "proxy":
		dr.handleProxyRequest(route, w, r)
	case "static":
		dr.handleStaticRequest(route, w, r)
	default:
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(gin.H{"error": "unknown handler type"})
	}
}

func (dr *DistributedRouter) handleSandboxRequest(route *RouteConfig, w http.ResponseWriter, r *http.Request) {
	// 获取健康的沙箱实例
	instance, err := dr.sandboxPool.GetHealthyInstance(route.SandboxType)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(gin.H{"error": err.Error()})
		return
	}

	// 构建符合沙箱期望的请求格式
	executionReq := map[string]interface{}{
		"language":       "python3",
		"code":           route.Code,
		"preload":        "",
		"enable_network": true,
		"timeout":        route.Timeout,
	}

	// 转发到沙箱执行，传递原始请求
	dr.forwardToSandbox(instance, executionReq, w, r)
}

func (dr *DistributedRouter) forwardToSandbox(instance *SandboxInstance, reqData map[string]interface{}, w http.ResponseWriter, r *http.Request) {
	timeout := 30 * time.Second
	if to, ok := reqData["timeout"].(int); ok {
		timeout = time.Duration(to) * time.Second
	}

	client := &http.Client{Timeout: timeout}

	reqJSON, _ := json.Marshal(reqData)
	
	req, err := http.NewRequest("POST", instance.URL+"/run", bytes.NewBuffer(reqJSON))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(gin.H{"error": err.Error()})
		return
	}

	req.Header.Set("Content-Type", "application/json")
	
	// 关键修改：使用客户端传递的 API Key，如果不存在则使用配置的默认值
	apiKey := r.Header.Get("X-Api-Key")
	if apiKey == "" {
		// 如果没有传递 API Key，使用配置的默认值
		config := static.GetDifySandboxGlobalConfigurations()
		apiKey = config.App.GatewayKey
		if apiKey == "" {
			apiKey = "xai-sandbox" // 最终回退
		}
	}
	req.Header.Set("X-Api-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(gin.H{"error": "sandbox unavailable: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// 流式传输响应
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (dr *DistributedRouter) handleProxyRequest(route *RouteConfig, w http.ResponseWriter, r *http.Request) {
	// TODO: 实现代理请求处理
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(gin.H{"error": "proxy handler not implemented"})
}

func (dr *DistributedRouter) handleStaticRequest(route *RouteConfig, w http.ResponseWriter, r *http.Request) {
	// TODO: 实现静态文件处理
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(gin.H{"error": "static handler not implemented"})
}

// 管理接口处理器
func (dr *DistributedRouter) listRoutesHandler(c *gin.Context) {
	routes := dr.routeManager.GetAllRoutes()
	c.JSON(200, gin.H{"routes": routes})
}

func (dr *DistributedRouter) addRouteHandler(c *gin.Context) {
	var route RouteConfig
	if err := c.BindJSON(&route); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if err := dr.routeManager.AddRoute(route); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "route added", "id": route.ID})
}

func (dr *DistributedRouter) updateRouteHandler(c *gin.Context) {
	id := c.Param("id")
	
	var route RouteConfig
	if err := c.BindJSON(&route); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if err := dr.routeManager.UpdateRoute(id, route); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "route updated", "id": route.ID})
}

func (dr *DistributedRouter) deleteRouteHandler(c *gin.Context) {
	id := c.Param("id")
	if err := dr.routeManager.DeleteRoute(id); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "route deleted"})
}

func (dr *DistributedRouter) listSandboxesHandler(c *gin.Context) {
	instances := dr.sandboxPool.GetAllInstances()
	c.JSON(200, gin.H{"sandboxes": instances})
}

func (dr *DistributedRouter) registerSandboxHandler(c *gin.Context) {
	var instance SandboxInstance
	if err := c.BindJSON(&instance); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if err := dr.sandboxPool.RegisterInstance(&instance); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "sandbox registered"})
}

func (dr *DistributedRouter) deleteSandboxHandler(c *gin.Context) {
	id := c.Param("id")
	if err := dr.sandboxPool.RemoveInstance(id); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "sandbox deleted"})
}

func (dr *DistributedRouter) healthHandler(c *gin.Context) {
	// 检查Redis连接
	_, err := dr.redisClient.Ping(context.Background()).Result()
	if err != nil {
		c.JSON(503, gin.H{
			"status": "unhealthy",
			"error":  "Redis connection failed: " + err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"routes":    len(dr.routeManager.GetAllRoutes()),
		"sandboxes": len(dr.sandboxPool.GetAllInstances()),
	})
}

func (dr *DistributedRouter) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func (dr *DistributedRouter) Run(addr string) error {
	// 启动Gin服务器（管理API）
	go func() {
		managementAddr := ":" + strconv.Itoa(dr.managementPort)
		log.Printf("Starting management API on %s", managementAddr)
		if err := dr.ginRouter.Run(managementAddr); err != nil {
			log.Printf("Gin server error: %v", err)
		}
	}()

	// 启动Mux服务器（动态路由）
	gatewayAddr := ":" + strconv.Itoa(dr.gatewayPort)
	log.Printf("Starting gateway server on %s", gatewayAddr)
	return http.ListenAndServe(gatewayAddr, dr.muxRouter)
}