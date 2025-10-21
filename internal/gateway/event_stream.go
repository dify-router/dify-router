package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// 事件流管理器
type EventStreamManager struct {
	redisClient *redis.Client
	streamKey   string
	consumers   map[string]*EventConsumer
	mutex       sync.RWMutex
}

// 事件消费者
type EventConsumer struct {
	config      EventConsumerConfig
	handler     EventHandler
	stopChan    chan struct{}
	running     bool
	redisClient *redis.Client
	streamKey   string
}

// 事件处理器接口
type EventHandler interface {
	HandleEvent(event *RouteEvent) error
}

// 创建新的事件流管理器
func NewEventStreamManager(redisClient *redis.Client) *EventStreamManager {
	return &EventStreamManager{
		redisClient: redisClient,
		streamKey:   "gateway:route:events",
		consumers:   make(map[string]*EventConsumer),
	}
}

// 发布路由事件
func (esm *EventStreamManager) PublishRouteEvent(ctx context.Context, event *RouteEvent) error {
	event.Timestamp = time.Now().Unix()
	if event.Source == "" {
		event.Source = "gateway"
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %v", err)
	}

	fields := map[string]interface{}{
		"event_data": string(eventData),
		"timestamp":  event.Timestamp,
		"event_type": event.EventType,
		"route_id":   event.RouteID,
	}

	// 发布到Redis Stream
	messageID, err := esm.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: esm.streamKey,
		Values: fields,
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to publish event: %v", err)
	}

	log.Printf("📨 Published event: %s - %s - %s", event.EventType, event.RouteID, messageID)
	return nil
}

// 创建事件消费者
func (esm *EventStreamManager) CreateConsumer(config EventConsumerConfig, handler EventHandler) (*EventConsumer, error) {
	consumer := &EventConsumer{
		config:      config,
		handler:     handler,
		stopChan:    make(chan struct{}),
		redisClient: esm.redisClient,
		streamKey:   esm.streamKey,
	}

	// 创建消费者组
	ctx := context.Background()
	err := esm.redisClient.XGroupCreateMkStream(ctx, esm.streamKey, config.ConsumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return nil, fmt.Errorf("failed to create consumer group: %v", err)
	}

	esm.mutex.Lock()
	esm.consumers[config.ConsumerName] = consumer
	esm.mutex.Unlock()

	return consumer, nil
}

// 启动事件消费者
func (ec *EventConsumer) Start() {
	if ec.running {
		return
	}

	ec.running = true
	go ec.consumeEvents()
	log.Printf("🚀 Started event consumer: %s", ec.config.ConsumerName)
}

// 停止事件消费者
func (ec *EventConsumer) Stop() {
	if !ec.running {
		return
	}

	close(ec.stopChan)
	ec.running = false
	log.Printf("🛑 Stopped event consumer: %s", ec.config.ConsumerName)
}

// 消费事件
func (ec *EventConsumer) consumeEvents() {
	ctx := context.Background()

	for {
		select {
		case <-ec.stopChan:
			return
		default:
			// 从Stream读取消息
			streams, err := ec.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    ec.config.ConsumerGroup,
				Consumer: ec.config.ConsumerName,
				Streams:  []string{ec.streamKey, ">"},
				Count:    ec.config.BatchSize,
				Block:    ec.config.BlockTime,
			}).Result()

			if err != nil && err != redis.Nil {
				log.Printf("Error reading from stream: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}

			if len(streams) == 0 || len(streams[0].Messages) == 0 {
				continue
			}

			// 处理消息
			for _, message := range streams[0].Messages {
				if err := ec.processMessage(ctx, message); err != nil {
					log.Printf("Error processing message %s: %v", message.ID, err)
				}
			}
		}
	}
}

// 处理单个消息
func (ec *EventConsumer) processMessage(ctx context.Context, message redis.XMessage) error {
	eventData, exists := message.Values["event_data"].(string)
	if !exists {
		return fmt.Errorf("missing event_data in message")
	}

	var event RouteEvent
	if err := json.Unmarshal([]byte(eventData), &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %v", err)
	}

	// 调用事件处理器
	if err := ec.handler.HandleEvent(&event); err != nil {
		return fmt.Errorf("event handler failed: %v", err)
	}

	// 确认消息
	if ec.config.AutoAck {
		if err := ec.redisClient.XAck(ctx, ec.streamKey, ec.config.ConsumerGroup, message.ID).Err(); err != nil {
			return fmt.Errorf("failed to ack message: %v", err)
		}
	}

	return nil
}

// 获取Stream信息
func (esm *EventStreamManager) GetStreamInfo(ctx context.Context) (map[string]interface{}, error) {
	info, err := esm.redisClient.XInfoStream(ctx, esm.streamKey).Result()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"length":            info.Length,
		"last_generated_id": info.LastGeneratedID,
		"first_entry":       info.FirstEntry,
		"last_entry":        info.LastEntry,
	}, nil
}

// 获取待处理消息
func (esm *EventStreamManager) GetPendingMessages(ctx context.Context, consumerGroup string) ([]redis.XPendingExt, error) {
	return esm.redisClient.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream:   esm.streamKey,
		Group:    consumerGroup,
		Start:    "-",
		End:      "+",
		Count:    100,
	}).Result()
}
