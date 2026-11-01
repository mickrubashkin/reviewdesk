package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

var (
	errAliasTaken      = errors.New("this address already belongs to a user or an alias")
	errMergeIntoSelf   = errors.New("pick two different users")
	errMergeSelf       = errors.New("you cannot merge your own account into another one")
	errUserNotFound    = errors.New("user not found")
	errAliasNotFound   = errors.New("alias not found")
	errAliasNotAllowed = errors.New("this address is not on an allowed domain")
)

// userIDReferences are the columns that point at a user by id; merging moves
// them to the user that stays. Sessions are dropped instead, so the merged
// person signs in again.
var userIDReferences = []struct{ table, column string }{
	{"auth_events", "user_id"},
	{"comments", "user_id"},
	{"comments", "resolved_by"},
	{"comment_messages", "user_id"},
	{"comment_mentions", "user_id"},
	{"notifications", "user_id"},
	{"notifications", "actor_user_id"},
	{"emails", "archived_by"},
	{"email_events", "actor_user_id"},
	{"email_versions", "created_by_user_id"},
	{"ai_analysis_logs", "user_id"},
	{"operational_events", "user_id"},
}

// userEmailReferences are the columns that assign work by address. History
// columns (who did what) keep the address that was used at the time.
var userEmailReferences = []struct{ table, column string }{
	{"emails", "owner_email"},
	{"emails", "reviewer_email"},
}

type mergeUsersRequest struct {
	FromUserID string `json:"from_user_id"`
}

type aliasRequest struct {
	Email string `json:"email"`
}

// resolveAliasUser returns the user an alias belongs to, if it is one.
func resolveAliasUser(ctx context.Context, dbpool *pgxpool.Pool, email string) (AuthUser, bool, error) {
	var user AuthUser
	err := dbpool.QueryRow(ctx, `
		SELECT users.id, users.email, users.role
		FROM user_email_aliases
		JOIN users ON users.id = user_email_aliases.user_id
		WHERE user_email_aliases.email = $1;
	`, email).Scan(&user.ID, &user.Email, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthUser{}, false, nil
	}
	return user, err == nil, err
}

type aliasQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func loadUserAliases(ctx context.Context, db aliasQuerier, userIDs []string) (map[string][]string, error) {
	rows, err := db.Query(ctx, `
		SELECT user_id, email FROM user_email_aliases WHERE user_id = ANY($1) ORDER BY email;
	`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	aliases := map[string][]string{}
	for rows.Next() {
		var userID, email string
		if err := rows.Scan(&userID, &email); err != nil {
			return nil, err
		}
		aliases[userID] = append(aliases[userID], email)
	}
	return aliases, rows.Err()
}

// mergeUsers folds the user fromID into intoID: their work, history and
// languages and managed boards move over, their address becomes an alias, and fromID is
// deleted. intoID keeps its own role, and its name unless it has none.
func mergeUsers(ctx context.Context, tx pgx.Tx, intoID, fromID, actorEmail string) (UserAdminItem, error) {
	if intoID == fromID {
		return UserAdminItem{}, errMergeIntoSelf
	}
	var intoEmail, fromEmail string
	var fromName string
	var fromLanguages, fromBoards []string
	if err := tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1 FOR UPDATE;`, intoID).Scan(&intoEmail); err != nil {
		return UserAdminItem{}, errUserNotFound
	}
	if err := tx.QueryRow(ctx, `SELECT email, name, languages, managed_boards FROM users WHERE id = $1 FOR UPDATE;`, fromID).Scan(&fromEmail, &fromName, &fromLanguages, &fromBoards); err != nil {
		return UserAdminItem{}, errUserNotFound
	}

	// A message that mentions both people keeps one mention after the merge.
	if _, err := tx.Exec(ctx, `
		DELETE FROM comment_mentions
		WHERE user_id = $2
			AND message_id IN (SELECT message_id FROM comment_mentions WHERE user_id = $1);
	`, intoID, fromID); err != nil {
		return UserAdminItem{}, fmt.Errorf("merge mentions: %w", err)
	}
	for _, ref := range userIDReferences {
		query := fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE %s = $2;`, ref.table, ref.column, ref.column)
		if _, err := tx.Exec(ctx, query, intoID, fromID); err != nil {
			return UserAdminItem{}, fmt.Errorf("move %s.%s: %w", ref.table, ref.column, err)
		}
	}
	for _, ref := range userEmailReferences {
		query := fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE lower(%s) = lower($2);`, ref.table, ref.column, ref.column)
		if _, err := tx.Exec(ctx, query, intoEmail, fromEmail); err != nil {
			return UserAdminItem{}, fmt.Errorf("move %s.%s: %w", ref.table, ref.column, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1;`, fromID); err != nil {
		return UserAdminItem{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE user_email_aliases SET user_id = $1 WHERE user_id = $2;`, intoID, fromID); err != nil {
		return UserAdminItem{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1;`, fromID); err != nil {
		return UserAdminItem{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_email_aliases (email, user_id, created_by_email) VALUES ($1, $2, $3);
	`, fromEmail, intoID, actorEmail); err != nil {
		return UserAdminItem{}, err
	}

	var merged UserAdminItem
	err := tx.QueryRow(ctx, `
		UPDATE users
		SET languages = ARRAY(SELECT DISTINCT unnest(languages || $2::text[]) ORDER BY 1),
			managed_boards = ARRAY(SELECT DISTINCT unnest(managed_boards || $4::text[]) ORDER BY 1),
			name = CASE WHEN name = '' THEN $3 ELSE name END,
			updated_at = now()
		WHERE id = $1
		RETURNING `+userAdminColumns+`;
	`, intoID, fromLanguages, fromName, fromBoards).Scan(merged.scanTargets()...)
	if err != nil {
		return UserAdminItem{}, err
	}
	aliases, err := loadUserAliases(ctx, tx, []string{intoID})
	if err != nil {
		return UserAdminItem{}, err
	}
	merged.Aliases = aliasesOrEmpty(aliases[intoID])
	return merged, nil
}

func aliasesOrEmpty(aliases []string) []string {
	if aliases == nil {
		return []string{}
	}
	return aliases
}

func mergeUsersHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := coreauth.FromRequest(r)
		if !ok || !isSuperAdminUser(actor) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var request mergeUsersRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		intoID := chi.URLParam(r, "id")
		if request.FromUserID == actor.ID {
			http.Error(w, errMergeSelf.Error(), http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to merge users", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		merged, err := mergeUsers(r.Context(), tx, intoID, request.FromUserID, actor.Email)
		switch {
		case errors.Is(err, errMergeIntoSelf):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		case errors.Is(err, errUserNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		case err != nil:
			fmt.Fprintf(os.Stderr, "failed to merge users: %v\n", err)
			http.Error(w, "failed to merge users", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to merge users", http.StatusInternalServerError)
			return
		}

		logAuthEvent(r.Context(), dbpool, r, AuthEvent{
			UserID:    &merged.ID,
			Email:     merged.Email,
			EventType: "admin_users_merged",
			Success:   true,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(merged)
	}
}

func addUserAliasHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := coreauth.FromRequest(r)
		if !ok || !isSuperAdminUser(actor) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var request aliasRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		email := normalizeEmail(request.Email)
		if isGenericAddress(email) {
			http.Error(w, errGenericAddress.Error(), http.StatusBadRequest)
			return
		}
		if !isAllowedAuthEmail(email) {
			http.Error(w, errAliasNotAllowed.Error(), http.StatusBadRequest)
			return
		}
		userID := chi.URLParam(r, "id")

		tag, err := dbpool.Exec(r.Context(), `
			INSERT INTO user_email_aliases (email, user_id, created_by_email)
			SELECT $1, users.id, $3 FROM users
			WHERE users.id = $2
				AND NOT EXISTS (SELECT 1 FROM users other WHERE lower(other.email) = $1)
			ON CONFLICT (email) DO NOTHING;
		`, email, userID, actor.Email)
		if err != nil {
			http.Error(w, "failed to add alias", http.StatusInternalServerError)
			return
		}
		if tag.RowsAffected() == 0 {
			http.Error(w, errAliasTaken.Error(), http.StatusConflict)
			return
		}
		writeUserAliases(w, r, dbpool, userID)
	}
}

func removeUserAliasHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := coreauth.FromRequest(r)
		if !ok || !isSuperAdminUser(actor) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		userID := chi.URLParam(r, "id")
		tag, err := dbpool.Exec(r.Context(), `
			DELETE FROM user_email_aliases WHERE user_id = $1 AND email = $2;
		`, userID, normalizeEmail(chi.URLParam(r, "email")))
		if err != nil {
			http.Error(w, "failed to remove alias", http.StatusInternalServerError)
			return
		}
		if tag.RowsAffected() == 0 {
			http.Error(w, errAliasNotFound.Error(), http.StatusNotFound)
			return
		}
		writeUserAliases(w, r, dbpool, userID)
	}
}

func writeUserAliases(w http.ResponseWriter, r *http.Request, dbpool *pgxpool.Pool, userID string) {
	aliases, err := loadUserAliases(r.Context(), dbpool, []string{userID})
	if err != nil {
		http.Error(w, "failed to load aliases", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string][]string{"aliases": aliasesOrEmpty(aliases[userID])})
}

// attachAliases fills Aliases on a list of users.
func attachAliases(ctx context.Context, db aliasQuerier, users []UserAdminItem) error {
	ids := make([]string, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	aliases, err := loadUserAliases(ctx, db, ids)
	if err != nil {
		return err
	}
	for i := range users {
		users[i].Aliases = aliasesOrEmpty(aliases[users[i].ID])
	}
	return nil
}
