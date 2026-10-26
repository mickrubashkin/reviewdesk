package comments

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type Notification struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	CreatedAt     time.Time `json:"created_at"`
	ActorEmail    *string   `json:"actor_email"`
	EmailID       string    `json:"email_id"`
	EmailTitle    string    `json:"email_title"`
	EmailLanguage string    `json:"email_language"`
	EmailVariant  string    `json:"email_variant"`
	EmailStage    *string   `json:"email_stage"`
	BoardKey      string    `json:"board_key"`
	CommentID     *string   `json:"comment_id"`
	Excerpt       string    `json:"excerpt"`
}

type MentionableUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// listNotificationsHandler returns the current person's unread notifications,
// newest first.
func listNotificationsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		rows, err := dbpool.Query(r.Context(), `
			SELECT
				notifications.id,
				notifications.kind,
				notifications.created_at,
				actors.email,
				emails.id,
				emails.title,
				emails.language,
				emails.variant,
				emails.stage,
				COALESCE(emails.sequence, ''),
				notifications.comment_id,
				notifications.excerpt
			FROM notifications
			INNER JOIN emails ON emails.id = notifications.email_id
			LEFT JOIN users AS actors ON actors.id = notifications.actor_user_id
			WHERE notifications.user_id = $1
				AND notifications.read_at IS NULL
			ORDER BY notifications.created_at DESC
			LIMIT 200;
		`, user.ID)
		if err != nil {
			http.Error(w, "failed to load notifications", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		notifications := []Notification{}
		for rows.Next() {
			var n Notification
			if err := rows.Scan(
				&n.ID, &n.Kind, &n.CreatedAt, &n.ActorEmail,
				&n.EmailID, &n.EmailTitle, &n.EmailLanguage, &n.EmailVariant, &n.EmailStage, &n.BoardKey,
				&n.CommentID, &n.Excerpt,
			); err != nil {
				http.Error(w, "failed to read notifications", http.StatusInternalServerError)
				return
			}
			notifications = append(notifications, n)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "failed to read notifications", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(notifications)
	}
}

type markNotificationsReadRequest struct {
	// IDs to mark read; empty marks all of the person's notifications.
	IDs []string `json:"ids"`
}

// markNotificationsReadHandler marks the current person's notifications read.
// Other people's ids are ignored.
func markNotificationsReadHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var request markNotificationsReadRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		_, err := dbpool.Exec(r.Context(), `
			UPDATE notifications
			SET read_at = now()
			WHERE user_id = $1
				AND read_at IS NULL
				AND (cardinality($2::text[]) = 0 OR id::text = ANY($2));
		`, user.ID, append([]string{}, request.IDs...))
		if err != nil {
			http.Error(w, "failed to mark notifications read", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// listMentionableUsersHandler lists everyone who can be @mentioned.
func listMentionableUsersHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Board-scoped users (demo visitors) are out of each other's sight.
		rows, err := dbpool.Query(r.Context(), `
			SELECT id, lower(email), name
			FROM users
			WHERE NOT board_scoped OR id = $1
			ORDER BY CASE WHEN name = '' THEN lower(email) ELSE lower(name) END;
		`, user.ID)
		if err != nil {
			http.Error(w, "failed to load users", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		users := []MentionableUser{}
		for rows.Next() {
			var u MentionableUser
			if err := rows.Scan(&u.ID, &u.Email, &u.Name); err != nil {
				http.Error(w, "failed to read users", http.StatusInternalServerError)
				return
			}
			users = append(users, u)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "failed to read users", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(users)
	}
}

func registerNotificationRoutes(r chi.Router, dbpool *pgxpool.Pool) {
	r.Get("/api/notifications", listNotificationsHandler(dbpool))
	r.Post("/api/notifications/read", markNotificationsReadHandler(dbpool))
	r.Get("/api/users/mentionable", listMentionableUsersHandler(dbpool))
}
