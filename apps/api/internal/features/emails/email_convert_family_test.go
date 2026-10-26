package emails

import (
	"context"
	"os"
	"strings"
	"testing"
)

// A translation switches with its EN email only when it gets the same
// blocks: a two-field signature takes EN's one-field one, other blocks stop
// the family. Comments on the paragraph that became one block move there.
func TestLayoutConversionFamilies(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	resetEmailBlocks(t, dbpool)
	admin := createTestUserWithRole(t, dbpool, "admin")

	read := func(seed string) string {
		content, err := os.ReadFile("../../../../../db/seeds/emails/" + seed + "/en.html")
		if os.IsNotExist(err) {
			t.Skip("seed emails are not in this checkout")
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}
	twoFields := read("06_partner-app-installed/01-next-step")
	signature := twoFields[strings.Index(twoFields, `<span data-review-block="signature">`):]
	signature = signature[:strings.Index(signature, "</span>")+len("</span>")]
	oneField := strings.Replace(twoFields, signature,
		`<span data-review-block="signature" data-edit-text="signature">Best regards,<br>Bitrix24 Team<br></span>`, 1)

	master := createConvertTestEmail(t, dbpool, "family-test-en", 10, "en", oneField, "")
	brazilian := createConvertTestEmail(t, dbpool, "family-test-br", 10, "br", twoFields, master)
	listMaster := createConvertTestEmail(t, dbpool, "family-list-en", 11, "en", read("10_silver/01-next-step"), "")
	createConvertTestEmail(t, dbpool, "family-list-de", 11, "de", twoFields, listMaster)

	// BR's robot has its latest version; EN was never pasted.
	if _, err := dbpool.Exec(ctx, `INSERT INTO email_robot_uploads (email_id, robot, version, uploaded_at, uploaded_by_email) VALUES ($1, '', 1, now() - interval '1 day', 'test');`, brazilian); err != nil {
		t.Fatal(err)
	}
	var commentID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO comments (email_id, review_block, selected_text, start_offset, end_offset, body)
		VALUES ($1, 'body_outro_001', 'not sure', 13, 21, 'Shorter?') RETURNING id;
	`, brazilian).Scan(&commentID); err != nil {
		t.Fatal(err)
	}

	families, err := PlanLayoutConversion(ctx, dbpool, convertTestBoard)
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		switch family.Master.ID {
		case master:
			if blocker := family.Blocker(false); blocker != "" {
				t.Fatalf("the signature should align: %s", blocker)
			}
			if got := family.Translations[0].MovedComments; got != 1 {
				t.Fatalf("the comment moves, got %d", got)
			}
		case listMaster:
			if blocker := family.Blocker(false); !strings.Contains(blocker, "other blocks than EN") {
				t.Fatalf("a translation without EN's list must stay, got %q", blocker)
			}
		}
	}

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	switched, kept, err := ApplyLayoutConversion(ctx, tx, admin, families, LayoutConversionOptions{KeepUpload: true})
	if err != nil {
		t.Fatal(err)
	}
	if switched != 2 || kept != 1 {
		t.Fatalf("EN and BR switch, BR keeps its upload mark: %d switched, %d kept", switched, kept)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	state := loadLayoutTestState(t, dbpool, brazilian)
	if !state.layout.Uses("signature_text") || state.fields["signature"].Value != "Best regards,\nBitrix24 Team" {
		t.Fatalf("BR takes the one-field signature: %v %+v", state.layout.Keys(), state.fields["signature"])
	}
	var uploaded, latest int
	if err := dbpool.QueryRow(ctx, `
		SELECT u.version, (SELECT max(version_number) FROM email_versions WHERE email_id = u.email_id) FROM email_robot_uploads u WHERE u.email_id = $1;
	`, brazilian).Scan(&uploaded, &latest); err != nil {
		t.Fatal(err)
	}
	if latest < 2 || uploaded != latest {
		t.Fatalf("the upload mark moves to the new version: uploaded v%d, latest v%d", uploaded, latest)
	}
	var block, selected string
	var start, end int
	if err := dbpool.QueryRow(ctx, `
		SELECT review_block, selected_text, start_offset, end_offset FROM comments WHERE id = $1;
	`, commentID).Scan(&block, &selected, &start, &end); err != nil {
		t.Fatal(err)
	}
	if block != "closing_text" || selected != "not sure" || end-start != len("not sure") {
		t.Fatalf("the comment moves to the closing text at its words: %s %q %d-%d", block, selected, start, end)
	}
}
