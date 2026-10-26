package emails

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestBoardManagerManagesOnlyTheirBoard(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	own := createTestBoard(t, dbpool, "board-access-own", []string{"test"})
	other := createTestBoard(t, dbpool, "board-access-other", []string{"test"})
	ownEmail := createBoardTestEmail(t, dbpool, own, "test")
	otherEmail := createBoardTestEmail(t, dbpool, other, "test")
	manager := createTestUserWithRole(t, dbpool, "reviewer")
	if _, err := dbpool.Exec(ctx, `UPDATE users SET managed_boards = $2 WHERE id = $1;`, manager.ID, []string{own}); err != nil {
		t.Fatal(err)
	}

	for emailID, want := range map[string]bool{ownEmail: true, otherEmail: false} {
		allowed, err := CanEditEmail(ctx, dbpool, manager, emailID)
		if err != nil {
			t.Fatal(err)
		}
		if allowed != want {
			t.Fatalf("CanEditEmail(%s) = %v, want %v", emailID, allowed, want)
		}
	}

	router := chi.NewRouter()
	router.Patch("/api/emails/{id}/archive", archiveEmailHandler(dbpool))
	router.Patch("/api/slots", updateSlotHandler(dbpool))
	send := func(method, path, body string) int {
		request := withAuthUser(httptest.NewRequest(method, path, bytes.NewBufferString(body)), manager)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response.Code
	}

	if code := send(http.MethodPatch, "/api/emails/"+otherEmail+"/archive", ""); code != http.StatusForbidden {
		t.Fatalf("archiving on another board: expected 403, got %d", code)
	}
	if code := send(http.MethodPatch, "/api/slots", `{"sequence":"board-access-other","stage":"test","sort_order":9999,"title":"x"}`); code != http.StatusForbidden {
		t.Fatalf("editing a slot on another board: expected 403, got %d", code)
	}
	if code := send(http.MethodPatch, "/api/emails/"+ownEmail+"/archive", ""); code != http.StatusNoContent {
		t.Fatalf("archiving on the managed board: expected 204, got %d", code)
	}
}
