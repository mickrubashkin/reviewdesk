package emails

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// checkOwner tells whether the person with ownerEmail may own every one of
// the emails: someone who manages the email's board (admins manage every
// board), or a translator whose languages include the email's language. The
// owner pickers offer the same people. The error is a createEmailInputError
// when they may not.
func checkOwner(ctx context.Context, tx pgx.Tx, ownerEmail string, emailIDs []string) error {
	var title string
	err := tx.QueryRow(ctx, `
		SELECT e.title
		FROM emails e
		JOIN users u ON lower(u.email) = lower($1)
		WHERE e.id::text = ANY($2::text[])
			AND u.role NOT IN ('admin', 'super_admin')
			AND NOT (e.sequence = ANY(u.managed_boards))
			AND NOT (lower(e.language) = ANY(u.languages))
		LIMIT 1;
	`, ownerEmail, emailIDs).Scan(&title)
	if err == nil {
		return createEmailInputError(fmt.Sprintf("%s neither manages the board of %q nor translates its language", ownerEmail, title))
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var known bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1));`, ownerEmail).Scan(&known); err != nil {
		return err
	}
	if !known {
		return createEmailInputError("the owner must be a ReviewDesk user")
	}
	return nil
}
