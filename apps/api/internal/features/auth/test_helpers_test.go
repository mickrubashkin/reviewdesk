package auth

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

func testDBPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for auth handler integration tests")
	}

	dbpool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("failed to create test db pool: %v", err)
	}

	t.Cleanup(func() { dbpool.Close() })
	return dbpool
}

func createTestUserWithRole(t *testing.T, dbpool *pgxpool.Pool, role string) coreauth.AuthUser {
	t.Helper()

	var user coreauth.AuthUser
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO users (email, role)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET updated_at = now()
		RETURNING id, email, role;
	`, "test-"+role+"@example.com", role).Scan(&user.ID, &user.Email, &user.Role)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM users WHERE id = $1;`, user.ID)
	})

	return user
}

func withAuthUser(r *http.Request, user coreauth.AuthUser) *http.Request {
	return r.WithContext(coreauth.WithUser(r.Context(), user))
}

// deleteTestUserByEmail removes a user a test is about to create, before the
// test and after it, so reruns against the same database start clean.
func deleteTestUserByEmail(t *testing.T, dbpool *pgxpool.Pool, email string) {
	t.Helper()

	remove := func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM auth_events WHERE email = $1;`, email)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM users WHERE email = $1;`, email)
	}
	remove()
	t.Cleanup(remove)
}
