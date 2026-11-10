package split

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoadFindsEmailsSentInSeveralTexts(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for split integration tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(db.Close)

	const board = "split-test"
	cleanup := func() {
		_, _ = db.Exec(ctx, `DELETE FROM portal_email_groups WHERE board_key = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM portal_sent_emails WHERE board_key = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM emails WHERE sequence = $1;`, board)
	}
	cleanup()
	t.Cleanup(cleanup)

	email := func(slug string, sortOrder int) string {
		t.Helper()
		var id string
		if err := db.QueryRow(ctx, `
			INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, subject)
			VALUES ($1, $1, $2, 'test', $3, 'en', 'new', '<p>x</p>', '<p>x</p>', 'Subject')
			RETURNING id;
		`, slug, board, sortOrder).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	split, replaced := email("split-test-split", 9990), email("split-test-replaced", 9991)

	now := time.Now()
	var activity int64 = 9200000
	// text is a portal group tied to the email, sent at each of the days ago.
	text := func(emailID, fingerprint string, daysAgo ...int) {
		t.Helper()
		first, last := now.AddDate(0, 0, -daysAgo[0]), now.AddDate(0, 0, -daysAgo[len(daysAgo)-1])
		if _, err := db.Exec(ctx, `
			INSERT INTO portal_email_groups (board_key, fingerprint, subject, sample_activity_id, first_sent_at, last_sent_at,
				match_email_id, match_status)
			VALUES ($1, $2, 'Subject', 1, $3, $4, $5, 'matched');
		`, board, fingerprint, first, last, emailID); err != nil {
			t.Fatal(err)
		}
		for _, ago := range daysAgo {
			activity++
			if _, err := db.Exec(ctx, `
				INSERT INTO portal_sent_emails (activity_id, board_key, deal_id, sent_at, body_html, fingerprint)
				VALUES ($1, $2, 1, $3, '', $4);
			`, activity, board, now.AddDate(0, 0, -ago), fingerprint); err != nil {
				t.Fatal(err)
			}
		}
	}
	text(split, "split-old", 40, 10, 2)
	text(split, "split-new", 8, 1)
	text(split, "split-gone", 60, 30) // nothing in the window
	text(replaced, "replaced-old", 40, 12, 9)
	text(replaced, "replaced-new", 9, 3)

	emails, err := Load(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	var found []Email
	for _, email := range emails {
		if email.ID == split || email.ID == replaced {
			found = append(found, email)
		}
	}
	if len(found) != 1 || found[0].ID != split || len(found[0].Texts) != 2 {
		t.Fatalf("expected only the split email with 2 texts, got %+v", found)
	}
	if texts := found[0].Texts; texts[0].Sends != 2 || texts[1].Sends != 2 || !texts[0].FirstSentAt.After(texts[1].FirstSentAt) {
		t.Fatalf("expected the window's sends, newest text first, got %+v", found[0].Texts)
	}
}
