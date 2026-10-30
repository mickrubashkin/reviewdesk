package digest

import (
	"context"
	"errors"
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
	err  error
}

func (s *fakeSender) Send(_ context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.sent = append(s.sent, text)
	return nil
}

func TestFormat(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 29, 0, 0, 0, 0, madrid)
	at := func(hour, minute int) time.Time { return time.Date(2026, 10, 29, hour, minute, 0, 0, madrid) }

	people := []Person{
		{Email: "sam@example.com", Name: "Sam", FirstAt: at(9, 12), LastAt: at(17, 40), Boards: map[string]map[string]int{
			"Onboarding":    {"email_updated": 3, "email_planning_updated": 1, "comment_created": 1, "comment_replied": 1, "email_placed": 2},
			"Regional": {"comment_resolved": 1, "email_uploaded": 2},
		}},
		{Email: "bob@example.com", FirstAt: at(11, 3), LastAt: at(11, 3), Boards: map[string]map[string]int{}},
	}
	want := strings.Join([]string{
		"ReviewDesk · Thu, Oct 29 · 2 people",
		"",
		"Sam · 09:12–17:40",
		"  Onboarding: edited 4 · commented 2 · other 2",
		"  Regional: resolved 1 · uploaded 2",
		"",
		"bob@example.com · 11:03",
		"  Looked around, changed nothing.",
	}, "\n")
	if got := Format(day, people, madrid); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}

	if got := Format(day, nil, madrid); got != "ReviewDesk · Thu, Oct 29\nNobody was in." {
		t.Fatalf("unexpected empty digest %q", got)
	}

	many := make([]Person, 300)
	for i := range many {
		many[i] = Person{Email: "someone@example.com", FirstAt: at(9, 0), LastAt: at(10, 0), Boards: map[string]map[string]int{}}
	}
	long := Format(day, many, madrid)
	if len([]rune(long)) > maxTextLength || !strings.Contains(long, " more.") {
		t.Fatalf("expected a cut digest within %d characters, got %d", maxTextLength, len([]rune(long)))
	}
}

func TestSendDue(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for digest tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	// A day far back, so that real rows in the test database do not mix in.
	loc := time.UTC
	day := time.Date(2001, 2, 3, 0, 0, 0, 0, loc)
	at := func(hour, minute int) time.Time {
		return day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	}

	const boardKey = "digest-test-board"
	var userID, emailID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (email, name, role) VALUES ('digest-test@example.com', 'Digest Tester', 'reviewer') RETURNING id;
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		ctx := context.Background()
		_, _ = db.Exec(ctx, `DELETE FROM email_events WHERE actor_email = 'digest-test@example.com';`)
		_, _ = db.Exec(ctx, `DELETE FROM emails WHERE sequence = $1;`, boardKey)
		_, _ = db.Exec(ctx, `DELETE FROM boards WHERE key = $1;`, boardKey)
		_, _ = db.Exec(ctx, `DELETE FROM users WHERE email = 'digest-test@example.com';`)
		_, _ = db.Exec(ctx, `DELETE FROM activity_digests WHERE day = $1;`, day.Format(time.DateOnly))
	}
	t.Cleanup(cleanup)
	if _, err := db.Exec(ctx, `INSERT INTO boards (key, name) VALUES ($1, 'Digest Board');`, boardKey); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO emails (title, slug, original_html, template_html, sequence)
		VALUES ('Digest email', 'digest-test-email', '<p>x</p>', '<p>x</p>', $1) RETURNING id;
	`, boardKey).Scan(&emailID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_presence (user_id, seen_at) VALUES ($1, $2), ($1, $3), ($1, $4);
	`, userID, at(9, 5), at(12, 0), at(1, 0).AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO email_events (actor_user_id, actor_email, action, email_id, created_at) VALUES
			($1, 'digest-test@example.com', 'email_updated', $2, $3),
			($1, 'digest-test@example.com', 'comment_created', $2, $4);
	`, userID, emailID, at(10, 0), at(16, 30)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO email_events (actor_user_id, actor_email, action, metadata, created_at)
		VALUES ($1, 'digest-test@example.com', 'board_stage_created', $2, $3);
	`, userID, `{"board_key": "`+boardKey+`"}`, at(16, 30)); err != nil {
		t.Fatal(err)
	}

	sender := &fakeSender{}
	morning := at(8, 0).AddDate(0, 0, 1)

	// Before the hour: nothing yet.
	if err := SendDue(ctx, db, sender, morning.Add(-time.Hour), loc, 8); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sent before the hour: %v", sender.sent)
	}

	// A failed send leaves the day to try again.
	sender.err = errors.New("chat down")
	if err := SendDue(ctx, db, sender, morning, loc, 8); err == nil {
		t.Fatal("expected the send error")
	}
	sender.err = nil

	for range 2 {
		if err := SendDue(ctx, db, sender, morning, loc, 8); err != nil {
			t.Fatal(err)
		}
	}
	want := strings.Join([]string{
		"ReviewDesk · Sat, Feb 3 · 1 person",
		"",
		"Digest Tester · 09:05–16:30",
		"  Digest Board: edited 1 · commented 1 · other 1",
	}, "\n")
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Fatalf("expected one digest\n%s\ngot %q", want, sender.sent)
	}

}
