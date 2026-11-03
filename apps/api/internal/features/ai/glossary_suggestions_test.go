package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNormalizeGlossarySuggestion(t *testing.T) {
	got, err := normalizeGlossarySuggestion(glossarySuggestionInput{Term: "  Deals ", Language: "DE", Translation: " Aufträge "})
	if err != nil || got.Term != "Deals" || got.Language != "de" || got.Translation != "Aufträge" {
		t.Fatalf("unexpected %+v %v", got, err)
	}
	if got, err := normalizeGlossarySuggestion(glossarySuggestionInput{Term: "CRM", Language: "de"}); err != nil || got.Language != "" {
		t.Fatalf("a term without a translation keeps no language, got %+v %v", got, err)
	}
	if _, err := normalizeGlossarySuggestion(glossarySuggestionInput{Term: " "}); err == nil {
		t.Fatal("expected an empty term to be rejected")
	}
	if _, err := normalizeGlossarySuggestion(glossarySuggestionInput{Term: "Deals", Language: "fr", Translation: "Affaires"}); err == nil {
		t.Fatal("expected a language the glossary has no column for to be rejected")
	}
}

func TestMergeGlossarySuggestion(t *testing.T) {
	terms := []GlossaryTerm{{Term: "Deals", Translations: map[string]string{"de": "Aufträge"}}}

	merged := mergeGlossarySuggestion(terms, glossarySuggestionInput{Term: "deals", Language: "es", Translation: "Negociaciones", Note: "CRM"})
	if len(merged) != 1 || merged[0].Translations["de"] != "Aufträge" || merged[0].Translations["es"] != "Negociaciones" || merged[0].Note != "CRM" {
		t.Fatalf("expected the ES translation added to the existing term, got %+v", merged)
	}
	if terms[0].Translations["es"] != "" {
		t.Fatal("merge must not change the glossary it was given")
	}

	merged = mergeGlossarySuggestion(terms, glossarySuggestionInput{Term: "CoPilot"})
	if len(merged) != 2 || merged[1].Term != "CoPilot" || len(merged[1].Translations) != 0 {
		t.Fatalf("expected a new term kept in English at the end, got %+v", merged)
	}
}

func TestGlossarySuggestionFlow(t *testing.T) {
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

	var translatorID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO users (email, role, languages) VALUES ('glossary-suggester@acme.example', 'reviewer', '{de}')
		ON CONFLICT (email) DO UPDATE SET role = 'reviewer' RETURNING id;
	`).Scan(&translatorID); err != nil {
		t.Fatal(err)
	}
	var saved []byte
	if err := dbpool.QueryRow(ctx, `
		SELECT COALESCE(jsonb_agg(to_jsonb(g) ORDER BY position), '[]') FROM translation_glossary AS g;
	`).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(ctx, `DELETE FROM glossary_suggestions WHERE suggested_by_user_id = $1;`, translatorID)
		_, _ = dbpool.Exec(ctx, `DELETE FROM users WHERE id = $1;`, translatorID)
		_, _ = dbpool.Exec(ctx, `DELETE FROM translation_glossary;`)
		_, _ = dbpool.Exec(ctx, `INSERT INTO translation_glossary SELECT * FROM jsonb_populate_recordset(NULL::translation_glossary, $1::jsonb);`, saved)
	})
	if _, err := dbpool.Exec(ctx, `DELETE FROM translation_glossary;`); err != nil {
		t.Fatal(err)
	}

	translator := AuthUser{ID: translatorID, Email: "glossary-suggester@acme.example", Role: "reviewer"}
	superAdmin := AuthUser{ID: "00000000-0000-0000-0000-000000000000", Email: "owner@acme.example", Role: "super_admin"}
	router := chi.NewRouter()
	registerGlossarySuggestionRoutes(router, dbpool)
	call := func(user AuthUser, method, path string, body any) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		request := httptest.NewRequest(method, path, bytes.NewReader(payload))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, withAuthUser(request, user))
		return response
	}

	response := call(translator, http.MethodPost, "/api/translation-glossary/suggestions",
		glossarySuggestionInput{Term: "Deals", Language: "de", Translation: "Deals"})
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var created GlossarySuggestion
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	accept := "/api/translation-glossary/suggestions/" + created.ID + "/accept"
	edited := glossarySuggestionInput{Term: "Deals", Language: "de", Translation: "Aufträge"}
	if response := call(translator, http.MethodPost, accept, edited); response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a translator accepting, got %d", response.Code)
	}

	response = call(superAdmin, http.MethodGet, "/api/translation-glossary/suggestions", nil)
	var pending []GlossarySuggestion
	if err := json.NewDecoder(response.Body).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	if len(pending) == 0 || pending[0].ID != created.ID {
		t.Fatalf("expected the suggestion in the super admin's queue, got %+v", pending)
	}

	if response := call(superAdmin, http.MethodPost, accept, edited); response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	terms, err := loadGlossary(ctx, dbpool)
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 1 || terms[0].Term != "Deals" || terms[0].Translations["de"] != "Aufträge" {
		t.Fatalf("expected the edited term in the glossary, got %+v", terms)
	}
	if response := call(superAdmin, http.MethodPost, accept, edited); response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 accepting twice, got %d", response.Code)
	}

	response = call(translator, http.MethodGet, "/api/translation-glossary/suggestions", nil)
	var own []GlossarySuggestion
	if err := json.NewDecoder(response.Body).Decode(&own); err != nil {
		t.Fatal(err)
	}
	if len(own) != 1 || own[0].Status != "accepted" || own[0].DecidedByEmail == nil {
		t.Fatalf("expected the translator to see their suggestion accepted, got %+v", own)
	}
}
