package accountimport

import (
	"encoding/json"
	"fmt"
	"strings"
)

// NormalizeSellerRecords accepts the seller-side plaintext file: a single
// OpenAI/Codex account object, an array of those objects, or NDJSON.
func NormalizeSellerRecords(content string) ([]PlaintextAccount, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("empty account file")
	}
	values, err := decodeJSONStream(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse account file: %w", err)
	}
	var records []PlaintextAccount
	for _, value := range flattenSellerValues(values) {
		record, err := normalizeSellerValue(value)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no account records found")
	}
	return records, nil
}

func flattenSellerValues(values []any) []any {
	out := make([]any, 0, len(values))
	var walk func(any)
	walk = func(value any) {
		if arr, ok := value.([]any); ok {
			for _, item := range arr {
				walk(item)
			}
			return
		}
		out = append(out, value)
	}
	for _, value := range values {
		walk(value)
	}
	return out
}

func normalizeSellerValue(value any) (PlaintextAccount, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return PlaintextAccount{}, err
	}
	var record PlaintextAccount
	if err := json.Unmarshal(raw, &record); err != nil {
		return PlaintextAccount{}, fmt.Errorf("invalid account record: %w", err)
	}
	obj, _ := value.(map[string]any)
	if record.ID == "" && obj != nil {
		record.ID = firstString(obj, "id", "account_id", "chatgpt_account_id")
	}
	if record.Name == "" && obj != nil {
		record.Name = firstString(obj, "name", "alias", "label")
	}
	if record.Platform == "" {
		record.Platform = "openai"
	}
	if record.Type == "" {
		record.Type = "oauth"
	}
	if record.Version == 0 {
		record.Version = Version
	}
	if len(record.Credentials) == 0 && len(record.Session) == 0 && obj != nil {
		if session, ok := obj["tokens"].(map[string]any); ok {
			record.Session = obj
			_ = session
		} else if hasSellerTokenFields(obj) {
			record.Credentials = obj
		}
	}
	if err := record.Validate(); err != nil {
		return PlaintextAccount{}, err
	}
	return record, nil
}

func hasSellerTokenFields(obj map[string]any) bool {
	keys := []string{
		"access_token", "accessToken", "refresh_token", "refreshToken",
		"id_token", "idToken", "agent_private_key", "agentPrivateKey",
	}
	for _, key := range keys {
		if firstString(obj, key) != "" {
			return true
		}
	}
	return false
}

func firstString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			if s, ok := value.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					return s
				}
			}
		}
	}
	return ""
}
