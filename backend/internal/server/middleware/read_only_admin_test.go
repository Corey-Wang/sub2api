//go:build unit

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReadOnlyAdminGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		role       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "readonly_admin_can_read",
			role:       service.RoleReadOnlyAdmin,
			method:     http.MethodGet,
			path:       "/api/v1/admin/users",
			wantStatus: http.StatusOK,
		},
		{
			name:       "readonly_admin_can_submit_read_query",
			role:       service.RoleReadOnlyAdmin,
			method:     http.MethodPost,
			path:       "/api/v1/admin/dashboard/users-usage",
			wantStatus: http.StatusOK,
		},
		{
			name:       "readonly_admin_cannot_create",
			role:       service.RoleReadOnlyAdmin,
			method:     http.MethodPost,
			path:       "/api/v1/admin/users",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "readonly_admin_cannot_update",
			role:       service.RoleReadOnlyAdmin,
			method:     http.MethodPut,
			path:       "/api/v1/admin/settings",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "readonly_admin_cannot_read_api_key_values",
			role:       service.RoleReadOnlyAdmin,
			method:     http.MethodGet,
			path:       "/api/v1/admin/users/42/api-keys",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "full_admin_is_unaffected",
			role:       service.RoleAdmin,
			method:     http.MethodPost,
			path:       "/api/v1/admin/users",
			wantStatus: http.StatusOK,
		},
		{
			name:       "user_is_unaffected",
			role:       service.RoleUser,
			method:     http.MethodPost,
			path:       "/api/v1/admin/users",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyUserRole), tc.role)
				c.Next()
			})
			router.Use(ReadOnlyAdminGuard())
			router.Handle(tc.method, tc.path, func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, nil)
			router.ServeHTTP(recorder, request)

			require.Equal(t, tc.wantStatus, recorder.Code)
			if tc.wantStatus == http.StatusForbidden {
				require.Contains(t, recorder.Body.String(), "READ_ONLY_ADMIN")
			}
		})
	}
}

func TestReadOnlyAdminRequestAllowedNormalizesPaths(t *testing.T) {
	require.True(t, readOnlyAdminRequestAllowed(http.MethodGet, "/api/v1/admin/users/?page=1"))
	require.True(t, readOnlyAdminRequestAllowed(
		http.MethodPost,
		"/api/v1/admin/groups/7/composite-routes/preview/",
	))
	require.False(t, readOnlyAdminRequestAllowed(
		http.MethodPost,
		"/api/v1/admin/groups/7/composite-routes",
	))
}

func TestReadOnlyAdminPolicyProtectsSideEffectsAndAllowsAcknowledgement(t *testing.T) {
	for _, path := range []string{
		"/api/v1/admin/accounts/1/claude/reset-credits",
		"/api/v1/admin/accounts/1/models",
		"/api/v1/admin/accounts/data",
		"/api/v1/admin/proxies/data",
		"/api/v1/admin/settings/admin-api-key",
		"/api/v1/admin/backups/1/download-url",
		"/api/v1/admin/groups/1/api-keys",
	} {
		require.False(t, readOnlyAdminRequestAllowed(http.MethodGet, path), path)
	}
	require.True(t, readOnlyAdminRequestAllowed(http.MethodGet, "/api/v1/admin/channels/pricing/sync-models"))
	require.True(t, readOnlyAdminRequestAllowed(http.MethodPost, "/api/v1/admin/compliance/accept"))
	require.False(t, readOnlyAdminRequestAllowed(http.MethodDelete, "/api/v1/admin/compliance/accept"))
}
