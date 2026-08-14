package accountimport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func unmarshalStrict(data []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	return nil
}

// ParseImportDocuments accepts one upload/paste: a bundle, an array of
// bundles, a single item, an array of items, or NDJSON of any of those.
func ParseImportDocuments(content string) ([]Item, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty content", ErrUnsupportedBundle)
	}
	values, err := decodeJSONStream(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedBundle, err)
	}
	var items []Item
	for _, value := range values {
		parsed, err := itemsFromValue(value)
		if err != nil {
			return nil, err
		}
		items = append(items, parsed...)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: no encrypted items", ErrUnsupportedBundle)
	}
	return items, nil
}

func itemsFromValue(value any) ([]Item, error) {
	switch typed := value.(type) {
	case []any:
		var items []Item
		for _, item := range typed {
			parsed, err := itemsFromValue(item)
			if err != nil {
				return nil, err
			}
			items = append(items, parsed...)
		}
		return items, nil
	case map[string]any:
		if kind, _ := typed["kind"].(string); strings.TrimSpace(kind) == Kind {
			bundle, err := bundleFromMap(typed)
			if err != nil {
				return nil, err
			}
			if err := bundle.ValidateHeader(); err != nil {
				return nil, err
			}
			return bundle.Items, nil
		}
		item, err := itemFromMap(typed)
		if err != nil {
			return nil, err
		}
		return []Item{item}, nil
	default:
		return nil, fmt.Errorf("%w: unexpected JSON value", ErrUnsupportedBundle)
	}
}

func bundleFromMap(value map[string]any) (Bundle, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return Bundle{}, err
	}
	var bundle Bundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return Bundle{}, fmt.Errorf("%w: %v", ErrUnsupportedBundle, err)
	}
	return bundle, nil
}

func itemFromMap(value map[string]any) (Item, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return Item{}, err
	}
	var item Item
	if err := json.Unmarshal(raw, &item); err != nil {
		return Item{}, fmt.Errorf("%w: %v", ErrUnsupportedBundle, err)
	}
	if strings.TrimSpace(item.Ciphertext) == "" || strings.TrimSpace(item.Nonce) == "" || strings.TrimSpace(item.EphemeralPublicKey) == "" {
		return Item{}, fmt.Errorf("%w: item missing ciphertext fields", ErrUnsupportedBundle)
	}
	return item, nil
}

func decodeJSONStream(content string) ([]any, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	var values []any
	for {
		var value any
		err := decoder.Decode(&value)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("empty JSON")
	}
	return values, nil
}
