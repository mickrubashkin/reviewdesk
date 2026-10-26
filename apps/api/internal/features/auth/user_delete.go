package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

var errDeleteSelf = errors.New("you cannot delete your own account")

// deleteUser removes a user with their sessions and aliases. Emails assigned
// to any of their addresses become unassigned; history rows keep the record
// but lose the link to the person.
func deleteUser(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var email string
	if err := tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1 FOR UPDATE;`, userID).Scan(&email); err != nil {
		return "", errUserNotFound
	}
	addresses := []string{email}
	aliases, err := loadUserAliases(ctx, tx, []string{userID})
	if err != nil {
		return "", err
	}
	addresses = append(addresses, aliases[userID]...)

	for _, ref := range userEmailReferences {
		query := fmt.Sprintf(`UPDATE %s SET %s = NULL WHERE lower(%s) = ANY($1);`, ref.table, ref.column, ref.column)
		if _, err := tx.Exec(ctx, query, lowerAll(addresses)); err != nil {
			return "", fmt.Errorf("unassign %s.%s: %w", ref.table, ref.column, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1;`, userID); err != nil {
		return "", err
	}
	return email, nil
}

func lowerAll(values []string) []string {
	lowered := make([]string, len(values))
	for i, value := range values {
		lowered[i] = normalizeEmail(value)
	}
	return lowered
}

func deleteUserHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := coreauth.FromRequest(r)
		if !ok || !isSuperAdminUser(actor) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		userID := chi.URLParam(r, "id")
		if userID == actor.ID {
			http.Error(w, errDeleteSelf.Error(), http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to delete user", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		email, err := deleteUser(r.Context(), tx, userID)
		switch {
		case errors.Is(err, errUserNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		case err != nil:
			fmt.Fprintf(os.Stderr, "failed to delete user: %v\n", err)
			http.Error(w, "failed to delete user", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to delete user", http.StatusInternalServerError)
			return
		}

		logAuthEvent(r.Context(), dbpool, r, AuthEvent{
			Email:     email,
			EventType: "admin_user_deleted",
			Success:   true,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}
