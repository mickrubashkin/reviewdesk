package demo

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeSender struct {
	mu   sync.Mutex
	sent []string
}

func (s *fakeSender) Send(_ context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, text)
	return nil
}

func TestVisitReport(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	entered := time.Date(2026, 10, 30, 9, 20, 0, 0, time.UTC)
	left := entered.Add(12*time.Minute + 20*time.Second)

	busy := Visit{CreatedAt: entered, LastSeenAt: &left, Source: "linkedin", Mobile: true, Actions: map[string]int{
		"email_updated": 2, "comment_created": 1, "comment_replied": 2, "marked_ready": 1, "ai_calls": 1, "email_placed": 1,
	}}
	want := "Demo visit · linkedin · mobile\nFri 10:20, about 12 min\nEdited 2 · commented 3 · used AI 1 · other 1 · marked an email ready"
	if got := VisitReport(busy, madrid); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}

	idle := Visit{CreatedAt: entered, LastSeenAt: &entered, Actions: map[string]int{}}
	want = "Demo visit · direct · desktop\nFri 10:20, a moment\nLooked around, changed nothing."
	if got := VisitReport(idle, madrid); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}

	ready := Visit{CreatedAt: entered, Actions: map[string]int{"marked_ready": 1}}
	if got := VisitReport(ready, madrid); !strings.HasSuffix(got, "\nMarked an email ready") {
		t.Fatalf("unexpected report %q", got)
	}
}

func TestVisitAnnouncement(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	entered := time.Date(2026, 10, 30, 9, 20, 0, 0, time.UTC)
	first := time.Date(2026, 10, 28, 23, 30, 0, 0, time.UTC)

	for _, test := range []struct {
		visit Visit
		want  string
	}{
		{Visit{CreatedAt: entered, Number: 1, FirstAt: entered}, "Demo: new visitor · direct · desktop"},
		{
			Visit{CreatedAt: entered, Source: "linkedin", TimeZone: "Europe/Warsaw", Language: "pl-PL", Number: 1, FirstAt: entered},
			"Demo: new visitor · linkedin · desktop · Europe/Warsaw, pl-PL",
		},
		{
			Visit{CreatedAt: entered, Mobile: true, Language: "de", Number: 3, FirstAt: first},
			"Demo: returning visitor (visit 3, first Oct 29) · direct · mobile · de",
		},
		{Visit{CreatedAt: entered, Owner: true, Number: 7, FirstAt: first}, "Demo: you · direct · desktop"},
	} {
		if got := VisitAnnouncement(test.visit, madrid); got != test.want {
			t.Errorf("got %q, want %q", got, test.want)
		}
	}

	returning := Visit{CreatedAt: entered, Owner: true, TimeZone: "Europe/Madrid", Number: 2, FirstAt: first, Actions: map[string]int{}}
	want := "Demo visit (you) · direct · desktop · Europe/Madrid · visit 2, first Oct 29"
	if got := VisitReport(returning, madrid); !strings.HasPrefix(got, want+"\n") {
		t.Fatalf("got %q, want it to start with %q", got, want)
	}
}

func TestReportQuietVisits(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for demo visit tests")
	}
	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dbpool.Close)

	sender := &fakeSender{}
	Notifier = sender
	t.Cleanup(func() { Notifier = nil })

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	visitor, err := CreateSandbox(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecordVisit(ctx, tx, visitor, VisitEntry{UTMSource: "report-test", VisitorID: "report-test-browser", TimeZone: "Europe/Warsaw"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = deleteSandboxesNow(dbpool, visitor.BoardKey)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM demo_visits WHERE visitor_email = $1;`, visitor.Email)
	})
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO email_events (actor_user_id, actor_email, action, changes) VALUES ($1, $2, 'email_updated', '{}');
	`, visitor.ID, visitor.Email); err != nil {
		t.Fatal(err)
	}
	ours := func() []string {
		found := []string{}
		for _, text := range sender.sent {
			if strings.Contains(text, "report-test") {
				found = append(found, text)
			}
		}
		return found
	}

	// Still active: no report.
	if err := ReportQuietVisits(ctx, dbpool, time.Now(), time.UTC); err != nil {
		t.Fatal(err)
	}
	if got := ours(); len(got) != 0 {
		t.Fatalf("reported an active visit: %v", got)
	}

	// Quiet: one report, and only once.
	later := time.Now().Add(visitQuietAfter + time.Minute)
	for range 2 {
		if err := ReportQuietVisits(ctx, dbpool, later, time.UTC); err != nil {
			t.Fatal(err)
		}
	}
	got := ours()
	if len(got) != 1 || !strings.Contains(got[0], "Edited 1") || !strings.Contains(got[0], "Europe/Warsaw") {
		t.Fatalf("unexpected reports %v", got)
	}
}

func deleteSandboxesNow(db *pgxpool.Pool, key string) (int, error) {
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	count, err := deleteSandboxes(ctx, tx, []string{key})
	if err != nil {
		return 0, err
	}
	return count, tx.Commit(ctx)
}
