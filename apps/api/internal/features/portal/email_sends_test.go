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
	send("old", now.AddDate(0, 0, -40))                        // before the period
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

func TestLoadEmailSendsCountsStagesAndDeals(t *testing.T) {
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

	const board = "sends-stage-test"
	cleanup := func() {
		_, _ = db.Exec(ctx, `DELETE FROM portal_email_groups WHERE board_key = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM portal_sent_emails WHERE board_key = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM portal_stages WHERE board_key = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM emails WHERE sequence = $1;`, board)
	}
	cleanup()
	t.Cleanup(cleanup)

	email := func(slug, stage string) string {
		t.Helper()
		var id string
		if err := db.QueryRow(ctx, `
			INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, subject)
			VALUES ($1, $1, $2, $3, 9990, 'en', 'new', '<p>x</p>', '<p>x</p>', 'Subject')
			RETURNING id;
		`, slug, board, stage).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	intro, later := email("sends-stage-intro", "intro"), email("sends-stage-later", "later")
	for i, name := range []string{"Start", "Next", "Odd"} {
		if _, err := db.Exec(ctx, `INSERT INTO portal_stages (board_key, stage_id, name, sort) VALUES ($1, $2, $3, $4);`,
			board, "S"+name, name, (i+1)*10); err != nil {
			t.Fatal(err)
		}
	}
	group := func(fingerprint, emailID string) {
		t.Helper()
		if _, err := db.Exec(ctx, `
			INSERT INTO portal_email_groups (board_key, fingerprint, sample_activity_id, first_sent_at, last_sent_at, match_email_id, match_status)
			VALUES ($1, $2, 1, now(), now(), $3, 'matched');
		`, board, fingerprint, emailID); err != nil {
			t.Fatal(err)
		}
	}
	group("intro-a", intro)
	group("intro-b", intro)
	group("later", later)

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	var activity int64 = 9200000
	send := func(fingerprint string, deal int64, stage string, at time.Time) {
		t.Helper()
		activity++
		if _, err := db.Exec(ctx, `
			INSERT INTO portal_sent_emails (activity_id, board_key, deal_id, sent_at, body_html, fingerprint, stage_id)
			VALUES ($1, $2, $3, $4, '', $5, NULLIF($6, ''));
		`, activity, board, deal, at, fingerprint, stage); err != nil {
			t.Fatal(err)
		}
	}
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }
	// "later" owns Next and Odd; "intro" owns Start.
	send("later", 10, "SNext", day(5))
	send("later", 11, "SNext", day(5))
	send("later", 12, "SNext", day(5))
	send("later", 13, "SOdd", day(5))
	send("later", 14, "SOdd", day(5))
	send("intro-a", 1, "SStart", day(3))
	send("intro-a", 1, "SStart", day(3).Add(2*time.Hour)) // the same text twice in a day
	send("intro-a", 2, "SStart", day(9))
	send("intro-b", 2, "SStart", day(2)) // another text later: repeated, not doubled
	send("intro-a", 3, "SNext", day(4))
	send("intro-a", 4, "SOdd", day(4))
	send("intro-a", 5, "", day(4))
	send("intro-a", 6, "SStart", day(60)) // before the period

	got, err := LoadEmailSends(ctx, db, intro, 30, time.UTC, now)
	if err != nil {
		t.Fatal(err)
	}
	want := []StageSends{
		{StageID: "SStart", Name: "Start", Sends: 4},
		{StageID: "SNext", Name: "Next", Sends: 1, Other: true},
		{StageID: "SOdd", Name: "Odd", Sends: 1, Other: true},
		{StageID: "", Name: "", Sends: 1},
	}
	if len(got.Stages) != len(want) {
		t.Fatalf("stages = %+v", got.Stages)
	}
	for i := range want {
		if got.Stages[i] != want[i] {
			t.Errorf("stages[%d] = %+v, want %+v", i, got.Stages[i], want[i])
		}
	}
	if want := (SendDeals{Deals: 5, Repeated: 2, Doubled: 1}); got.Deals != want {
		t.Errorf("deals = %+v, want %+v", got.Deals, want)
	}

	// Sent only from stages of another board stage: nothing to compare with.
	got, err = LoadEmailSends(ctx, db, later, 30, time.UTC, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range got.Stages {
		if stage.Other {
			t.Errorf("later: %s marked as another stage", stage.Name)
		}
	}
}
