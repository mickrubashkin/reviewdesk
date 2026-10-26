package portal

import (
	"context"
	"testing"
	"time"
)

func TestSentCopyIsCurrent(t *testing.T) {
	previous := "Welcome to the partner program. Book your onboarding call with your manager today and start selling."
	current := "Welcome to the partner program. Complete the training first, then book your onboarding call with your manager."
	cases := []struct {
		name        string
		sent        string
		previous    string
		hasPrevious bool
		want        bool
	}{
		{"sends the current version", current, previous, true, true},
		{"sends the previous version", previous, previous, true, false},
		{"only version, sent", current, "", false, true},
		{"text unchanged between versions", current, current, true, false},
		{"unrelated text", "Your invoice is attached. Thank you for your payment.", previous, true, false},
	}
	for _, c := range cases {
		if got := sentCopyIsCurrent(c.sent, current, c.previous, c.hasPrevious); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLoadUploadSuggestions(t *testing.T) {
	dbpool := testPool(t)
	ctx := context.Background()
	const board = "suggest-test"

	template := `<html><body><p data-edit-text="intro">x</p></body></html>`
	v1 := `{"intro":{"type":"text","value":"Welcome to the partner program. Book your onboarding call with your manager today and start selling."}}`
	v2 := `{"intro":{"type":"text","value":"Welcome to the partner program. Complete the training first, then book your onboarding call with your manager."}}`
	textV1, _ := renderVersionText(template, []byte(v1), "")
	textV2, _ := renderVersionText(template, []byte(v2), "")

	var emailID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, editable_fields, subject, review_status)
		VALUES ('suggest-test-email', 'Suggest test', $1, 'test', 9990, 'en', 'new', $2, $2, $3::jsonb, 'Subject', 'ready')
		RETURNING id;
	`, board, template, v2).Scan(&emailID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO email_robot_uploads (email_id, robot, version, uploaded_by_email) VALUES ($1, '', 1, 'test');`, emailID); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_, _ = dbpool.Exec(ctx, `DELETE FROM portal_email_groups WHERE board_key = $1;`, board)
		_, _ = dbpool.Exec(ctx, `DELETE FROM portal_sent_emails WHERE board_key = $1;`, board)
		_, _ = dbpool.Exec(ctx, `DELETE FROM emails WHERE id = $1;`, emailID)
	}
	t.Cleanup(cleanup)

	v2Saved := time.Now().Add(-48 * time.Hour)
	for number, fields := range map[int]string{1: v1, 2: v2} {
		created := v2Saved
		if number == 1 {
			created = v2Saved.Add(-30 * 24 * time.Hour)
		}
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO email_versions (email_id, version_number, created_at, created_by_email, source, title, subject, original_html, template_html, editable_fields)
			VALUES ($1, $2, $3, 'test', 'manual', 'Suggest test', 'Subject', $4, $4, $5::jsonb);
		`, emailID, number, created, template, fields); err != nil {
			t.Fatal(err)
		}
	}

	activity := int64(990000000)
	addGroup := func(fingerprint, text string, sentAt ...time.Time) {
		for _, at := range sentAt {
			activity++
			if _, err := dbpool.Exec(ctx, `
				INSERT INTO portal_sent_emails (activity_id, board_key, deal_id, sent_at, subject, body_html, fingerprint)
				VALUES ($1, $2, 1, $3, 'Subject', '', $4);
			`, activity, board, at, fingerprint); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO portal_email_groups (board_key, fingerprint, language, subject, normalized_text, sample_activity_id,
				send_count, first_sent_at, last_sent_at, match_email_id, match_score, match_status)
			VALUES ($1, $2, 'en', 'Subject', $3, $4, $5, $6, $6, $7, 1, 'matched');
		`, board, fingerprint, text, activity, len(sentAt), sentAt[0], emailID); err != nil {
			t.Fatal(err)
		}
	}

	// Bitrix24 still sends version 1: no suggestion.
	addGroup("old", textV1, time.Now().Add(-2*time.Hour))
	suggestions, err := LoadUploadSuggestions(ctx, dbpool)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range suggestions {
		if s.EmailID == emailID {
			t.Fatalf("suggested while only version 1 is sent: %+v", s)
		}
	}

	// Version 2 goes out: one send before it was saved does not count.
	addGroup("new", textV2, v2Saved.Add(-time.Hour), time.Now().Add(-3*time.Hour), time.Now().Add(-time.Hour))
	suggestions, err = LoadUploadSuggestions(ctx, dbpool)
	if err != nil {
		t.Fatal(err)
	}
	var found *UploadSuggestion
	for i := range suggestions {
		if suggestions[i].EmailID == emailID {
			found = &suggestions[i]
		}
	}
	if found == nil || found.Version != 2 || found.Sends != 2 {
		t.Fatalf("want version 2 with 2 sends, got %+v", found)
	}

	// Once version 2 is marked, nothing is left to suggest.
	if _, err := dbpool.Exec(ctx, `UPDATE email_robot_uploads SET version = 2 WHERE email_id = $1;`, emailID); err != nil {
		t.Fatal(err)
	}
	suggestions, err = LoadUploadSuggestions(ctx, dbpool)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range suggestions {
		if s.EmailID == emailID {
			t.Fatalf("suggested after the mark: %+v", s)
		}
	}
}
