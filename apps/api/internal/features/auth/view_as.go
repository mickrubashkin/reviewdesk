package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// View as: a super admin opens ReviewDesk as another person sees it (their
// boards, My tasks, mentions and buttons) to check what that person gets.
// The person is kept in a cookie rather than a header so that EventSource
// and iframes carry it too. While viewing, the API is read-only: nothing is
// written in the person's name, and their last_seen_at does not move.

const viewAsCookieName = "email_review_view_as"

var errViewAsReadOnly = errors.New("read-only: you are viewing ReviewDesk as another user")

type viewAsRequest struct {
	UserID string `json:"user_id"`
}

func startViewAsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		real, err := currentUserFromRequest(r.Context(), dbpool, r)
		if err != nil {
			writeUnauthorized(w, err)
			return
		}
		if !isSuperAdminUser(real) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var payload viewAsRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		target, err := viewAsTarget(r.Context(), dbpool, strings.TrimSpace(payload.UserID))
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load view-as user: %v\n", err)
			http.Error(w, "failed to load user", http.StatusInternalServerError)
			return
		}
		if isSuperAdminUser(target) {
			http.Error(w, "a CRM operator sees everything already", http.StatusBadRequest)
			return
		}

		// The event is the viewed person's, so the log shows whom; only
		// super admins can start it.
		logAuthEvent(r.Context(), dbpool, r, AuthEvent{
			UserID:    &target.ID,
			Email:     target.Email,
			EventType: "view_as",
			Success:   true,
		})
		http.SetCookie(w, viewAsCookie(target.ID, 0))
		writeAuthOK(w)
	}
}

func stopViewAsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, viewAsCookie("", -1))
		writeAuthOK(w)
	}
}

// viewedUser returns whom real is viewing ReviewDesk as, if anyone. A cookie
// left from before real lost the super admin role, or naming a user that is
// gone or became a super admin, is ignored.
func viewedUser(ctx context.Context, dbpool *pgxpool.Pool, r *http.Request, real AuthUser) (AuthUser, bool) {
	cookie, err := r.Cookie(viewAsCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" || !isSuperAdminUser(real) {
		return AuthUser{}, false
	}
	target, err := viewAsTarget(ctx, dbpool, cookie.Value)
	if err != nil || isSuperAdminUser(target) {
		return AuthUser{}, false
	}
	return target, true
}

func viewAsTarget(ctx context.Context, dbpool *pgxpool.Pool, userID string) (AuthUser, error) {
	var user AuthUser
	err := dbpool.QueryRow(ctx, `
		SELECT id, email, role FROM users WHERE id::text = $1;
	`, userID).Scan(&user.ID, &user.Email, &user.Role)
	return user, err
}

// allowedWhileViewing lets reads through, but not the AI analysis: it calls
// OpenAI and stores its result although it is a GET.
func allowedWhileViewing(r *http.Request) bool {
	if isMutatingMethod(r.Method) {
		return false
	}
	return !strings.HasSuffix(r.URL.Path, "/ai-analysis-stream") &&
		!strings.HasSuffix(r.URL.Path, "/ai-analysis-debug")
}

// viewAsCookie lives as long as the browser session; maxAge -1 removes it.
func viewAsCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     viewAsCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   authCookieSecure(),
	}
}
