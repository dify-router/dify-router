package gateway

import "time"

// 路由配置
type RouteConfig struct {
	ID          string            `json:"id"`
	Path        string            `json:"path"`
	Method      string            `json:"method"`
	Handler     string            `json:"handler"` // "sandbox", "proxy", "static"
	SandboxType string            `json:"sandbox_type,omitempty"` // "python", "nodejs", "go"
	Code        string            `json:"code,omitempty"`
	Target      string            `json:"target,omitempty"`
	Timeout     int               `json:"timeout,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   int64             `json:"created_at,omitempty"`
	UpdatedAt   int64             `json:"updated_at,omitempty"`
	Version     int64             `json:"version,omitempty"` // 🔧 新增：版本号
}

// 配置版本信息
type ConfigVersion struct {
	LastUpdated int64             `json:"last_updated"`
	RouteVersions map[string]int64 `json:"route_versions"` // 每个路由的版本号
	TotalRoutes  int              `json:"total_routes"`
}


// 沙箱服务实例
type SandboxInstance struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Type     string `json:"type"`
	Status   string `json:"status"` // "healthy", "unhealthy", "starting"
	Load     int    `json:"load"`   // 当前负载
	LastPing int64  `json:"last_ping"`
}

// 负载均衡器接口
type LoadBalancerInterface interface {
	Select(instances []*SandboxInstance) *SandboxInstance
	SetStrategy(strategy string)
}

// 路由事件
type RouteEvent struct {
	EventID   string      `json:"event_id"`
	EventType string      `json:"event_type"` // CREATE, UPDATE, DELETE, HEALTH_UPDATE
	RouteID   string      `json:"route_id"`
	RouteData *RouteConfig `json:"route_data,omitempty"`
	Timestamp int64       `json:"timestamp"`
	Source    string      `json:"source"`
}

// 事件消费者配置
type EventConsumerConfig struct {
	ConsumerGroup string        `json:"consumer_group"`
	ConsumerName  string        `json:"consumer_name"`
	BatchSize     int64         `json:"batch_size"`
	BlockTime     time.Duration `json:"block_time"`
	AutoAck       bool          `json:"auto_ack"`
}
