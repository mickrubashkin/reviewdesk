// Package appurl is the app's address, for links in messages sent outside it.
package appurl

import (
	"os"
	"strings"
)

// Get is AUTH_APP_URL, or the first allowed origin; empty when neither is set.
func Get() string {
	if value := strings.TrimSpace(os.Getenv("AUTH_APP_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	origins := strings.TrimSpace(os.Getenv("AUTH_ALLOWED_ORIGINS"))
	if origins == "" {
		origins = strings.TrimSpace(os.Getenv("CORS_ORIGIN"))
	}
	for _, origin := range strings.Split(origins, ",") {
		if origin = strings.TrimRight(strings.TrimSpace(origin), "/"); origin != "" {
			return origin
		}
	}
	return ""
}
