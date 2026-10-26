package demo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVisitSource(t *testing.T) {
	cases := []struct {
		entry VisitEntry
		want  string
	}{
		{VisitEntry{}, ""},
		{VisitEntry{Referrer: "https://www.linkedin.com/"}, "linkedin"},
		{VisitEntry{Referrer: "android-app://com.linkedin.android/"}, "linkedin"},
		{VisitEntry{Referrer: "https://lnkd.in/abc"}, "linkedin"},
		{VisitEntry{Referrer: "https://news.ycombinator.com/item?id=1"}, "news.ycombinator.com"},
		{VisitEntry{Referrer: "https://www.google.com/", UTMSource: " Newsletter "}, "newsletter"},
		{VisitEntry{Referrer: "not a url ::"}, ""},
	}
	for _, c := range cases {
		if got := visitSource(c.entry); got != c.want {
			t.Errorf("visitSource(%+v) = %q, want %q", c.entry, got, c.want)
		}
	}
}

func TestCountVisits(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	// 23:30 UTC on the 25th is already the 26th in Madrid.
	late := time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC)
	visits := []Visit{
		{CreatedAt: late, Source: "linkedin", Mobile: true, Actions: map[string]int{"email_updated": 1, "comment_resolved": 1, "marked_ready": 1}},
		{CreatedAt: late.Add(time.Hour), Source: "linkedin", Actions: map[string]int{}},
		{CreatedAt: late.Add(-48 * time.Hour), Actions: map[string]int{"comment_created": 2, "ai_calls": 1}},
	}

	days, total := CountVisitsByDay(visits, madrid)
	if len(days) != 2 || days[0].Label != "2026-10-26 Mon" || days[0].Visitors != 2 || days[0].MarkedReady != 1 {
		t.Fatalf("unexpected days: %+v", days)
	}
	want := VisitCounts{Label: "Total", Visitors: 3, LinkedIn: 2, Mobile: 1, Active: 2, Edited: 1, Commented: 1, Resolved: 1, MarkedReady: 1, UsedAI: 1}
	if total != want {
		t.Fatalf("total = %+v, want %+v", total, want)
	}

	sources := CountVisitsBySource(visits)
	if len(sources) != 2 || sources[0].Label != "linkedin" || sources[0].Visitors != 2 || sources[1].Label != "direct" {
		t.Fatalf("unexpected sources: %+v", sources)
	}
}

func TestVisitsOutliveSandboxes(t *testing.T) {
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

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	visitor, err := CreateSandbox(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if err := RecordVisit(ctx, tx, visitor, VisitEntry{Referrer: "https://www.linkedin.com/", UserAgent: "Mozilla/5.0 (iPhone) Mobile/15E148"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM demo_visits WHERE visitor_email = $1;`, visitor.Email)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM demo_ai_calls WHERE visitor_email = $1;`, visitor.Email)
	})

	// The visitor edits, marks the hero email ready and asks the AI.
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO email_events (actor_user_id, actor_email, action, changes)
		VALUES
			($1, $2, 'email_updated', '{}'),
			($1, $2, 'email_review_status_updated', '{"review_status": {"before": "draft", "after": "ready"}}');
	`, visitor.ID, visitor.Email); err != nil {
		t.Fatal(err)
	}
	if err := ReserveAICall(ctx, dbpool, visitor.Email); err != nil {
		t.Fatal(err)
	}

	find := func() Visit {
		visits, err := ListVisits(ctx, dbpool, time.Now().Add(-time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		for _, visit := range visits {
			if visit.Source == "linkedin" && visit.Mobile && visit.Edited() {
				return visit
			}
		}
		t.Fatalf("visit of %s not listed in %+v", visitor.Email, visits)
		return Visit{}
	}
	check := func(stage string, visit Visit) {
		if !visit.MarkedReady() || !visit.UsedAI() || visit.Actions["email_updated"] != 1 {
			t.Fatalf("%s: unexpected actions %v", stage, visit.Actions)
		}
	}
	check("live sandbox", find())

	if _, err := DeleteExpiredSandboxes(ctx, dbpool, time.Now().Add(SandboxLifetime+time.Minute)); err != nil {
		t.Fatal(err)
	}
	var users int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id::text = $1;`, visitor.ID).Scan(&users); err != nil || users != 0 {
		t.Fatalf("expected the visitor deleted, %d left (%v)", users, err)
	}
	check("deleted sandbox", find())
}
