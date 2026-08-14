package accountimport

import (
	"fmt"
	"strings"
)

// SafeDisplayName never uses an email as the account label. Seller-provided
// aliases are kept when they do not look like an email address.
func SafeDisplayName(alias, importID, accountID string, index int) string {
	alias = strings.TrimSpace(alias)
	if alias != "" && !looksLikeEmail(alias) {
		return alias
	}
	for _, candidate := range []string{importID, accountID} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && !looksLikeEmail(candidate) {
			return "ChatGPT " + shortLabel(candidate)
		}
	}
	return fmt.Sprintf("ChatGPT 导入账号 %d", index)
}

func looksLikeEmail(value string) bool {
	value = strings.TrimSpace(value)
	at := strings.IndexByte(value, '@')
	return at > 0 && at < len(value)-1 && !strings.Contains(value, " ")
}

func shortLabel(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:8] + "…"
}
