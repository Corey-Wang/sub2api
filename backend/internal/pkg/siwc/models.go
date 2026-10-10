package siwc

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FilterListedModels keeps catalog entries whose visibility is "list".
// The documented /v1/models payload is {"models":[{"slug":"...","visibility":"list"}]}.
func FilterListedModels(body []byte) ([]byte, error) {
	var doc struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("siwc models: %w", err)
	}
	if doc.Models == nil {
		return nil, fmt.Errorf("siwc models: missing models array")
	}
	kept := make([]map[string]any, 0, len(doc.Models))
	for _, model := range doc.Models {
		if strings.TrimSpace(stringField(model, "visibility")) != "list" {
			continue
		}
		if strings.TrimSpace(stringField(model, "slug")) == "" {
			continue
		}
		kept = append(kept, model)
	}
	return json.Marshal(map[string]any{"models": kept})
}
