package emails

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

func TestDiffVersionTextListsOnlyTranslatableChanges(t *testing.T) {
	before := versionText{subject: "Welcome", preheader: "Start", fields: map[string]string{"intro": "Hi", "cta": "Go"}}
	after := versionText{subject: "Welcome!", preheader: "Start", fields: map[string]string{"intro": "Hi", "cta": "Go now", "outro": "Bye"}}

	changes := diffVersionText(before, after)

	got := map[string]translationChange{}
	for _, change := range changes {
		got[change.Key] = change
	}
	if len(changes) != 3 || got["subject"].After != "Welcome!" || got["cta"].Before != "Go" || got["outro"].After != "Bye" {
		t.Fatalf("unexpected changes: %+v", changes)
	}
	if len(diffVersionText(before, before)) != 0 {
		t.Error("identical text must not count as a change")
	}
}

func TestConfirmTranslationRequiresAdmin(t *testing.T) {
	router := chi.NewRouter()
	router.Post("/api/emails/{id}/translation/confirm", confirmTranslationHandler(nil))

	request := httptest.NewRequest(http.MethodPost, "/api/emails/x/translation/confirm", nil)
	request = request.WithContext(auth.WithUser(request.Context(), auth.AuthUser{ID: "r", Email: "r@example.com", Role: "reviewer"}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a reviewer, got %d", response.Code)
	}
}

func TestPlaceAndLinkRequireAdmin(t *testing.T) {
	router := chi.NewRouter()
	router.Patch("/api/emails/{id}/place", placeEmailHandler(nil))
	router.Patch("/api/emails/{id}/translation-link", translationLinkHandler(nil))

	for _, path := range []string{"/api/emails/x/place", "/api/emails/x/translation-link"} {
		request := httptest.NewRequest(http.MethodPatch, path, nil)
		request = request.WithContext(auth.WithUser(request.Context(), auth.AuthUser{ID: "r", Email: "r@example.com", Role: "reviewer"}))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s: expected 403 for a reviewer, got %d", path, response.Code)
		}
	}
}
