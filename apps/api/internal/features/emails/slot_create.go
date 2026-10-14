package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

const slotMasterLanguage = "en"

type createSlotRequest struct {
	Sequence      string                   `json:"sequence"`
	Stage         string                   `json:"stage"`
	Title         string                   `json:"title"`
	SortOrder     *int                     `json:"sort_order"`
	SendTiming    *string                  `json:"send_timing"`
	SendCondition *string                  `json:"send_condition"`
	Emails        []createSlotEmailRequest `json:"emails"`
}

type createSlotEmailRequest struct {
	Language     string  `json:"language"`
	Subject      *string `json:"subject"`
	Preheader    *string `json:"preheader"`
	OriginalHTML string  `json:"original_html"`
}

type createSlotResponse struct {
	SortOrder int           `json:"sort_order"`
	Emails    []EmailDetail `json:"emails"`
}

var errSlotPositionTaken = errors.New("this position in the stage is already used by another email")

// createSlotHandler creates one slot of the sequence with all its languages
// in a single transaction. EN is required because it is the source language.
func createSlotHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.IsAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var request createSlotRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := validateSlotRequest(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to create slot", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		sortOrder, err := resolveSlotSortOrder(r.Context(), tx, request)
		if errors.Is(err, errSlotPositionTaken) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			writeCreateEmailError(w, err)
			return
		}

		// EN first: the other languages are created as its translations.
		sort.SliceStable(request.Emails, func(i, j int) bool {
			return request.Emails[i].Language == slotMasterLanguage && request.Emails[j].Language != slotMasterLanguage
		})
		response := createSlotResponse{SortOrder: sortOrder, Emails: []EmailDetail{}}
		masterID := ""
		for _, email := range request.Emails {
			created, err := CreateEmailTx(r, dbpool, tx, user, NewEmailParams{
				Sequence:      request.Sequence,
				Title:         request.Title,
				Subject:       email.Subject,
				Preheader:     email.Preheader,
				SendTiming:    request.SendTiming,
				SendCondition: request.SendCondition,
				Stage:         request.Stage,
				SortOrder:     sortOrder,
				Language:      email.Language,
				Variant:       "v1",
				OriginalHTML:  email.OriginalHTML,
			})
			if err != nil {
				var inputErr createEmailInputError
				if errors.As(err, &inputErr) {
					err = createEmailInputError(strings.ToUpper(email.Language) + ": " + inputErr.Error())
				}
				writeCreateEmailError(w, err)
				return
			}
			if email.Language == slotMasterLanguage {
				masterID = created.ID
			} else if masterID != "" {
				if err := linkTranslation(r.Context(), tx, created.ID, masterID); err != nil {
					writeCreateEmailError(w, err)
					return
				}
			}
			response.Emails = append(response.Emails, created)
		}

		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to create slot", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(response)
	}
}

func validateSlotRequest(request *createSlotRequest) error {
	request.Sequence = strings.TrimSpace(request.Sequence)
	request.Stage = strings.TrimSpace(request.Stage)
	request.Title = strings.TrimSpace(request.Title)
	if request.Sequence == "" || request.Stage == "" || request.Title == "" {
		return errors.New("sequence, stage and title are required")
	}
	if len(request.Emails) == 0 {
		return errors.New("at least one email is required")
	}

	seen := map[string]bool{}
	for i := range request.Emails {
		language := strings.ToLower(strings.TrimSpace(request.Emails[i].Language))
		if language == "" {
			return errors.New("every email needs a language")
		}
		if seen[language] {
			return fmt.Errorf("language %s is listed twice", strings.ToUpper(language))
		}
		if strings.TrimSpace(request.Emails[i].OriginalHTML) == "" {
			return fmt.Errorf("%s: HTML is empty", strings.ToUpper(language))
		}
		seen[language] = true
		request.Emails[i].Language = language
	}
	if !seen[slotMasterLanguage] {
		return errors.New("EN is required: it is the source language of the slot")
	}
	return nil
}

// resolveSlotSortOrder checks a requested position or picks the next free one
// in the stage. Empty stages start at (stage index + 1) * 100 + 1, matching
// the existing numbering (registered 101, qualified 201, …).
func resolveSlotSortOrder(ctx context.Context, tx pgx.Tx, request createSlotRequest) (int, error) {
	if request.SortOrder != nil {
		var taken bool
		err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM emails
				WHERE sequence = $1 AND stage = $2 AND sort_order = $3 AND archived_at IS NULL
			);
		`, request.Sequence, request.Stage, *request.SortOrder).Scan(&taken)
		if err != nil {
			return 0, err
		}
		if taken {
			return 0, errSlotPositionTaken
		}
		return *request.SortOrder, nil
	}

	var maxSortOrder *int
	if err := tx.QueryRow(ctx, `
		SELECT max(sort_order) FROM emails
		WHERE sequence = $1 AND stage = $2 AND archived_at IS NULL;
	`, request.Sequence, request.Stage).Scan(&maxSortOrder); err != nil {
		return 0, err
	}
	if maxSortOrder != nil {
		return *maxSortOrder + 1, nil
	}

	var stagesJSON []byte
	err := tx.QueryRow(ctx, `SELECT stages FROM boards WHERE key = $1;`, request.Sequence).Scan(&stagesJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, createEmailInputError("board not found")
	}
	if err != nil {
		return 0, err
	}
	var stages []string
	if err := json.Unmarshal(stagesJSON, &stages); err != nil {
		return 0, err
	}
	for i, stage := range stages {
		if stage == request.Stage {
			return (i+1)*100 + 1, nil
		}
	}
	return 0, createEmailInputError("stage does not belong to the board")
}
