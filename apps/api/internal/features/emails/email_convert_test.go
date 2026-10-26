package emails

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

const convertTestBoard = "convert-test"

func createConvertTestEmail(t *testing.T, dbpool *pgxpool.Pool, slug string, position int, language, template, translationOf string) string {
	t.Helper()
	fields, err := emailedit.ExtractEditableFields(template)
	if err != nil {
		t.Fatal(err)
	}
	fieldsJSON, _ := fields.JSON()
	rendered, err := emailedit.RenderEditableHTML(template, fields)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err := dbpool.QueryRow(context.Background(), `
		INSERT INTO emails (slug, title, subject, sequence, stage, sort_order, language, variant, review_status,
			original_html, review_html, template_html, editable_fields, translation_of, translated_from_version)
		VALUES ($1, $1, 'Subject '||$1, $7, 'test', $8, $2, 'new', 'ready', $3, $4, $3, $5::jsonb,
			nullif($6, '')::uuid, CASE WHEN $6 = '' THEN NULL ELSE 1 END)
		RETURNING id;
	`, slug, language, template, rendered, fieldsJSON, translationOf, convertTestBoard, position).Scan(&id); err != nil {
		t.Fatalf("create %s: %v", slug, err)
	}
	if err := insertEmailVersion(context.Background(), dbpool, emailVersionSnapshot{
		EmailID: id, CreatedByEmail: "test", Source: "initial", Title: slug,
		OriginalHTML: template, TemplateHTML: template, EditableFieldsJSON: fieldsJSON,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE email_id = $1;`, id)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, id)
	})
	return id
}

func TestLayoutConversion(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	resetEmailBlocks(t, dbpool)
	admin := createTestUserWithRole(t, dbpool, "admin")

	content, err := os.ReadFile("../../../../../db/seeds/emails/07_first-sale/01-next-step/en.html")
	if os.IsNotExist(err) {
		t.Skip("seed emails are not in this checkout")
	}
	if err != nil {
		t.Fatal(err)
	}
	seed := string(content)
	german := strings.Replace(seed, "Dear Partner,", "Lieber Partner,", 1)
	padded := strings.Replace(seed, "padding: 0 0 0 0;", "padding: 0 0 26px 0;", 1)
	fixedText := strings.Replace(seed, "<tr>", "<tr><td>Fixed text</td></tr><tr>", 2)

	master := createConvertTestEmail(t, dbpool, "convert-test-en", 1, "en", seed, "")
	translation := createConvertTestEmail(t, dbpool, "convert-test-de", 1, "de", german, master)
	markup := createConvertTestEmail(t, dbpool, "convert-test-markup", 2, "en", padded, "")
	manual := createConvertTestEmail(t, dbpool, "convert-test-manual", 3, "en", fixedText, "")
	markupTranslation := createConvertTestEmail(t, dbpool, "convert-test-mixed-en", 4, "en", seed, "")
	createConvertTestEmail(t, dbpool, "convert-test-mixed-de", 4, "de", padded, markupTranslation)

	plan := func() map[string]LayoutFamily {
		families, err := PlanLayoutConversion(ctx, dbpool, convertTestBoard)
		if err != nil {
			t.Fatal(err)
		}
		byMaster := map[string]LayoutFamily{}
		for _, family := range families {
			byMaster[family.Master.ID] = family
		}
		return byMaster
	}
	families := plan()
	if len(families) != 4 || len(families[master].Translations) != 1 {
		t.Fatalf("expected 4 families, the first with its translation: %+v", families)
	}
	if blocker := families[master].Blocker(true); blocker != "" {
		t.Fatalf("the same render switches: %s", blocker)
	}
	if blocker := families[markup].Blocker(true); !strings.Contains(blocker, "-allow-markup") {
		t.Fatalf("a markup difference waits for -allow-markup: %q", blocker)
	}
	if blocker := families[markupTranslation].Blocker(true); !strings.HasPrefix(blocker, "DE: ") {
		t.Fatalf("a translation that differs holds back its EN email: %q", blocker)
	}
	if blocker := families[manual].Blocker(false); !strings.HasPrefix(blocker, "text: ") {
		t.Fatalf("a text outside the fields stays for manual work: %q", blocker)
	}

	apply := func(strict bool) int {
		tx, err := dbpool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		families, err := PlanLayoutConversion(ctx, tx, convertTestBoard)
		if err != nil {
			t.Fatal(err)
		}
		switched, _, err := ApplyLayoutConversion(ctx, tx, admin, families, LayoutConversionOptions{Strict: strict})
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return switched
	}
	if switched := apply(true); switched != 2 {
		t.Fatalf("strict switches the EN email and its translation, got %d", switched)
	}
	want := emailblocks.Layout{{Block: "hero_banner"}, {Block: "intro_body"}, {Block: "primary_cta"}, {Block: "signature"}, {Block: "footer"}}
	for id, intro := range map[string]string{master: "Dear Partner,", translation: "Lieber Partner,"} {
		state := loadLayoutTestState(t, dbpool, id)
		if !sameLayout(state.layout, want) || !sameLayout(state.versions, want) {
			t.Fatalf("unexpected layout: %+v", state.layout)
		}
		if state.status != "ready" || state.version != 2 || state.source != "manual" {
			t.Errorf("the status stays and a version is written: %s v%d %s", state.status, state.version, state.source)
		}
		if value, _ := state.fields["intro_body"].Value.(string); !strings.HasPrefix(value, intro) {
			t.Errorf("each email keeps its own text, got %q", value)
		}
	}
	if state := loadLayoutTestState(t, dbpool, markup); state.layout != nil {
		t.Fatalf("strict leaves a markup difference alone")
	}

	if switched := apply(false); switched != 3 {
		t.Fatalf("-allow-markup switches the emails with other markup, got %d", switched)
	}
	if state := loadLayoutTestState(t, dbpool, markup); state.layout == nil || state.status != "ready" {
		t.Fatalf("the email with other markup switches and stays ready: %+v", state.status)
	}
	if state := loadLayoutTestState(t, dbpool, manual); state.layout != nil || state.version != 1 {
		t.Fatalf("an email that cannot switch stays untouched")
	}
	if families := plan(); len(families) != 1 {
		t.Fatalf("switched emails leave the plan, got %d families", len(families))
	}
}
