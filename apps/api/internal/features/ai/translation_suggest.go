package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

// languageNames names the translation languages for the model.
var languageNames = map[string]string{
	"de": "German",
	"es": "Spanish",
	"pl": "Polish",
	"br": "Brazilian Portuguese",
}

const translationInstructions = `You update translations of emails that Bitrix24 sends to companies in its partner program.

For each field you get the English text before and after a change, and the current %[1]s translation, which matched the English before.
Return the updated %[1]s translation of every field:
- Change only what the English change requires. Keep the rest of the current translation word for word, with its tone, form of address and terminology.
- If the current translation is empty, translate the English after.
- Copy Bitrix24 expressions such as {{Responsible person: Calendly onboarding link}}, {{=date('Y')}} or {=Document:ASSIGNED_BY_PRINTABLE} exactly, character for character, and keep them where the sentence needs them.
- Keep Markdown formatting as it is: **bold**, *italic*, [link text](url), # headings and lines starting with "- " or "1. ". Translate the words inside it, never the URL.
- Keep line breaks, URLs, email addresses, emoji and numbers. Do not translate brand, product or plan names (Bitrix24, Silver, Gold).
- Return the key of each field unchanged.`

// TranslationField is one changed field of the master and what the
// translator gets for it.
type TranslationField struct {
	Key        string `json:"key"`
	EnBefore   string `json:"en_before"`
	EnAfter    string `json:"en_after"`
	Current    string `json:"current"`
	Suggestion string `json:"suggestion"`
	// Problem says why the suggestion must not be applied as is, e.g. a
	// Bitrix24 expression got lost; empty when it is fine.
	Problem string `json:"problem"`
}

type translationSuggestionsResponse struct {
	Model  string             `json:"model"`
	Fields []TranslationField `json:"fields"`
	// Skipped are changed master fields the translation has no field for
	// (its keys differ); the translator handles them by hand.
	Skipped []string `json:"skipped"`
	// Glossary lists the glossary terms sent with the request.
	Glossary []string `json:"glossary"`
}

// translationSuggester asks a model for updated translations; a fake stands
// in for OpenAI in tests.
type translationSuggester interface {
	suggest(ctx context.Context, language string, fields []TranslationField, glossary []GlossaryTerm) (map[string]string, AIAnalysisMetrics, error)
}

type openAITranslationSuggester struct {
	apiKey string
	model  string
	client *http.Client
}

func newOpenAITranslationSuggester(apiKey string) openAITranslationSuggester {
	// Translation may use its own model; otherwise the one the reviews use.
	model := os.Getenv("OPENAI_TRANSLATION_MODEL")
	if model == "" {
		model = os.Getenv("OPENAI_MODEL")
	}
	if model == "" {
		model = "gpt-5-mini"
	}
	return openAITranslationSuggester{apiKey: apiKey, model: model, client: &http.Client{Timeout: 90 * time.Second}}
}

func (s openAITranslationSuggester) suggest(ctx context.Context, language string, fields []TranslationField, glossary []GlossaryTerm) (map[string]string, AIAnalysisMetrics, error) {
	metrics := AIAnalysisMetrics{Kind: "translation", Model: s.model}
	type inputField struct {
		Key                string `json:"key"`
		EnglishBefore      string `json:"english_before"`
		EnglishAfter       string `json:"english_after"`
		CurrentTranslation string `json:"current_translation"`
	}
	input := make([]inputField, 0, len(fields))
	for _, field := range fields {
		input = append(input, inputField{field.Key, field.EnBefore, field.EnAfter, field.Current})
	}
	inputJSON, err := json.Marshal(map[string]any{"fields": input})
	if err != nil {
		return nil, metrics, err
	}

	body := map[string]any{
		"model":        s.model,
		"instructions": fmt.Sprintf(translationInstructions, languageNames[language]) + glossaryInstructions(language, glossary),
		"input":        string(inputJSON),
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "translations",
				"strict": true,
				"schema": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"fields"},
					"properties": map[string]any{
						"fields": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"required":             []string{"key", "translation"},
								"properties": map[string]any{
									"key":         map[string]any{"type": "string"},
									"translation": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	}
	if strings.HasPrefix(s.model, "gpt-5") {
		body["reasoning"] = map[string]any{"effort": "low"}
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, metrics, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, metrics, err
	}
	request.Header.Set("Authorization", "Bearer "+s.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, metrics, err
	}
	defer response.Body.Close()
	responseBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, metrics, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, metrics, fmt.Errorf("openai returned %d: %s", response.StatusCode, string(responseBytes))
	}
	applyOpenAIUsage(&metrics, responseBytes)

	text, err := extractOpenAIOutputText(responseBytes)
	if err != nil {
		return nil, metrics, err
	}
	var output struct {
		Fields []struct {
			Key         string `json:"key"`
			Translation string `json:"translation"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(text), &output); err != nil {
		return nil, metrics, fmt.Errorf("parse translations: %w", err)
	}
	translations := map[string]string{}
	for _, field := range output.Fields {
		translations[field.Key] = field.Translation
	}
	return translations, metrics, nil
}

// loadTranslationText returns a translation's language and its subject,
// preheader and text fields, by the keys the master's changes use.
func loadTranslationText(ctx context.Context, dbpool *pgxpool.Pool, emailID string) (string, map[string]string, error) {
	var language string
	var subject, preheader *string
	var fieldsJSON []byte
	if err := dbpool.QueryRow(ctx, `
		SELECT lower(language), subject, preheader, editable_fields FROM emails WHERE id = $1 AND archived_at IS NULL;
	`, emailID).Scan(&language, &subject, &preheader, &fieldsJSON); err != nil {
		return "", nil, err
	}
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return "", nil, err
	}
	text := map[string]string{}
	if subject != nil {
		text["subject"] = *subject
	} else {
		text["subject"] = ""
	}
	if preheader != nil {
		text["preheader"] = *preheader
	} else {
		text["preheader"] = ""
	}
	for key, field := range fields {
		if field.Type == emailedit.FieldTypeText {
			text[key] = fmt.Sprint(field.Value)
		}
	}
	return language, text, nil
}

// buildTranslationFields pairs each change of the master with the
// translation's current text; changes the translation has no field for are
// returned as skipped.
func buildTranslationFields(changes []emails.TranslationChange, current map[string]string) ([]TranslationField, []string) {
	fields := []TranslationField{}
	skipped := []string{}
	for _, change := range changes {
		value, ok := current[change.Key]
		if !ok {
			skipped = append(skipped, change.Key)
			continue
		}
		fields = append(fields, TranslationField{Key: change.Key, EnBefore: change.Before, EnAfter: change.After, Current: value})
	}
	return fields, skipped
}

// checkSuggestion explains what is wrong with a suggestion, or returns "".
func checkSuggestion(field TranslationField) string {
	if strings.TrimSpace(field.Suggestion) == "" && strings.TrimSpace(field.EnAfter) != "" {
		return "No suggestion came back for this field."
	}
	want := emailedit.FindBitrixExpressions(field.EnAfter)
	got := emailedit.FindBitrixExpressions(field.Suggestion)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		return "Bitrix24 expressions differ from the English text; fix them before applying."
	}
	return ""
}

func suggestTranslationHandler(dbpool *pgxpool.Pool, suggester translationSuggester, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		if canEdit, err := emails.CanEditEmail(r.Context(), dbpool, user, id); err != nil || !canEdit {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !enabled {
			http.Error(w, "OPENAI_API_KEY is not configured", http.StatusServiceUnavailable)
			return
		}

		status, err := emails.LoadTranslationStatus(r.Context(), dbpool, id)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "this email is not a translation", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "failed to load translation status", http.StatusInternalServerError)
			return
		}
		if !status.Stale {
			http.Error(w, "this translation is up to date with EN", http.StatusConflict)
			return
		}
		language, current, err := loadTranslationText(r.Context(), dbpool, id)
		if err != nil {
			http.Error(w, "failed to load translation", http.StatusInternalServerError)
			return
		}
		if _, known := languageNames[language]; !known {
			http.Error(w, "no AI translation for this language", http.StatusBadRequest)
			return
		}

		fields, skipped := buildTranslationFields(status.Changes, current)
		result := translationSuggestionsResponse{Fields: fields, Skipped: skipped, Glossary: []string{}}
		if len(fields) > 0 {
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
			suggestions, metrics, err := suggester.suggest(r.Context(), language, fields, glossary)
			metrics = finishAIAnalysisMetrics(metrics, startedAt, err)
			metrics.Status = "success"
			if err != nil {
				metrics.Status = "error"
			}
			if logErr := insertAIAnalysisLog(r.Context(), dbpool, id, &user, metrics, false); logErr != nil {
				fmt.Fprintf(os.Stderr, "failed to log translation suggestion for %s: %v\n", id, logErr)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "translation suggestion for %s failed: %v\n", id, err)
				http.Error(w, "the AI translation failed, try again", http.StatusBadGateway)
				return
			}
			result.Model = metrics.Model
			for i := range result.Fields {
				result.Fields[i].Suggestion = suggestions[result.Fields[i].Key]
				result.Fields[i].Problem = checkSuggestion(result.Fields[i])
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}
