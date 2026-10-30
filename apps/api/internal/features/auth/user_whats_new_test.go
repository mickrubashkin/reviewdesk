package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestUpdateUserWhatsNewKeepsTheNewestDate(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "reviewer")
	if _, err := dbpool.Exec(context.Background(), `UPDATE users SET whats_new_seen = '' WHERE id = $1;`, user.ID); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, nil)
	seen := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/users/me/whats-new", strings.NewReader(body))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, withAuthUser(request, user))
		return response
	}

	for _, body := range []string{`{"seen":""}`, `{"seen":"29 Oct"}`, `{"seen":"2026-13-01"}`} {
		if response := seen(body); response.Code != http.StatusBadRequest {
			t.Fatalf("expected %s to be rejected, got %d", body, response.Code)
		}
	}
	for _, step := range []struct {
		body string
		want string
	}{
		{`{"seen":"2026-10-29"}`, "2026-10-29"},
		{`{"seen":"2026-10-01"}`, "2026-10-29"},
	} {
		response := seen(step.body)
		if response.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
		}
		var got userWhatsNewResponse
		if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.WhatsNewSeen != step.want {
			t.Fatalf("after %s expected %q, got %q", step.body, step.want, got.WhatsNewSeen)
		}
	}
}
