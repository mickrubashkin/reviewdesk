package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

type createTranslationRequest struct {
	Language string `json:"language"`
	// Translate asks the model for the text; false copies the English.
	Translate bool `json:"translate"`
}

type createTranslationResponse struct {
	Email    emails.EmailDetail `json:"email"`
	Model    string             `json:"model"`
	Glossary []string           `json:"glossary"`
	// KeptEnglish are fields left in English because the model's text was
	// unusable (e.g. a Bitrix24 expression got lost); fix them by hand.
	KeptEnglish []TranslationField `json:"kept_english"`
}

// createTranslationHandler creates the missing translation of an EN email in
// its slot: a draft whose text the model translated, glossary applied.
func createTranslationHandler(dbpool *pgxpool.Pool, suggester translationSuggester, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var request createTranslationRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		language := strings.ToLower(strings.TrimSpace(request.Language))
		if _, known := languageNames[language]; !known {
			http.Error(w, "pick one of the translation languages", http.StatusBadRequest)
			return
		}
		masterID := chi.URLParam(r, "id")
		if allowed, err := emails.CanTranslateInto(r.Context(), dbpool, user, masterID, language); err != nil || !allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if request.Translate && !enabled {
			http.Error(w, "OPENAI_API_KEY is not configured", http.StatusServiceUnavailable)
			return
		}

		master, err := emails.MasterText(r.Context(), dbpool, masterID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			http.Error(w, "email not found", http.StatusNotFound)
			return
		case errors.Is(err, emails.ErrNotAMaster):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		case err != nil:
			http.Error(w, "failed to load the EN email", http.StatusInternalServerError)
			return
		}

		result := createTranslationResponse{Glossary: []string{}, KeptEnglish: []TranslationField{}}
		text := map[string]string{}
		var metrics *AIAnalysisMetrics
		if request.Translate {
			fields := masterFieldsToTranslate(master)
			terms, err := loadGlossary(r.Context(), dbpool)
			if err != nil {
				http.Error(w, "failed to load the glossary", http.StatusInternalServerError)
				return
			}
			glossary := glossaryForFields(terms, fields)
			for _, term := range glossary {
				result.Glossary = append(result.Glossary, term.Term)
			}
			if refusal, status := demoAIQuotaRefusal(r, dbpool); refusal != "" {
				http.Error(w, refusal, status)
				return
			}
			startedAt := time.Now()
			translations, usage, err := suggester.suggest(r.Context(), language, fields, glossary)
			usage = finishAIAnalysisMetrics(usage, startedAt, err)
			usage.Status = "success"
			if err != nil {
				usage.Status = "error"
				logTranslationCall(r, dbpool, masterID, user, usage)
				fmt.Fprintf(os.Stderr, "translation of %s into %s failed: %v\n", masterID, language, err)
				http.Error(w, "the AI translation failed, try again", http.StatusBadGateway)
				return
			}
			metrics = &usage
			result.Model = usage.Model
			for _, field := range fields {
				field.Suggestion = translations[field.Key]
				if field.Problem = checkSuggestion(field); field.Problem != "" {
					result.KeptEnglish = append(result.KeptEnglish, field)
					continue
				}
				text[field.Key] = field.Suggestion
			}
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to create the translation", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())
		created, err := emails.CreateTranslationTx(r, tx, user, masterID, language, text, map[string]any{
			"ai_model":     result.Model,
			"kept_english": len(result.KeptEnglish),
		})
		switch {
		case errors.Is(err, emails.ErrTranslationExists):
			http.Error(w, err.Error(), http.StatusConflict)
			return
		case errors.Is(err, pgx.ErrNoRows):
			http.Error(w, "email not found", http.StatusNotFound)
			return
		case err != nil:
			fmt.Fprintf(os.Stderr, "create translation of %s into %s: %v\n", masterID, language, err)
			http.Error(w, "failed to create the translation", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to create the translation", http.StatusInternalServerError)
			return
		}
		if metrics != nil {
			logTranslationCall(r, dbpool, created.ID, user, *metrics)
		}

		result.Email = created
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// masterFieldsToTranslate turns the master's non-empty text into fields
// with no current translation, so the model translates them whole.
func masterFieldsToTranslate(master map[string]string) []TranslationField {
	keys := make([]string, 0, len(master))
	for key, value := range master {
		if strings.TrimSpace(value) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	fields := make([]TranslationField, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, TranslationField{Key: key, EnAfter: master[key]})
	}
	return fields
}

// logTranslationCall records the model call on the new email, or on the
// master when the call failed and nothing was created.
func logTranslationCall(r *http.Request, dbpool *pgxpool.Pool, emailID string, user coreauth.AuthUser, metrics AIAnalysisMetrics) {
	if err := insertAIAnalysisLog(r.Context(), dbpool, emailID, &user, metrics, false); err != nil {
		fmt.Fprintf(os.Stderr, "failed to log translation call: %v\n", err)
	}
}
