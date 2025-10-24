package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// 沙箱池管理
type SandboxPool struct {
	redisClient  *redis.Client
	instances    map[string]*SandboxInstance
	loadBalancer *LoadBalancer
}

func NewSandboxPool(rdb *redis.Client) *SandboxPool {
	pool := &SandboxPool{
		redisClient:  rdb,
		instances:    make(map[string]*SandboxInstance),
		loadBalancer: NewLoadBalancer(),
	}

	// 从Redis加载现有实例
	pool.loadInstancesFromRedis()

	// 启动健康检查
	go pool.healthCheckLoop()

	return pool
}

func (sp *SandboxPool) loadInstancesFromRedis() {
	instances, err := sp.redisClient.HGetAll(context.Background(), "sandbox:instances").Result()
	if err != nil {
		log.Printf("Failed to load instances from Redis: %v", err)
		return
	}

	for _, instanceJSON := range instances {
		var instance SandboxInstance
		if err := json.Unmarshal([]byte(instanceJSON), &instance); err == nil {
			sp.instances[instance.ID] = &instance
		}
	}
}

func (sp *SandboxPool) healthCheckLoop() {
	ticker := time.NewTicker(15 * time.Second)
	for range ticker.C {
		sp.checkInstancesHealth()
	}
}

func (sp *SandboxPool) checkInstancesHealth() {
	for id, instance := range sp.instances {
		// 构建完整的健康检查URL - 关键修复
		healthURL := sp.buildHealthCheckURL(instance)
		if healthURL == "" {
			instance.Status = "unhealthy"
			log.Printf("❌ Sandbox %s has invalid URL: %s", id, instance.URL)
			sp.updateInstanceInRedis(instance)
			continue
		}

		log.Printf("🔍 Health checking sandbox %s at %s", id, healthURL)

		// 检查沙箱健康状态
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(healthURL)
		if err != nil {
			instance.Status = "unhealthy"
			log.Printf("❌ Sandbox %s is unhealthy: %v", id, err)
		} else {
			if resp.StatusCode == 200 {
				instance.Status = "healthy"
				instance.LastPing = time.Now().Unix()
				log.Printf("✅ Sandbox %s is healthy (status: %d)", id, resp.StatusCode)
			} else {
				instance.Status = "unhealthy"
				log.Printf("❌ Sandbox %s returned non-200 status: %d", id, resp.StatusCode)
			}
			resp.Body.Close() // 记得关闭响应体
		}

		// 更新到 Redis
		sp.updateInstanceInRedis(instance)
	}
}

// 新增：构建健康检查URL - 这是关键的修复
func (sp *SandboxPool) buildHealthCheckURL(instance *SandboxInstance) string {
	if instance.URL == "" {
		log.Printf("⚠️ Sandbox %s has empty URL", instance.ID)
		return ""
	}
	
	// 如果URL已经包含协议，直接使用
	if strings.HasPrefix(instance.URL, "http://") || strings.HasPrefix(instance.URL, "https://") {
		healthURL := instance.URL + "/health"
		log.Printf("🔗 Using existing protocol URL: %s", healthURL)
		return healthURL
	}
	
	// 否则添加默认的http协议
	healthURL := "http://" + instance.URL + "/health"
	log.Printf("🔗 Adding HTTP protocol to URL: %s", healthURL)
	return healthURL
}

func (sp *SandboxPool) updateInstanceInRedis(instance *SandboxInstance) {
	instanceJSON, _ := json.Marshal(instance)
	err := sp.redisClient.HSet(context.Background(), 
		"sandbox:instances", instance.ID, instanceJSON).Err()
	if err != nil {
		log.Printf("Failed to update instance in Redis: %v", err)
	}
}

func (sp *SandboxPool) RegisterInstance(instance *SandboxInstance) error {
	// 确保URL有协议
	if instance.URL != "" && !strings.HasPrefix(instance.URL, "http://") && !strings.HasPrefix(instance.URL, "https://") {
		instance.URL = "http://" + instance.URL
		log.Printf("🔗 Added protocol to new instance URL: %s", instance.URL)
	}
	
	sp.instances[instance.ID] = instance

	// 注册到 Redis
	sp.updateInstanceInRedis(instance)
	return nil
}

// 删除沙箱实例
func (sp *SandboxPool) RemoveInstance(instanceID string) error {
	delete(sp.instances, instanceID)

	// 从 Redis 中删除
	ctx := context.Background()
	err := sp.redisClient.HDel(ctx, "sandbox:instances", instanceID).Err()
	if err != nil {
		log.Printf("Failed to remove instance from Redis: %v")
		return err
	}
	return nil
}

func (sp *SandboxPool) GetHealthyInstance(sandboxType string) (*SandboxInstance, error) {
	var candidates []*SandboxInstance

	for _, instance := range sp.instances {
		if instance.Type == sandboxType && instance.Status == "healthy" {
			candidates = append(candidates, instance)
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no healthy %s sandbox available", sandboxType)
	}

	// 使用负载均衡选择实例
	return sp.loadBalancer.Select(candidates), nil
}

func (sp *SandboxPool) GetAllInstances() map[string]*SandboxInstance {
	return sp.instances
}
