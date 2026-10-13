package routes

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func readOnlyRouteAuth() middleware.AdminAuthMiddleware {
	return middleware.AdminAuthMiddleware(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUserRole), service.RoleReadOnlyAdmin)
		c.Next()
	})
}

// Exercise the actual route groups, not a standalone copy of the guard. Every
// registered mutation must abort before it can reach its business handler.
func TestReadOnlyAdminBlocksRegisteredManagementMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := &handler.Handlers{Admin: &handler.AdminHandlers{}}
	audit := middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	stepUp := middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), h, readOnlyRouteAuth(), audit, stepUp, nil, nil)
	RegisterPaymentRoutes(router.Group("/api/v1"), nil, nil, nil, nil, readOnlyRouteAuth(), audit, nil, nil, nil)
	readPosts := map[string]bool{
		"/api/v1/admin/compliance/accept":                     true,
		"/api/v1/admin/dashboard/users-usage":                 true,
		"/api/v1/admin/dashboard/api-keys-usage":              true,
		"/api/v1/admin/accounts/check-mixed-channel":          true,
		"/api/v1/admin/accounts/usage/batch":                  true,
		"/api/v1/admin/accounts/today-stats/batch":            true,
		"/api/v1/admin/accounts/sync/crs/preview":             true,
		"/api/v1/admin/accounts/models/sync-upstream-preview": true,
		"/api/v1/admin/settings/email-template-preview":       true,
		"/api/v1/admin/user-attributes/batch":                 true,
		"/api/v1/admin/groups/:id/composite-routes/preview":   true,
	}
	param := regexp.MustCompile(`:[^/]+`)
	checked := 0
	for _, route := range router.Routes() {
		if !strings.HasPrefix(route.Path, "/api/v1/admin/") {
			continue
		}
		if route.Method == http.MethodGet || route.Method == http.MethodHead || route.Method == http.MethodOptions ||
			(route.Method == http.MethodPost && readPosts[route.Path]) {
			continue
		}
		checked++
		t.Run(route.Method+" "+route.Path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(route.Method, param.ReplaceAllString(route.Path, "1"), nil)
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusForbidden, recorder.Code)
			require.Contains(t, recorder.Body.String(), "READ_ONLY_ADMIN")
		})
	}
	require.Greater(t, checked, 100, "management and payment mutations must both be registered")
}
