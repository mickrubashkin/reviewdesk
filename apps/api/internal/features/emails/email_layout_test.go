package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

const layoutTestTemplate = `<!doctype html><html><head><title>Old</title></head><body><table><tr>` +
	`<td data-review-block="intro_body" data-edit-text="intro_body">Hello</td></tr></table></body></html>`

func createLayoutTestEmail(t *testing.T, dbpool *pgxpool.Pool, slug, language, intro, translationOf string) string {
	t.Helper()
	fields := emailedit.EditableFields{"intro_body": {Type: "text", Value: intro, Order: 1}}
	fieldsJSON, _ := fields.JSON()
	rendered, err := emailedit.RenderEditableHTML(layoutTestTemplate, fields)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err := dbpool.QueryRow(context.Background(), `
		INSERT INTO emails (slug, title, subject, sequence, stage, sort_order, language, variant, review_status,
			original_html, review_html, template_html, editable_fields, translation_of, translated_from_version)
		VALUES ($1, $1, 'Subject '||$1, 'layout-test', 'test', 1, $2, 'new', 'ready', $3, $4, $3, $5::jsonb,
			nullif($6, '')::uuid, CASE WHEN $6 = '' THEN NULL ELSE 1 END)
		RETURNING id;
	`, slug, language, layoutTestTemplate, rendered, fieldsJSON, translationOf).Scan(&id); err != nil {
		t.Fatalf("create %s: %v", slug, err)
	}
	if err := insertEmailVersion(context.Background(), dbpool, emailVersionSnapshot{
		EmailID: id, CreatedByEmail: "test", Source: "initial", Title: slug,
		OriginalHTML: layoutTestTemplate, TemplateHTML: layoutTestTemplate, EditableFieldsJSON: fieldsJSON,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE email_id = $1;`, id)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, id)
	})
	return id
}

type layoutTestState struct {
	layout   emailblocks.Layout
	template string
	review   string
	status   string
	fields   emailedit.EditableFields
	version  int
	source   string
	versions emailblocks.Layout
}

func loadLayoutTestState(t *testing.T, dbpool *pgxpool.Pool, id string) layoutTestState {
	t.Helper()
	var state layoutTestState
	var layoutRaw, fieldsRaw, versionLayoutRaw []byte
	if err := dbpool.QueryRow(context.Background(), `
		SELECT e.layout, e.template_html, e.review_html, e.review_status, e.editable_fields,
			v.version_number, v.source, v.layout
		FROM emails e
		JOIN LATERAL (SELECT * FROM email_versions WHERE email_id = e.id ORDER BY version_number DESC LIMIT 1) v ON true
		WHERE e.id = $1;
	`, id).Scan(&layoutRaw, &state.template, &state.review, &state.status, &fieldsRaw,
		&state.version, &state.source, &versionLayoutRaw); err != nil {
		t.Fatal(err)
	}
	var err error
	if state.layout, err = parseLayout(layoutRaw); err != nil {
		t.Fatal(err)
	}
	if state.versions, err = parseLayout(versionLayoutRaw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fieldsRaw, &state.fields); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestEmailBuiltFromBlocks(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	resetEmailBlocks(t, dbpool)
	admin := createTestUserWithRole(t, dbpool, "admin")
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")

	master := createLayoutTestEmail(t, dbpool, "layout-test-en", "en", "Hello Alice", "")
	translation := createLayoutTestEmail(t, dbpool, "layout-test-de", "de", "Hallo Alice", master)

	layout := emailblocks.Layout{{Block: "intro_body"}, {Block: "primary_cta"}, {Block: "footer"}}
	putLayout := func(id string, layout emailblocks.Layout) int {
		return blockRequest(t, dbpool, admin, http.MethodPut, "/api/emails/"+id+"/layout",
			updateEmailLayoutRequest{Layout: layout}).Code
	}
	if code := putLayout(master, emailblocks.Layout{{Block: "nope"}}); code != http.StatusBadRequest {
		t.Fatalf("an unknown block must be refused, got %d", code)
	}
	if code := putLayout(translation, layout); code != http.StatusBadRequest {
		t.Fatalf("a translation takes its blocks from EN, got %d", code)
	}

	// Switching to blocks keeps the text and sends a ready email to draft.
	if code := putLayout(master, layout); code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", code)
	}
	state := loadLayoutTestState(t, dbpool, master)
	if !sameLayout(state.layout, layout) || !sameLayout(state.versions, layout) {
		t.Fatalf("the email and its version must have the layout: %+v / %+v", state.layout, state.versions)
	}
	if state.fields["intro_body"].Value != "Hello Alice" || state.fields["primary_cta_text"].Value != "Open Partner Portal" {
		t.Fatalf("unexpected fields: %+v", state.fields)
	}
	if !strings.Contains(state.template, `data-review-block="primary_cta"`) || !strings.Contains(state.review, ">Subject layout-test-en</title>") {
		t.Fatal("the template must be assembled from the blocks, with the subject as title")
	}
	if state.status != "draft" || state.version != 2 || state.source != "manual" {
		t.Fatalf("unexpected status/version: %s v%d %s", state.status, state.version, state.source)
	}

	// The translation follows, with the master's text in the new blocks,
	// and shows as outdated.
	translated := loadLayoutTestState(t, dbpool, translation)
	if !sameLayout(translated.layout, layout) || translated.fields["intro_body"].Value != "Hallo Alice" ||
		translated.fields["primary_cta_text"].Value != "Open Partner Portal" {
		t.Fatalf("the translation must follow the master: %+v %+v", translated.layout, translated.fields)
	}
	status, err := loadTranslationStatus(ctx, dbpool, translation)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Stale || len(status.BlockChanges) != 0 {
		t.Fatalf("unexpected translation status: %+v", status)
	}

	// The blocks report their emails; a block change rebuilds them and
	// keeps their status.
	if _, err := dbpool.Exec(ctx, `UPDATE emails SET review_status = 'ready' WHERE id = $1;`, master); err != nil {
		t.Fatal(err)
	}
	response := blockRequest(t, dbpool, superAdmin, http.MethodGet, "/api/blocks", nil)
	var listed struct {
		Blocks []EmailBlock `json:"blocks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	usage := map[string]int{}
	for _, block := range listed.Blocks {
		usage[block.Key] = block.Emails
	}
	if usage[emailblocks.FrameKey] != 2 || usage["primary_cta"] != 2 || usage["hero_banner"] != 0 {
		t.Fatalf("unexpected usage: %+v", usage)
	}

	cta, _ := findEmailBlock(listed.Blocks, "primary_cta")
	edited := strings.Replace(cta.HTML, "border-radius: 10px;", "border-radius: 12px;", 1)
	response = blockRequest(t, dbpool, superAdmin, http.MethodPut, "/api/blocks/primary_cta",
		emailBlockRequest{HTML: edited, Version: cta.Version})
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var saved updateEmailBlockResponse
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.UpdatedEmails != 2 || saved.LiveEmails != 0 {
		t.Fatalf("unexpected result: %+v", saved)
	}
	state = loadLayoutTestState(t, dbpool, master)
	if !strings.Contains(state.review, "border-radius: 12px;") || state.source != "block" || state.status != "ready" || state.version != 3 {
		t.Fatalf("the email must be rebuilt with the block, ready: %s v%d %s", state.status, state.version, state.source)
	}

	// Removing a block and restoring the version before brings it back,
	// in the translation too, with the block as it is now.
	if code := putLayout(master, layout[:2]); code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", code)
	}
	if translated = loadLayoutTestState(t, dbpool, translation); len(translated.layout) != 2 {
		t.Fatalf("the translation must lose the block: %+v", translated.layout)
	}
	// Checked after the switch: now the removed block is what changed.
	if _, err := dbpool.Exec(ctx, `UPDATE emails SET translated_from_version = 2 WHERE id = $1;`, translation); err != nil {
		t.Fatal(err)
	}
	if status, err = loadTranslationStatus(ctx, dbpool, translation); err != nil {
		t.Fatal(err)
	}
	if !status.Stale || len(status.BlockChanges) != 1 ||
		status.BlockChanges[0] != (emailblocks.BlockChange{Action: "removed", Block: "footer", ID: "footer", Name: "Footer"}) {
		t.Fatalf("unexpected translation status: %+v", status)
	}
	restore := func(user AuthUser, versionNumber int) int {
		var id string
		if err := dbpool.QueryRow(ctx, `SELECT id FROM email_versions WHERE email_id = $1 AND version_number = $2;`,
			master, versionNumber).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return blockRequest(t, dbpool, user, http.MethodPost, fmt.Sprintf("/api/emails/%s/versions/%s/restore", master, id), nil).Code
	}
	if code := restore(admin, 2); code != http.StatusNoContent {
		t.Fatalf("restoring blocks is for board managers, got %d", code)
	}
	state = loadLayoutTestState(t, dbpool, master)
	if !sameLayout(state.layout, layout) || !strings.Contains(state.review, "border-radius: 12px;") || state.source != "restore" {
		t.Fatalf("unexpected state after restore: %+v %s", state.layout, state.source)
	}
	if translated = loadLayoutTestState(t, dbpool, translation); !sameLayout(translated.layout, layout) {
		t.Fatalf("the translation must follow the restore: %+v", translated.layout)
	}

	// Going back to the email's own HTML is a HTML change: super admins only.
	if code := restore(admin, 1); code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", code)
	}
	if code := restore(superAdmin, 1); code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", code)
	}
	if state = loadLayoutTestState(t, dbpool, master); state.layout != nil || state.template != layoutTestTemplate {
		t.Fatalf("the email must be back in its own template: %+v", state.layout)
	}
}
