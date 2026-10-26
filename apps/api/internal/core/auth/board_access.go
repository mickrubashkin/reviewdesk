package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CanManageBoard reports whether user may change a board and its emails:
// admins on every board, others on the boards listed in Users → Manages.
func CanManageBoard(ctx context.Context, db rowQuerier, user AuthUser, boardKey string) (bool, error) {
	if IsAdmin(user) {
		return true, nil
	}
	var allowed bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND $2 = ANY(managed_boards));
	`, user.ID, boardKey).Scan(&allowed)
	return allowed, err
}

// CanManageEmail reports whether user may manage the board an email is on.
func CanManageEmail(ctx context.Context, db rowQuerier, user AuthUser, emailID string) (bool, error) {
	if IsAdmin(user) {
		return true, nil
	}
	var allowed bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM emails e
			JOIN users u ON u.id = $2
			WHERE e.id::text = $1 AND e.sequence = ANY(u.managed_boards)
		);
	`, emailID, user.ID).Scan(&allowed)
	return allowed, err
}

// ManagesAnyBoard reports whether user manages at least one board, for
// actions that are not tied to a board yet (inspecting pasted HTML, the
// user list for owner pickers).
func ManagesAnyBoard(ctx context.Context, db rowQuerier, user AuthUser) (bool, error) {
	if IsAdmin(user) {
		return true, nil
	}
	var allowed bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND cardinality(managed_boards) > 0);
	`, user.ID).Scan(&allowed)
	return allowed, err
}

// CanManageEmails reports whether user manages the boards of all the emails.
func CanManageEmails(ctx context.Context, db rowQuerier, user AuthUser, emailIDs []string) (bool, error) {
	if IsAdmin(user) {
		return true, nil
	}
	var allowed bool
	err := db.QueryRow(ctx, `
		SELECT NOT EXISTS (
			SELECT 1
			FROM emails e
			JOIN users u ON u.id = $2
			WHERE e.id::text = ANY($1::text[]) AND NOT (e.sequence = ANY(u.managed_boards))
		);
	`, emailIDs, user.ID).Scan(&allowed)
	return allowed, err
}

// ScopedBoards returns the boards a board-scoped user may see, the ones
// they manage, and whether the user is scoped at all; everyone else sees
// every board.
func ScopedBoards(ctx context.Context, db rowQuerier, user AuthUser) ([]string, bool, error) {
	var boards []string
	var scoped bool
	err := db.QueryRow(ctx, `
		SELECT managed_boards, board_scoped FROM users WHERE id = $1;
	`, user.ID).Scan(&boards, &scoped)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil || !scoped {
		return nil, false, err
	}
	return boards, true, nil
}
