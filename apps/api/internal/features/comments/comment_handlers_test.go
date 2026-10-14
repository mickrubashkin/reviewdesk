package comments

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

func TestCommentHandlersCreateListResolve(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUser(t, dbpool)
	emailID := createTestEmail(t, dbpool)

	router := chi.NewRouter()
	RegisterCommentRoutes(router, dbpool)

	getRequest := httptest.NewRequest(http.MethodGet, "/api/emails/"+emailID+"/comments", nil)
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)

	if getResponse.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d: %s", getResponse.Code, getResponse.Body.String())
	}

	var initialComments []EmailComment
	if err := json.NewDecoder(getResponse.Body).Decode(&initialComments); err != nil {
		t.Fatalf("expected no comments, got %d", len(initialComments))
	}

	createBody := []byte(`{
		"review_block": "body-001",
		"selected_text": "selected text",
		"start_offset": 0,
		"end_offset": 13,
		"severity": "blocking",
		"body": "Please clarify this sentence."
	}`)

	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/comments",
		bytes.NewReader(createBody),
	)
	createRequest = createRequest.WithContext(
		auth.WithUser(createRequest.Context(), user),
	)
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)

	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", createResponse.Code, createResponse.Body.String())
	}

	var createdComment EmailComment
	if err := json.NewDecoder(createResponse.Body).Decode(&createdComment); err != nil {
		t.Fatalf("failed to decode created comment: %v", err)
	}
	if createdComment.ID == "" {
		t.Fatalf("expected created commemt id")
	}
	if createdComment.AuthorEmail == nil || *createdComment.AuthorEmail != user.Email {
		t.Fatalf("expected author email %s, got %#v", user.Email, createdComment.AuthorEmail)
	}
	if createdComment.Status != "open" {
		t.Fatalf("expected open comment status, got %q", createdComment.Status)
	}
	if createdComment.Severity != "blocking" {
		t.Fatalf("expected blocking comment severity, got %q", createdComment.Severity)
	}
	if len(createdComment.Messages) != 1 {
		t.Fatalf("expected one initial message, got %d", len(createdComment.Messages))
	}
	if createdComment.Messages[0].Body != "Please clarify this sentence." {
		t.Fatalf("expected initial message body, got %q", createdComment.Messages[0].Body)
	}
	if createdComment.Messages[0].AuthorEmail == nil || *createdComment.Messages[0].AuthorEmail != user.Email {
		t.Fatalf("expected initial message author email %s, got %#v", user.Email, createdComment.Messages[0].AuthorEmail)
	}

	replyRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/comments/"+createdComment.ID+"/messages",
		bytes.NewReader([]byte(`{"body":"Agreed, let's tighten it."}`)),
	)
	replyRequest = replyRequest.WithContext(
		auth.WithUser(replyRequest.Context(), user),
	)
	replyResponse := httptest.NewRecorder()
	router.ServeHTTP(replyResponse, replyRequest)

	if replyResponse.Code != http.StatusCreated {
		t.Fatalf("expected reply POST status 201, got %d: %s", replyResponse.Code, replyResponse.Body.String())
	}

	var reply EmailCommentMessage
	if err := json.NewDecoder(replyResponse.Body).Decode(&reply); err != nil {
		t.Fatalf("failed to decode reply: %v", err)
	}
	if reply.CommentID != createdComment.ID {
		t.Fatalf("expected reply comment id %s, got %q", createdComment.ID, reply.CommentID)
	}
	if reply.Body != "Agreed, let's tighten it." {
		t.Fatalf("expected reply body, got %q", reply.Body)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/emails/"+emailID+"/comments", nil)
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)

	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", listResponse.Code, listResponse.Body.String())
	}

	var listedComments []EmailComment
	if err := json.NewDecoder(listResponse.Body).Decode(&listedComments); err != nil {
		t.Fatalf("failed to decode listed comments: %v", err)
	}
	if len(listedComments) != 1 {
		t.Fatalf("expected one listed comment, got %d", len(listedComments))
	}
	if listedComments[0].Severity != "blocking" {
		t.Fatalf("expected listed blocking comment severity, got %q", listedComments[0].Severity)
	}
	if len(listedComments[0].Messages) != 2 {
		t.Fatalf("expected two listed messages, got %d", len(listedComments[0].Messages))
	}
	if listedComments[0].Messages[0].Body != "Please clarify this sentence." ||
		listedComments[0].Messages[1].Body != "Agreed, let's tighten it." {
		t.Fatalf("expected listed messages in creation order, got %#v", listedComments[0].Messages)
	}

	unauthorizedReplyRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/comments/"+createdComment.ID+"/messages",
		bytes.NewReader([]byte(`{"body":"No auth."}`)),
	)
	unauthorizedReplyResponse := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedReplyResponse, unauthorizedReplyRequest)
	if unauthorizedReplyResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized reply status 401, got %d", unauthorizedReplyResponse.Code)
	}

	emptyReplyRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/comments/"+createdComment.ID+"/messages",
		bytes.NewReader([]byte(`{"body":"   "}`)),
	)
	emptyReplyRequest = emptyReplyRequest.WithContext(
		auth.WithUser(emptyReplyRequest.Context(), user),
	)
	emptyReplyResponse := httptest.NewRecorder()
	router.ServeHTTP(emptyReplyResponse, emptyReplyRequest)
	if emptyReplyResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected empty reply status 400, got %d", emptyReplyResponse.Code)
	}

	resolveRequest := httptest.NewRequest(
		http.MethodPatch,
		"/api/comments/"+createdComment.ID+"/resolve",
		nil,
	)
	resolveRequest = resolveRequest.WithContext(
		auth.WithUser(resolveRequest.Context(), user),
	)
	resolveResponse := httptest.NewRecorder()
	router.ServeHTTP(resolveResponse, resolveRequest)

	if resolveResponse.Code != http.StatusOK {
		t.Fatalf("expected PATCH status 200, got %d: %s", resolveResponse.Code, resolveResponse.Body.String())
	}

	var resolvedComment EmailComment
	if err := json.NewDecoder(resolveResponse.Body).Decode(&resolvedComment); err != nil {
		t.Fatalf("failed to decode resolved comment: %v", err)
	}
	if resolvedComment.Status != "resolved" {
		t.Fatalf("expected resolved status, got %q", resolvedComment.Status)
	}
	if resolvedComment.ResolvedAt == nil {
		t.Fatalf("expected resolved_at")
	}
	if resolvedComment.ResolvedBy == nil || *resolvedComment.ResolvedBy != user.ID {
		t.Fatalf("expected resolved_by %s, got %#v", user.ID, resolvedComment.ResolvedBy)
	}
	if resolvedComment.ResolvedByEmail == nil || *resolvedComment.ResolvedByEmail != user.Email {
		t.Fatalf("expected resolved_by_email %s, got %#v", user.Email, resolvedComment.ResolvedByEmail)
	}
	if len(resolvedComment.Messages) != 2 {
		t.Fatalf("expected resolved comment to include two messages, got %d", len(resolvedComment.Messages))
	}

	var commentEventCount int
	if err := dbpool.QueryRow(context.Background(), `
		SELECT count(*)::int
		FROM email_events
		WHERE email_id = $1
			AND action = ANY($2);
	`, emailID, []string{
		"comment_created",
		"comment_replied",
		"comment_resolved",
	}).Scan(&commentEventCount); err != nil {
		t.Fatalf("failed to count comment events: %v", err)
	}
	if commentEventCount != 3 {
		t.Fatalf("expected three comment email events, got %d", commentEventCount)
	}

	resolvedReplyRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/comments/"+createdComment.ID+"/messages",
		bytes.NewReader([]byte(`{"body":"Reopening by reply?"}`)),
	)
	resolvedReplyRequest = resolvedReplyRequest.WithContext(
		auth.WithUser(resolvedReplyRequest.Context(), user),
	)
	resolvedReplyResponse := httptest.NewRecorder()
	router.ServeHTTP(resolvedReplyResponse, resolvedReplyRequest)
	if resolvedReplyResponse.Code != http.StatusConflict {
		t.Fatalf("expected resolved reply status 409, got %d", resolvedReplyResponse.Code)
	}
}

func testDBPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for comment handler integration tests")
	}

	dbpool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("failed to create test db pool: %v", err)
	}

	t.Cleanup(dbpool.Close)
	return dbpool
}

func createTestUser(t *testing.T, dbpool *pgxpool.Pool) AuthUser {
	t.Helper()

	var user AuthUser
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO users (email, role)
		VALUES ('comment-test-user@example.com', 'reviewer')
		ON CONFLICT (email) DO UPDATE SET updated_at = now()
		RETURNING id, email, role;
	`).Scan(&user.ID, &user.Email, &user.Role)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM users WHERE id = $1;`, user.ID)
	})

	return user
}

func createTestEmail(t *testing.T, dbpool *pgxpool.Pool) string {
	t.Helper()

	var emailID string
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO emails (
			slug,
			title,
			stage,
			sort_order,
			language,
			variant,
			original_html,
			review_html,
			template_html,
			editable_fields
		)
		VALUES (
			'comment-test-email',
			'Comment Test Email',
			'test',
			9999,
			'en',
			'new',
			'<html><body>selected text</body></html>',
			'<html><body><p data-review-block="body-001">selected text</p></body></html>',
			'<html><body><p data-review-block="body-001">selected text</p></body></html>',
			'{}'::jsonb
		)
		ON CONFLICT (slug) DO UPDATE SET updated_at = now()
		RETURNING id;
	`).Scan(&emailID)
	if err != nil {
		t.Fatalf("failed to create test email: %v", err)
	}

	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, emailID)
	})

	return emailID
}
