package emails

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
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
			sequence,
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
			'onboarding',
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

func createTestBoard(t *testing.T, dbpool *pgxpool.Pool, key string, stages []string) string {
	t.Helper()
	stagesJSON, err := json.Marshal(stages)
	if err != nil {
		t.Fatalf("failed to encode test board stages: %v", err)
	}

	_, err = dbpool.Exec(context.Background(), `
		INSERT INTO boards (key, name, stages)
		VALUES ($1, $2, $3::jsonb)
		ON CONFLICT (key) DO UPDATE SET
			name = EXCLUDED.name,
			stages = EXCLUDED.stages,
			updated_at = now();
	`, key, key, stagesJSON)
	if err != nil {
		t.Fatalf("failed to create test board: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE metadata->>'board_key' = $1;`, key)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM boards WHERE key = $1;`, key)
	})

	return key
}

func createBoardTestEmail(t *testing.T, dbpool *pgxpool.Pool, sequence string, stage string) string {
	t.Helper()

	var emailID string
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO emails (
			slug,
			sequence,
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
			$1,
			$2,
			'Board Test Email',
			$3,
			9999,
			'en',
			'new',
			'<html><body>board test</body></html>',
			'<html><body><p data-review-block="body-001">board test</p></body></html>',
			'<html><body><p data-review-block="body-001">board test</p></body></html>',
			'{}'::jsonb
		)
		RETURNING id;
	`, sequence+"-"+stage+"-board-test-email", sequence, stage).Scan(&emailID)
	if err != nil {
		t.Fatalf("failed to create board test email: %v", err)
	}

	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, emailID)
	})

	return emailID
}
