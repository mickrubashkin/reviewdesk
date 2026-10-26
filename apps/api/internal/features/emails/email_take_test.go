package emails

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestTakeEmailByTranslator(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool) // language en
	translator := createTestUserWithRole(t, dbpool, "reviewer")
	other := createTestUser(t, dbpool)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	take := func(user AuthUser, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPatch, "/api/emails/"+emailID+"/take", bytes.NewReader([]byte(body)))
		request = withAuthUser(request, user)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	owner := func() string {
		var value *string
		if err := dbpool.QueryRow(context.Background(), `SELECT owner_email FROM emails WHERE id = $1;`, emailID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return stringFromPointer(value)
	}

	if response := take(translator, `{"take":true}`); response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a language the user does not translate, got %d", response.Code)
	}

	if _, err := dbpool.Exec(context.Background(), `UPDATE users SET languages = '{en}' WHERE id = $1;`, translator.ID); err != nil {
		t.Fatal(err)
	}
	if response := take(translator, `{"take":true}`); response.Code != http.StatusOK {
		t.Fatalf("expected 200 on take, got %d: %s", response.Code, response.Body.String())
	}
	if owner() != translator.Email {
		t.Fatalf("expected owner %q, got %q", translator.Email, owner())
	}

	if _, err := dbpool.Exec(context.Background(), `UPDATE users SET languages = '{en}' WHERE id = $1;`, other.ID); err != nil {
		t.Fatal(err)
	}
	if response := take(other, `{"take":false}`); response.Code != http.StatusConflict {
		t.Fatalf("expected 409 when giving back someone else's email, got %d", response.Code)
	}

	if response := take(translator, `{"take":false}`); response.Code != http.StatusOK {
		t.Fatalf("expected 200 on give back, got %d: %s", response.Code, response.Body.String())
	}
	if owner() != "" {
		t.Fatalf("expected no owner after giving back, got %q", owner())
	}
}
