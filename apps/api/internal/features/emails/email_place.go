package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

const (
	emailEventPlaced            = "email_placed"
	emailEventTranslationLinked = "email_translation_linked"
)

type placeEmailRequest struct {
	Sequence string `json:"sequence"`
	Stage    string `json:"stage"`
	// SortOrder is the slot to place the email in; nil makes a new slot at
	// the end of the stage.
	SortOrder *int `json:"sort_order"`
}

type placeEmailResponse struct {
	ID            string  `json:"id"`
	Sequence      string  `json:"sequence"`
	Stage         string  `json:"stage"`
	SortOrder     int     `json:"sort_order"`
	TranslationOf *string `json:"translation_of"`
}

var errSlotLanguageTaken = errors.New("this slot already has an email in this language with the same version and adaptation")

// placeEmailHandler moves an email into a slot of a board, e.g. a live email
// imported from the portal into the sequence it belongs to. A non-EN email
// becomes the translation of the slot's EN email.
func placeEmailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !requireManageEmail(w, r, dbpool, user, chi.URLParam(r, "id")) {
			return
		}
		var request placeEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		request.Sequence = strings.TrimSpace(request.Sequence)
		request.Stage = strings.TrimSpace(request.Stage)
		if request.Sequence == "" || request.Stage == "" {
			http.Error(w, "sequence and stage are required", http.StatusBadRequest)
			return
		}
		if !requireManageBoard(w, r, dbpool, user, request.Sequence) {
			return
		}

		response, err := placeEmail(r.Context(), dbpool, user, chi.URLParam(r, "id"), request)
		var inputErr createEmailInputError
		switch {
		case err == nil:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		case errors.As(err, &inputErr):
			http.Error(w, inputErr.Error(), http.StatusBadRequest)
		case errors.Is(err, errSlotLanguageTaken):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, pgx.ErrNoRows):
			http.Error(w, "email not found", http.StatusNotFound)
		default:
			fmt.Fprintf(os.Stderr, "place email: %v\n", err)
			http.Error(w, "failed to place email", http.StatusInternalServerError)
		}
	}
}

func placeEmail(ctx context.Context, db *pgxpool.Pool, user AuthUser, id string, request placeEmailRequest) (placeEmailResponse, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return placeEmailResponse{}, err
	}
	defer tx.Rollback(ctx)

	response, err := placeEmailTx(ctx, tx, user, id, request)
	if err != nil {
		return placeEmailResponse{}, err
	}
	return response, tx.Commit(ctx)
}

func placeEmailTx(ctx context.Context, tx pgx.Tx, user AuthUser, id string, request placeEmailRequest) (placeEmailResponse, error) {
	var err error
	var slug, title, language, fromSequence, fromStage string
	var fromSortOrder int
	if err := tx.QueryRow(ctx, `
		SELECT slug, title, language, sequence, stage, sort_order FROM emails
		WHERE id = $1 AND archived_at IS NULL FOR UPDATE;
	`, id).Scan(&slug, &title, &language, &fromSequence, &fromStage, &fromSortOrder); err != nil {
		return placeEmailResponse{}, err
	}

	sortOrder := 0
	if request.SortOrder != nil {
		sortOrder = *request.SortOrder
		if err := requireStageOnBoard(ctx, tx, request.Sequence, request.Stage); err != nil {
			return placeEmailResponse{}, err
		}
	} else {
		sortOrder, err = resolveSlotSortOrder(ctx, tx, createSlotRequest{Sequence: request.Sequence, Stage: request.Stage})
		if err != nil {
			return placeEmailResponse{}, err
		}
	}

	// The slot's timing and condition apply to every language; take them
	// from an email already in the slot when there is one.
	if _, err := tx.Exec(ctx, `
		UPDATE emails e
		SET sequence = $2, stage = $3, sort_order = $4,
			send_timing = coalesce(slot.send_timing, e.send_timing),
			send_condition = coalesce(slot.send_condition, e.send_condition),
			updated_at = now()
		FROM (SELECT 1) AS one
		LEFT JOIN LATERAL (
			SELECT send_timing, send_condition FROM emails
			WHERE sequence = $2 AND stage = $3 AND sort_order = $4 AND archived_at IS NULL AND id <> $1
			ORDER BY (language = 'en') DESC, (variant <> 'old') DESC, created_at
			LIMIT 1
		) slot ON true
		WHERE e.id = $1;
	`, id, request.Sequence, request.Stage, sortOrder); err != nil {
		if isEmailCreationConflict(err) {
			return placeEmailResponse{}, errSlotLanguageTaken
		}
		return placeEmailResponse{}, err
	}

	response := placeEmailResponse{ID: id, Sequence: request.Sequence, Stage: request.Stage, SortOrder: sortOrder}
	// A region's own version follows the reference email of the new slot
	// instead of becoming a translation.
	adapts, err := adaptsReference(ctx, tx, id)
	if err != nil {
		return placeEmailResponse{}, err
	}
	if adapts {
		if err := linkToSlotReference(ctx, tx, id); err != nil {
			return placeEmailResponse{}, err
		}
	} else if language == "en" {
		if _, err := tx.Exec(ctx, `UPDATE emails SET translation_of = NULL, translated_from_version = NULL WHERE id = $1;`, id); err != nil {
			return placeEmailResponse{}, err
		}
	} else {
		var masterID string
		err := tx.QueryRow(ctx, `
			SELECT id FROM emails
			WHERE sequence = $1 AND stage = $2 AND sort_order = $3 AND language = 'en' AND archived_at IS NULL
			ORDER BY (variant <> 'old') DESC, (adaptation_key = 'default') DESC, created_at DESC
			LIMIT 1;
		`, request.Sequence, request.Stage, sortOrder).Scan(&masterID)
		switch {
		case err == nil:
			if err := linkTranslation(ctx, tx, id, masterID); err != nil {
				return placeEmailResponse{}, err
			}
			response.TranslationOf = &masterID
		case errors.Is(err, pgx.ErrNoRows):
			if _, err := tx.Exec(ctx, `UPDATE emails SET translation_of = NULL, translated_from_version = NULL WHERE id = $1;`, id); err != nil {
				return placeEmailResponse{}, err
			}
		default:
			return placeEmailResponse{}, err
		}
	}

	if err := InsertEmailEvent(ctx, tx, EmailEventParam{
		ActorUserID: user.ID,
		ActorEmail:  user.Email,
		Action:      emailEventPlaced,
		EmailID:     &id,
		EmailSlug:   &slug,
		EmailTitle:  &title,
		Metadata: map[string]any{
			"from": map[string]any{"sequence": fromSequence, "stage": fromStage, "sort_order": fromSortOrder},
			"to":   map[string]any{"sequence": request.Sequence, "stage": request.Stage, "sort_order": sortOrder},
		},
	}); err != nil {
		return placeEmailResponse{}, err
	}
	return response, nil
}

func requireStageOnBoard(ctx context.Context, tx pgx.Tx, boardKey, stage string) error {
	var stagesJSON []byte
	err := tx.QueryRow(ctx, `SELECT stages FROM boards WHERE key = $1;`, boardKey).Scan(&stagesJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return createEmailInputError("board not found")
	}
	if err != nil {
		return err
	}
	var stages []string
	if err := json.Unmarshal(stagesJSON, &stages); err != nil {
		return err
	}
	for _, s := range stages {
		if s == stage {
			return nil
		}
	}
	return createEmailInputError("stage does not belong to the board")
}

type translationLinkRequest struct {
	// MasterID is the EN email this one translates; null removes the link.
	MasterID *string `json:"master_id"`
}

// translationLinkHandler sets by hand which EN email a translation follows,
// for translations the automatic matching could not pair.
func translationLinkHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !requireManageEmail(w, r, dbpool, user, chi.URLParam(r, "id")) {
			return
		}
		var request translationLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		id := chi.URLParam(r, "id")

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to update translation", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		var slug, title, language, sequence string
		if err := tx.QueryRow(r.Context(), `
			SELECT slug, title, language, sequence FROM emails WHERE id = $1 AND archived_at IS NULL;
		`, id).Scan(&slug, &title, &language, &sequence); err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}
		if request.MasterID == nil {
			if _, err := tx.Exec(r.Context(), `UPDATE emails SET translation_of = NULL, translated_from_version = NULL WHERE id = $1;`, id); err != nil {
				http.Error(w, "failed to update translation", http.StatusInternalServerError)
				return
			}
		} else {
			if language == "en" {
				http.Error(w, "an EN email is a master, not a translation", http.StatusBadRequest)
				return
			}
			var masterLanguage, masterSequence string
			if err := tx.QueryRow(r.Context(), `
				SELECT language, sequence FROM emails WHERE id = $1 AND archived_at IS NULL;
			`, *request.MasterID).Scan(&masterLanguage, &masterSequence); err != nil {
				http.Error(w, "master email not found", http.StatusBadRequest)
				return
			}
			if masterLanguage != "en" || masterSequence != sequence {
				http.Error(w, "the master must be an EN email on the same board", http.StatusBadRequest)
				return
			}
			if err := linkTranslation(r.Context(), tx, id, *request.MasterID); err != nil {
				http.Error(w, "failed to update translation", http.StatusInternalServerError)
				return
			}
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventTranslationLinked,
			EmailID:     &id,
			EmailSlug:   &slug,
			EmailTitle:  &title,
			Metadata:    map[string]any{"master_id": request.MasterID},
		}); err != nil {
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update translation", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
