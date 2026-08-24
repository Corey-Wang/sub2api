package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	readOnlyAdminErrorCode    = "READ_ONLY_ADMIN"
	readOnlyAdminErrorMessage = "Read-only administrators cannot modify administrative resources."
)

// ReadOnlyAdminGuard protects the administrative surface for the
// readonly_admin role. Full administrators and all non-admin requests are
// passed through unchanged; AdminAuth/AdminOnly remain responsible for
// authenticating and authorizing access to the admin surface.
//
// The default policy is deliberately deny-by-default for mutation methods.
// A small, explicit allowlist covers POST endpoints that are used only to
// submit complex read queries or previews. Sensitive credential/export reads
// are denied even though they use GET.
func ReadOnlyAdminGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := GetUserRoleFromContext(c)
		if !ok || role != service.RoleReadOnlyAdmin {
			c.Next()
			return
		}

		if readOnlyAdminRequestAllowed(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}

		AbortWithError(c, http.StatusForbidden, readOnlyAdminErrorCode, readOnlyAdminErrorMessage)
	}
}

func readOnlyAdminRequestAllowed(method, path string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = normalizeReadOnlyAdminPath(path)

	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return !isReadOnlyAdminSensitiveRead(path)
	case http.MethodPost:
		return isReadOnlyAdminReadPost(path)
	default:
		return false
	}
}

func normalizeReadOnlyAdminPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}

// These endpoints are reads from an HTTP perspective but can disclose
// credentials, bearer tokens, API keys, or complete backup material.
func isReadOnlyAdminSensitiveRead(path string) bool {
	switch {
	case path == "/api/v1/admin/accounts/data":
		return true
	case path == "/api/v1/admin/proxies/data":
		return true
	case path == "/api/v1/admin/redeem-codes/export":
		return true
	case path == "/api/v1/admin/settings/admin-api-key":
		return true
	case path == "/api/v1/admin/backups" || strings.HasPrefix(path, "/api/v1/admin/backups/"):
		return strings.HasSuffix(path, "/download-url")
	case strings.HasPrefix(path, "/api/v1/admin/users/") && strings.HasSuffix(path, "/api-keys"):
		return true
	case strings.HasPrefix(path, "/api/v1/admin/groups/") && strings.HasSuffix(path, "/api-keys"):
		return true
	default:
		return false
	}
}

// POST is used for a handful of admin queries because their request payloads
// are too complex for query parameters. Keep this list narrow and explicit:
// adding a new POST read endpoint requires an intentional policy decision.
func isReadOnlyAdminReadPost(path string) bool {
	switch path {
	case "/api/v1/admin/dashboard/users-usage",
		"/api/v1/admin/dashboard/api-keys-usage",
		"/api/v1/admin/accounts/check-mixed-channel",
		"/api/v1/admin/accounts/usage/batch",
		"/api/v1/admin/accounts/today-stats/batch",
		"/api/v1/admin/accounts/sync/crs/preview",
		"/api/v1/admin/accounts/models/sync-upstream-preview",
		"/api/v1/admin/settings/email-template-preview",
		"/api/v1/admin/user-attributes/batch":
		return true
	default:
		return strings.HasPrefix(path, "/api/v1/admin/groups/") &&
			strings.HasSuffix(path, "/composite-routes/preview")
	}
}
