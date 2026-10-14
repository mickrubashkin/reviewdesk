package comments

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

func TestCanResolveComment(t *testing.T) {
	authorID := "author-id"

	tests := []struct {
		name     string
		user     AuthUser
		authorID *string
		want     bool
	}{
		{name: "author", user: AuthUser{ID: authorID, Role: "reviewer"}, authorID: &authorID, want: true},
		{name: "other reviewer", user: AuthUser{ID: "other-id", Role: "reviewer"}, authorID: &authorID, want: false},
		{name: "admin", user: AuthUser{ID: "admin-id", Role: "admin"}, authorID: &authorID, want: true},
		{name: "super admin", user: AuthUser{ID: "super-id", Role: "super_admin"}, authorID: &authorID, want: true},
		{name: "reviewer on comment without author", user: AuthUser{ID: "other-id", Role: "reviewer"}, authorID: nil, want: false},
		{name: "admin on comment without author", user: AuthUser{ID: "admin-id", Role: "admin"}, authorID: nil, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := canResolveComment(test.user, test.authorID); got != test.want {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestResolveBlockingCommentRequiresAuthorOrAdmin(t *testing.T) {
	dbpool := testDBPool(t)
	author := createTestUser(t, dbpool)
	otherReviewer := createTestUserWithRole(t, dbpool, "comment-test-other@example.com", "reviewer")
	admin := createTestUserWithRole(t, dbpool, "comment-test-admin@example.com", "admin")
	emailID := createTestEmail(t, dbpool)

	router := chi.NewRouter()
	RegisterCommentRoutes(router, dbpool)

	commentID := createBlockingComment(t, router, emailID, author)

	if code := resolveCommentAs(router, commentID, otherReviewer); code != http.StatusForbidden {
		t.Fatalf("expected other reviewer to get 403, got %d", code)
	}

	var status string
	if err := dbpool.QueryRow(context.Background(), `SELECT status FROM comments WHERE id = $1;`, commentID).Scan(&status); err != nil {
		t.Fatalf("failed to read comment status: %v", err)
	}
	if status != "open" {
		t.Fatalf("expected comment to stay open after forbidden resolve, got %q", status)
	}

	if code := resolveCommentAs(router, commentID, admin); code != http.StatusOK {
		t.Fatalf("expected admin to resolve, got %d", code)
	}

	if code := resolveCommentAs(router, "00000000-0000-0000-0000-000000000000", admin); code != http.StatusNotFound {
		t.Fatalf("expected missing comment to return 404, got %d", code)
	}
}

func createBlockingComment(t *testing.T, router chi.Router, emailID string, author AuthUser) string {
	t.Helper()

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/comments",
		bytes.NewReader([]byte(`{
			"review_block": "body-001",
			"selected_text": "selected text",
			"start_offset": 0,
			"end_offset": 13,
			"severity": "blocking",
			"body": "Wrong currency for this market."
		}`)),
	)
	request = request.WithContext(auth.WithUser(request.Context(), author))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected comment to be created, got %d: %s", response.Code, response.Body.String())
	}

	var comment EmailComment
	if err := json.NewDecoder(response.Body).Decode(&comment); err != nil {
		t.Fatalf("failed to decode comment: %v", err)
	}
	return comment.ID
}

func resolveCommentAs(router chi.Router, commentID string, user AuthUser) int {
	request := httptest.NewRequest(http.MethodPatch, "/api/comments/"+commentID+"/resolve", nil)
	request = request.WithContext(auth.WithUser(request.Context(), user))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response.Code
}

func createTestUserWithRole(t *testing.T, dbpool *pgxpool.Pool, email string, role string) AuthUser {
	t.Helper()

	var user AuthUser
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO users (email, role)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET role = EXCLUDED.role, updated_at = now()
		RETURNING id, email, role;
	`, email, role).Scan(&user.ID, &user.Email, &user.Role)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM users WHERE id = $1;`, user.ID)
	})

	return user
}
