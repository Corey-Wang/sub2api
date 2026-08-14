package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/accountimport"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type EncryptedAccountImportRequest struct {
	Content                 string          `json:"content"`
	Contents                []string        `json:"contents"`
	Bundle                  json.RawMessage `json:"bundle"`
	Name                    string          `json:"name"`
	Notes                   *string         `json:"notes"`
	GroupIDs                []int64         `json:"group_ids"`
	ProxyID                 *int64          `json:"proxy_id"`
	Concurrency             *int            `json:"concurrency"`
	Priority                *int            `json:"priority"`
	RateMultiplier          *float64        `json:"rate_multiplier"`
	LoadFactor              *int            `json:"load_factor"`
	ExpiresAt               *int64          `json:"expires_at"`
	AutoPauseOnExpired      *bool           `json:"auto_pause_on_expired"`
	Extra                   map[string]any  `json:"extra"`
	UpdateExisting          *bool           `json:"update_existing"`
	SkipDefaultGroupBind    *bool           `json:"skip_default_group_bind"`
	ConfirmMixedChannelRisk *bool           `json:"confirm_mixed_channel_risk"`
}

func (h *AccountHandler) GetEncryptedImportPublicKey(c *gin.Context) {
	pub, err := h.accountImportPublicKey()
	if err != nil {
		response.Success(c, accountimport.PublicKeyInfo{
			Configured: false,
			Kind:       accountimport.Kind,
			Version:    accountimport.Version,
			Alg:        accountimport.Algorithm,
		})
		return
	}
	response.Success(c, pub.Info())
}

func (h *AccountHandler) ImportEncryptedAccounts(c *gin.Context) {
	var req EncryptedAccountImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if h.accountImportKey == nil {
		response.BadRequest(c, "未配置账号导入私钥。请设置 ACCOUNT_IMPORT_PRIVATE_KEY 后重启服务。")
		return
	}
	if err := service.ValidateOpenAILongContextBillingExtra(service.PlatformOpenAI, req.Extra); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	items, err := parseEncryptedImportItems(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if len(items) == 0 {
		response.BadRequest(c, "请上传或粘贴加密账号包")
		return
	}

	executeAdminIdempotentJSON(c, "admin.accounts.import_encrypted", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importEncryptedAccounts(ctx, req, items)
	})
}

func (h *AccountHandler) importEncryptedAccounts(ctx context.Context, req EncryptedAccountImportRequest, items []accountimport.Item) (CodexSessionImportResult, error) {
	result := CodexSessionImportResult{
		Total: len(items),
		Items: make([]CodexSessionImportItem, 0, len(items)),
	}
	pub, err := h.accountImportPublicKey()
	if err != nil {
		return result, err
	}
	kid := pub.KID()

	entries := make([]codexImportEntry, 0, len(items))
	itemByIndex := map[int]accountimport.PlaintextAccount{}
	for i, item := range items {
		index := i + 1
		record, decErr := h.accountImportKey.Decrypt(item)
		if decErr != nil {
			slog.Warn("encrypted account import item rejected", "index", index, "item_id", item.ID, "err", decErr.Error())
			result.Failed++
			message := decryptFailureMessage(decErr)
			result.Items = append(result.Items, CodexSessionImportItem{
				Index:   index,
				Name:    accountimport.SafeDisplayName("", item.ID, "", index),
				Action:  "failed",
				Message: message,
			})
			result.Errors = append(result.Errors, CodexSessionImportMessage{
				Index:   index,
				Name:    item.ID,
				Message: message,
			})
			continue
		}
		itemByIndex[index] = record
		value := record.CodexValue()
		if meta, ok := value[accountimport.EncryptedImportMetaKey].(map[string]any); ok {
			meta["kid"] = kid
		}
		entries = append(entries, codexImportEntry{Index: index, Value: value})
	}
	if len(entries) == 0 {
		return result, nil
	}

	codexReq := CodexSessionImportRequest{
		Name:                    req.Name,
		Notes:                   req.Notes,
		GroupIDs:                req.GroupIDs,
		ProxyID:                 req.ProxyID,
		Concurrency:             req.Concurrency,
		Priority:                req.Priority,
		RateMultiplier:          req.RateMultiplier,
		LoadFactor:              req.LoadFactor,
		ExpiresAt:               req.ExpiresAt,
		AutoPauseOnExpired:      req.AutoPauseOnExpired,
		Extra:                   mergeCodexImportMap(req.Extra, map[string]any{}),
		UpdateExisting:          req.UpdateExisting,
		SkipDefaultGroupBind:    req.SkipDefaultGroupBind,
		ConfirmMixedChannelRisk: req.ConfirmMixedChannelRisk,
	}
	imported, err := h.importCodexSessions(ctx, codexReq, entries)
	if err != nil {
		return result, err
	}

	for i := range imported.Items {
		item := &imported.Items[i]
		record, ok := itemByIndex[item.Index]
		if !ok {
			continue
		}
		safeName := accountimport.SafeDisplayName(encryptedImportFirstNonEmpty(req.Name, record.Name), record.ID, "", item.Index)
		if item.Name == "" || encryptedImportLooksLikeEmail(item.Name) {
			item.Name = safeName
		}
	}

	result.Created += imported.Created
	result.Updated += imported.Updated
	result.Skipped += imported.Skipped
	result.Failed += imported.Failed
	result.Items = append(result.Items, imported.Items...)
	result.Warnings = append(result.Warnings, imported.Warnings...)
	result.Errors = append(result.Errors, imported.Errors...)
	return result, nil
}

func (h *AccountHandler) accountImportPublicKey() (*accountimport.PublicKey, error) {
	if h == nil || h.accountImportKey == nil {
		return nil, accountimport.ErrNotConfigured
	}
	return h.accountImportKey.Public()
}

func parseEncryptedImportItems(req EncryptedAccountImportRequest) ([]accountimport.Item, error) {
	contents := make([]string, 0, 2+len(req.Contents))
	if len(req.Bundle) > 0 && strings.TrimSpace(string(req.Bundle)) != "" && string(req.Bundle) != "null" {
		contents = append(contents, string(req.Bundle))
	}
	if strings.TrimSpace(req.Content) != "" {
		contents = append(contents, req.Content)
	}
	for _, content := range req.Contents {
		if strings.TrimSpace(content) != "" {
			contents = append(contents, content)
		}
	}
	var items []accountimport.Item
	for _, content := range contents {
		parsed, err := accountimport.ParseImportDocuments(content)
		if err != nil {
			return nil, err
		}
		items = append(items, parsed...)
	}
	return items, nil
}

func decryptFailureMessage(err error) string {
	if err == nil {
		return "解密失败"
	}
	if err == accountimport.ErrUnsupportedRecord {
		return "解密成功但账号类型不受支持（仅支持 OpenAI/ChatGPT OAuth）"
	}
	return "解密失败：密钥不匹配或数据已被篡改"
}

func encryptedImportFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func encryptedImportLooksLikeEmail(value string) bool {
	value = strings.TrimSpace(value)
	at := strings.IndexByte(value, '@')
	return at > 0 && at < len(value)-1 && !strings.Contains(value, " ")
}
