package auth

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type updateUserBoardsRequest struct {
	Boards []string `json:"boards"`
}

// updateAdminUserBoardsHandler sets which boards a person manages: there they
// can do what an admin does, other boards stay read-only for them.
func updateAdminUserBoardsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
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

		var request updateUserBoardsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		boards := []string{}
		for _, board := range request.Boards {
			board = strings.TrimSpace(board)
			if board != "" && !slices.Contains(boards, board) {
				boards = append(boards, board)
			}
		}
		var known int
		if err := dbpool.QueryRow(r.Context(), `
			SELECT count(*) FROM boards WHERE key = ANY($1::text[]);
		`, boards).Scan(&known); err != nil {
			http.Error(w, "failed to check boards", http.StatusInternalServerError)
			return
		}
		if known != len(boards) {
			http.Error(w, "unknown board", http.StatusBadRequest)
			return
		}

		var updatedUser UserAdminItem
		err := dbpool.QueryRow(r.Context(), `
			UPDATE users
			SET managed_boards = $2,
				updated_at = now()
			WHERE id = $1
			RETURNING `+userAdminColumns+`;
		`, chi.URLParam(r, "id"), boards).Scan(updatedUser.scanTargets()...)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updatedUser)
	}
}
