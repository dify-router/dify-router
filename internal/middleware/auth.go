package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/dify-router/dify-router/internal/static"
)

// GatewayAuth 网关端口认证 - 用于运行沙箱等业务接口
func GatewayAuth() gin.HandlerFunc {
	config := static.GetDifySandboxGlobalConfigurations()
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-Api-Key")
		
		// 优先级：gateway_key > key（向后兼容）
		expectedKey := config.App.GatewayKey
		if expectedKey == "" {
			expectedKey = config.App.Key // 兼容旧配置
		}
		
		if expectedKey == "" || expectedKey != apiKey {
			c.AbortWithStatusJSON(401, gin.H{
				"error": "invalid gateway api key",
			})
			return
		}
		c.Next()
	}
}

// AdminAuth 管理端口认证 - 用于依赖管理等管理操作
func AdminAuth() gin.HandlerFunc {
	config := static.GetDifySandboxGlobalConfigurations()
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-Api-Key")
		
		// 优先级：admin_key > key（向后兼容）
		expectedKey := config.App.AdminKey
		if expectedKey == "" {
			expectedKey = config.App.Key // 兼容旧配置
		}
		
		if expectedKey == "" || expectedKey != apiKey {
			c.AbortWithStatusJSON(401, gin.H{
				"error": "invalid admin api key",
			})
			return
		}
		c.Next()
	}
}

// Auth 通用认证（保持向后兼容）
func Auth() gin.HandlerFunc {
	return GatewayAuth() // 默认使用网关认证
}