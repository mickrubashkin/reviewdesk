package ai

import (
	"context"
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
	Email emails.EmailDetail `json:"email"`
	translatedSummary
}

// translatedSummary is how the AI translation of a new draft went; a job
// keeps it as its result.
type translatedSummary struct {
	Model    string   `json:"model"`
	Glossary []string `json:"glossary"`
	// KeptEnglish are fields left in English because the model's text was
	// unusable (e.g. a Bitrix24 expression got lost); fix them by hand.
	KeptEnglish []TranslationField `json:"kept_english"`
}

// createTranslationHandler creates the missing translation of an EN email in
// its slot. With the English text it answers with the new draft; with AI it
// starts a background job (202) whose draft appears when the model is done.
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
			http.Error(w, "AI is not configured", http.StatusServiceUnavailable)
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

		if !request.Translate {
			created, err := createTranslation(r.Context(), dbpool, user, masterID, language, map[string]string{}, map[string]any{})
			if err != nil {
				writeCreateError(w, masterID, language, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(createTranslationResponse{created, translatedSummary{Glossary: []string{}, KeptEnglish: []TranslationField{}}})
			return
		}

		fields := masterFieldsToTranslate(master)
		terms, err := loadGlossary(r.Context(), dbpool)
		if err != nil {
			http.Error(w, "failed to load the glossary", http.StatusInternalServerError)
			return
		}
		glossary := glossaryForFields(terms, fields)
		if refusal, status := demoAIQuotaRefusal(r, dbpool); refusal != "" {
			http.Error(w, refusal, status)
			return
		}
		job, err := startTranslationJob(r.Context(), dbpool, translationJobCreate, masterID, language, nil, user,
			func(ctx context.Context) (any, *string, error) {
				return translateAndCreate(ctx, dbpool, suggester, user, masterID, language, fields, glossary)
			})
		if err != nil {
			fmt.Fprintf(os.Stderr, "start translation of %s into %s: %v\n", masterID, language, err)
			http.Error(w, "failed to start the translation", http.StatusInternalServerError)
			return
		}
		writeTranslationJob(w, http.StatusAccepted, job)
	}
}

// translateAndCreate is the job behind "Translate and create": the model
// translates the master's text, then the draft is created with it.
func translateAndCreate(ctx context.Context, dbpool *pgxpool.Pool, suggester translationSuggester, user coreauth.AuthUser, masterID, language string, fields []TranslationField, glossary []GlossaryTerm) (any, *string, error) {
	result := translatedSummary{Glossary: []string{}, KeptEnglish: []TranslationField{}}
	for _, term := range glossary {
		result.Glossary = append(result.Glossary, term.Term)
	}
	startedAt := time.Now()
	translations, metrics, err := suggester.suggest(ctx, language, fields, glossary)
	metrics = finishAIAnalysisMetrics(metrics, startedAt, err)
	metrics.Status = "success"
	if err != nil {
		metrics.Status = "error"
		logTranslationCall(dbpool, masterID, user, metrics)
		return nil, nil, fmt.Errorf("translate %s into %s: %w", masterID, language, err)
	}
	result.Model = metrics.Model
	text := map[string]string{}
	for _, field := range fields {
		field.Suggestion = translations[field.Key]
		if field.Problem = checkSuggestion(field); field.Problem != "" {
			result.KeptEnglish = append(result.KeptEnglish, field)
			continue
		}
		text[field.Key] = field.Suggestion
	}

	created, err := createTranslation(ctx, dbpool, user, masterID, language, text, map[string]any{
		"ai_model":     result.Model,
		"kept_english": len(result.KeptEnglish),
	})
	if err != nil {
		logTranslationCall(dbpool, masterID, user, metrics)
		if errors.Is(err, emails.ErrTranslationExists) {
			return nil, nil, jobError{"This language was created meanwhile."}
		}
		return nil, nil, err
	}
	logTranslationCall(dbpool, created.ID, user, metrics)
	return result, &created.ID, nil
}

// createTranslation creates the draft in one transaction.
func createTranslation(ctx context.Context, dbpool *pgxpool.Pool, user coreauth.AuthUser, masterID, language string, text map[string]string, metadata map[string]any) (emails.EmailDetail, error) {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return emails.EmailDetail{}, err
	}
	defer tx.Rollback(ctx)
	created, err := emails.CreateTranslationTx(ctx, tx, user, masterID, language, text, metadata)
	if err != nil {
		return emails.EmailDetail{}, err
	}
	return created, tx.Commit(ctx)
}

func writeCreateError(w http.ResponseWriter, masterID, language string, err error) {
	switch {
	case errors.Is(err, emails.ErrTranslationExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "email not found", http.StatusNotFound)
	default:
		fmt.Fprintf(os.Stderr, "create translation of %s into %s: %v\n", masterID, language, err)
		http.Error(w, "failed to create the translation", http.StatusInternalServerError)
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
