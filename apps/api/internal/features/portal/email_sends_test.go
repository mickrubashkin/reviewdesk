package portal

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParseSendDays(t *testing.T) {
	for value, want := range map[string]int{"": defaultSendDays, "x": defaultSendDays, "0": defaultSendDays, "30": 30, "999": SendRetentionDays} {
		if got := parseSendDays(value); got != want {
			t.Errorf("parseSendDays(%q) = %d, want %d", value, got, want)
		}
	}
}

func TestLoadEmailSendsSplitsDaysByText(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for portal integration tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(db.Close)

	const board = "sends-test"
	cleanup := func() {
		_, _ = db.Exec(ctx, `DELETE FROM portal_email_groups WHERE board_key = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM portal_sent_emails WHERE board_key = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM emails WHERE slug = 'sends-test-email';`)
	}
	cleanup()
	t.Cleanup(cleanup)

	var emailID string
	if err := db.QueryRow(ctx, `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, subject)
		VALUES ('sends-test-email', 'Sends test', $1, 'test', 9990, 'en', 'new', '<p>x</p>', '<p>x</p>', 'Subject')
		RETURNING id;
	`, board).Scan(&emailID); err != nil {
		t.Fatal(err)
	}

	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, berlin)
	var activity int64 = 9100000
	send := func(fingerprint string, at time.Time) {
		t.Helper()
		activity++
		if _, err := db.Exec(ctx, `
			INSERT INTO portal_sent_emails (activity_id, board_key, deal_id, sent_at, body_html, fingerprint)
			VALUES ($1, $2, 1, $3, '', $4);
		`, activity, board, at, fingerprint); err != nil {
			t.Fatal(err)
		}
	}
	group := func(fingerprint, subject, decision string, firstSent time.Time) {
		t.Helper()
		if _, err := db.Exec(ctx, `
			INSERT INTO portal_email_groups (board_key, fingerprint, subject, sample_activity_id, first_sent_at, last_sent_at,
				match_email_id, match_status, decision, decision_email_id)
			VALUES ($1, $2, $3, 1, $4, $4, $5, 'matched', NULLIF($6, ''), CASE WHEN $6 = 'confirmed' THEN $5::uuid END);
		`, board, fingerprint, subject, firstSent, emailID, decision); err != nil {
			t.Fatal(err)
		}
	}

	group("old", "Old text", "", now.AddDate(0, 0, -100))
	group("new", "New text", "confirmed", now.AddDate(0, 0, -3))
	group("ignored", "Not this one", "ignored", now.AddDate(0, 0, -3))
	send("old", now.AddDate(0, 0, -10))
	send("old", time.Date(2026, 10, 18, 23, 30, 0, 0, berlin)) // 21:30 UTC: the 18th in Berlin
	send("old", now.AddDate(0, 0, -40))                       // before the period
	send("new", time.Date(2026, 10, 18, 9, 0, 0, 0, berlin))
	send("new", time.Date(2026, 10, 19, 0, 30, 0, 0, berlin)) // 22:30 UTC on the 18th: the 19th in Berlin
	send("ignored", now.AddDate(0, 0, -1))

	got, err := LoadEmailSends(ctx, db, emailID, 30, berlin, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Since != "2026-09-21" || got.Until != "2026-10-20" {
		t.Errorf("period = %s..%s", got.Since, got.Until)
	}
	if len(got.Texts) != 2 || got.Texts[0].Subject != "Old text" || got.Texts[0].Sends != 2 ||
		got.Texts[1].Subject != "New text" || got.Texts[1].Sends != 2 {
		t.Fatalf("texts = %+v", got.Texts)
	}
	old, newer := got.Texts[0].GroupID, got.Texts[1].GroupID
	want := []DailySends{
		{"2026-10-10", old, 1}, {"2026-10-18", old, 1},
		{"2026-10-18", newer, 1}, {"2026-10-19", newer, 1},
	}
	if len(got.Daily) != len(want) {
		t.Fatalf("daily = %+v", got.Daily)
	}
	for i := range want {
		if got.Daily[i] != want[i] {
			t.Errorf("daily[%d] = %+v, want %+v", i, got.Daily[i], want[i])
		}
	}
}
