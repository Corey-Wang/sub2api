package siwc

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// SanitizeResult is a Responses request after preview-limitation cleanup.
type SanitizeResult struct {
	Body     []byte
	Stripped []string
	Notes    []string
}

// SanitizeRequest forces store=false and stream=true, drops documented
// unsupported fields, wraps a string input as a user message, and rewrites
// system message items to developer.
func SanitizeRequest(body []byte) (*SanitizeResult, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("siwc sanitize: empty body")
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("siwc sanitize: body must be a JSON object")
	}
	if doc == nil {
		return nil, fmt.Errorf("siwc sanitize: body must be a JSON object")
	}
	result := &SanitizeResult{}
	for _, name := range UnsupportedFields {
		if _, ok := doc[name]; ok {
			delete(doc, name)
			result.Stripped = append(result.Stripped, name)
		}
	}
	if store, ok := doc["store"].(bool); !ok || store {
		result.Notes = append(result.Notes, "store forced to false")
	}
	if stream, ok := doc["stream"].(bool); !ok || !stream {
		result.Notes = append(result.Notes, "stream forced to true")
	}
	doc["store"] = false
	doc["stream"] = true
	if text, ok := doc["input"].(string); ok {
		doc["input"] = []any{map[string]any{"role": "user", "content": text}}
		result.Notes = append(result.Notes, "string input wrapped as a user message array")
	}
	if items, ok := doc["input"].([]any); ok {
		rewritten := false
		for _, item := range items {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := obj["role"].(string)
			itemType, hasType := obj["type"].(string)
			if role == "system" && (!hasType || itemType == "message") {
				obj["role"] = "developer"
				rewritten = true
			}
		}
		if rewritten {
			result.Notes = append(result.Notes, "system role -> developer")
		}
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	result.Body = encoded
	return result, nil
}
