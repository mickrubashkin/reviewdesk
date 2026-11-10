package emails

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestPlanningOwnerMustManageOrTranslate(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	emailID := createTestEmail(t, dbpool)
	if _, err := dbpool.Exec(ctx, `UPDATE emails SET language = 'es', sequence = 'owner-test' WHERE id = $1;`, emailID); err != nil {
		t.Fatal(err)
	}
	admin := createTestUserWithRole(t, dbpool, "admin")
	person := func(address string, languages []string, boards []string) {
		t.Helper()
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO users (email, role, languages, managed_boards) VALUES ($1, 'reviewer', $2, $3)
			ON CONFLICT (email) DO UPDATE SET languages = EXCLUDED.languages, managed_boards = EXCLUDED.managed_boards;
		`, address, languages, boards); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM users WHERE email = $1;`, address) })
	}
	person("owner-test-es@example.com", []string{"es"}, []string{})
	person("owner-test-de@example.com", []string{"de"}, []string{})
	person("owner-test-manager@example.com", []string{}, []string{"owner-test"})

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	setOwner := func(owner string) *httptest.ResponseRecorder {
		t.Helper()
		request := withAuthUser(httptest.NewRequest(http.MethodPatch, "/api/emails/"+emailID+"/planning-fields",
			bytes.NewReader([]byte(`{"owner_email":"`+owner+`"}`))), admin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	for owner, want := range map[string]int{
		"owner-test-es@example.com":      http.StatusOK,
		"owner-test-manager@example.com": http.StatusOK,
		admin.Email:                      http.StatusOK,
		"owner-test-de@example.com":      http.StatusBadRequest,
		"owner-test-nobody@example.com":  http.StatusBadRequest,
	} {
		if response := setOwner(owner); response.Code != want {
			t.Errorf("owner %s: got %d (%s), want %d", owner, response.Code, response.Body.String(), want)
		}
	}

	// An owner set before the rule keeps their place when other fields change.
	if _, err := dbpool.Exec(ctx, `UPDATE emails SET owner_email = 'owner-test-de@example.com' WHERE id = $1;`, emailID); err != nil {
		t.Fatal(err)
	}
	if response := setOwner("owner-test-de@example.com"); response.Code != http.StatusOK {
		t.Fatalf("keeping the owner returned %d: %s", response.Code, response.Body.String())
	}
}
