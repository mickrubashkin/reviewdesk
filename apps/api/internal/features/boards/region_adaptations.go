package boards

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrRegionSlotTaken is returned when two emails would end up as the same
// language and version of one region in a slot.
var ErrRegionSlotTaken = errors.New("the slot already has an email in this language and version in that region")

// AdaptationChange is an email whose adaptation followed its region.
type AdaptationChange struct {
	EmailID, Slug, Title string
	Before, After        string
}

// SyncRegionAdaptations makes the adaptation of a board's emails follow
// their region: the main region of a language (the reference for its
// language, else the first region with it) is the default version, any
// other region is an adaptation named after it. So the slot's versions, the
// switchers and the filters show regions. Emails without a region keep
// their adaptation. emailID limits it to one email; "" does the whole
// board.
func SyncRegionAdaptations(ctx context.Context, tx pgx.Tx, board, emailID string) ([]AdaptationChange, error) {
	rows, err := tx.Query(ctx, `
		WITH main AS (
			SELECT DISTINCT ON (language) language, key
			FROM board_regions
			WHERE board_key = $1
			ORDER BY language, reference DESC, sort_order, key
		), target AS (
			SELECT e.id, e.adaptation_key AS old_key, e.adaptation_label AS old_label,
				CASE WHEN m.key = r.key THEN 'default' ELSE r.key END AS new_key,
				CASE WHEN m.key = r.key THEN 'Default' ELSE r.name END AS new_label
			FROM emails e
			JOIN board_regions r ON r.board_key = e.sequence AND r.key = e.region_key
			JOIN main m ON m.language = r.language
			WHERE e.sequence = $1 AND e.archived_at IS NULL AND ($2 = '' OR e.id::text = $2)
		)
		UPDATE emails e
		SET adaptation_key = t.new_key, adaptation_label = t.new_label, updated_at = now()
		FROM target t
		WHERE e.id = t.id AND (t.old_key, t.old_label) IS DISTINCT FROM (t.new_key, t.new_label)
		RETURNING e.id, e.slug, e.title, t.old_label, t.new_label;
	`, board, emailID)
	if err != nil {
		return nil, slotTaken(err)
	}
	defer rows.Close()
	changes := []AdaptationChange{}
	for rows.Next() {
		var change AdaptationChange
		if err := rows.Scan(&change.EmailID, &change.Slug, &change.Title, &change.Before, &change.After); err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		return nil, slotTaken(err)
	}
	return changes, nil
}

// RegionAdaptation is the adaptation an email of a region gets, by the rule
// of SyncRegionAdaptations; ok is false when the board has no such region.
func RegionAdaptation(ctx context.Context, db interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, board, regionKey string) (key, label string, ok bool, err error) {
	err = db.QueryRow(ctx, `
		SELECT CASE WHEN m.key = r.key THEN 'default' ELSE r.key END,
			CASE WHEN m.key = r.key THEN 'Default' ELSE r.name END
		FROM board_regions r
		JOIN LATERAL (
			SELECT key FROM board_regions
			WHERE board_key = r.board_key AND language = r.language
			ORDER BY reference DESC, sort_order, key
			LIMIT 1
		) m ON true
		WHERE r.board_key = $1 AND r.key = $2;
	`, board, regionKey).Scan(&key, &label)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	return key, label, err == nil, err
}

func slotTaken(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "emails_active_adaptation_unique" {
		return ErrRegionSlotTaken
	}
	return err
}
