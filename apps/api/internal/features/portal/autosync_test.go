package portal

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNextAutoSync(t *testing.T) {
	cases := []struct{ now, want string }{
		{"2026-10-16T12:00:00Z", "2026-10-17T01:00:00Z"}, // afternoon → next night
		{"2026-10-16T00:30:00Z", "2026-10-16T01:00:00Z"}, // 02:30 UTC+2 → same night
		{"2026-10-16T01:00:00Z", "2026-10-17T01:00:00Z"}, // exactly 03:00 → the next one
		{"2026-10-31T23:00:00Z", "2026-11-01T01:00:00Z"}, // past local midnight
	}
	for _, c := range cases {
		now, _ := time.Parse(time.RFC3339, c.now)
		want, _ := time.Parse(time.RFC3339, c.want)
		if got := nextAutoSync(now); !got.Equal(want) {
			t.Errorf("nextAutoSync(%s) = %s, want %s", c.now, got.UTC().Format(time.RFC3339), c.want)
		}
	}
}

func TestSyncSettingsValidate(t *testing.T) {
	category := 10
	cases := []struct {
		settings SyncSettings
		ok       bool
	}{
		{SyncSettings{Days: 3}, true},
		{SyncSettings{AutoSync: true, CategoryID: &category, Days: 3}, true},
		{SyncSettings{AutoSync: true, Days: 3}, false},
		{SyncSettings{Days: 0}, false},
		{SyncSettings{Days: 366}, false},
	}
	for _, c := range cases {
		if err := c.settings.validate(); (err == nil) != c.ok {
			t.Errorf("validate(%+v) = %v, want ok=%v", c.settings, err, c.ok)
		}
	}
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for this integration test")
	}
	dbpool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dbpool.Close)
	return dbpool
}

func TestBeginRunAllowsOneRunningSyncPerBoard(t *testing.T) {
	dbpool := testPool(t)
	ctx := context.Background()
	const board = "autosync-test"
	cleanup := func() { _, _ = dbpool.Exec(ctx, `DELETE FROM portal_sync_runs WHERE board_key = $1;`, board) }
	cleanup()
	t.Cleanup(cleanup)
	options := SyncOptions{BoardKey: board, CategoryID: 10, Days: 3}

	first, err := BeginRun(ctx, dbpool, options, "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BeginRun(ctx, dbpool, options, AutoSyncBy); !errors.Is(err, ErrSyncRunning) {
		t.Fatalf("second run: got %v, want ErrSyncRunning", err)
	}

	// A run left behind by a stopped server stops blocking once it is old.
	if _, err := dbpool.Exec(ctx, `UPDATE portal_sync_runs SET started_at = now() - interval '2 hours' WHERE id = $1;`, first); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginRun(ctx, dbpool, options, AutoSyncBy); err != nil {
		t.Fatalf("run after a stale one: %v", err)
	}
	var status, message string
	if err := dbpool.QueryRow(ctx, `SELECT status, COALESCE(error, '') FROM portal_sync_runs WHERE id = $1;`, first).Scan(&status, &message); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || message != "interrupted" {
		t.Errorf("stale run: got %s/%q, want failed/interrupted", status, message)
	}
}

func TestSaveSyncSettings(t *testing.T) {
	dbpool := testPool(t)
	ctx := context.Background()
	const board = "autosync-settings-test"
	if _, err := dbpool.Exec(ctx, `INSERT INTO boards (key, name) VALUES ($1, 'Autosync test') ON CONFLICT DO NOTHING;`, board); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM boards WHERE key = $1;`, board) })

	if got, err := loadSyncSettings(ctx, dbpool, board); err != nil || got.AutoSync || got.Days != 3 {
		t.Fatalf("defaults: got %+v, %v", got, err)
	}
	category := 10
	if _, err := saveSyncSettings(ctx, dbpool, board, SyncSettings{AutoSync: true, CategoryID: &category, Days: 5}, "a@example.com"); err != nil {
		t.Fatal(err)
	}
	got, err := loadSyncSettings(ctx, dbpool, board)
	if err != nil || !got.AutoSync || got.CategoryID == nil || *got.CategoryID != 10 || got.Days != 5 {
		t.Fatalf("saved: got %+v, %v", got, err)
	}
	if _, err := saveSyncSettings(ctx, dbpool, "no-such-board", SyncSettings{Days: 3}, "a@example.com"); !errors.Is(err, errBoardNotFound) {
		t.Fatalf("unknown board: got %v, want errBoardNotFound", err)
	}
}
