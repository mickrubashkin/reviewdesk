package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type updateUserWhatsNewRequest struct {
	Seen string `json:"seen"`
}

type userWhatsNewResponse struct {
	WhatsNewSeen string `json:"whats_new_seen"`
}

// updateUserWhatsNewHandler records the date of the newest "What's new"
// entry the signed-in person has opened. It never moves back, so an old
// build of the app does not bring the mark back.
func updateUserWhatsNewHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var request updateUserWhatsNewRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if _, err := time.Parse(time.DateOnly, request.Seen); err != nil {
			http.Error(w, "seen must be a date (YYYY-MM-DD)", http.StatusBadRequest)
			return
		}

		// ISO dates compare as text.
		var response userWhatsNewResponse
		if err := dbpool.QueryRow(r.Context(), `
			UPDATE users
			SET whats_new_seen = GREATEST(whats_new_seen, $2)
			WHERE id = $1
			RETURNING whats_new_seen;
		`, user.ID, request.Seen).Scan(&response.WhatsNewSeen); err != nil {
			http.Error(w, "failed to update user", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}
