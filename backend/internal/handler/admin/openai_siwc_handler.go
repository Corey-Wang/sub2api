package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type siwcGenerateAuthURLRequest struct {
	ProxyID     *int64 `json:"proxy_id"`
	RedirectURI string `json:"redirect_uri"`
	AccountID   int64  `json:"account_id"`
	Reconsent   bool   `json:"reconsent"`
}

type siwcExchangeCallbackRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	CallbackURL string `json:"callback_url" binding:"required"`
}

func (h *OpenAIOAuthHandler) siwcService(c *gin.Context) *service.OpenAISIWCService {
	if h == nil || h.openaiOAuthService == nil || h.openaiOAuthService.SIWC() == nil {
		response.Error(c, http.StatusInternalServerError, "SIWC service is not configured")
		return nil
	}
	return h.openaiOAuthService.SIWC()
}

// GenerateSIWCAuthURL starts an official Sign in with ChatGPT authorization.
// POST /api/v1/admin/openai/siwc/generate-auth-url
func (h *OpenAIOAuthHandler) GenerateSIWCAuthURL(c *gin.Context) {
	svc := h.siwcService(c)
	if svc == nil {
		return
	}
	var req siwcGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = siwcGenerateAuthURLRequest{}
	}
	result, err := svc.GenerateAuthURL(c.Request.Context(), req.ProxyID, req.RedirectURI, req.AccountID, req.Reconsent)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// ExchangeSIWCCallback completes login from the pasted loopback callback URL.
// POST /api/v1/admin/openai/siwc/exchange-callback
func (h *OpenAIOAuthHandler) ExchangeSIWCCallback(c *gin.Context) {
	svc := h.siwcService(c)
	if svc == nil {
		return
	}
	var req siwcExchangeCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := svc.ExchangeCallback(c.Request.Context(), req.SessionID, req.CallbackURL)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// RevokeSIWCAccount revokes the stored refresh token.
// POST /api/v1/admin/openai/siwc/accounts/:id/revoke
func (h *OpenAIOAuthHandler) RevokeSIWCAccount(c *gin.Context) {
	svc := h.siwcService(c)
	if svc == nil {
		return
	}
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if err := svc.RevokeAccount(c.Request.Context(), accountID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "revoked"})
}
