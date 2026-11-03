package emails

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

func commentTexts() commentEditTexts {
	text := func(value string, order int) emailedit.EditableField {
		return emailedit.EditableField{Type: emailedit.FieldTypeText, Value: value, Order: order}
	}
	signature := text("Mit freundlichen Grüßen,\nIhr Bitrix24-Team", 5)
	signature.Shared = true
	return commentEditTexts{
		Subject:   "Bitrix24: Konsistenz führt zu Wachstum",
		Preheader: "Ohne Training erhalten Sie keinen Zugang zu Leads",
		Fields: emailedit.EditableFields{
			"intro_body":       text("Sehr geehrter Partner,\n\nSie sind fast da. Sie sind fast am Ziel.", 1),
			"primary_cta_text": text("Das Training abschließen", 2),
			"primary_cta_url":  {Type: emailedit.FieldTypeURL, Value: "https://example.com"},
			"hero_banner_alt":  text("Example Partners. Schneller wachsen.", 0),
			"closing_text":     text("Mehr Leads generieren und gewinnen", 3),
			"signature":        signature,
		},
	}
}

func TestPlanCommentEdit(t *testing.T) {
	texts := commentTexts()
	for _, tc := range []struct {
		name, block, selected, body string
		status, field, after        string
	}{
		{"a field named after the block", "intro_body", "Sehr geehrter Partner,", "Hallo,", CommentEditApply, "intro_body",
			"Hallo,\n\nSie sind fast da. Sie sind fast am Ziel."},
		{"a link's text", "primary_cta", "Das Training abschließen", "Training abschließen", CommentEditApply, "primary_cta_text",
			"Training abschließen"},
		{"the subject", "subject", "Konsistenz führt zu Wachstum", "Schritt für Schritt zum Wachstum", CommentEditApply, "subject",
			"Bitrix24: Schritt für Schritt zum Wachstum"},
		{"text that occurs twice", "intro_body", "Sie sind fast", "Sie sind beinahe", CommentEditAmbiguous, "intro_body", ""},
		{"text that changed since", "primary_cta", "Jetzt starten", "Los geht's", CommentEditStale, "primary_cta_text", ""},
		{"words drawn on a banner", "hero_banner", "Schneller wachsen.", "Schneller wachsen!", CommentEditImage, "hero_banner_alt", ""},
		{"a shared text", "signature", "Ihr Bitrix24-Team", "Ihr Bitrix24 Team", CommentEditShared, "signature", ""},
		{"a link instead of text", "intro_body", "Sehr geehrter Partner,", "https://youtu.be/x", CommentEditNotText, "intro_body", ""},
		{"a fragment", "intro_body", "Partner,", ",", CommentEditNotText, "intro_body", ""},
		{"a request", "intro_body", "Sehr geehrter Partner,", "Bitte kürzer formulieren", CommentEditQuestion, "intro_body", ""},
		{"a question", "intro_body", "Sehr geehrter Partner,", "Lieber Hallo?", CommentEditQuestion, "intro_body", ""},
		{"a note to the editor in front", "primary_cta", "Das Training abschließen", "Надо: Training abschließen", CommentEditApply, "primary_cta_text",
			"Training abschließen"},
		{"an instruction in Russian", "intro_body", "Sehr geehrter Partner,", "Hallo, вместо Sehr geehrter Partner", CommentEditQuestion, "intro_body", ""},
		{"text in another field", "intro_body", "Mehr Leads generieren", "Mehr Leads gewinnen", CommentEditElsewhere, "intro_body", ""},
	} {
		edits := planEmailCommentEdits([]CommentEditInput{{ID: "c", ReviewBlock: tc.block, Selected: tc.selected, Body: tc.body}}, texts)
		got := edits[0]
		if got.Status != tc.status || got.Field != tc.field || got.After != tc.after {
			t.Errorf("%s: got %s %s %q, want %s %s %q", tc.name, got.Status, got.Field, got.After, tc.status, tc.field, tc.after)
		}
	}
}

func TestPlanEmailCommentEditsCombinesAndHoldsBackOverlaps(t *testing.T) {
	texts := commentTexts()
	edits := planEmailCommentEdits([]CommentEditInput{
		{ID: "a", ReviewBlock: "closing_text", Selected: "gewinnen", Body: "erzielen"},
		{ID: "b", ReviewBlock: "closing_text", Selected: "generieren", Body: "finden"},
	}, texts)
	for _, edit := range edits {
		if edit.Status != CommentEditApply || edit.After != "Mehr Leads finden und erzielen" {
			t.Errorf("%s: %s %q", edit.CommentID, edit.Status, edit.After)
		}
	}

	edits = planEmailCommentEdits([]CommentEditInput{
		{ID: "a", ReviewBlock: "closing_text", Selected: "Leads generieren", Body: "Kunden finden"},
		{ID: "b", ReviewBlock: "closing_text", Selected: "generieren und", Body: "finden und"},
		{ID: "c", ReviewBlock: "primary_cta", Selected: "Das Training", Body: "Training"},
	}, texts)
	if edits[0].Status != CommentEditConflict || edits[1].Status != CommentEditConflict || edits[2].Status != CommentEditApply {
		t.Errorf("overlapping edits: %s %s %s", edits[0].Status, edits[1].Status, edits[2].Status)
	}
	if edits[0].After != "" || edits[0].Before != "Mehr Leads generieren und gewinnen" {
		t.Errorf("a held-back edit shows the field as it stays: %q → %q", edits[0].Before, edits[0].After)
	}
}

func TestApplyCommentEditsSavesOneVersionAndResolvesApplied(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	emailID := createTestEmail(t, dbpool)
	actor := createTestUserWithRole(t, dbpool, "super_admin")
	setTestEmailTemplate(t, dbpool, emailID, `<html><body>
		<p data-review-block="intro_body" data-edit-text="intro_body">Sehr geehrter Partner, willkommen.</p>
		<a href="https://example.com" data-review-block="primary_cta" data-edit-text="primary_cta_text">Das Training abschließen</a>
	</body></html>`)
	if _, err := dbpool.Exec(ctx, `
		UPDATE emails SET subject = 'Old subject', editable_fields = $2 WHERE id = $1;
	`, emailID, `{"intro_body": {"type": "text", "value": "Sehr geehrter Partner, willkommen.", "order": 1},
		"primary_cta_text": {"type": "text", "value": "Das Training abschließen", "order": 2}}`); err != nil {
		t.Fatal(err)
	}
	insertTestEmailVersion(t, dbpool, emailID, "system", "initial")
	comment := func(block, selected, body string) string {
		t.Helper()
		var id string
		if err := dbpool.QueryRow(ctx, `
			INSERT INTO comments (email_id, user_id, review_block, selected_text, start_offset, end_offset, body)
			VALUES ($1, $2, $3, $4, 0, length($4), $5) RETURNING id;
		`, emailID, actor.ID, block, selected, body).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	intro := comment("intro_body", "Sehr geehrter Partner,", "Hallo,")
	cta := comment("primary_cta", "Das Training abschließen", "Training abschließen")
	subject := comment("subject", "Old", "New")
	question := comment("intro_body", "willkommen", "Bitte freundlicher?")

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	planned, err := PlanCommentEdits(ctx, tx, CommentEditsOptions{Board: "onboarding"})
	if err != nil {
		t.Fatal(err)
	}
	var mine []CommentEditEmail
	for _, email := range planned {
		if email.ID == emailID {
			mine = append(mine, email)
		}
	}
	if len(mine) != 1 || len(mine[0].Edits) != 4 {
		t.Fatalf("planned %+v", mine)
	}
	if err := ApplyCommentEdits(ctx, tx, actor, mine); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if !mine[0].Changed {
		t.Error("the email was not marked changed")
	}

	var fieldsJSON []byte
	var gotSubject string
	var versions int
	if err := dbpool.QueryRow(ctx, `
		SELECT editable_fields, subject, (SELECT count(*) FROM email_versions v WHERE v.email_id = e.id)
		FROM emails e WHERE id = $1;
	`, emailID).Scan(&fieldsJSON, &gotSubject, &versions); err != nil {
		t.Fatal(err)
	}
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["intro_body"].Value != "Hallo, willkommen." || fields["primary_cta_text"].Value != "Training abschließen" || gotSubject != "New subject" {
		t.Errorf("after: intro %q, cta %q, subject %q", fields["intro_body"].Value, fields["primary_cta_text"].Value, gotSubject)
	}
	if versions != 2 {
		t.Errorf("versions = %d, want one new version for all edits", versions)
	}
	for id, want := range map[string]string{intro: "resolved", cta: "resolved", subject: "resolved", question: "open"} {
		var status string
		var replies int
		if err := dbpool.QueryRow(ctx, `
			SELECT status, (SELECT count(*) FROM comment_messages m WHERE m.comment_id = c.id) FROM comments c WHERE id = $1;
		`, id).Scan(&status, &replies); err != nil {
			t.Fatal(err)
		}
		if status != want || (want == "resolved") != (replies == 1) {
			t.Errorf("comment %s: %s with %d replies, want %s", id, status, replies, want)
		}
	}
}
