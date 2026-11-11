package portal

import (
	"context"
	"testing"
	"time"
)

func TestSuggestRobotUser(t *testing.T) {
	users := []portalUser{
		{ID: 1, Name: "Alexander Rossi", Email: "a.rossi@example.com"},
		{ID: 2, Name: "Bennett Lopez", Email: "b.lopez@example.com"},
		{ID: 3, Name: "José Silva", Email: "silva@example.com"},
		{ID: 4, Name: "Olga Kowalska", Email: "o.kowalska@example.com"},
		{ID: 5, Name: "Mark One", Email: "one@example.com"},
		{ID: 6, Name: "Mark Two", Email: "two@example.com"},
		{ID: 7, Name: "", Email: "petra@example.com"},
	}
	cases := []struct {
		robot string
		want  int64 // 0: no suggestion
	}{
		{"Alex", 1},            // short form
		{"Bennet", 2},          // one letter off
		{"Jose", 3},            // no accent
		{"Olga Kovalska", 4},   // one letter off in the last name
		{"Mark", 0},            // two users match
		{"Mark Two", 6},        // the last name settles it
		{"Petra", 7},           // from the email when the name is missing
		{"Zoe", 0},             // nobody
		{"Al", 0},              // too short for a short form
		{"Alexander Lopez", 0}, // every word has to match
	}
	for _, c := range cases {
		got := suggestRobotUser(c.robot, users)
		switch {
		case c.want == 0 && got != nil:
			t.Errorf("%q: want no suggestion, got %d", c.robot, *got)
		case c.want != 0 && (got == nil || *got != c.want):
			t.Errorf("%q: want %d, got %v", c.robot, c.want, got)
		}
	}
}

func TestEditDistanceAtMostOne(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"bennet", "bennett", true},
		{"bennett", "bennet", true},
		{"kovalska", "kowalska", true},
		{"kovalska", "kowalsky", false},
		{"abc", "abcde", false},
		{"maria", "marian", true},
		{"xmaria", "maria", true},
	}
	for _, c := range cases {
		if got := editDistanceAtMostOne(c.a, c.b); got != c.want {
			t.Errorf("%q vs %q: got %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// Two managers' robots both carry a copied mark of version 1. Version 2 goes
// out from the deals of one of them: once that robot is paired with its
// portal user, the suggestion names it, and the copied mark of the other
// robot shows no sends.
func TestRobotSendersFollowSends(t *testing.T) {
	db := testPool(t)
	ctx := context.Background()
	const board = "robot-senders-test"
	cleanup := func() {
		_, _ = db.Exec(ctx, `DELETE FROM portal_email_groups WHERE board_key = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM portal_sent_emails WHERE board_key = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM portal_users WHERE user_id IN (990501, 990502);`)
		_, _ = db.Exec(ctx, `DELETE FROM emails WHERE sequence = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM boards WHERE key = $1;`, board)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := db.Exec(ctx, `INSERT INTO boards (key, name) VALUES ($1, 'Robot senders');`, board); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO board_regions (board_key, key, name, language, reference, managers, sort_order)
		VALUES ($1, 'ref', 'Reference', 'en', true, '{Ann,Ben}', 1);
	`, board); err != nil {
		t.Fatal(err)
	}

	template := `<html><body><p data-edit-text="intro">x</p></body></html>`
	v1 := `{"intro":{"type":"text","value":"Welcome to the partner program. Book your onboarding call with your manager today and start selling."}}`
	v2 := `{"intro":{"type":"text","value":"Welcome to the partner program. Complete the training first, then book your onboarding call with your manager."}}`
	textV2, _ := renderVersionText(template, []byte(v2), "")

	var emailID string
	if err := db.QueryRow(ctx, `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, region_key, original_html, template_html, editable_fields, subject, review_status)
		VALUES ('robot-senders-email', 'Robot senders', $1, 'test', 9990, 'en', 'new', 'ref', $2, $2, $3::jsonb, 'Subject', 'ready')
		RETURNING id;
	`, board, template, v2).Scan(&emailID); err != nil {
		t.Fatal(err)
	}
	saved := time.Now().Add(-48 * time.Hour)
	for number, fields := range map[int]string{1: v1, 2: v2} {
		if _, err := db.Exec(ctx, `
			INSERT INTO email_versions (email_id, version_number, created_at, created_by_email, source, title, subject, original_html, template_html, editable_fields)
			VALUES ($1, $2, $3, 'test', 'manual', 'Robot senders', 'Subject', $4, $4, $5::jsonb);
		`, emailID, number, saved.Add(time.Duration(number-2)*24*time.Hour), template, fields); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO email_robot_uploads (email_id, robot, version, uploaded_by_email)
		VALUES ($1, 'Ann', 1, 'migration'), ($1, 'Ben', 1, 'migration');
	`, emailID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO portal_users (user_id, name, email) VALUES (990501, 'Ann Example', 'ann@example.com'), (990502, 'Someone Else', 'else@example.com');
	`); err != nil {
		t.Fatal(err)
	}
	for i, at := range []time.Time{time.Now().Add(-3 * time.Hour), time.Now().Add(-time.Hour)} {
		if _, err := db.Exec(ctx, `
			INSERT INTO portal_sent_emails (activity_id, board_key, deal_id, sent_at, subject, body_html, fingerprint, responsible_id)
			VALUES ($1, $2, 1, $3, 'Subject', '', 'v2', 990501);
		`, 990500000+i, board, at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO portal_email_groups (board_key, fingerprint, language, subject, normalized_text, sample_activity_id,
			send_count, first_sent_at, last_sent_at, match_email_id, match_score, match_status)
		VALUES ($1, 'v2', 'en', 'Subject', $2, 990500000, 2, now(), now(), $3, 1, 'matched');
	`, board, textV2, emailID); err != nil {
		t.Fatal(err)
	}

	suggestion := func() UploadSuggestion {
		t.Helper()
		suggestions, err := LoadUploadSuggestions(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range suggestions {
			if s.EmailID == emailID {
				return s
			}
		}
		t.Fatal("no suggestion for the email")
		return UploadSuggestion{}
	}

	// Not paired yet: the sends are nobody's, and Ann is suggested.
	if s := suggestion(); s.Sends != 2 || s.UnknownSends != 2 || len(s.Robots) != 0 {
		t.Fatalf("before pairing: %+v", s)
	}
	robots, err := LoadRobotSenders(ctx, db, board)
	if err != nil {
		t.Fatal(err)
	}
	if len(robots.Robots) != 2 || robots.Robots[0].Robot != "Ann" || robots.Robots[0].SuggestedID == nil ||
		*robots.Robots[0].SuggestedID != 990501 || robots.Robots[1].SuggestedID != nil || robots.UnpairedSends != 2 {
		t.Fatalf("before pairing: %+v", robots)
	}

	if err := SetRobotSender(ctx, db, board, "Ann", &[]int64{990501}[0], "test"); err != nil {
		t.Fatal(err)
	}
	if err := SetRobotSender(ctx, db, board, "Ben", &[]int64{990501}[0], "test"); err != errUserPaired {
		t.Fatalf("pairing a user twice: %v", err)
	}
	if err := SetRobotSender(ctx, db, board, "Nobody", &[]int64{990502}[0], "test"); err != errNotBoardRobot {
		t.Fatalf("pairing an unknown robot: %v", err)
	}

	s := suggestion()
	if s.UnknownSends != 0 || len(s.Robots) != 1 || s.Robots[0] != (RobotSends{Robot: "Ann", Sends: 2}) {
		t.Fatalf("after pairing: %+v", s)
	}
	robots, err = LoadRobotSenders(ctx, db, board)
	if err != nil {
		t.Fatal(err)
	}
	ann, ben := robots.Robots[0], robots.Robots[1]
	if ann.PortalUserID == nil || ann.Sends != 2 || ann.Marks != 1 || ann.MarksSeenSent != 1 {
		t.Fatalf("Ann: %+v", ann)
	}
	if ben.PortalUserID != nil || ben.Marks != 1 || ben.MarksSeenSent != 0 || robots.UnpairedSends != 0 {
		t.Fatalf("Ben: %+v, unpaired %d", ben, robots.UnpairedSends)
	}

	if err := SetRobotSender(ctx, db, board, "Ann", nil, "test"); err != nil {
		t.Fatal(err)
	}
	if s := suggestion(); s.UnknownSends != 2 {
		t.Fatalf("after unpairing: %+v", s)
	}
}
