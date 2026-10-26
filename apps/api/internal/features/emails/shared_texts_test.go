package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

func TestApplySharedValuesTakesSharedTextAndDropsMissing(t *testing.T) {
	fields := emailedit.EditableFields{
		"footer":  {Type: "text", Value: "old", Shared: true},
		"logo":    {Type: "image", Value: "https://a/x.png", Shared: true},
		"gone":    {Type: "text", Value: "kept", Shared: true},
		"intro":   {Type: "text", Value: "Hi"},
		"privacy": {Type: "text", Value: "Privacy", Shared: true},
	}
	values := map[string]sharedValue{
		"footer":  {Type: "text", Value: "new"},
		"logo":    {Type: "image", Value: "https://a/y.png"},
		"intro":   {Type: "text", Value: "Hello"},
		"privacy": {Type: "url", Value: "https://a/privacy"},
	}

	got := applySharedValues(fields, values)

	if got["footer"].Value != "new" || !got["footer"].Shared {
		t.Errorf("footer: %+v", got["footer"])
	}
	if got["logo"].Value != "https://a/y.png" {
		t.Errorf("logo: %+v", got["logo"])
	}
	if got["gone"].Value != "kept" || got["gone"].Shared {
		t.Errorf("a field without a shared text keeps its value and stops being shared: %+v", got["gone"])
	}
	if got["intro"].Value != "Hi" || got["intro"].Shared {
		t.Errorf("a field that is not shared must not change: %+v", got["intro"])
	}
	if got["privacy"].Value != "Privacy" || got["privacy"].Shared {
		t.Errorf("a shared text of another type must not apply: %+v", got["privacy"])
	}
	if fields["footer"].Value != "old" {
		t.Error("the input must not be modified")
	}
}

func TestMarkSharedFieldsLinksOnlyEqualValues(t *testing.T) {
	fields := emailedit.EditableFields{
		"footer":    {Type: "text", Value: "© Acme"},
		"signature": {Type: "text", Value: "Jane"},
		"stale":     {Type: "text", Value: "old", Shared: true},
	}
	values := map[string]sharedValue{
		"footer":    {Type: "text", Value: "© Acme"},
		"signature": {Type: "text", Value: "Bitrix24 Team"},
		"stale":     {Type: "text", Value: "new"},
	}

	linked := markSharedFields(fields, values, true)
	if !linked["footer"].Shared {
		t.Error("an equal value must get linked")
	}
	if linked["signature"].Shared {
		t.Error("an email's own signature must stay its own")
	}
	if linked["stale"].Shared || linked["stale"].Value != "old" {
		t.Errorf("a shared mark on a different value must be dropped, value kept: %+v", linked["stale"])
	}

	if markSharedFields(fields, values, false)["footer"].Shared {
		t.Error("without autoLink nothing new gets linked")
	}
}

const sharedTestTemplate = `<html><body>` +
	`<p data-review-block="body-001" data-edit-text="intro">Hello</p>` +
	`<p data-review-block="footer-001" data-edit-text="test_shared_footer">© Acme</p>` +
	`</body></html>`

func createSharedTestEmail(t *testing.T, dbpool *pgxpool.Pool, slug string, position int, language, status, footer string, shared bool, translationOf string) string {
	t.Helper()
	fields := emailedit.EditableFields{
		"intro":              {Type: "text", Value: "Hello", Order: 1},
		"test_shared_footer": {Type: "text", Value: footer, Order: 2, Shared: shared},
	}
	fieldsJSON, _ := fields.JSON()
	rendered, err := emailedit.RenderEditableHTML(sharedTestTemplate, fields)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	err = dbpool.QueryRow(context.Background(), `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, review_status,
			original_html, review_html, template_html, editable_fields, translation_of, translated_from_version)
		VALUES ($1, $1, 'shared-text-test', 'test', $8, $2, 'new', $3, $4, $5, $4, $6::jsonb,
			nullif($7, '')::uuid, CASE WHEN $7 = '' THEN NULL ELSE 1 END)
		RETURNING id;
	`, slug, language, status, sharedTestTemplate, rendered, fieldsJSON, translationOf, position).Scan(&id)
	if err != nil {
		t.Fatalf("create %s: %v", slug, err)
	}
	if err := insertEmailVersion(context.Background(), dbpool, emailVersionSnapshot{
		EmailID: id, CreatedByEmail: "test", Source: "initial", Title: slug,
		OriginalHTML: sharedTestTemplate, TemplateHTML: sharedTestTemplate, EditableFieldsJSON: fieldsJSON,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE email_id = $1;`, id)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, id)
	})
	return id
}

func createSharedTestText(t *testing.T, dbpool *pgxpool.Pool, language, value string) {
	t.Helper()
	if _, err := dbpool.Exec(context.Background(), `
		INSERT INTO shared_texts (key, language, type, value, en_version)
		VALUES ('test_shared_footer', $1, 'text', $2, CASE WHEN $1 = 'en' THEN NULL ELSE 1 END);
	`, language, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM shared_texts WHERE key = 'test_shared_footer';`)
	})
}

func emailFieldValue(t *testing.T, dbpool *pgxpool.Pool, id, key string) (string, bool) {
	t.Helper()
	var fieldsJSON []byte
	if err := dbpool.QueryRow(context.Background(), `SELECT editable_fields FROM emails WHERE id = $1;`, id).Scan(&fieldsJSON); err != nil {
		t.Fatal(err)
	}
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		t.Fatal(err)
	}
	return fields[key].Value.(string), fields[key].Shared
}

func putSharedText(t *testing.T, dbpool *pgxpool.Pool, user AuthUser, language string, body updateSharedTextRequest) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	router.Put("/api/shared-texts/{key}/{language}", updateSharedTextHandler(dbpool))
	payload, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPut, "/api/shared-texts/test_shared_footer/"+language, bytes.NewReader(payload))
	request = request.WithContext(auth.WithUser(request.Context(), user))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestUpdateSharedTextWritesLinkedEmails(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	admin := createTestUserWithRole(t, dbpool, "admin")
	createSharedTestText(t, dbpool, "en", "© Acme")
	createSharedTestText(t, dbpool, "de", "© Acme DE")

	master := createSharedTestEmail(t, dbpool, "shared-test-master", 1, "en", "ready", "© Acme", true, "")
	other := createSharedTestEmail(t, dbpool, "shared-test-other", 2, "en", "draft", "© Acme", true, "")
	own := createSharedTestEmail(t, dbpool, "shared-test-own", 3, "en", "draft", "© Someone else", false, "")
	translation := createSharedTestEmail(t, dbpool, "shared-test-de", 1, "de", "draft", "© Acme DE", true, master)

	if code := putSharedText(t, dbpool, admin, "en", updateSharedTextRequest{Value: "© 2027 Acme", Version: 1}).Code; code != http.StatusForbidden {
		t.Fatalf("expected 403 for an admin, got %d", code)
	}
	if code := putSharedText(t, dbpool, superAdmin, "en", updateSharedTextRequest{Value: "© 2027 Acme", Version: 7}).Code; code != http.StatusConflict {
		t.Fatalf("expected 409 for an outdated version, got %d", code)
	}

	response := putSharedText(t, dbpool, superAdmin, "en", updateSharedTextRequest{Value: "© 2027 Acme", Version: 1})
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var result updateSharedTextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.UpdatedEmails != 2 || result.Text.Version != 2 || result.Text.LinkedEmails != 2 || result.Text.OwnEmails != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	for _, id := range []string{master, other} {
		if value, shared := emailFieldValue(t, dbpool, id, "test_shared_footer"); value != "© 2027 Acme" || !shared {
			t.Errorf("linked email %s: value %q shared %v", id, value, shared)
		}
		var reviewHTML, source string
		if err := dbpool.QueryRow(ctx, `
			SELECT e.review_html, v.source FROM emails e
			JOIN email_versions v ON v.email_id = e.id
			WHERE e.id = $1 ORDER BY v.version_number DESC LIMIT 1;
		`, id).Scan(&reviewHTML, &source); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(reviewHTML, "© 2027 Acme") || source != "shared_text" {
			t.Errorf("email %s: expected re-rendered HTML and a shared_text version, got source %q", id, source)
		}
	}
	var status string
	if err := dbpool.QueryRow(ctx, `SELECT review_status FROM emails WHERE id = $1;`, master).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "ready" {
		t.Errorf("a shared text edit must keep a ready email ready, got %q", status)
	}
	if value, _ := emailFieldValue(t, dbpool, own, "test_shared_footer"); value != "© Someone else" {
		t.Errorf("an email with its own text must not change, got %q", value)
	}
	if value, _ := emailFieldValue(t, dbpool, translation, "test_shared_footer"); value != "© Acme DE" {
		t.Errorf("the DE email must keep the DE text, got %q", value)
	}

	// The DE translation of the email is still up to date: the footer is
	// shared there and has its own translation, which now shows EN changed.
	status2, err := loadTranslationStatus(ctx, dbpool, translation)
	if err != nil {
		t.Fatal(err)
	}
	if status2.Stale {
		t.Errorf("a change to a shared field must not make the translation outdated: %+v", status2.Changes)
	}
	var stale bool
	if err := dbpool.QueryRow(ctx, `
		SELECT email_text_hash(fv.subject, fv.preheader, fv.editable_fields, t.editable_fields)
			<> email_text_hash(lv.subject, lv.preheader, lv.editable_fields, t.editable_fields)
		FROM emails t
		JOIN email_versions fv ON fv.email_id = t.translation_of AND fv.version_number = t.translated_from_version
		JOIN LATERAL (SELECT * FROM email_versions WHERE email_id = t.translation_of ORDER BY version_number DESC LIMIT 1) lv ON true
		WHERE t.id = $1;
	`, translation).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Error("the board must not show the translation as outdated either")
	}
	texts, err := listSharedTexts(ctx, dbpool)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range texts {
		if text.Key == "test_shared_footer" && text.Language == "de" && !text.ENChanged {
			t.Error("the DE shared text must show that EN changed")
		}
	}

	// Saving DE with the same value confirms it.
	response = putSharedText(t, dbpool, superAdmin, "de", updateSharedTextRequest{Value: "© Acme DE", Version: 1})
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.UpdatedEmails != 0 || result.Text.ENChanged || result.Text.Version != 1 {
		t.Errorf("confirming must not change emails or the version: %+v", result)
	}
}

func TestSaveEmailContentKeepsSharedValue(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	admin := createTestUserWithRole(t, dbpool, "admin")
	createSharedTestText(t, dbpool, "en", "© Acme")
	id := createSharedTestEmail(t, dbpool, "shared-test-save", 1, "en", "draft", "© Acme", true, "")

	save := func(footer emailedit.EditableField) {
		t.Helper()
		tx, err := dbpool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := saveEmailContent(ctx, tx, admin, id, emailContentUpdate{EditableFields: emailedit.EditableFields{
			"intro":              {Type: "text", Value: "Hello", Order: 1},
			"test_shared_footer": footer,
		}}, emailContentSaveOptions{Source: "manual"}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	save(emailedit.EditableField{Type: "text", Value: "edited by hand", Order: 2, Shared: true})
	if value, shared := emailFieldValue(t, dbpool, id, "test_shared_footer"); value != "© Acme" || !shared {
		t.Errorf("a shared field must keep the shared value, got %q shared %v", value, shared)
	}

	save(emailedit.EditableField{Type: "text", Value: "Own footer", Order: 2})
	if value, shared := emailFieldValue(t, dbpool, id, "test_shared_footer"); value != "Own footer" || shared {
		t.Errorf("unlinking must keep the email's own text, got %q shared %v", value, shared)
	}
}

func sendSharedText(t *testing.T, dbpool *pgxpool.Pool, user AuthUser, method, board string, body any) (int, updateSharedTextResponse) {
	t.Helper()
	router := chi.NewRouter()
	router.Put("/api/shared-texts/{key}/{language}", updateSharedTextHandler(dbpool))
	router.Post("/api/shared-texts/{key}/{language}", createBoardSharedTextHandler(dbpool))
	router.Delete("/api/shared-texts/{key}/{language}", deleteBoardSharedTextHandler(dbpool))
	payload, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/shared-texts/test_shared_footer/en?board="+board, bytes.NewReader(payload))
	request = request.WithContext(auth.WithUser(request.Context(), user))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var result updateSharedTextResponse
	if response.Code == http.StatusOK {
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
	}
	return response.Code, result
}

func TestBoardSharedTextOverridesTheTextForAllBoards(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	board := createTestBoard(t, dbpool, "shared-text-test", []string{"test"})
	createTestBoard(t, dbpool, "shared-text-other", []string{"test"})
	manager := createTestUserWithRole(t, dbpool, "reviewer")
	if _, err := dbpool.Exec(ctx, `UPDATE users SET managed_boards = $2 WHERE id = $1;`, manager.ID, []string{board}); err != nil {
		t.Fatal(err)
	}
	createSharedTestText(t, dbpool, "en", "© Acme")
	onBoard := createSharedTestEmail(t, dbpool, "shared-test-board", 1, "en", "draft", "© Acme", true, "")
	elsewhere := createSharedTestEmail(t, dbpool, "shared-test-elsewhere", 2, "en", "draft", "© Acme", true, "")
	if _, err := dbpool.Exec(ctx, `UPDATE emails SET sequence = 'shared-text-other' WHERE id = $1;`, elsewhere); err != nil {
		t.Fatal(err)
	}

	if code, _ := sendSharedText(t, dbpool, manager, http.MethodPut, "", updateSharedTextRequest{Value: "x", Version: 1}); code != http.StatusForbidden {
		t.Fatalf("a board manager must not edit the text for all boards, got %d", code)
	}
	if code, _ := sendSharedText(t, dbpool, manager, http.MethodPost, "shared-text-other", updateSharedTextRequest{Value: "x"}); code != http.StatusForbidden {
		t.Fatalf("a board manager must not add a text to another board, got %d", code)
	}

	code, result := sendSharedText(t, dbpool, manager, http.MethodPost, board, updateSharedTextRequest{Value: "© Acme Region"})
	if code != http.StatusOK || result.UpdatedEmails != 1 || result.Text.Board != board || result.Text.LinkedEmails != 1 {
		t.Fatalf("create: code %d, %+v", code, result)
	}
	if code, _ := sendSharedText(t, dbpool, manager, http.MethodPost, board, updateSharedTextRequest{Value: "again"}); code != http.StatusConflict {
		t.Fatalf("a second board version must be refused, got %d", code)
	}
	if value, shared := emailFieldValue(t, dbpool, onBoard, "test_shared_footer"); value != "© Acme Region" || !shared {
		t.Errorf("the board's email must take the board's text, got %q shared %v", value, shared)
	}
	if value, _ := emailFieldValue(t, dbpool, elsewhere, "test_shared_footer"); value != "© Acme" {
		t.Errorf("other boards must keep the text for all boards, got %q", value)
	}

	// The text for all boards no longer reaches the board's emails.
	code, result = sendSharedText(t, dbpool, superAdmin, http.MethodPut, "", updateSharedTextRequest{Value: "© 2027 Acme", Version: 1})
	if code != http.StatusOK || result.UpdatedEmails != 1 || result.Text.LinkedEmails != 1 {
		t.Fatalf("update for all boards: code %d, %+v", code, result)
	}
	if value, _ := emailFieldValue(t, dbpool, onBoard, "test_shared_footer"); value != "© Acme Region" {
		t.Errorf("the board's email must keep the board's text, got %q", value)
	}

	// Saving the email keeps the board's text.
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := saveEmailContent(ctx, tx, superAdmin, onBoard, emailContentUpdate{EditableFields: emailedit.EditableFields{
		"intro":              {Type: "text", Value: "Hi", Order: 1},
		"test_shared_footer": {Type: "text", Value: "typed", Order: 2, Shared: true},
	}}, emailContentSaveOptions{Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if value, _ := emailFieldValue(t, dbpool, onBoard, "test_shared_footer"); value != "© Acme Region" {
		t.Errorf("saving must keep the board's text, got %q", value)
	}

	code, result = sendSharedText(t, dbpool, manager, http.MethodPut, board, updateSharedTextRequest{Value: "© Acme Region 2", Version: 1})
	if code != http.StatusOK || result.UpdatedEmails != 1 || result.Text.Version != 2 {
		t.Fatalf("update the board's text: code %d, %+v", code, result)
	}

	code, result = sendSharedText(t, dbpool, manager, http.MethodDelete, board, nil)
	if code != http.StatusOK || result.UpdatedEmails != 1 || result.Text.Board != "" || result.Text.LinkedEmails != 2 {
		t.Fatalf("remove: code %d, %+v", code, result)
	}
	if value, shared := emailFieldValue(t, dbpool, onBoard, "test_shared_footer"); value != "© 2027 Acme" || !shared {
		t.Errorf("after removing, the board's email must go back to the text for all boards, got %q shared %v", value, shared)
	}
	if code, _ := sendSharedText(t, dbpool, superAdmin, http.MethodDelete, "", nil); code != http.StatusBadRequest {
		t.Fatalf("the text for all boards must not be removable, got %d", code)
	}
}
