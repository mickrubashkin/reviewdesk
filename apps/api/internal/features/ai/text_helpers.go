package ai

import (
	"strings"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
)

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPointerIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func htmlToText(value string) string {
	return emailtext.HTMLToText(value)
}

func limitText(value string, maxLength int) string {
	if len(value) <= maxLength {
		return value
	}

	return strings.TrimSpace(value[:maxLength])
}
