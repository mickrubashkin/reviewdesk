package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateTranslationFromEN(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for this integration test")
	}
	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dbpool.Close)

	template := `<html><body><p data-edit-text="intro">Book a call</p><a href="#" data-edit-text="cta">Go</a></body></html>`
	fields := `{"intro":{"type":"text","value":"Book a call at {{Responsible person: Calendly onboarding link}}"},"cta":{"type":"text","value":"Go now"}}`
	var masterID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, editable_fields, subject, send_timing)
		VALUES ('create-translation-test-en', 'Create test', 'create-translation-test', 'test', 9980, 'en', 'new', $1, $1, $2::jsonb, 'Welcome', '1 day')
		RETURNING id;
	`, template, fields).Scan(&masterID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO email_versions (email_id, version_number, created_by_email, source, title, subject, original_html, template_html, editable_fields)
		VALUES ($1, 3, 'test', 'manual', 'Create test', 'Welcome', $2, $2, $3::jsonb);
	`, masterID, template, fields); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		rows, _ := dbpool.Query(ctx, `SELECT id FROM emails WHERE sequence = 'create-translation-test';`)
		var ids []string
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			for _, table := range []string{"ai_analysis_logs", "email_events", "email_versions"} {
				_, _ = dbpool.Exec(ctx, `DELETE FROM `+table+` WHERE email_id = $1;`, id)
			}
		}
		_, _ = dbpool.Exec(ctx, `DELETE FROM emails WHERE sequence = 'create-translation-test' AND language <> 'en';`)
		_, _ = dbpool.Exec(ctx, `DELETE FROM emails WHERE id = $1;`, masterID)
	})
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO boards (key, name, stages) VALUES ('create-translation-test', 'Create test', '["test"]'::jsonb)
		ON CONFLICT (key) DO NOTHING;
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM boards WHERE key = 'create-translation-test';`) })

	var translatorID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO users (email, role, languages) VALUES ('create-translator@acme.example', 'reviewer', '{de}')
		ON CONFLICT (email) DO UPDATE SET role = 'reviewer', languages = '{de}' RETURNING id;
	`).Scan(&translatorID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM users WHERE id = $1;`, translatorID) })
	translator := AuthUser{ID: translatorID, Email: "create-translator@acme.example", Role: "reviewer"}

	// The CTA comes back with an invented expression: it stays in English.
	fake := &fakeSuggester{reply: map[string]string{
		"subject": "Willkommen",
		"intro":   "Buchen Sie einen Termin unter {{Responsible person: Calendly onboarding link}}",
		"cta":     "Jetzt los {{Verantwortliche Person}}",
	}}
	router := chi.NewRouter()
	router.Post("/api/emails/{id}/translations", createTranslationHandler(dbpool, fake, true))
	call := func(language string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"language": language, "translate": true})
		request := withAuthUser(httptest.NewRequest(http.MethodPost, "/api/emails/"+masterID+"/translations", bytes.NewReader(body)), translator)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	if response := call("es"); response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 outside the translator's languages, got %d", response.Code)
	}

	response := call("de")
	if response.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", response.Code, response.Body.String())
	}
	var result createTranslationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if fake.gotLanguage != "de" || len(fake.gotFields) != 3 {
		t.Fatalf("expected subject, intro and cta sent for DE, got %s %+v", fake.gotLanguage, fake.gotFields)
	}
	if len(result.KeptEnglish) != 1 || result.KeptEnglish[0].Key != "cta" {
		t.Fatalf("expected the CTA kept in English, got %+v", result.KeptEnglish)
	}

	var language, stage, status, subject, reviewHTML, fieldsJSON, timing string
	var sortOrder, fromVersion int
	var translationOf string
	if err := dbpool.QueryRow(ctx, `
		SELECT language, stage, sort_order, review_status, subject, review_html, editable_fields::text, send_timing, translation_of, translated_from_version
		FROM emails WHERE id = $1;
	`, result.Email.ID).Scan(&language, &stage, &sortOrder, &status, &subject, &reviewHTML, &fieldsJSON, &timing, &translationOf, &fromVersion); err != nil {
		t.Fatal(err)
	}
	if language != "de" || stage != "test" || sortOrder != 9980 || status != "draft" || timing != "1 day" {
		t.Fatalf("unexpected place or status: %s %s #%d %s %s", language, stage, sortOrder, status, timing)
	}
	if translationOf != masterID || fromVersion != 3 {
		t.Fatalf("expected a translation of the master's v3, got %s v%d", translationOf, fromVersion)
	}
	if subject != "Willkommen" || !strings.Contains(fieldsJSON, "Buchen Sie") || !strings.Contains(fieldsJSON, "Go now") {
		t.Fatalf("unexpected text: %s %s", subject, fieldsJSON)
	}
	if !strings.Contains(reviewHTML, "Buchen Sie einen Termin") {
		t.Fatalf("the HTML must be rendered from the translated fields: %s", reviewHTML)
	}

	if response := call("de"); response.Code != http.StatusConflict {
		t.Fatalf("expected 409 when the slot has DE already, got %d", response.Code)
	}
}
