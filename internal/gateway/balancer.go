package gateway

import "math/rand"

type LoadBalancer struct {
	strategy string // "round-robin", "least-connections", "random"
	counters map[string]int
}

func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{
		strategy: "least-connections",
		counters: make(map[string]int),
	}
}

func (lb *LoadBalancer) SetStrategy(strategy string) {
	lb.strategy = strategy
}

func (lb *LoadBalancer) Select(instances []*SandboxInstance) *SandboxInstance {
	if len(instances) == 0 {
		return nil
	}

	switch lb.strategy {
	case "least-connections":
		return lb.leastConnections(instances)
	case "round-robin":
		return lb.roundRobin(instances)
	case "random":
		return lb.random(instances)
	default:
		return lb.leastConnections(instances)
	}
}

func (lb *LoadBalancer) leastConnections(instances []*SandboxInstance) *SandboxInstance {
	var selected *SandboxInstance
	minLoad := int(^uint(0) >> 1) // max int

	for _, instance := range instances {
		if instance.Load < minLoad {
			minLoad = instance.Load
			selected = instance
		}
	}
	
	if selected != nil {
		selected.Load++ // 增加负载计数
	}
	return selected
}

func (lb *LoadBalancer) roundRobin(instances []*SandboxInstance) *SandboxInstance {
	if len(instances) == 0 {
		return nil
	}
	
	// 简单的轮询实现 - 在实际生产环境中可能需要更复杂的实现
	selected := instances[rand.Intn(len(instances))]
	if selected != nil {
		selected.Load++
	}
	return selected
}

func (lb *LoadBalancer) random(instances []*SandboxInstance) *SandboxInstance {
	if len(instances) == 0 {
		return nil
	}
	selected := instances[rand.Intn(len(instances))]
	if selected != nil {
		selected.Load++
	}
	return selected
}
