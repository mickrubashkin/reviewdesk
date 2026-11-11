package auth

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

// "Changed since your visit" (#205). Marks start for a person the first time
// they load ReviewDesk after it shipped, so nothing older shows as new.

func startTrackingChanges(ctx context.Context, dbpool *pgxpool.Pool, user AuthUser) {
	_, err := dbpool.Exec(ctx, `
		UPDATE users SET changes_tracked_since = now() WHERE id = $1 AND changes_tracked_since IS NULL;
	`, user.ID)
	if err != nil && !IsRequestCanceledError(err) {
		fmt.Fprintf(os.Stderr, "failed to start tracking changes for %s: %v\n", user.Email, err)
	}
}

// recordMyTasksVisitHandler notes that the person opened My tasks. Opening it
// again within 30 minutes is the same visit: my_tasks_previous_seen_at keeps
// when the visit before began, which is what "New" compares with. The web app
// works out the same from /api/auth/me (visit.ts).
func recordMyTasksVisitHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, err := dbpool.Exec(r.Context(), `
			UPDATE users SET
				my_tasks_previous_seen_at = CASE
					WHEN my_tasks_seen_at < now() - interval '30 minutes' THEN my_tasks_seen_at
					ELSE my_tasks_previous_seen_at
				END,
				my_tasks_seen_at = now()
			WHERE id = $1;
		`, user.ID)
		if err != nil {
			http.Error(w, "failed to record visit", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
