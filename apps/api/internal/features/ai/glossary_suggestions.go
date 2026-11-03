package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

// GlossarySuggestion is a term someone proposes for the glossary. Language
// and Translation are empty when only the English term is proposed.
type GlossarySuggestion struct {
	ID               string     `json:"id"`
	Term             string     `json:"term"`
	Language         string     `json:"language"`
	Translation      string     `json:"translation"`
	Note             string     `json:"note"`
	EmailID          *string    `json:"email_id"`
	EmailTitle       *string    `json:"email_title"`
	SuggestedByEmail string     `json:"suggested_by_email"`
	Status           string     `json:"status"`
	DecidedByEmail   *string    `json:"decided_by_email"`
	DecidedAt        *time.Time `json:"decided_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

type glossarySuggestionInput struct {
	Term        string  `json:"term"`
	Language    string  `json:"language"`
	Translation string  `json:"translation"`
	Note        string  `json:"note"`
	EmailID     *string `json:"email_id"`
}

func registerGlossarySuggestionRoutes(r chi.Router, dbpool *pgxpool.Pool) {
	r.Get("/api/translation-glossary/suggestions", listGlossarySuggestionsHandler(dbpool))
	r.Post("/api/translation-glossary/suggestions", createGlossarySuggestionHandler(dbpool))
	r.Post("/api/translation-glossary/suggestions/{id}/accept", acceptGlossarySuggestionHandler(dbpool))
	r.Post("/api/translation-glossary/suggestions/{id}/reject", rejectGlossarySuggestionHandler(dbpool))
}

// normalizeGlossarySuggestion trims the input and checks it against the
// glossary's own limits.
func normalizeGlossarySuggestion(input glossarySuggestionInput) (glossarySuggestionInput, error) {
	input.Term = strings.TrimSpace(input.Term)
	input.Language = strings.ToLower(strings.TrimSpace(input.Language))
	input.Translation = strings.TrimSpace(input.Translation)
	input.Note = strings.TrimSpace(input.Note)
	if input.EmailID != nil && strings.TrimSpace(*input.EmailID) == "" {
		input.EmailID = nil
	}
	if input.Translation == "" {
		input.Language = ""
	}
	switch {
	case input.Term == "":
		return input, errors.New("the English term is required")
	case utf8.RuneCountInString(input.Term) > maxGlossaryTerm:
		return input, errors.New("the term is too long")
	case utf8.RuneCountInString(input.Translation) > maxGlossaryTranslation:
		return input, errors.New("the translation is too long")
	case utf8.RuneCountInString(input.Note) > maxGlossaryNote:
		return input, errors.New("the note is too long")
	}
	if input.Translation != "" {
		if _, known := languageNames[input.Language]; !known {
			return input, errors.New("pick the language of the translation")
		}
	}
	return input, nil
}

// mergeGlossarySuggestion adds an accepted suggestion to the glossary: a new
// term goes to the end; for a term already there it sets the translation and
// fills an empty note.
func mergeGlossarySuggestion(terms []GlossaryTerm, input glossarySuggestionInput) []GlossaryTerm {
	merged := make([]GlossaryTerm, len(terms))
	copy(merged, terms)
	for i, term := range merged {
		if !strings.EqualFold(term.Term, input.Term) {
			continue
		}
		translations := maps.Clone(term.Translations)
		if translations == nil {
			translations = map[string]string{}
		}
		if input.Translation != "" {
			translations[input.Language] = input.Translation
		}
		merged[i].Translations = translations
		if merged[i].Note == "" {
			merged[i].Note = input.Note
		}
		return merged
	}
	translations := map[string]string{}
	if input.Translation != "" {
		translations[input.Language] = input.Translation
	}
	return append(merged, GlossaryTerm{Term: input.Term, Translations: translations, Note: input.Note})
}

// listGlossarySuggestionsHandler gives a super admin every pending
// suggestion and anyone else their own recent ones, with what became of them.
func listGlossarySuggestionsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		suggestions, err := loadGlossarySuggestions(r.Context(), dbpool, user)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load glossary suggestions: %v\n", err)
			http.Error(w, "failed to load suggestions", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(suggestions)
	}
}

func loadGlossarySuggestions(ctx context.Context, dbpool *pgxpool.Pool, user AuthUser) ([]GlossarySuggestion, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT
			s.id, s.term, s.language, s.translation, s.note, s.email_id, emails.title,
			s.suggested_by_email, s.status, s.decided_by_email, s.decided_at, s.created_at
		FROM glossary_suggestions AS s
		LEFT JOIN emails ON emails.id = s.email_id
		WHERE CASE WHEN $2 THEN s.status = 'pending' ELSE s.suggested_by_user_id = $1::uuid END
		ORDER BY s.created_at DESC
		LIMIT 100;
	`, user.ID, auth.IsSuperAdmin(user))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	suggestions := []GlossarySuggestion{}
	for rows.Next() {
		var s GlossarySuggestion
		if err := rows.Scan(
			&s.ID, &s.Term, &s.Language, &s.Translation, &s.Note, &s.EmailID, &s.EmailTitle,
			&s.SuggestedByEmail, &s.Status, &s.DecidedByEmail, &s.DecidedAt, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, s)
	}
	return suggestions, rows.Err()
}

// createGlossarySuggestionHandler lets anyone signed in propose a term; a
// super admin decides.
func createGlossarySuggestionHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var input glossarySuggestionInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		input, err := normalizeGlossarySuggestion(input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var suggestion GlossarySuggestion
		err = dbpool.QueryRow(r.Context(), `
			INSERT INTO glossary_suggestions (term, language, translation, note, email_id, suggested_by_user_id, suggested_by_email)
			VALUES ($1, $2, $3, $4, (SELECT id FROM emails WHERE id::text = $5), $6, $7)
			RETURNING id, term, language, translation, note, email_id, suggested_by_email, status, created_at;
		`, input.Term, input.Language, input.Translation, input.Note, input.EmailID, user.ID, user.Email).Scan(
			&suggestion.ID, &suggestion.Term, &suggestion.Language, &suggestion.Translation, &suggestion.Note,
			&suggestion.EmailID, &suggestion.SuggestedByEmail, &suggestion.Status, &suggestion.CreatedAt,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to save glossary suggestion: %v\n", err)
			http.Error(w, "failed to save the suggestion", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(suggestion)
	}
}

// acceptGlossarySuggestionHandler adds a pending suggestion to the glossary,
// as the super admin edited it, and marks it accepted, in one transaction.
func acceptGlossarySuggestionHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireSuperAdmin(w, r)
		if !ok {
			return
		}
		var input glossarySuggestionInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		input, err := normalizeGlossarySuggestion(input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		terms, status, err := acceptGlossarySuggestion(r.Context(), dbpool, chi.URLParam(r, "id"), input, user.Email)
		if err != nil {
			if status == http.StatusInternalServerError {
				fmt.Fprintf(os.Stderr, "failed to accept glossary suggestion: %v\n", err)
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(glossaryPayload{Terms: terms})
	}
}

func acceptGlossarySuggestion(ctx context.Context, dbpool *pgxpool.Pool, id string, input glossarySuggestionInput, decidedBy string) ([]GlossaryTerm, int, error) {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE glossary_suggestions
		SET status = 'accepted', decided_by_email = $2, decided_at = now()
		WHERE id::text = $1 AND status = 'pending';
	`, id, decidedBy)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if tag.RowsAffected() == 0 {
		return nil, http.StatusNotFound, errors.New("no pending suggestion with this id")
	}
	// Serialize with other glossary writes: replaceGlossary rewrites the table.
	if _, err := tx.Exec(ctx, `LOCK TABLE translation_glossary IN EXCLUSIVE MODE;`); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	current, err := loadGlossary(ctx, tx)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	terms, err := normalizeGlossary(mergeGlossarySuggestion(current, input))
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	if err := replaceGlossary(ctx, tx, terms, decidedBy); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return terms, http.StatusOK, nil
}

func rejectGlossarySuggestionHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireSuperAdmin(w, r)
		if !ok {
			return
		}
		tag, err := dbpool.Exec(r.Context(), `
			UPDATE glossary_suggestions
			SET status = 'rejected', decided_by_email = $2, decided_at = now()
			WHERE id::text = $1 AND status = 'pending';
		`, chi.URLParam(r, "id"), user.Email)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to reject glossary suggestion: %v\n", err)
			http.Error(w, "failed to reject the suggestion", http.StatusInternalServerError)
			return
		}
		if tag.RowsAffected() == 0 {
			http.Error(w, "no pending suggestion with this id", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
