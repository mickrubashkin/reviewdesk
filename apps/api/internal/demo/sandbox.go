package demo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A visitor of the public demo gets a sandbox: a user of their own and a
// private copy of the demo board. Visitors are board-scoped, so nobody sees
// another visitor's board, comments or edits, and nothing needs moderating.
// A sandbox lives for SandboxLifetime; the cleanup removes it afterwards.
const (
	sandboxBoardPrefix  = "demo-visit-"
	visitorEmailPrefix  = "visitor-"
	visitorEmailDomain  = "@visitors.demo.invalid"
	visitorName         = "Demo visitor"
	SandboxLifetime     = 24 * time.Hour
	maxSandboxes        = 500
	sandboxCleanupEvery = time.Hour
)

// Visitor is the user a sandbox belongs to.
type Visitor struct {
	ID       string
	Email    string
	Role     string
	BoardKey string
}

// CreateSandbox makes a visitor with their own seeded demo board. When the
// demo is at its sandbox limit, the oldest sandboxes make room.
func CreateSandbox(ctx context.Context, tx pgx.Tx) (Visitor, error) {
	if err := makeRoomForSandbox(ctx, tx); err != nil {
		return Visitor{}, err
	}
	token, err := randomToken()
	if err != nil {
		return Visitor{}, err
	}
	board := Board{Key: sandboxBoardPrefix + token, Name: demoBoardName}
	visitor := Visitor{Email: visitorEmailPrefix + token + visitorEmailDomain, BoardKey: board.Key}
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, name, role, managed_boards, board_scoped)
		VALUES ($1, $2, 'reviewer', ARRAY[$3], true)
		RETURNING id::text, role;
	`, visitor.Email, visitorName, board.Key).Scan(&visitor.ID, &visitor.Role); err != nil {
		return Visitor{}, fmt.Errorf("create visitor: %w", err)
	}
	if _, err := Seed(ctx, tx, board); err != nil {
		return Visitor{}, fmt.Errorf("seed visitor board: %w", err)
	}
	return visitor, nil
}

func randomToken() (string, error) {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// makeRoomForSandbox deletes the oldest sandboxes beyond the limit.
func makeRoomForSandbox(ctx context.Context, tx pgx.Tx) error {
	keys, err := collectKeys(ctx, tx, `
		SELECT key FROM boards
		WHERE key LIKE $1 || '%'
		ORDER BY created_at DESC
		OFFSET $2;
	`, sandboxBoardPrefix, maxSandboxes-1)
	if err != nil || len(keys) == 0 {
		return err
	}
	_, err = deleteSandboxes(ctx, tx, keys)
	return err
}

// DeleteExpiredSandboxes removes the sandboxes created before now minus
// SandboxLifetime and returns how many it removed.
func DeleteExpiredSandboxes(ctx context.Context, db *pgxpool.Pool, now time.Time) (int, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	keys, err := collectKeys(ctx, tx, `
		SELECT key FROM boards WHERE key LIKE $1 || '%' AND created_at < $2;
	`, sandboxBoardPrefix, now.Add(-SandboxLifetime))
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	count, err := deleteSandboxes(ctx, tx, keys)
	if err != nil {
		return 0, err
	}
	return count, tx.Commit(ctx)
}

// RunSandboxCleanup deletes expired sandboxes every hour until ctx ends.
func RunSandboxCleanup(ctx context.Context, db *pgxpool.Pool) {
	ticker := time.NewTicker(sandboxCleanupEvery)
	defer ticker.Stop()
	for {
		if count, err := DeleteExpiredSandboxes(ctx, db, time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "demo sandbox cleanup: %v\n", err)
		} else if count > 0 {
			fmt.Printf("demo sandbox cleanup: removed %d\n", count)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func collectKeys(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// deleteSandboxes removes the sandbox boards with their emails and history,
// and the visitors who had only those boards; their visits stay, with what
// they did. nil means every sandbox.
func deleteSandboxes(ctx context.Context, tx pgx.Tx, keys []string) (int, error) {
	if keys == nil {
		var err error
		if keys, err = collectKeys(ctx, tx, `SELECT key FROM boards WHERE key LIKE $1 || '%';`, sandboxBoardPrefix); err != nil {
			return 0, err
		}
	}
	if err := keepVisitActions(ctx, tx, keys); err != nil {
		return 0, err
	}
	statements := []string{
		// History that would outlive the emails and the visitors.
		`DELETE FROM email_events WHERE email_id IN (SELECT id FROM emails WHERE sequence = ANY($1));`,
		`DELETE FROM ai_analysis_logs WHERE email_id IN (SELECT id FROM emails WHERE sequence = ANY($1));`,
		`DELETE FROM emails WHERE sequence = ANY($1);`,
		`DELETE FROM boards WHERE key = ANY($1);`,
		`DELETE FROM auth_events WHERE user_id IN (SELECT id FROM users WHERE board_scoped AND email LIKE '` + visitorEmailPrefix + `%' AND managed_boards <@ $1);`,
		`DELETE FROM operational_events WHERE user_id IN (SELECT id FROM users WHERE board_scoped AND email LIKE '` + visitorEmailPrefix + `%' AND managed_boards <@ $1);`,
		// Sessions, notifications and mentions go with the user.
		`DELETE FROM users WHERE board_scoped AND email LIKE '` + visitorEmailPrefix + `%' AND managed_boards <@ $1;`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement, keys); err != nil {
			return 0, fmt.Errorf("delete sandboxes: %w", err)
		}
	}
	return len(keys), nil
}
