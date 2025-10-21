Introduction

XAI Router Gateway is a high-performance API gateway designed for managing and routing API requests with built-in sandbox execution capabilities. It provides a secure and scalable solution for running untrusted code in isolated environments while offering comprehensive management interfaces for route configuration, event monitoring, and system synchronization.

Features

Dynamic Route Management: Create, update, and delete API routes on-the-fly
Sandbox Execution: Secure execution of Python code in isolated environments
Event-Driven Architecture: Real-time event system for route synchronization
Dual Authentication: Separate authentication for management and gateway endpoints
Health Monitoring: Comprehensive system health and performance monitoring
Configuration Synchronization: Manual and automatic configuration synchronization
Use


Requirements

XAI Router Gateway requires the following dependencies:

Redis server for data persistence and event streaming
Python 3.8+ for sandbox execution
Linux environment (recommended for production deployment)
Installation Steps

Ensure Redis server is installed and running on the system
Download the XAI Router Gateway binary or build from source
Configure environment variables for Redis connection and authentication keys
Start the gateway service with appropriate configuration

作者：david@yqlee.com 



🚀 XAI Router Gateway 管理接口文档

📋 基础信息


动态路由管理：实时创建、更新和删除 API 路由
沙箱执行：在隔离环境中安全执行 Python 代码
事件驱动架构：用于路由同步的实时事件系统
双重认证：管理和网关端点采用独立的认证机制
健康监控：全面的系统健康状态和性能监控
配置同步：手动和自动配置同步功能


管理端口: 8195 (带认证)
认证头: X-Api-Key: xai-admin-key
网关端口: 8080 (带认证，与dify-sandbox保持一致)
认证头: X-Api-Key: dify-sandbox
🔧 系统状态接口

1. 健康检查

bash
# 基础健康检查
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/health
2. 配置版本信息

bash
# 配置版本信息
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/config/version
3. 系统状态报告

bash
# 获取完整系统状态
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/health && \
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/config/version && \
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/events/stats
🛣️ 路由管理接口

4. 获取所有路由列表

bash
# 获取路由列表
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/routes
5. 创建路由

bash
# 创建路由
curl -X POST -H "X-Api-Key: xai-admin-key" \
  -H "Content-Type: application/json" \
  http://localhost:8195/admin/routes \
  -d '{
    "id": "example-route",
    "path": "/api/example",
    "method": "GET",
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "print(\"Hello World\")",
    "timeout": 5
  }'
6. 更新路由

bash
# 更新路由
curl -X PUT -H "X-Api-Key: xai-admin-key" \
  -H "Content-Type: application/json" \
  http://localhost:8195/admin/routes/example-route \
  -d '{
    "id": "example-route",
    "path": "/api/example",
    "method": "GET",
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "print(\"Updated Hello World\")",
    "timeout": 10
  }'
7. 删除路由

bash
# 删除路由
curl -X DELETE -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/routes/example-route
8. 路由详情查询

bash
# 查询路由详情
curl -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/routes/example-route/details
🌊 事件系统接口

9. 事件统计信息

bash
# 获取事件统计
curl -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/events/stats
10. 事件流信息

bash
# 获取事件流信息
curl -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/events/stream-info
11. 事件消费者状态

bash
# 获取消费者状态
curl -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/events/consumers
12. 事件清理

bash
# 清理事件
curl -X POST -H "X-Api-Key: xai-admin-key" \
  -H "Content-Type: application/json" \
  http://localhost:8195/admin/events/cleanup \
  -d '{"max_age_hours": 1}'
13. 测试事件发布

bash
# 发布测试事件
curl -X POST -H "X-Api-Key: xai-admin-key" \
  -H "Content-Type: application/json" \
  http://localhost:8195/admin/events/test \
  -d '{
    "event_type": "CREATE",
    "route_id": "test-route",
    "route_data": {
      "id": "test-route",
      "path": "/api/test",
      "method": "GET",
      "handler": "sandbox",
      "sandbox_type": "python",
      "code": "print(\"Test event\")",
      "timeout": 5
    }
  }'
🔄 同步管理接口

14. 手动触发配置同步

bash
# 触发同步
curl -X POST -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/sync/trigger
🧪 测试和验证接口

15. 完整CRUD操作测试

bash
# 测试完整的创建、读取、更新、删除操作
curl -X POST -H "X-Api-Key: xai-admin-key" \
  -H "Content-Type: application/json" \
  http://localhost:8195/admin/routes \
  -d '{
    "id": "crud-test",
    "path": "/api/crud-test",
    "method": "GET",
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "print(\"CREATE test\")",
    "timeout": 5
  }' && \
curl -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/routes/crud-test/details && \
curl -X PUT -H "X-Api-Key: xai-admin-key" \
  -H "Content-Type: application/json" \
  http://localhost:8195/admin/routes/crud-test \
  -d '{
    "id": "crud-test",
    "path": "/api/crud-test",
    "method": "GET",
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "print(\"UPDATE test\")",
    "timeout": 5
  }' && \
curl -X DELETE -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/routes/crud-test
16. 立即生效测试

bash
# 测试路由操作是否立即生效
curl -X POST -H "X-Api-Key: xai-admin-key" \
  -H "Content-Type: application/json" \
  http://localhost:8195/admin/routes \
  -d '{
    "id": "immediate-test",
    "path": "/api/immediate",
    "method": "GET",
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "print(\"Immediate test\")",
    "timeout": 5
  }' && \
curl -H "X-Api-Key: dify-sandbox" \
  http://localhost:8080/api/immediate && \
curl -X DELETE -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/routes/immediate-test
17. 双重保证测试

bash
# 测试直接API和事件广播双重机制
curl -X POST -H "X-Api-Key: xai-admin-key" \
  -H "Content-Type: application/json" \
  http://localhost:8195/admin/routes \
  -d '{
    "id": "critical-route",
    "path": "/api/critical",
    "method": "POST",
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "print(\"Direct API creation\")",
    "timeout": 5
  }' && \
curl -X POST -H "X-Api-Key: xai-admin-key" \
  -H "Content-Type: application/json" \
  http://localhost:8195/admin/events/test \
  -d '{
    "event_type": "UPDATE",
    "route_id": "critical-route",
    "route_data": {
      "id": "critical-route",
      "path": "/api/critical",
      "method": "POST",
      "handler": "sandbox",
      "sandbox_type": "python",
      "code": "print(\"Event broadcast update\")",
      "timeout": 5
    }
  }'
18. 集成测试

bash
# 完整集成测试流程
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/health && \
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/config/version && \
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/events/stats && \
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/routes
📊 快速开始示例

创建并测试一个简单路由：

bash
# 1. 创建路由
curl -X POST -H "X-Api-Key: xai-admin-key" \
  -H "Content-Type: application/json" \
  http://localhost:8195/admin/routes \
  -d '{
    "id": "hello-world",
    "path": "/api/hello",
    "method": "GET",
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "print(\"Hello from XAI Gateway!\")",
    "timeout": 5
  }'

# 2. 测试路由执行 (使用网关认证)
curl -H "X-Api-Key: dify-sandbox" http://localhost:8080/api/hello

# 3. 查看路由详情
curl -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/routes/hello-world/details

# 4. 清理路由
curl -X DELETE -H "X-Api-Key: xai-admin-key" \
  http://localhost:8195/admin/routes/hello-world
检查系统完整状态：

bash
# 快速系统检查
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/health && \
echo "---" && \
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/config/version && \
echo "---" && \
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/events/stats && \
echo "---" && \
curl -H "X-Api-Key: xai-admin-key" http://localhost:8195/admin/routes | jq '.routes | length'
🎯 接口特性总结

功能类别	接口数量	主要用途	认证方式
系统状态	3个	健康检查、版本信息、状态报告	X-Api-Key: xai-admin-key
路由管理	5个	路由的增删改查和详情查询	X-Api-Key: xai-admin-key
事件系统	5个	事件统计、监控、清理和测试	X-Api-Key: xai-admin-key
同步管理	1个	手动触发配置同步	X-Api-Key: xai-admin-key
测试工具	4个	功能验证、集成测试	X-Api-Key: xai-admin-key
端口和认证说明：

管理端口8195: 所有管理接口，认证头 X-Api-Key: xai-admin-key
网关端口8080: 路由执行接口，认证头 X-Api-Key: dify-sandbox
所有接口都提供完整的CRUD操作，支持实时生效和事件驱动的双重保证机制。
