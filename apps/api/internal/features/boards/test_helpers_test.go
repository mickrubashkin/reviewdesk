package boards

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

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

func createTestUserWithRole(t *testing.T, dbpool *pgxpool.Pool, role string) AuthUser {
	t.Helper()

	var user AuthUser
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO users (email, role)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET
			role = EXCLUDED.role,
			updated_at = now()
		RETURNING id, email, role;
	`, "email-handler-"+role+"@example.com", role).Scan(&user.ID, &user.Email, &user.Role)
	if err != nil {
		t.Fatalf("failed to create %s test user: %v", role, err)
	}

	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM users WHERE id = $1;`, user.ID)
	})

	return user
}

func withAuthUser(request *http.Request, user AuthUser) *http.Request {
	return request.WithContext(auth.WithUser(request.Context(), user))
}
