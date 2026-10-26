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
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/boards"
)

type updateEmailRegionRequest struct {
	// RegionKey is a region of the email's board; "" leaves the email
	// without one.
	RegionKey string `json:"region_key"`
}

// updateEmailRegionHandler puts an email in a region of its board.
func updateEmailRegionHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !requireManageEmail(w, r, dbpool, user, id) {
			return
		}
		var request updateEmailRegionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to save the region", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())
		err = setEmailRegion(r.Context(), tx, user, id, strings.TrimSpace(request.RegionKey))
		var contentErr emailContentError
		switch {
		case errors.As(err, &contentErr):
			http.Error(w, contentErr.message, contentErr.status)
			return
		case err != nil:
			fmt.Fprintf(os.Stderr, "failed to set the region of %s: %v\n", id, err)
			http.Error(w, "failed to save the region", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to save the region", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func setEmailRegion(ctx context.Context, tx pgx.Tx, user AuthUser, id, regionKey string) error {
	var slug, title, board string
	var current *string
	err := tx.QueryRow(ctx, `
		SELECT slug, title, coalesce(sequence, ''), region_key FROM emails
		WHERE id = $1 AND archived_at IS NULL FOR UPDATE;
	`, id).Scan(&slug, &title, &board, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return emailContentError{http.StatusNotFound, "email not found"}
	}
	if err != nil {
		return err
	}
	if regionKey != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM board_regions WHERE board_key = $1 AND key = $2);
		`, board, regionKey).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return emailContentError{http.StatusBadRequest, "the email's board has no region " + regionKey}
		}
	}
	if stringFromPointer(current) == regionKey {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE emails SET region_key = nullif($2, ''), updated_at = now() WHERE id = $1;
	`, id, regionKey); err != nil {
		return err
	}
	if err := linkToSlotReference(ctx, tx, id); err != nil {
		return err
	}
	changes := map[string]any{
		"region": map[string]any{"before": stringFromPointer(current), "after": regionKey},
	}
	// The email's version in the slot follows the region.
	adaptations, err := boards.SyncRegionAdaptations(ctx, tx, board, id)
	if errors.Is(err, boards.ErrRegionSlotTaken) {
		return emailContentError{http.StatusConflict, "the slot already has an email in this language and version in that region"}
	}
	if err != nil {
		return err
	}
	for _, change := range adaptations {
		changes["adaptation_label"] = map[string]any{"before": change.Before, "after": change.After}
	}
	return InsertEmailEvent(ctx, tx, EmailEventParam{
		ActorUserID: user.ID,
		ActorEmail:  user.Email,
		Action:      emailEventUpdated,
		EmailID:     &id,
		EmailSlug:   &slug,
		EmailTitle:  &title,
		Changes:     changes,
	})
}
