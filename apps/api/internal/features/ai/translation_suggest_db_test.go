package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeSuggester struct {
	gotLanguage string
	gotFields   []TranslationField
	gotGlossary []GlossaryTerm
	reply       map[string]string
}

func (f *fakeSuggester) suggest(_ context.Context, language string, fields []TranslationField, glossary []GlossaryTerm) (map[string]string, AIAnalysisMetrics, error) {
	f.gotLanguage, f.gotFields, f.gotGlossary = language, fields, glossary
	return f.reply, AIAnalysisMetrics{Kind: "translation", Model: "fake-model"}, nil
}

func TestSuggestTranslationForTranslator(t *testing.T) {
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

	insertEmail := func(slug, language, fields string, translationOf *string) string {
		var id string
		if err := dbpool.QueryRow(ctx, `
			INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, editable_fields, subject, translation_of, translated_from_version)
			VALUES ($1, 'Suggest test', 'suggest-test', 'test', 9990, $2, 'new', '<html></html>', '<html></html>', $3::jsonb, 'Subject', $4, CASE WHEN $4::uuid IS NULL THEN NULL ELSE 1 END)
			RETURNING id;
		`, slug, language, fields, translationOf).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM emails WHERE id = $1;`, id) })
		return id
	}
	addVersion := func(emailID string, number int, fields string) {
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO email_versions (email_id, version_number, created_by_email, source, title, subject, original_html, template_html, editable_fields)
			VALUES ($1, $2, 'test', 'manual', 'Suggest test', 'Subject', '<html></html>', '<html></html>', $3::jsonb);
		`, emailID, number, fields); err != nil {
			t.Fatal(err)
		}
	}

	v1 := `{"intro":{"type":"text","value":"Book a call"},"cta":{"type":"text","value":"Go"},"signature":{"type":"text","value":"Alice"}}`
	v2 := `{"intro":{"type":"text","value":"Book a call at {{Responsible person: Calendly onboarding link}}"},"cta":{"type":"text","value":"Go now"},"signature":{"type":"text","value":"Alice K."}}`
	masterID := insertEmail("suggest-test-en", "en", v2, nil)
	addVersion(masterID, 1, v1)
	addVersion(masterID, 2, v2)
	translationID := insertEmail("suggest-test-de", "de",
		`{"intro":{"type":"text","value":"Termin buchen"},"cta":{"type":"text","value":"Los"},"signature_sender":{"type":"text","value":"Alice"}}`, &masterID)

	var translatorID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO users (email, role, languages) VALUES ('suggest-translator@acme.example', 'reviewer', '{de}')
		ON CONFLICT (email) DO UPDATE SET role = 'reviewer', languages = '{de}' RETURNING id;
	`).Scan(&translatorID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(ctx, `DELETE FROM ai_analysis_logs WHERE email_id = $1;`, translationID)
		_, _ = dbpool.Exec(ctx, `DELETE FROM users WHERE id = $1;`, translatorID)
	})
	translator := AuthUser{ID: translatorID, Email: "suggest-translator@acme.example", Role: "reviewer"}
	outsider := AuthUser{ID: "00000000-0000-0000-0000-000000000000", Email: "x@acme.example", Role: "reviewer"}

	if _, err := dbpool.Exec(ctx, `
		INSERT INTO translation_glossary (position, term, translations) VALUES
			(9990, 'book a call', '{"de":"Termin buchen"}'), (9991, 'Suggest test unused term', '{}');
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM translation_glossary WHERE position IN (9990, 9991);`) })

	fake := &fakeSuggester{reply: map[string]string{
		"intro": "Termin buchen unter {{Responsible person: Calendly onboarding link}}",
		"cta":   "Jetzt los {{Verantwortliche Person}}",
	}}
	router := chi.NewRouter()
	router.Post("/api/emails/{id}/translation/suggestions", suggestTranslationHandler(dbpool, fake, true))
	call := func(user AuthUser) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/emails/"+translationID+"/translation/suggestions", nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, withAuthUser(request, user))
		return response
	}

	if response := call(outsider); response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for someone who does not translate DE, got %d", response.Code)
	}
	response := call(translator)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var result translationSuggestionsResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if fake.gotLanguage != "de" || len(fake.gotFields) != 2 {
		t.Fatalf("expected two DE fields sent to the model, got %q %+v", fake.gotLanguage, fake.gotFields)
	}
	if len(fake.gotGlossary) != 1 || fake.gotGlossary[0].Translations["de"] != "Termin buchen" || len(result.Glossary) != 1 || result.Glossary[0] != "book a call" {
		t.Fatalf("expected only the term the email uses, got %+v %v", fake.gotGlossary, result.Glossary)
	}
	byKey := map[string]TranslationField{}
	for _, field := range result.Fields {
		byKey[field.Key] = field
	}
	if byKey["intro"].Current != "Termin buchen" || byKey["intro"].Problem != "" {
		t.Fatalf("unexpected intro %+v", byKey["intro"])
	}
	if byKey["cta"].Problem == "" {
		t.Fatalf("expected the made-up expression in cta to be flagged, got %+v", byKey["cta"])
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "signature" || result.Model != "fake-model" {
		t.Fatalf("unexpected skipped/model %v %q", result.Skipped, result.Model)
	}
	var kind, status string
	if err := dbpool.QueryRow(ctx, `SELECT kind, status FROM ai_analysis_logs WHERE email_id = $1;`, translationID).Scan(&kind, &status); err != nil {
		t.Fatal(err)
	}
	if kind != "translation" || status != "success" {
		t.Fatalf("expected a successful translation log, got %s %s", kind, status)
	}
}
