package auth

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

// meResponse is the signed-in user plus the languages they translate and the
// boards they manage.
type meResponse struct {
	AuthUser
	Languages     []string `json:"languages"`
	ManagedBoards []string `json:"managed_boards"`
}

type updateUserLanguagesRequest struct {
	Languages []string `json:"languages"`
}

var languageCodePattern = regexp.MustCompile(`^[a-z]{2,8}$`)

// updateAdminUserLanguagesHandler sets which languages a person translates,
// so outdated translations reach them in My tasks.
func updateAdminUserLanguagesHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !isAdminUser(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var request updateUserLanguagesRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		languages := []string{}
		for _, language := range request.Languages {
			language = strings.ToLower(strings.TrimSpace(language))
			if !languageCodePattern.MatchString(language) {
				http.Error(w, "invalid language code", http.StatusBadRequest)
				return
			}
			if !slices.Contains(languages, language) {
				languages = append(languages, language)
			}
		}

		var updatedUser UserAdminItem
		err := dbpool.QueryRow(r.Context(), `
			UPDATE users
			SET languages = $2,
				updated_at = now()
			WHERE id = $1
			RETURNING `+userAdminColumns+`;
		`, chi.URLParam(r, "id"), languages).Scan(updatedUser.scanTargets()...)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updatedUser)
	}
}
