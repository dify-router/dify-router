package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
)

// 路由管理器
type RouteManager struct {
	redisClient      *redis.Client
	eventStream      *EventStreamManager
	routeCache       map[string]RouteConfig
	routeVersions    map[string]int64 // 🔧 新增：内存中的路由版本
	router           *mux.Router
	updateChannel    chan struct{}
	mutex            sync.RWMutex
	redisEnabled     bool
	eventConsumers   []*EventConsumer
	lastConfigUpdate int64            // 🔧 新增：最后配置更新时间
	instanceID       string           // 🔧 新增：实例ID
}

func NewRouteManager(redisClient *redis.Client) *RouteManager {
	rm := &RouteManager{
		redisClient:    redisClient,
		routeCache:     make(map[string]RouteConfig),
		routeVersions:  make(map[string]int64), // 🔧 初始化版本映射
		router:         mux.NewRouter(),
		updateChannel:  make(chan struct{}, 1),
		redisEnabled:   true,
		instanceID:     fmt.Sprintf("instance-%d", time.Now().UnixNano()), // 🔧 实例标识
	}

	// 测试 Redis 连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		log.Printf("⚠️  Redis not available, using in-memory storage only")
		rm.redisEnabled = false
	} else {
		// 初始化事件流管理器
		rm.eventStream = NewEventStreamManager(redisClient)
		
		// 🔧 修改：使用增量加载代替全量加载
		rm.loadRoutesIncremental()
		
		// 启动事件消费者
		rm.startEventConsumers()
	}

	// 🔧 修改：延长配置监听间隔到1分钟
	go rm.watchConfigurationChanges(60 * time.Second)

	return rm
}

// 🔧 新增：增量加载路由
func (rm *RouteManager) loadRoutesIncremental() {
	if !rm.redisEnabled {
		return
	}

	ctx := context.Background()
	
	// 1. 获取全局配置版本
	configVersionJSON, err := rm.redisClient.Get(ctx, "gateway:config:version").Result()
	if err != nil && err != redis.Nil {
		log.Printf("Failed to get config version: %v", err)
		return
	}

	var currentConfigVersion int64
	if configVersionJSON != "" {
		currentConfigVersion, _ = strconv.ParseInt(configVersionJSON, 10, 64)
	}

	// 2. 如果版本没有变化，跳过加载
	if currentConfigVersion <= rm.lastConfigUpdate {
		return
	}

	// 3. 获取有变更的路由ID列表
	updatedRoutes, err := rm.redisClient.SMembers(ctx, "gateway:routes:updated").Result()
	if err != nil && err != redis.Nil {
		log.Printf("Failed to get updated routes: %v", err)
		return
	}

	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	updateCount := 0
	deleteCount := 0

	if len(updatedRoutes) > 0 {
		// 4. 增量更新：只加载有变更的路由
		for _, routeID := range updatedRoutes {
			if routeID == "" {
				continue
			}

			if strings.HasPrefix(routeID, "DELETE:") {
				// 处理删除的路由
				actualRouteID := strings.TrimPrefix(routeID, "DELETE:")
				if _, exists := rm.routeCache[actualRouteID]; exists {
					delete(rm.routeCache, actualRouteID)
					delete(rm.routeVersions, actualRouteID)
					deleteCount++
					log.Printf("🗑️  Incremental delete: %s", actualRouteID)
				}
			} else {
				// 处理新增/更新的路由
				routeJSON, err := rm.redisClient.HGet(ctx, "gateway:routes", routeID).Result()
				if err == nil {
					var route RouteConfig
					if err := json.Unmarshal([]byte(routeJSON), &route); err == nil {
						// 检查版本，避免重复更新
						if route.Version > rm.routeVersions[routeID] {
							rm.routeCache[routeID] = route
							rm.routeVersions[routeID] = route.Version
							updateCount++
							log.Printf("🔄 Incremental update: %s (v%d)", routeID, route.Version)
						}
					}
				}
			}
		}

		// 5. 清理更新标记
		rm.redisClient.Del(ctx, "gateway:routes:updated")
	} else {
		// 6. 如果没有更新信息，回退到全量加载（安全机制）
		log.Printf("⚠️  No update info, falling back to full load")
		rm.loadAllRoutesFromRedis()
		updateCount = len(rm.routeCache)
	}

	// 7. 更新配置版本
	rm.lastConfigUpdate = currentConfigVersion

	log.Printf("📦 Incremental load: %d updated, %d deleted, total: %d routes", 
		updateCount, deleteCount, len(rm.routeCache))
}

// 🔧 新增：全量加载（备用）
func (rm *RouteManager) loadAllRoutesFromRedis() {
	ctx := context.Background()
	routes, err := rm.redisClient.HGetAll(ctx, "gateway:routes").Result()
	if err != nil {
		log.Printf("Failed to load routes from Redis: %v", err)
		return
	}

	rm.routeCache = make(map[string]RouteConfig)
	rm.routeVersions = make(map[string]int64)

	for routeID, routeJSON := range routes {
		var route RouteConfig
		if err := json.Unmarshal([]byte(routeJSON), &route); err == nil {
			rm.routeCache[routeID] = route
			rm.routeVersions[routeID] = route.Version
		}
	}
}

// 加载初始路由
func (rm *RouteManager) loadInitialRoutes() {
	if !rm.redisEnabled {
		return
	}

	ctx := context.Background()
	routes, err := rm.redisClient.HGetAll(ctx, "gateway:routes").Result()
	if err != nil {
		log.Printf("Failed to load routes from Redis: %v", err)
		return
	}

	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	for _, routeJSON := range routes {
		var route RouteConfig
		if err := json.Unmarshal([]byte(routeJSON), &route); err == nil {
			rm.routeCache[route.ID] = route
		}
	}

	log.Printf("Loaded %d routes from Redis", len(rm.routeCache))
}

// 启动事件消费者
func (rm *RouteManager) startEventConsumers() {
	if !rm.redisEnabled {
		return
	}

	// 创建路由事件消费者
	routeHandler := &RouteEventHandler{routeManager: rm}
	consumerConfig := EventConsumerConfig{
		ConsumerGroup: "route-managers",
		ConsumerName:  fmt.Sprintf("consumer-%d", time.Now().UnixNano()),
		BatchSize:     10,
		BlockTime:     5 * time.Second,
		AutoAck:       true,
	}

	consumer, err := rm.eventStream.CreateConsumer(consumerConfig, routeHandler)
	if err != nil {
		log.Printf("Failed to create event consumer: %v", err)
		return
	}

	consumer.Start()
	rm.eventConsumers = append(rm.eventConsumers, consumer)
	log.Printf("✅ Started route event consumer: %s", consumerConfig.ConsumerName)
}

// 路由事件处理器
type RouteEventHandler struct {
	routeManager *RouteManager
}

func (h *RouteEventHandler) HandleEvent(event *RouteEvent) error {
	startTime := time.Now()
	log.Printf("🎬 [EVENT] 开始处理事件 | 类型: %s | ID: %s | 路由: %s", 
		event.EventType, event.EventID, event.RouteID)

	var err error
	switch event.EventType {
	case "CREATE":
		err = h.handleCreateEvent(event)
	case "UPDATE":
		err = h.handleUpdateEvent(event)
	case "DELETE":
		err = h.handleDeleteEvent(event)
	default:
		log.Printf("❌ [EVENT] 未知事件类型: %s", event.EventType)
		err = nil
	}

	duration := time.Since(startTime)
	if err != nil {
		log.Printf("💥 [EVENT] 事件处理失败 | 类型: %s | ID: %s | 耗时: %v | 错误: %v", 
			event.EventType, event.EventID, duration, err)
	} else {
		log.Printf("🎉 [EVENT] 事件处理成功 | 类型: %s | ID: %s | 耗时: %v", 
			event.EventType, event.EventID, duration)
	}
	
	return err
}

func (h *RouteEventHandler) handleCreateEvent(event *RouteEvent) error {
    if event.RouteData == nil {
        return fmt.Errorf("missing route data for CREATE event")
    }

    targetRouteID := event.RouteData.ID
    if targetRouteID == "" {
        targetRouteID = event.RouteID
    }

    h.routeManager.mutex.Lock()
    defer h.routeManager.mutex.Unlock()

    // 检查是否已存在
    if existing, exists := h.routeManager.routeCache[targetRouteID]; exists {
        log.Printf("⚠️ [CREATE] 路由已存在，将被覆盖: %s (原版本: %d)", targetRouteID, existing.Version)
    }

    h.routeManager.routeCache[targetRouteID] = *event.RouteData
    h.routeManager.routeVersions[targetRouteID] = event.RouteData.Version
    log.Printf("✅ [CREATE] 路由创建成功: %s (版本: %d)", targetRouteID, event.RouteData.Version)
    
    return nil
}

func (h *RouteEventHandler) handleUpdateEvent(event *RouteEvent) error {
    if event.RouteData == nil {
        return fmt.Errorf("missing route data for UPDATE event")
    }

    targetRouteID := event.RouteData.ID
    if targetRouteID == "" {
        targetRouteID = event.RouteID
    }

    h.routeManager.mutex.Lock()
    defer h.routeManager.mutex.Unlock()

    log.Printf("📊 [UPDATE] 处理路由更新: %s (事件ID: %s)", targetRouteID, event.RouteID)
    
    if existing, exists := h.routeManager.routeCache[targetRouteID]; exists {
        log.Printf("📝 [UPDATE] 更新现有路由: %s", targetRouteID)
        log.Printf("   📋 旧版本: %d, 新版本: %d", existing.Version, event.RouteData.Version)
        
        h.routeManager.routeCache[targetRouteID] = *event.RouteData
        h.routeManager.routeVersions[targetRouteID] = event.RouteData.Version
        log.Printf("✅ [UPDATE] 路由更新成功: %s (版本: %d)", targetRouteID, event.RouteData.Version)
    } else {
        log.Printf("⚠️ [UPDATE] 路由不存在，创建新路由: %s", targetRouteID)
        h.routeManager.routeCache[targetRouteID] = *event.RouteData
        h.routeManager.routeVersions[targetRouteID] = event.RouteData.Version
        log.Printf("✅ [UPDATE] 新路由创建成功: %s (版本: %d)", targetRouteID, event.RouteData.Version)
    }
    
    return nil
}

func (h *RouteEventHandler) handleDeleteEvent(event *RouteEvent) error {
    h.routeManager.mutex.Lock()
    defer h.routeManager.mutex.Unlock()

    targetRouteID := event.RouteID
    
    log.Printf("🗑️ [DELETE] 处理路由删除: %s", targetRouteID)
    
    if _, exists := h.routeManager.routeCache[targetRouteID]; exists {
        delete(h.routeManager.routeCache, targetRouteID)
        delete(h.routeManager.routeVersions, targetRouteID)
        log.Printf("✅ [DELETE] 路由删除成功: %s", targetRouteID)
    } else {
        log.Printf("⚠️ [DELETE] 路由不存在: %s", targetRouteID)
        // 尝试从事件数据中查找路由ID
        if event.RouteData != nil && event.RouteData.ID != "" {
            alternativeID := event.RouteData.ID
            if _, exists := h.routeManager.routeCache[alternativeID]; exists {
                delete(h.routeManager.routeCache, alternativeID)
                delete(h.routeManager.routeVersions, alternativeID)
                log.Printf("✅ [DELETE] 通过备用ID删除成功: %s", alternativeID)
            } else {
                log.Printf("❌ [DELETE] 备用ID也不存在: %s", alternativeID)
            }
        }
    }
    
    return nil
}

// 🔧 修改：配置监听方法，支持自定义间隔
func (rm *RouteManager) watchConfigurationChanges(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("⏰ Configuration watcher started (interval: %v)", interval)

	for {
		select {
		case <-rm.updateChannel:
			rm.loadRoutesIncremental() // 🔧 使用增量加载
		case <-ticker.C:
			rm.checkForConfigurationUpdates()
		}
	}
}

func (rm *RouteManager) checkForConfigurationUpdates() {
	if !rm.redisEnabled {
		return
	}

	rm.loadRoutesIncremental() // 🔧 直接使用增量加载
}

// 🔧 新增：更新配置版本（在CUD操作中调用）
func (rm *RouteManager) updateConfigVersion() {
	if !rm.redisEnabled {
		return
	}

	ctx := context.Background()
	newVersion := time.Now().UnixNano()
	
	err := rm.redisClient.Set(ctx, "gateway:config:version", newVersion, 0).Err()
	if err != nil {
		log.Printf("Failed to update config version: %v", err)
	}
}

// 关键算法：路由匹配
func (rm *RouteManager) matchRoute(path, method string) *RouteConfig {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	var matchedRoute *RouteConfig
	var matchPriority int

	for _, route := range rm.routeCache {
		priority := rm.calculateMatchPriority(route, path, method)
		if priority > matchPriority {
			matchedRoute = &route
			matchPriority = priority
		}
	}

	return matchedRoute
}

// 计算匹配优先级
func (rm *RouteManager) calculateMatchPriority(route RouteConfig, path, method string) int {
	if route.Method != method && route.Method != "ANY" {
		return 0
	}

	// 1. 精确匹配最高优先级
	if route.Path == path {
		return 100
	}

	// 2. 参数匹配次之 /users/{id}
	if rm.matchPathWithParams(route.Path, path) {
		return 90
	}

	// 3. 前缀匹配 /api/
	if strings.HasPrefix(path, route.Path+"/") {
		return 80
	}

	// 4. 通配符匹配 /api/*
	if strings.Contains(route.Path, "*") {
		pattern := strings.ReplaceAll(route.Path, "*", ".*")
		if matched, _ := regexp.MatchString("^"+pattern+"$", path); matched {
			return 70
		}
	}

	return 0
}

// 匹配带参数的路由
func (rm *RouteManager) matchPathWithParams(routePath, requestPath string) bool {
	route := mux.NewRouter()
	route.Path(routePath).Methods("GET")
	
	req, _ := http.NewRequest("GET", requestPath, nil)
	var match mux.RouteMatch
	return route.Match(req, &match)
}

// 添加路由（发布事件 + 持久化存储）
func (rm *RouteManager) AddRoute(route RouteConfig) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	// 验证路由配置
	if err := rm.validateRouteConfiguration(route); err != nil {
		return err
	}

	// 设置时间戳和版本
	now := time.Now().Unix()
	if route.CreatedAt == 0 {
		route.CreatedAt = now
	}
	route.UpdatedAt = now
	route.Version = time.Now().UnixNano() // 🔧 设置版本号

	// 保存到Redis（持久化存储）
	if rm.redisEnabled {
		ctx := context.Background()
		routeJSON, _ := json.Marshal(route)
		
		// 🔧 修复：保存到Redis哈希表
		err := rm.redisClient.HSet(ctx, "gateway:routes", route.ID, routeJSON).Err()
		if err != nil {
			log.Printf("Failed to save route to Redis: %v", err)
			// 继续在内存中保存，但记录错误
		} else {
			// 🔧 新增：标记路由为已更新（用于增量同步）
			rm.redisClient.SAdd(ctx, "gateway:routes:updated", route.ID)
			// 🔧 新增：更新全局配置版本
			rm.updateConfigVersion()
			
			log.Printf("💾 Route saved to Redis: %s", route.ID)
		}
	}

	// 发布创建事件（用于实时同步）
	if rm.redisEnabled {
		event := &RouteEvent{
			EventID:   fmt.Sprintf("create-%d", now),
			EventType: "CREATE", 
			RouteID:   route.ID,
			RouteData: &route,
			Timestamp: now,
			Source:    "route-manager",
		}

		if err := rm.eventStream.PublishRouteEvent(context.Background(), event); err != nil {
			log.Printf("Failed to publish CREATE event: %v", err)
		}
	}

	// 更新内存缓存
	rm.routeCache[route.ID] = route
	rm.routeVersions[route.ID] = route.Version

	// 通知更新
	select {
	case rm.updateChannel <- struct{}{}:
	default:
		// 通道已满，跳过
	}

	return nil
}

// 更新路由（发布事件 + 持久化存储）
func (rm *RouteManager) UpdateRoute(routeID string, newRoute RouteConfig) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	// 检查路由是否存在
	if _, exists := rm.routeCache[routeID]; !exists {
		return fmt.Errorf("route %s not found", routeID)
	}

	// 验证新的路由配置
	if err := rm.validateRouteConfiguration(newRoute); err != nil {
		return err
	}

	// 确保ID一致
	if routeID != newRoute.ID {
		return fmt.Errorf("route ID cannot be changed")
	}

	// 设置更新时间戳和版本
	newRoute.UpdatedAt = time.Now().Unix()
	newRoute.Version = time.Now().UnixNano() // 🔧 设置版本号

	// 保存到Redis（持久化存储）
	if rm.redisEnabled {
		ctx := context.Background()
		routeJSON, _ := json.Marshal(newRoute)
		
		// 🔧 修复：更新Redis哈希表
		err := rm.redisClient.HSet(ctx, "gateway:routes", routeID, routeJSON).Err()
		if err != nil {
			log.Printf("Failed to update route in Redis: %v", err)
			// 继续在内存中更新，但记录错误
		} else {
			// 🔧 新增：标记路由为已更新（用于增量同步）
			rm.redisClient.SAdd(ctx, "gateway:routes:updated", routeID)
			// 🔧 新增：更新全局配置版本
			rm.updateConfigVersion()
			
			log.Printf("💾 Route updated in Redis: %s", routeID)
		}
	}

	// 发布更新事件（用于实时同步）
	if rm.redisEnabled {
		event := &RouteEvent{
			EventID:   fmt.Sprintf("update-%d", time.Now().Unix()),
			EventType: "UPDATE",
			RouteID:   routeID,
			RouteData: &newRoute,
			Timestamp: time.Now().Unix(),
			Source:    "route-manager",
		}

		if err := rm.eventStream.PublishRouteEvent(context.Background(), event); err != nil {
			log.Printf("Failed to publish UPDATE event: %v", err)
		}
	}

	// 更新内存缓存
	rm.routeCache[routeID] = newRoute
	rm.routeVersions[routeID] = newRoute.Version // 🔧 更新版本映射

	// 通知更新
	select {
	case rm.updateChannel <- struct{}{}:
	default:
	}

	return nil
}

// 删除路由（发布事件 + 持久化存储）
func (rm *RouteManager) DeleteRoute(routeID string) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	ctx := context.Background()
	
	// 从Redis删除（持久化存储）
	if rm.redisEnabled {
		// 🔧 修复：从Redis哈希表中删除路由
		err := rm.redisClient.HDel(ctx, "gateway:routes", routeID).Err()
		if err != nil {
			log.Printf("Failed to delete route from Redis: %v", err)
			// 继续删除内存中的路由，但记录错误
		} else {
			// 🔧 新增：标记路由为已删除（用于增量同步）
			rm.redisClient.SAdd(ctx, "gateway:routes:updated", "DELETE:"+routeID)
			// 🔧 新增：更新全局配置版本
			rm.updateConfigVersion()
			
			log.Printf("💾 Route deleted from Redis: %s", routeID)
		}
	}

	// 发布删除事件（用于实时同步）
	if rm.redisEnabled {
		event := &RouteEvent{
			EventID:   fmt.Sprintf("delete-%d", time.Now().Unix()),
			EventType: "DELETE",
			RouteID:   routeID,
			Timestamp: time.Now().Unix(),
			Source:    "route-manager",
		}

		if err := rm.eventStream.PublishRouteEvent(context.Background(), event); err != nil {
			log.Printf("Failed to publish DELETE event: %v", err)
		}
	}

	// 从内存缓存删除
	delete(rm.routeCache, routeID)
	delete(rm.routeVersions, routeID) // 🔧 清理版本映射

	// 通知更新
	select {
	case rm.updateChannel <- struct{}{}:
	default:
	}

	return nil
}

// 验证路由配置
func (rm *RouteManager) validateRouteConfiguration(route RouteConfig) error {
	if route.ID == "" {
		return fmt.Errorf("route ID is required")
	}
	if route.Path == "" {
		return fmt.Errorf("route path is required")
	}
	if route.Method == "" {
		return fmt.Errorf("route method is required")
	}
	if route.Handler == "" {
		return fmt.Errorf("route handler is required")
	}

	validHandlers := map[string]bool{
		"sandbox": true,
		"proxy":   true,
		"static":  true,
	}
	if !validHandlers[route.Handler] {
		return fmt.Errorf("invalid handler type: %s", route.Handler)
	}

	if route.Handler == "sandbox" {
		validSandboxTypes := map[string]bool{
			"python": true,
			"nodejs": true,
			"go":     true,
		}
		if !validSandboxTypes[route.SandboxType] {
			return fmt.Errorf("invalid sandbox type: %s", route.SandboxType)
		}
	}

	return nil
}

// 获取所有路由
func (rm *RouteManager) GetAllRoutes() []RouteConfig {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	routes := make([]RouteConfig, 0, len(rm.routeCache))
	for _, route := range rm.routeCache {
		routes = append(routes, route)
	}
	return routes
}

// 获取事件流管理器（用于管理接口）
func (rm *RouteManager) GetEventStream() *EventStreamManager {
	return rm.eventStream
}
