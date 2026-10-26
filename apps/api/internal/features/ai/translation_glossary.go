package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

const (
	maxGlossaryTerms       = 200
	maxGlossaryTerm        = 100
	maxGlossaryTranslation = 200
	maxGlossaryNote        = 300
)

// GlossaryTerm is an English term the AI pre-translation must handle
// strictly. Translations maps a language to the required wording; a language
// without one keeps the English term as is.
type GlossaryTerm struct {
	Term         string            `json:"term"`
	Translations map[string]string `json:"translations"`
	Note         string            `json:"note"`
}

type glossaryPayload struct {
	Terms []GlossaryTerm `json:"terms"`
}

func getGlossaryHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.FromRequest(r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		terms, err := loadGlossary(r.Context(), dbpool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load translation glossary: %v\n", err)
			http.Error(w, "failed to load the glossary", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(glossaryPayload{Terms: terms})
	}
}

func putGlossaryHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireSuperAdmin(w, r)
		if !ok {
			return
		}
		var request glossaryPayload
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		terms, err := normalizeGlossary(request.Terms)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveGlossary(r.Context(), dbpool, terms, user.Email); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save translation glossary: %v\n", err)
			http.Error(w, "failed to save the glossary", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(glossaryPayload{Terms: terms})
	}
}

func loadGlossary(ctx context.Context, dbpool *pgxpool.Pool) ([]GlossaryTerm, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT term, translations, note FROM translation_glossary ORDER BY position;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	terms := []GlossaryTerm{}
	for rows.Next() {
		var term GlossaryTerm
		var translationsJSON []byte
		if err := rows.Scan(&term.Term, &translationsJSON, &term.Note); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(translationsJSON, &term.Translations); err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}
	return terms, rows.Err()
}

func saveGlossary(ctx context.Context, dbpool *pgxpool.Pool, terms []GlossaryTerm, updatedBy string) error {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM translation_glossary;`); err != nil {
		return err
	}
	for position, term := range terms {
		translationsJSON, err := json.Marshal(term.Translations)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO translation_glossary (position, term, translations, note, updated_by_email)
			VALUES ($1, $2, $3::jsonb, $4, $5);
		`, position, term.Term, translationsJSON, term.Note, updatedBy); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func normalizeGlossary(input []GlossaryTerm) ([]GlossaryTerm, error) {
	if len(input) > maxGlossaryTerms {
		return nil, fmt.Errorf("at most %d terms", maxGlossaryTerms)
	}
	terms := make([]GlossaryTerm, 0, len(input))
	seen := map[string]bool{}
	for _, term := range input {
		term.Term = strings.TrimSpace(term.Term)
		term.Note = strings.TrimSpace(term.Note)
		translations := map[string]string{}
		for language, translation := range term.Translations {
			translation = strings.TrimSpace(translation)
			if _, known := languageNames[language]; !known || translation == "" {
				continue
			}
			if utf8.RuneCountInString(translation) > maxGlossaryTranslation {
				return nil, fmt.Errorf("%q: the %s translation is too long", term.Term, languageNames[language])
			}
			translations[language] = translation
		}
		term.Translations = translations
		folded := strings.ToLower(term.Term)
		switch {
		case term.Term == "":
			return nil, errors.New("every row needs an English term")
		case utf8.RuneCountInString(term.Term) > maxGlossaryTerm:
			return nil, fmt.Errorf("term %q is too long", term.Term)
		case utf8.RuneCountInString(term.Note) > maxGlossaryNote:
			return nil, fmt.Errorf("%q: the note is too long", term.Term)
		case seen[folded]:
			return nil, fmt.Errorf("%q is already in the glossary", term.Term)
		}
		seen[folded] = true
		terms = append(terms, term)
	}
	return terms, nil
}

// glossaryForFields keeps the terms that occur, as whole words and in any
// case, in the English text of the fields, so the prompt carries only what
// this email needs.
func glossaryForFields(terms []GlossaryTerm, fields []TranslationField) []GlossaryTerm {
	var english strings.Builder
	for _, field := range fields {
		english.WriteString(field.EnBefore)
		english.WriteString("\n")
		english.WriteString(field.EnAfter)
		english.WriteString("\n")
	}
	text := english.String()
	used := []GlossaryTerm{}
	for _, term := range terms {
		pattern := `(?i)(^|[^\pL\pN])` + regexp.QuoteMeta(term.Term) + `($|[^\pL\pN])`
		if regexp.MustCompile(pattern).MatchString(text) {
			used = append(used, term)
		}
	}
	return used
}

// glossaryInstructions renders the terms for one language as an addition to
// the translation instructions; "" when there are none.
func glossaryInstructions(language string, terms []GlossaryTerm) string {
	if len(terms) == 0 {
		return ""
	}
	lines := make([]string, 0, len(terms))
	for _, term := range terms {
		line := fmt.Sprintf("- %q: keep in English", term.Term)
		if translation := term.Translations[language]; translation != "" {
			line = fmt.Sprintf("- %q: %q", term.Term, translation)
		}
		if term.Note != "" {
			line += " (" + term.Note + ")"
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	return fmt.Sprintf(`

Glossary. Render these English terms exactly as given, adapting only grammatical endings the %s sentence requires. Where the current translation renders one of them differently, correct it too:
%s`, languageNames[language], strings.Join(lines, "\n"))
}
