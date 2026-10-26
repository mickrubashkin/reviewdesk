package comments

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

// An archived email is history: its comments can be read, not added to.
func TestCommentsOnArchivedEmail(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUser(t, dbpool)
	emailID := createTestEmail(t, dbpool)

	router := chi.NewRouter()
	RegisterCommentRoutes(router, dbpool)
	post := func(path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
		request = request.WithContext(auth.WithUser(request.Context(), user))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	comment := `{"review_block": "body-001", "selected_text": "text", "start_offset": 0, "end_offset": 4, "severity": "issue", "body": "Why?"}`
	created := post("/api/emails/"+emailID+"/comments", comment)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var commentID string
	if err := dbpool.QueryRow(context.Background(), `SELECT id FROM comments WHERE email_id = $1;`, emailID).Scan(&commentID); err != nil {
		t.Fatal(err)
	}

	if _, err := dbpool.Exec(context.Background(), `UPDATE emails SET archived_at = now() WHERE id = $1;`, emailID); err != nil {
		t.Fatal(err)
	}
	if response := post("/api/emails/"+emailID+"/comments", comment); response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "archived") {
		t.Fatalf("a comment on an archived email must be refused: %d %s", response.Code, response.Body.String())
	}
	if response := post("/api/comments/"+commentID+"/messages", `{"body": "Because."}`); response.Code != http.StatusConflict {
		t.Fatalf("a reply on an archived email must be refused: %d %s", response.Code, response.Body.String())
	}
}
