package emails

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CanEditEmail reports whether user may edit an email's content: admins
// always, board managers on their boards (Users → Manages), and translators
// for emails in a language they translate (Users → Translates), whatever
// their role.
func CanEditEmail(ctx context.Context, db rowQuerier, user auth.AuthUser, emailID string) (bool, error) {
	if auth.IsAdmin(user) {
		return true, nil
	}
	var allowed bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM emails e
			JOIN users u ON u.id = $2
			WHERE e.id::text = $1
				AND (lower(e.language) = ANY(u.languages) OR e.sequence = ANY(u.managed_boards))
		);
	`, emailID, user.ID).Scan(&allowed)
	return allowed, err
}

// requireManageEmail writes 403 and returns false unless user manages the
// board the email is on.
func requireManageEmail(w http.ResponseWriter, r *http.Request, db rowQuerier, user auth.AuthUser, emailID string) bool {
	allowed, err := auth.CanManageEmail(r.Context(), db, user, emailID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to check board access for email %s: %v\n", emailID, err)
		http.Error(w, "failed to check access", http.StatusInternalServerError)
		return false
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// requireManageBoard writes 403 and returns false unless user manages the
// board.
func requireManageBoard(w http.ResponseWriter, r *http.Request, db rowQuerier, user auth.AuthUser, boardKey string) bool {
	allowed, err := auth.CanManageBoard(r.Context(), db, user, boardKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to check access to board %s: %v\n", boardKey, err)
		http.Error(w, "failed to check access", http.StatusInternalServerError)
		return false
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// requireManageEmails writes 403 and returns false unless user manages the
// boards of all the emails.
func requireManageEmails(w http.ResponseWriter, r *http.Request, db rowQuerier, user auth.AuthUser, emailIDs []string) bool {
	allowed, err := auth.CanManageEmails(r.Context(), db, user, emailIDs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to check board access for %d emails: %v\n", len(emailIDs), err)
		http.Error(w, "failed to check access", http.StatusInternalServerError)
		return false
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}
