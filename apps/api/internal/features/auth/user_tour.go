package auth

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type updateUserTourRequest struct {
	Version int `json:"version"`
}

type userTourResponse struct {
	TourVersion int `json:"tour_version"`
}

// updateUserTourHandler records that the signed-in person finished or
// skipped the tour of this version. It never lowers the stored version, so
// replaying an older tour does not bring the newer one back.
func updateUserTourHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var request updateUserTourRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Version < 1 {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		var response userTourResponse
		if err := dbpool.QueryRow(r.Context(), `
			UPDATE users
			SET tour_version = GREATEST(tour_version, $2)
			WHERE id = $1
			RETURNING tour_version;
		`, user.ID, request.Version).Scan(&response.TourVersion); err != nil {
			http.Error(w, "failed to update user", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}
