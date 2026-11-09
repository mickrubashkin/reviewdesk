package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

// On VibeCode a user's Bitrix24 ID lets the bot write to them before they
// ever open ReviewDesk through the gateway. RunPortalUserIDFill looks the IDs
// up by email on start and then once a day.

const portalUserIDFillEvery = 24 * time.Hour

// findPortalUserIDs returns the IDs of active Bitrix24 employees with the
// address; tests replace it.
var findPortalUserIDs = fetchPortalUserIDsByEmail

func fetchPortalUserIDsByEmail(ctx context.Context, email string) ([]string, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("BITRIX_API_BASE_URL")), "/")
	if base == "" {
		base = "https://vibecode.bitrix24.tech/v1"
	}
	key := strings.TrimSpace(os.Getenv("BITRIX_API_KEY"))
	if key == "" {
		return nil, errors.New("BITRIX_API_KEY is not set")
	}
	query := url.Values{"filter[email]": {email}, "select": {"id,email,active"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/users?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Api-Key", key)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("platform users API returned %d: %.300s", response.StatusCode, body)
	}
	return parsePortalUserIDs(body, email)
}

// parsePortalUserIDs keeps the active employees whose address is exactly the
// one asked for: the filter may match more loosely.
func parsePortalUserIDs(body []byte, email string) ([]string, error) {
	var answer struct {
		Data []struct {
			ID     json.Number `json:"id"`
			Email  string      `json:"email"`
			Active bool        `json:"active"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, err
	}
	ids := []string{}
	for _, user := range answer.Data {
		if user.Active && normalizeEmail(user.Email) == email && portalUserIDPattern.MatchString(user.ID.String()) {
			ids = append(ids, user.ID.String())
		}
	}
	return ids, nil
}

func RunPortalUserIDFill(ctx context.Context, dbpool *pgxpool.Pool) {
	for {
		if linked, err := fillPortalUserIDs(ctx, dbpool); err != nil {
			fmt.Fprintf(os.Stderr, "Bitrix24 IDs: %v\n", err)
		} else if linked > 0 {
			fmt.Printf("Bitrix24 IDs: linked %d users\n", linked)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(portalUserIDFillEvery):
		}
	}
}

// fillPortalUserIDs gives each user without a Bitrix24 ID the one employee
// found by their address or one of their aliases. Several matches, or an ID
// another user already has, leave the user for a CRM operator to set.
func fillPortalUserIDs(ctx context.Context, dbpool *pgxpool.Pool) (int, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT users.id, users.email, COALESCE(array_agg(aliases.email) FILTER (WHERE aliases.email IS NOT NULL), '{}')
		FROM users
		LEFT JOIN user_email_aliases AS aliases ON aliases.user_id = users.id
		WHERE users.portal_user_id IS NULL
		GROUP BY users.id, users.email
		ORDER BY users.created_at;
	`)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		id     string
		emails []string
	}
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		var email string
		var aliases []string
		if err := rows.Scan(&c.id, &email, &aliases); err != nil {
			rows.Close()
			return 0, err
		}
		c.emails = append([]string{email}, aliases...)
		candidates = append(candidates, c)
	}
	rows.Close()

	linked := 0
	for _, c := range candidates {
		found := map[string]bool{}
		for _, email := range c.emails {
			if isGenericAddress(email) {
				continue
			}
			ids, err := findPortalUserIDs(ctx, email)
			if err != nil {
				return linked, fmt.Errorf("look up %s: %w", email, err)
			}
			for _, id := range ids {
				found[id] = true
			}
		}
		if len(found) != 1 {
			continue
		}
		var portalUserID string
		for id := range found {
			portalUserID = id
		}
		tag, err := dbpool.Exec(ctx, `
			UPDATE users SET portal_user_id = $2, updated_at = now()
			WHERE id = $1 AND portal_user_id IS NULL
				AND NOT EXISTS (SELECT 1 FROM users WHERE portal_user_id = $2);
		`, c.id, portalUserID)
		if err != nil {
			return linked, err
		}
		linked += int(tag.RowsAffected())
	}
	return linked, nil
}

type updateUserPortalIDRequest struct {
	PortalUserID string `json:"portal_user_id"`
}

// updateAdminUserPortalIDHandler lets a CRM operator set or clear the
// Bitrix24 ID when the lookup by email found none or several.
func updateAdminUserPortalIDHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := coreauth.FromRequest(r)
		if !ok || !isSuperAdminUser(actor) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var request updateUserPortalIDRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		userID := chi.URLParam(r, "id")
		switch err := setPortalUserID(r.Context(), dbpool, userID, strings.TrimSpace(request.PortalUserID)); {
		case errors.Is(err, errInvalidPortalUserID):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		case errors.Is(err, errPortalUserIDTaken):
			http.Error(w, err.Error(), http.StatusConflict)
			return
		case errors.Is(err, errUserNotFound):
			http.Error(w, "user not found", http.StatusNotFound)
			return
		case err != nil:
			http.Error(w, "failed to update user", http.StatusInternalServerError)
			return
		}

		var updatedUser UserAdminItem
		if err := dbpool.QueryRow(r.Context(), `SELECT `+userAdminColumns+` FROM users WHERE id = $1;`, userID).Scan(updatedUser.scanTargets()...); err != nil {
			http.Error(w, "failed to read users", http.StatusInternalServerError)
			return
		}
		aliases, err := loadUserAliases(r.Context(), dbpool, []string{userID})
		if err != nil {
			http.Error(w, "failed to read users", http.StatusInternalServerError)
			return
		}
		updatedUser.Aliases = aliasesOrEmpty(aliases[userID])
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updatedUser)
	}
}

// setPortalUserID sets or, with "", clears a user's Bitrix24 ID by hand.
func setPortalUserID(ctx context.Context, dbpool *pgxpool.Pool, userID string, portalUserID string) error {
	var value *string
	if portalUserID != "" {
		if !portalUserIDPattern.MatchString(portalUserID) {
			return errInvalidPortalUserID
		}
		value = &portalUserID
	}
	tag, err := dbpool.Exec(ctx, `UPDATE users SET portal_user_id = $2, updated_at = now() WHERE id = $1;`, userID, value)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return errPortalUserIDTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return errUserNotFound
	}
	return nil
}

var (
	errInvalidPortalUserID = errors.New("a Bitrix24 ID is a number")
	errPortalUserIDTaken   = errors.New("another user already has this Bitrix24 ID")
)
