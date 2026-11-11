package emails

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

// recordEmailVisitHandler notes that the signed-in person opened an email,
// for "changed since your visit" (#205). Opening it again within 30 minutes
// is the same visit: previous_viewed_at keeps when the visit before began.
// The web app works out the same "since" from the list (visit.ts).
func recordEmailVisitHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, err := dbpool.Exec(r.Context(), `
			INSERT INTO email_views (user_id, email_id) VALUES ($1, $2::uuid)
			ON CONFLICT (user_id, email_id) DO UPDATE SET
				previous_viewed_at = CASE
					WHEN email_views.viewed_at < now() - interval '30 minutes' THEN email_views.viewed_at
					ELSE email_views.previous_viewed_at
				END,
				viewed_at = now();
		`, user.ID, chi.URLParam(r, "id"))
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "22P02") {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to record visit", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
