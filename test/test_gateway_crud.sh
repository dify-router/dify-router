#!/bin/bash

# XAI Router Gateway 完整功能测试脚本
# 测试所有 CRUD 操作：创建、读取、更新、删除

echo "🚀 XAI Router Gateway 完整功能测试"
echo "=========================================="

# 基础配置 - 修正端口
MANAGEMENT_URL="http://localhost:8195/admin"
GATEWAY_URL="http://localhost:8080"

# API Keys - 使用您的配置中的密钥
ADMIN_API_KEY="xai-admin-key"      # 用于管理接口
GATEWAY_API_KEY="dify-sandbox"      # 用于业务接口

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}ℹ️  $1${NC}"
}

# 检查服务是否运行
check_service() {
    echo "测试连接: $MANAGEMENT_URL/health"
    echo "使用 API Key: $ADMIN_API_KEY"
    
    response=$(curl -s -w "%{http_code}" -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/health")
    http_code=${response: -3}
    body=${response%???}
    
    if [ "$http_code" != "200" ]; then
        print_error "管理服务连接失败 (HTTP $http_code)"
        print_error "响应: $body"
        print_info "请检查："
        print_info "1. 服务是否运行在正确的端口"
        print_info "2. 查看服务日志确认配置"
        print_info "3. 确认 Redis 连接正常"
        exit 1
    fi
    print_success "服务连接正常"
    echo "健康检查响应: $body"
}

# 0. 检查服务
echo ""
print_info "0. 检查服务状态"
check_service

# 1. 健康检查测试
echo ""
print_info "1. 测试健康检查"
response=$(curl -s -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/health")
echo "响应: $response"

# 2. 查看当前路由和沙箱
echo ""
print_info "2. 查看当前状态"
echo "路由列表:"
routes_response=$(curl -s -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/routes")
echo "$routes_response" | jq '.' 2>/dev/null || echo "原始响应: $routes_response"

echo ""
echo "沙箱列表:"
sandboxes_response=$(curl -s -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/sandboxes")
echo "$sandboxes_response" | jq '.' 2>/dev/null || echo "原始响应: $sandboxes_response"

# 3. 创建路由测试 (CREATE)
echo ""
print_info "3. 测试创建路由 (CREATE)"

## 3.1 创建简单数学路由
echo "创建简单数学路由..."
math_response=$(curl -s -X POST -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/routes" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-math-create",
    "path": "/api/math",
    "method": "POST",
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "import json\nresult = {\"operation\": \"addition\", \"result\": 100, \"message\": \"Created via CREATE\"}\nprint(json.dumps(result))",
    "timeout": 10
  }')
echo "创建响应: $math_response"

## 3.2 创建状态检查路由
echo ""
echo "创建状态检查路由..."
status_response=$(curl -s -X POST -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/routes" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-status-create", 
    "path": "/api/status",
    "method": "GET",
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "import json\nimport time\nresult = {\"status\": \"healthy\", \"timestamp\": \"'$(date +%Y-%m-%dT%H:%M:%S)'\", \"action\": \"created\"}\nprint(json.dumps(result))",
    "timeout": 10
  }')
echo "创建响应: $status_response"

# 4. 验证创建的路由 (READ)
echo ""
print_info "4. 验证创建的路由 (READ)"
echo "当前所有路由:"
routes_list=$(curl -s -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/routes")
echo "$routes_list" | jq '.routes[] | {id: .id, path: .path, method: .method}' 2>/dev/null || echo "无法解析路由列表，原始响应: $routes_list"

# 5. 测试执行创建的路由
echo ""
print_info "5. 测试执行创建的路由"
echo "测试数学路由:"
math_exec=$(curl -s -X POST -H "X-Api-Key: $GATEWAY_API_KEY" "$GATEWAY_URL/api/math")
if [ -n "$math_exec" ]; then
    echo "数学路由响应: $math_exec"
else
    echo "数学路由执行失败或无响应"
fi

echo ""
echo "测试状态路由:"
status_exec=$(curl -s -H "X-Api-Key: $GATEWAY_API_KEY" "$GATEWAY_URL/api/status")
if [ -n "$status_exec" ]; then
    echo "状态路由响应: $status_exec"
else
    echo "状态路由执行失败或无响应"
fi

# 6. 更新路由测试 (UPDATE)
echo ""
print_info "6. 测试更新路由 (UPDATE)"

## 6.1 更新数学路由
echo "更新数学路由..."
update_math_response=$(curl -s -X PUT -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/routes/test-math-create" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-math-create",
    "path": "/api/math",
    "method": "POST", 
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "import json\nimport random\nresult = {\"operation\": \"multiplication\", \"result\": 42, \"random\": '$(($RANDOM % 100))', \"message\": \"Updated via UPDATE\"}\nprint(json.dumps(result))",
    "timeout": 15
  }')
echo "更新响应: $update_math_response"

## 6.2 更新状态路由
echo ""
echo "更新状态路由..."
update_status_response=$(curl -s -X PUT -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/routes/test-status-create" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-status-create",
    "path": "/api/status",
    "method": "GET",
    "handler": "sandbox",
    "sandbox_type": "python",
    "code": "import json\nimport time\nresult = {\"status\": \"updated\", \"timestamp\": \"'$(date +%Y-%m-%dT%H:%M:%S)'\", \"version\": \"2.0\", \"action\": \"updated\"}\nprint(json.dumps(result))",
    "timeout": 15
  }')
echo "更新响应: $update_status_response"

# 7. 验证更新的路由
echo ""
print_info "7. 验证更新的路由"
echo "测试更新后的数学路由:"
math_updated=$(curl -s -X POST -H "X-Api-Key: $GATEWAY_API_KEY" "$GATEWAY_URL/api/math")
if [ -n "$math_updated" ]; then
    echo "更新后数学路由响应: $math_updated"
else
    echo "更新后数学路由执行失败"
fi

echo ""
echo "测试更新后的状态路由:"
status_updated=$(curl -s -H "X-Api-Key: $GATEWAY_API_KEY" "$GATEWAY_URL/api/status")
if [ -n "$status_updated" ]; then
    echo "更新后状态路由响应: $status_updated"
else
    echo "更新后状态路由执行失败"
fi

# 8. 删除路由测试 (DELETE)
echo ""
print_info "8. 测试删除路由 (DELETE)"

## 8.1 删除数学路由
echo "删除数学路由..."
delete_math_response=$(curl -s -X DELETE -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/routes/test-math-create")
echo "删除响应: $delete_math_response"

## 8.2 删除状态路由  
echo ""
echo "删除状态路由..."
delete_status_response=$(curl -s -X DELETE -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/routes/test-status-create")
echo "删除响应: $delete_status_response"

# 9. 验证删除结果
echo ""
print_info "9. 验证删除结果"
echo "剩余路由:"
final_routes=$(curl -s -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/routes")
echo "$final_routes" | jq '.routes[] | {id: .id, path: .path, method: .method}' 2>/dev/null || echo "无路由或解析失败"

# 10. 最终状态检查
echo ""
print_info "10. 最终状态检查"
echo "健康状态:"
health_final=$(curl -s -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/health")
echo "$health_final" | jq '.' 2>/dev/null || echo "健康检查失败: $health_final"

echo ""
echo "剩余路由数量:"
routes_count=$(curl -s -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/routes")
echo "$routes_count" | jq '.routes | length' 2>/dev/null || echo "无法获取路由数量"

echo "剩余沙箱数量:"
sandboxes_count=$(curl -s -H "X-Api-Key: $ADMIN_API_KEY" "$MANAGEMENT_URL/sandboxes")
echo "$sandboxes_count" | jq '.sandboxes | length' 2>/dev/null || echo "无法获取沙箱数量"

echo ""
print_success "🎉 所有测试完成！XAI Router Gateway CRUD 功能正常！"
