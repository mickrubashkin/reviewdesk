package emails

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type takeEmailRequest struct {
	// Take makes the signed-in person the owner; false gives the email back.
	Take bool `json:"take"`
}

type takeEmailResponse struct {
	OwnerEmail *string `json:"owner_email"`
}

// takeEmailHandler lets a translator claim an email in one of their
// languages, so others translating the same language see it is taken.
// Admins can take any email.
func takeEmailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var request takeEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		id := chi.URLParam(r, "id")
		var slug, title, language string
		var currentOwner *string
		err = tx.QueryRow(r.Context(), `
			SELECT slug, title, language, owner_email FROM emails
			WHERE id = $1 AND archived_at IS NULL
			FOR UPDATE;
		`, id).Scan(&slug, &title, &language, &currentOwner)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}

		manages, err := auth.CanManageEmail(r.Context(), tx, user, id)
		if err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}
		if !manages {
			var languages []string
			if err := tx.QueryRow(r.Context(), `SELECT languages FROM users WHERE id = $1;`, user.ID).Scan(&languages); err != nil {
				http.Error(w, "failed to update email", http.StatusInternalServerError)
				return
			}
			if !slices.Contains(languages, strings.ToLower(language)) {
				http.Error(w, "you can only take emails in the languages you translate", http.StatusForbidden)
				return
			}
		}

		isMine := currentOwner != nil && strings.EqualFold(*currentOwner, user.Email)
		var nextOwner *string
		switch {
		case request.Take:
			nextOwner = &user.Email
		case isMine:
			nextOwner = nil
		default:
			http.Error(w, "only the owner can give the email back", http.StatusConflict)
			return
		}

		changes := map[string]any{}
		addStringChange(changes, "owner_email", currentOwner, nextOwner)
		if len(changes) > 0 {
			if _, err := tx.Exec(r.Context(), `
				UPDATE emails SET owner_email = $2, updated_at = now() WHERE id = $1;
			`, id, nextOwner); err != nil {
				http.Error(w, "failed to update email", http.StatusInternalServerError)
				return
			}
			if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
				ActorUserID: user.ID,
				ActorEmail:  user.Email,
				Action:      emailEventPlanningUpdated,
				EmailID:     &id,
				EmailSlug:   &slug,
				EmailTitle:  &title,
				Changes:     changes,
			}); err != nil {
				http.Error(w, "failed to record email event", http.StatusInternalServerError)
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(takeEmailResponse{OwnerEmail: nextOwner})
	}
}
