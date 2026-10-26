package emails

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

var (
	errRestoreNotArchived = errors.New("email is not archived")
	// errRestorePlaceTaken means an active email already holds the same
	// board, stage, position, language, variant and adaptation.
	errRestorePlaceTaken = errors.New("place taken")
)

func restoreEmailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
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

		err := restoreArchivedEmail(r.Context(), dbpool, id, user)
		switch {
		case errors.Is(err, pgx.ErrNoRows), errors.Is(err, errRestoreNotArchived):
			http.Error(w, "archived email not found", http.StatusNotFound)
		case errors.Is(err, errRestorePlaceTaken):
			http.Error(w, "an active email already takes this place (same stage, position, language, variant and adaptation); archive it or move it first", http.StatusConflict)
		case err != nil:
			fmt.Fprintf(os.Stderr, "failed to restore email %s: %v\n", id, err)
			http.Error(w, "failed to restore email", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

// restoreArchivedEmail puts an archived email back on its board, in the place
// it had, with its original slug when that is still free.
func restoreArchivedEmail(ctx context.Context, dbpool *pgxpool.Pool, id string, user AuthUser) error {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var snapshot emailArchiveSnapshot
	var archived bool
	if err := tx.QueryRow(ctx, `
		SELECT id, slug, title, language, variant, stage, archived_at IS NOT NULL
		FROM emails
		WHERE id = $1
		FOR UPDATE;
	`, id).Scan(&snapshot.ID, &snapshot.Slug, &snapshot.Title, &snapshot.Language, &snapshot.Variant, &snapshot.Stage, &archived); err != nil {
		return err
	}
	if !archived {
		return errRestoreNotArchived
	}

	var placeTaken bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM emails active, emails archived
			WHERE archived.id = $1
				AND active.archived_at IS NULL
				AND coalesce(active.sequence, '') = coalesce(archived.sequence, '')
				AND coalesce(active.stage, '') = coalesce(archived.stage, '')
				AND active.sort_order = archived.sort_order
				AND active.language = archived.language
				AND active.variant = archived.variant
				AND active.adaptation_key = archived.adaptation_key
		);
	`, id).Scan(&placeTaken); err != nil {
		return err
	}
	if placeTaken {
		return errRestorePlaceTaken
	}

	slug := restoredEmailSlug(snapshot.Slug, snapshot.ID)
	var slugTaken bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM emails WHERE slug = $1 AND id <> $2);`, slug, id).Scan(&slugTaken); err != nil {
		return err
	}
	if slugTaken {
		slug = fmt.Sprintf("%s-%s", slug, shortID(snapshot.ID))
	}

	if _, err := tx.Exec(ctx, `
		UPDATE emails
		SET archived_at = NULL,
			archived_by = NULL,
			slug = $2,
			updated_at = now()
		WHERE id = $1;
	`, id, slug); err != nil {
		if isEmailCreationConflict(err) {
			return errRestorePlaceTaken
		}
		return err
	}
	if err := InsertEmailEvent(ctx, tx, EmailEventParam{
		ActorUserID: user.ID,
		ActorEmail:  user.Email,
		Action:      emailEventRestored,
		EmailID:     &snapshot.ID,
		EmailSlug:   &slug,
		EmailTitle:  &snapshot.Title,
		Metadata: map[string]any{
			"slug":     slug,
			"title":    snapshot.Title,
			"language": snapshot.Language,
			"variant":  snapshot.Variant,
			"stage":    snapshot.Stage,
		},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// restoredEmailSlug undoes archivedEmailSlug.
func restoredEmailSlug(slug string, id string) string {
	return strings.TrimSuffix(slug, "--archived-"+id)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
