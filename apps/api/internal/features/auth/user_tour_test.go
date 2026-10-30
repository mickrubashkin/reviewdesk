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

func TestUpdateUserTourKeepsTheHighestVersion(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "reviewer")
	if _, err := dbpool.Exec(context.Background(), `UPDATE users SET tour_version = 0 WHERE id = $1;`, user.ID); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, nil)
	finish := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/users/me/tour", strings.NewReader(body))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, withAuthUser(request, user))
		return response
	}

	if response := finish(`{"version":0}`); response.Code != http.StatusBadRequest {
		t.Fatalf("expected version 0 to be rejected, got %d", response.Code)
	}
	for _, step := range []struct {
		body string
		want int
	}{
		{`{"version":2}`, 2},
		{`{"version":1}`, 2},
	} {
		response := finish(step.body)
		if response.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
		}
		var got userTourResponse
		if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.TourVersion != step.want {
			t.Fatalf("after %s expected tour version %d, got %d", step.body, step.want, got.TourVersion)
		}
	}
}
