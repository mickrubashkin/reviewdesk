package demo

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/httprate"
	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

// Anyone can enter the demo, so a visitor's writes are capped on top of the
// AI quota and the entry limit. The limits sit far above what a person
// clicking through the board does; they stop scripts from filling the
// database before the sandbox is deleted. Counters live in memory, per API
// instance, and reset on restart.
const (
	visitorWritesPerMinute  = 240
	visitorWritesPerHour    = 2000
	visitorCreatesPerHour   = 60
	visitorMaxRequestBytes  = 2 << 20
	visitorWriteLimitPrefix = "visitor-write:"
)

// Writes that only render or read something, often fired while the visitor
// types; they count as writes but do not create rows.
var nonCreatingPostSuffixes = []string{
	"/inspect-html",
	"/layout-preview",
	"/preview",
	"/export",
	"/notifications/read",
	"/ai-analysis",
	"/translation/suggestions",
	"/confirm",
	"/restore",
	"/move",
}

// LimitVisitorWrites caps how many writes a demo visitor makes and how large
// their request bodies are. It runs after the auth middleware and leaves
// reads, signed-out requests, and every other user alone.
func LimitVisitorWrites() func(http.Handler) http.Handler {
	perMinute := httprate.NewRateLimiter(visitorWritesPerMinute, time.Minute)
	perHour := httprate.NewRateLimiter(visitorWritesPerHour, time.Hour)
	creates := httprate.NewRateLimiter(visitorCreatesPerHour, time.Hour)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := coreauth.FromRequest(r)
			if !ok || !IsVisitorEmail(user.Email) || !isWrite(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			key := visitorWriteLimitPrefix + user.ID
			if perMinute.OnLimit(w, r, key) || perHour.OnLimit(w, r, key) {
				http.Error(w, "too many changes in this demo copy, try again later", http.StatusTooManyRequests)
				return
			}
			if createsRows(r) && creates.OnLimit(w, r, key) {
				http.Error(w, "this demo copy has added enough for now, try again in an hour", http.StatusTooManyRequests)
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, visitorMaxRequestBytes)
			next.ServeHTTP(w, r)
		})
	}
}

func isWrite(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// createsRows reports whether a write adds something: comments, replies,
// emails, slots, stages, translations. Unknown POSTs count as creating.
func createsRows(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	for _, suffix := range nonCreatingPostSuffixes {
		if strings.HasSuffix(path, suffix) {
			return false
		}
	}
	return true
}
