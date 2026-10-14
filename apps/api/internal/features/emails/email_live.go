package emails

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type markEmailLiveRequest struct {
	Live bool   `json:"live"`
	Note string `json:"note"`
}

type markEmailLiveResponse struct {
	LiveMarkedAt *time.Time `json:"live_marked_at"`
	LiveMarkedBy *string    `json:"live_marked_by"`
	LiveNote     *string    `json:"live_note"`
}

// markEmailLiveHandler lets a super admin confirm by hand that an email is
// live, for emails the portal sync cannot see (e.g. sent from the partner
// admin panel instead of a CRM robot).
func markEmailLiveHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok || !auth.IsSuperAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var request markEmailLiveRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		note := strings.TrimSpace(request.Note)
		if len([]rune(note)) > 500 {
			http.Error(w, "note must be at most 500 characters", http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		id := chi.URLParam(r, "id")
		var response markEmailLiveResponse
		var slug, title string
		err = tx.QueryRow(r.Context(), `
			UPDATE emails
			SET live_marked_at = CASE WHEN $2 THEN now() ELSE NULL END,
				live_marked_by = CASE WHEN $2 THEN $3 ELSE NULL END,
				live_note = CASE WHEN $2 THEN NULLIF($4, '') ELSE NULL END
			WHERE id = $1 AND archived_at IS NULL
			RETURNING slug, title, live_marked_at, live_marked_by, live_note;
		`, id, request.Live, user.Email, note).Scan(&slug, &title, &response.LiveMarkedAt, &response.LiveMarkedBy, &response.LiveNote)
		if err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		action := emailEventLiveUnmarked
		if request.Live {
			action = emailEventLiveMarked
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      action,
			EmailID:     &id,
			EmailSlug:   &slug,
			EmailTitle:  &title,
			Metadata:    map[string]any{"note": note},
		}); err != nil {
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}
