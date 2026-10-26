package emails

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
)

func TestSlotFromBlocks(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	resetEmailBlocks(t, dbpool)
	admin := createTestUserWithRole(t, dbpool, "admin")
	board := createTestBoard(t, dbpool, "slot-blocks-test", []string{"first", "second"})
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE email_id IN (SELECT id FROM emails WHERE sequence = $1);`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE sequence = $1 AND translation_of IS NOT NULL;`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE sequence = $1;`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM shared_texts WHERE key = 'signature_sender';`)
	})
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO shared_texts (key, language, type, value, en_version)
		VALUES ('signature_sender', 'en', 'text', 'Partner Team', NULL),
			('signature_sender', 'de', 'text', 'Partner-Team', 1);
	`); err != nil {
		t.Fatal(err)
	}

	call := func(method, path string, body any) (int, string) {
		response := blockRequest(t, dbpool, admin, method, path, body)
		return response.Code, response.Body.String()
	}
	defaultFor := func(stage string) emailblocks.Layout {
		code, body := call(http.MethodGet, "/api/default-layout?sequence="+board+"&stage="+stage, nil)
		if code != http.StatusOK {
			t.Fatalf("default layout: %d %s", code, body)
		}
		var response defaultLayoutResponse
		if err := json.Unmarshal([]byte(body), &response); err != nil {
			t.Fatal(err)
		}
		return response.Layout
	}

	// A board without emails from blocks starts with the library default.
	if layout := defaultFor("first"); !sameLayout(layout, emailblocks.DefaultLayout()) {
		t.Fatalf("expected the library default, got %+v", layout)
	}

	code, body := call(http.MethodPost, "/api/layout-preview", layoutPreviewRequest{
		Sequence: board, Language: "de", Layout: emailblocks.DefaultLayout(), Subject: stringPointer("Hallo"),
	})
	if code != http.StatusOK || !strings.Contains(body, "Partner-Team") || !strings.Contains(body, "Hallo") {
		t.Fatalf("the preview must use the DE shared text and the subject: %d %.300s", code, body)
	}

	slot := func(stage string, layout emailblocks.Layout, emails ...createSlotEmailRequest) (int, string) {
		return call(http.MethodPost, "/api/slots", createSlotRequest{
			Sequence: board, Stage: stage, Title: "Welcome", Layout: layout, Emails: emails,
		})
	}
	en := createSlotEmailRequest{Language: "en", Subject: stringPointer("Welcome aboard")}
	if code, body := slot("first", emailblocks.DefaultLayout(), createSlotEmailRequest{Language: "en", OriginalHTML: "<p>x</p>"}); code != http.StatusBadRequest {
		t.Fatalf("blocks and HTML together must be refused: %d %s", code, body)
	}
	if code, body := slot("first", emailblocks.Layout{{Block: "nope"}}, en); code != http.StatusBadRequest {
		t.Fatalf("an unknown block must be refused: %d %s", code, body)
	}

	code, body = slot("first", emailblocks.DefaultLayout(), en, createSlotEmailRequest{Language: "de"})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	var created createSlotResponse
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Emails) != 2 || created.Emails[0].Language != "en" {
		t.Fatalf("expected EN then DE, got %+v", created.Emails)
	}
	master, translation := created.Emails[0].ID, created.Emails[1].ID
	for _, id := range []string{master, translation} {
		state := loadLayoutTestState(t, dbpool, id)
		if !sameLayout(state.layout, emailblocks.DefaultLayout()) || !sameLayout(state.versions, emailblocks.DefaultLayout()) {
			t.Fatalf("%s: the email and its first version must have the blocks: %+v / %+v", id, state.layout, state.versions)
		}
		if state.fields["primary_cta_text"].Value != "Open Partner Portal" {
			t.Fatalf("%s: the email's own texts start as written in the block: %+v", id, state.fields["primary_cta_text"])
		}
	}
	if state := loadLayoutTestState(t, dbpool, translation); !strings.Contains(state.template, `<html lang="de"`) {
		t.Fatal("the DE email's document must be in German")
	}
	if value, shared := emailFieldValue(t, dbpool, translation, "signature_sender"); value != "Partner-Team" || !shared {
		t.Fatalf("DE takes its shared text: %q %v", value, shared)
	}
	if value, _ := emailFieldValue(t, dbpool, master, "signature_sender"); value != "Partner Team" {
		t.Fatalf("EN takes its shared text: %q", value)
	}
	if state := loadLayoutTestState(t, dbpool, master); !strings.Contains(state.review, ">Welcome aboard</title>") {
		t.Fatal("the subject must be the title of the email")
	}
	var translationOf string
	if err := dbpool.QueryRow(ctx, `SELECT translation_of FROM emails WHERE id = $1;`, translation).Scan(&translationOf); err != nil || translationOf != master {
		t.Fatalf("DE must be a translation of EN: %q %v", translationOf, err)
	}
	if issues := checkTestEmail(t, dbpool, master); len(issues) != 0 {
		t.Fatalf("a new email from the default blocks has no issues: %+v", issues)
	}

	// The most common layout of the stage wins, then of the board.
	short := emailblocks.Layout{{Block: "intro_body"}, {Block: "primary_cta"}}
	if code, body := slot("second", short, en); code != http.StatusCreated {
		t.Fatalf("create in second: %d %s", code, body)
	}
	if layout := defaultFor("second"); !sameLayout(layout, short) {
		t.Fatalf("expected the layout of the stage, got %+v", layout)
	}
	if layout := defaultFor("first"); !sameLayout(layout, emailblocks.DefaultLayout()) {
		t.Fatalf("the first stage keeps its own, got %+v", layout)
	}
}

func checkTestEmail(t *testing.T, dbpool *pgxpool.Pool, id string) []emailblocks.Issue {
	t.Helper()
	var layoutJSON, fieldsJSON []byte
	var subject, preheader *string
	if err := dbpool.QueryRow(context.Background(), `
		SELECT layout, editable_fields, subject, preheader FROM emails WHERE id = $1;
	`, id).Scan(&layoutJSON, &fieldsJSON, &subject, &preheader); err != nil {
		t.Fatal(err)
	}
	issues, err := checkStoredEmail(context.Background(), dbpool, layoutJSON, fieldsJSON, subject, preheader)
	if err != nil {
		t.Fatal(err)
	}
	return issues
}

func stringPointer(value string) *string { return &value }
