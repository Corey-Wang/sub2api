package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminOnly 管理员权限中间件
// 必须在JWTAuth中间件之后使用
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := GetUserRoleFromContext(c)
		if !ok {
			AbortWithError(c, 401, "UNAUTHORIZED", "User not found in context")
			return
		}

		// AdminOnly 保留“完整管理员”语义；需要同时允许只读管理员的
		// 查询路由应使用 AdminAuth + ReadOnlyAdminGuard。
		if role != service.RoleAdmin {
			AbortWithError(c, 403, "FORBIDDEN", "Admin access required")
			return
		}

		c.Next()
	}
}
