package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/appurl"
	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

// A user whose address (or alias) shares the part before @ with an older
// user's is a possible duplicate of that user: often the same person on
// another regional domain. It is only a hint for a super admin, who merges
// them or marks them as two people; nothing is merged on its own, because
// the same name on another domain can be someone else.

// Sender posts a message to the owner's chat.
type Sender interface {
	Send(ctx context.Context, text string) error
}

// LinkSender is a Sender that can put a link under a message as a button,
// like the Bitrix24 bot. Other senders get the link on a line of its own.
type LinkSender interface {
	SendWithLink(ctx context.Context, text string, label string, link string) error
}

// Notifier hears about each new user who looks like a duplicate, and on
// VibeCode about people refused at the gateway; nil sends nothing.
var Notifier Sender

// notifyOwner posts text to the owner's chat with a link to Settings → Users.
func notifyOwner(ctx context.Context, text string) error {
	base := appurl.Get()
	if base == "" {
		return Notifier.Send(ctx, text)
	}
	link := base + "/admin/users"
	if sender, ok := Notifier.(LinkSender); ok {
		return sender.SendWithLink(ctx, text, "Open Settings → Users", link)
	}
	return Notifier.Send(ctx, text+"\n"+link)
}

const notifyTimeout = 15 * time.Second

type notDuplicateRequest struct {
	OtherUserID string `json:"other_user_id"`
}

// loadPossibleDuplicates maps each user to the older users they may
// duplicate, oldest first. With userID set it looks at that user only.
func loadPossibleDuplicates(ctx context.Context, db aliasQuerier, userID string) (map[string][]string, error) {
	rows, err := db.Query(ctx, `
		WITH addresses AS (
			SELECT id AS user_id, created_at, lower(split_part(email, '@', 1)) AS local
			FROM users
			UNION
			SELECT users.id, users.created_at, lower(split_part(user_email_aliases.email, '@', 1))
			FROM user_email_aliases
			JOIN users ON users.id = user_email_aliases.user_id
		)
		SELECT newer.user_id, older.user_id, newer.local
		FROM addresses newer
		JOIN addresses older ON older.local = newer.local
			AND (older.created_at, older.user_id) < (newer.created_at, newer.user_id)
		WHERE ($1::text = '' OR newer.user_id::text = $1::text)
			AND NOT EXISTS (
				SELECT 1 FROM user_duplicate_dismissals
				WHERE user_a = least(newer.user_id, older.user_id)
					AND user_b = greatest(newer.user_id, older.user_id)
			)
		ORDER BY newer.user_id, older.created_at, older.user_id;
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	duplicates := map[string][]string{}
	for rows.Next() {
		var newerID, olderID, local string
		if err := rows.Scan(&newerID, &olderID, &local); err != nil {
			return nil, err
		}
		// sales@ on two domains is two shared mailboxes, not one person.
		if local == "" || isGenericAddress(local+"@x") {
			continue
		}
		list := duplicates[newerID]
		if len(list) == 0 || list[len(list)-1] != olderID {
			duplicates[newerID] = append(list, olderID)
		}
	}
	return duplicates, rows.Err()
}

// attachPossibleDuplicates fills PossibleDuplicates on a list of users.
func attachPossibleDuplicates(ctx context.Context, db aliasQuerier, users []UserAdminItem) error {
	duplicates, err := loadPossibleDuplicates(ctx, db, "")
	if err != nil {
		return err
	}
	for i := range users {
		users[i].PossibleDuplicates = aliasesOrEmpty(duplicates[users[i].ID])
	}
	return nil
}

// announcePossibleDuplicate tells the owner's chat that a new user looks like
// someone already in ReviewDesk. It does not wait for the chat.
func announcePossibleDuplicate(dbpool *pgxpool.Pool, user AuthUser) {
	if Notifier == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()
		duplicates, err := loadPossibleDuplicates(ctx, dbpool, user.ID)
		if err != nil || len(duplicates[user.ID]) == 0 {
			if err != nil {
				fmt.Fprintf(os.Stderr, "possible duplicate of %s: %v\n", user.Email, err)
			}
			return
		}
		rows, err := dbpool.Query(ctx, `SELECT email FROM users WHERE id = ANY($1) ORDER BY created_at;`, duplicates[user.ID])
		if err != nil {
			fmt.Fprintf(os.Stderr, "possible duplicate of %s: %v\n", user.Email, err)
			return
		}
		var others []string
		for rows.Next() {
			var email string
			if rows.Scan(&email) == nil {
				others = append(others, email)
			}
		}
		rows.Close()
		if len(others) == 0 {
			return
		}
		if err := notifyOwner(ctx, possibleDuplicateMessage(user.Email, others)); err != nil {
			fmt.Fprintf(os.Stderr, "possible duplicate of %s: %v\n", user.Email, err)
		}
	}()
}

func possibleDuplicateMessage(newEmail string, others []string) string {
	return fmt.Sprintf("Possible duplicate user: %s signed in for the first time; ReviewDesk already has %s. Merge them, or mark them as two people, in Settings → Users.",
		newEmail, strings.Join(others, ", "))
}

// notDuplicateHandler records that a user and a possible duplicate of theirs
// are two people, so the hint goes away.
func notDuplicateHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := coreauth.FromRequest(r)
		if !ok || !isSuperAdminUser(actor) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var request notDuplicateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		userID := chi.URLParam(r, "id")
		if userID == request.OtherUserID {
			http.Error(w, errMergeIntoSelf.Error(), http.StatusBadRequest)
			return
		}
		tag, err := dbpool.Exec(r.Context(), `
			INSERT INTO user_duplicate_dismissals (user_a, user_b, created_by_email)
			SELECT least(a.id, b.id), greatest(a.id, b.id), $3
			FROM users a, users b
			WHERE a.id::text = $1 AND b.id::text = $2
			ON CONFLICT DO NOTHING;
		`, userID, request.OtherUserID, actor.Email)
		if err != nil {
			http.Error(w, "failed to save", http.StatusInternalServerError)
			return
		}
		if tag.RowsAffected() == 0 {
			var found int
			_ = dbpool.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE id::text IN ($1, $2);`, userID, request.OtherUserID).Scan(&found)
			if found < 2 {
				http.Error(w, errUserNotFound.Error(), http.StatusNotFound)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
