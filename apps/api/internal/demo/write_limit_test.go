package demo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

func TestLimitVisitorWrites(t *testing.T) {
	handler := LimitVisitorWrites()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	visitor := coreauth.AuthUser{ID: "visitor-a", Email: visitorEmailPrefix + "limit-a" + visitorEmailDomain, Role: "reviewer"}
	other := coreauth.AuthUser{ID: "visitor-b", Email: visitorEmailPrefix + "limit-b" + visitorEmailDomain, Role: "reviewer"}
	admin := coreauth.AuthUser{ID: "admin", Email: "admin@example.com", Role: "admin"}

	send := func(user *coreauth.AuthUser, method, path, body string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if user != nil {
			r = r.WithContext(coreauth.WithUser(r.Context(), *user))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}

	for i := 0; i < visitorCreatesPerHour; i++ {
		if code := send(&visitor, http.MethodPost, "/api/emails/1/comments", "{}"); code != http.StatusNoContent {
			t.Fatalf("comment %d: got %d", i+1, code)
		}
	}
	if code := send(&visitor, http.MethodPost, "/api/emails/1/comments", "{}"); code != http.StatusTooManyRequests {
		t.Fatalf("comment over the limit: got %d, want 429", code)
	}

	// Edits and previews still go through, and so does everyone else.
	if code := send(&visitor, http.MethodPatch, "/api/emails/1/editable-fields", "{}"); code != http.StatusNoContent {
		t.Fatalf("edit after the create limit: got %d", code)
	}
	if code := send(&visitor, http.MethodPost, "/api/layout-preview", "{}"); code != http.StatusNoContent {
		t.Fatalf("preview after the create limit: got %d", code)
	}
	if code := send(&other, http.MethodPost, "/api/emails/1/comments", "{}"); code != http.StatusNoContent {
		t.Fatalf("another visitor was limited: got %d", code)
	}
	if code := send(&visitor, http.MethodGet, "/api/emails/1", ""); code != http.StatusNoContent {
		t.Fatalf("read was limited: got %d", code)
	}
	for i := 0; i < visitorWritesPerMinute+1; i++ {
		if code := send(&admin, http.MethodPost, "/api/emails/1/comments", "{}"); code != http.StatusNoContent {
			t.Fatalf("admin write %d was limited: got %d", i+1, code)
		}
	}
	if code := send(nil, http.MethodPost, "/api/auth/demo-login", "{}"); code != http.StatusNoContent {
		t.Fatalf("signed-out request was limited: got %d", code)
	}

	// All writes per minute, whatever they are.
	for i := 0; i < visitorWritesPerMinute; i++ {
		send(&other, http.MethodPatch, "/api/emails/1/editable-fields", "{}")
	}
	if code := send(&other, http.MethodPatch, "/api/emails/1/editable-fields", "{}"); code != http.StatusTooManyRequests {
		t.Fatalf("write over the per-minute limit: got %d, want 429", code)
	}

	third := coreauth.AuthUser{ID: "visitor-c", Email: visitorEmailPrefix + "limit-c" + visitorEmailDomain, Role: "reviewer"}
	big := strings.Repeat("x", visitorMaxRequestBytes+1)
	if code := send(&third, http.MethodPost, "/api/emails", big); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: got %d, want 413", code)
	}
}

func TestCreatesRows(t *testing.T) {
	cases := map[string]bool{
		"POST /api/emails/1/comments":                true,
		"POST /api/comments/1/messages":              true,
		"POST /api/emails":                           true,
		"POST /api/emails/1/duplicate":               true,
		"POST /api/boards/demo/stages":               true,
		"POST /api/emails/inspect-html":              false,
		"POST /api/layout-preview":                   false,
		"POST /api/blocks/footer/preview":            false,
		"POST /api/emails/1/translation/suggestions": false,
		"POST /api/emails/1/versions/2/restore":      false,
		"PATCH /api/emails/1/editable-fields":        false,
		"DELETE /api/boards/demo/stages/activation":  false,
	}
	for request, want := range cases {
		method, path, _ := strings.Cut(request, " ")
		if got := createsRows(httptest.NewRequest(method, path, nil)); got != want {
			t.Errorf("%s: got %v, want %v", request, got, want)
		}
	}
}
