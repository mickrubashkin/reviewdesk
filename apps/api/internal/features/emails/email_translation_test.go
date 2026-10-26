package emails

import (
	"context"
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

func TestConfirmTranslationNeedsAdminOrTranslator(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool) // language en, not a translation
	reviewer := createTestUserWithRole(t, dbpool, "reviewer")
	router := chi.NewRouter()
	router.Post("/api/emails/{id}/translation/confirm", confirmTranslationHandler(dbpool))
	confirm := func() int {
		request := httptest.NewRequest(http.MethodPost, "/api/emails/"+emailID+"/translation/confirm", nil)
		request = request.WithContext(auth.WithUser(request.Context(), reviewer))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response.Code
	}

	if code := confirm(); code != http.StatusForbidden {
		t.Fatalf("expected 403 for a reviewer who does not translate EN, got %d", code)
	}
	if _, err := dbpool.Exec(context.Background(), `UPDATE users SET languages = '{en}' WHERE id = $1;`, reviewer.ID); err != nil {
		t.Fatal(err)
	}
	if code := confirm(); code != http.StatusNotFound {
		t.Fatalf("expected a translator to get past the check (404: not a translation), got %d", code)
	}
}

func TestPlaceAndLinkRequireBoardManager(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	reviewer := createTestUserWithRole(t, dbpool, "reviewer")
	router := chi.NewRouter()
	router.Patch("/api/emails/{id}/place", placeEmailHandler(dbpool))
	router.Patch("/api/emails/{id}/translation-link", translationLinkHandler(dbpool))

	for _, path := range []string{"/api/emails/" + emailID + "/place", "/api/emails/" + emailID + "/translation-link"} {
		request := httptest.NewRequest(http.MethodPatch, path, nil)
		request = request.WithContext(auth.WithUser(request.Context(), reviewer))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s: expected 403 for a reviewer, got %d", path, response.Code)
		}
	}
}
