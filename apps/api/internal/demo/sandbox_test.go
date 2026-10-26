package demo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

func TestSandboxesAreScopedAndExpire(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for demo sandbox tests")
	}
	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dbpool.Close)

	create := func() Visitor {
		tx, err := dbpool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		visitor, err := CreateSandbox(ctx, tx)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return visitor
	}
	first, second := create(), create()
	t.Cleanup(func() {
		tx, err := dbpool.Begin(context.Background())
		if err != nil {
			return
		}
		_, _ = deleteSandboxes(context.Background(), tx, []string{first.BoardKey, second.BoardKey})
		_ = tx.Commit(context.Background())
	})

	visible, scoped, err := auth.ScopedBoards(ctx, dbpool, auth.AuthUser{ID: first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !scoped || len(visible) != 1 || visible[0] != first.BoardKey {
		t.Fatalf("expected the visitor to see only their board %s, got %v (scoped=%v)", first.BoardKey, visible, scoped)
	}
	var slugs int
	if err := dbpool.QueryRow(ctx, `
		SELECT count(*) FROM emails WHERE sequence = $1 AND slug LIKE $1 || '-%';
	`, second.BoardKey).Scan(&slugs); err != nil || slugs == 0 {
		t.Fatalf("expected the second board's emails to carry its key in their slugs, got %d (%v)", slugs, err)
	}

	// Nothing expires within the day; after it both go, with their visitors.
	if count, err := DeleteExpiredSandboxes(ctx, dbpool, time.Now()); err != nil || count != 0 {
		t.Fatalf("expected no expired sandboxes yet, removed %d (%v)", count, err)
	}
	if _, err := DeleteExpiredSandboxes(ctx, dbpool, time.Now().Add(SandboxLifetime+time.Minute)); err != nil {
		t.Fatal(err)
	}
	var boards, users int
	if err := dbpool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM boards WHERE key = ANY($1)), (SELECT count(*) FROM users WHERE id::text = ANY($2));
	`, []string{first.BoardKey, second.BoardKey}, []string{first.ID, second.ID}).Scan(&boards, &users); err != nil {
		t.Fatal(err)
	}
	if boards != 0 || users != 0 {
		t.Fatalf("expected the expired sandboxes gone, %d boards and %d visitors left", boards, users)
	}
}
