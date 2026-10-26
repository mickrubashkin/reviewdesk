package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

const maxUserNameLength = 100

type updateUserNameRequest struct {
	Name string `json:"name"`
}

// updateAdminUserNameHandler sets the name a person is shown by; an empty
// name falls back to their email.
func updateAdminUserNameHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := coreauth.FromRequest(r)
		if !ok || !isSuperAdminUser(actor) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var request updateUserNameRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		name := strings.Join(strings.Fields(request.Name), " ")
		if utf8.RuneCountInString(name) > maxUserNameLength {
			http.Error(w, "name is too long", http.StatusBadRequest)
			return
		}

		var updatedUser UserAdminItem
		err := dbpool.QueryRow(r.Context(), `
			UPDATE users
			SET name = $2,
				updated_at = now()
			WHERE id = $1
			RETURNING `+userAdminColumns+`;
		`, chi.URLParam(r, "id"), name).Scan(updatedUser.scanTargets()...)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		aliases, err := loadUserAliases(r.Context(), dbpool, []string{updatedUser.ID})
		if err != nil {
			http.Error(w, "failed to read users", http.StatusInternalServerError)
			return
		}
		updatedUser.Aliases = aliasesOrEmpty(aliases[updatedUser.ID])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updatedUser)
	}
}

// listUserNamesHandler maps every address (main and alias, lowercased) of a
// named person to their name, so any signed-in page can show names instead
// of emails.
func listUserNamesHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Board-scoped users (demo visitors) are out of each other's sight.
		rows, err := dbpool.Query(r.Context(), `
			SELECT lower(email), name FROM users WHERE name <> '' AND (NOT board_scoped OR id = $1)
			UNION ALL
			SELECT lower(user_email_aliases.email), users.name
			FROM user_email_aliases
			JOIN users ON users.id = user_email_aliases.user_id
			WHERE users.name <> '' AND (NOT users.board_scoped OR users.id = $1);
		`, user.ID)
		if err != nil {
			http.Error(w, "failed to load names", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		names := map[string]string{}
		for rows.Next() {
			var email, name string
			if err := rows.Scan(&email, &name); err != nil {
				http.Error(w, "failed to load names", http.StatusInternalServerError)
				return
			}
			names[email] = name
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "failed to load names", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(names)
	}
}
