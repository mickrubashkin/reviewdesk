package portal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AutoSyncBy is the started_by_email of runs started by the nightly sync.
const AutoSyncBy = "auto"

// syncTimeout bounds one run; runs still "running" well past it were left by
// a stopped server and no longer block new ones.
const syncTimeout = 30 * time.Minute

var ErrSyncRunning = errors.New("a sync for this board is already running")

// BeginRun records a new running sync for the board. The database allows one
// running sync per board, so a concurrent start gets ErrSyncRunning.
func BeginRun(ctx context.Context, db *pgxpool.Pool, options SyncOptions, startedBy string) (string, error) {
	if _, err := db.Exec(ctx, `
		UPDATE portal_sync_runs
		SET status = 'failed', error = 'interrupted', finished_at = now()
		WHERE board_key = $1 AND status = 'running' AND started_at < now() - $2::interval;
	`, options.BoardKey, syncTimeout+5*time.Minute); err != nil {
		return "", err
	}
	var runID string
	err := db.QueryRow(ctx, `
		INSERT INTO portal_sync_runs (board_key, category_id, days, started_by_email)
		VALUES ($1, $2, $3, $4)
		RETURNING id;
	`, options.BoardKey, options.CategoryID, options.Days, startedBy).Scan(&runID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return "", ErrSyncRunning
	}
	return runID, err
}

type SyncSettings struct {
	AutoSync   bool `json:"auto_sync"`
	CategoryID *int `json:"category_id"`
	Days       int  `json:"days"`
}

var defaultSyncSettings = SyncSettings{Days: 3}

func loadSyncSettings(ctx context.Context, db *pgxpool.Pool, boardKey string) (SyncSettings, error) {
	var s SyncSettings
	err := db.QueryRow(ctx, `
		SELECT auto_sync, category_id, days FROM portal_sync_settings WHERE board_key = $1;
	`, boardKey).Scan(&s.AutoSync, &s.CategoryID, &s.Days)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultSyncSettings, nil
	}
	return s, err
}

func (s SyncSettings) validate() error {
	if s.Days < 1 || s.Days > 365 {
		return errors.New("days must be between 1 and 365")
	}
	if s.AutoSync && s.CategoryID == nil {
		return errors.New("choose a Bitrix24 funnel to sync automatically")
	}
	return nil
}

// autoSyncZone and autoSyncHour: the portal team works in UTC+2 and nobody
// is on it at 03:00, so the nightly sync runs then.
var autoSyncZone = time.FixedZone("UTC+2", 2*60*60)

const autoSyncHour = 3

// nextAutoSync is the first 03:00 UTC+2 strictly after now.
func nextAutoSync(now time.Time) time.Time {
	local := now.In(autoSyncZone)
	next := time.Date(local.Year(), local.Month(), local.Day(), autoSyncHour, 0, 0, 0, autoSyncZone)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// RunAutoSync syncs every board with auto sync on once a night until ctx is
// done. Boards run one after another to keep the load on Bitrix24 low.
func RunAutoSync(ctx context.Context, db *pgxpool.Pool) {
	for {
		timer := time.NewTimer(time.Until(nextAutoSync(time.Now())))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err := autoSyncBoards(ctx, db); err != nil {
			fmt.Fprintf(os.Stderr, "portal auto sync: %v\n", err)
		}
	}
}

func autoSyncBoards(ctx context.Context, db *pgxpool.Pool) error {
	// A board an auto run already started for in the last hours is skipped:
	// during a deploy two servers can both reach 03:00.
	rows, err := db.Query(ctx, `
		SELECT s.board_key, s.category_id, s.days
		FROM portal_sync_settings s
		WHERE s.auto_sync AND s.category_id IS NOT NULL
			AND NOT EXISTS (
				SELECT 1 FROM portal_sync_runs r
				WHERE r.board_key = s.board_key AND r.started_by_email = $1 AND r.started_at > now() - interval '12 hours'
			)
		ORDER BY s.board_key;
	`, AutoSyncBy)
	if err != nil {
		return err
	}
	var boards []SyncOptions
	for rows.Next() {
		var o SyncOptions
		if err := rows.Scan(&o.BoardKey, &o.CategoryID, &o.Days); err != nil {
			rows.Close()
			return err
		}
		boards = append(boards, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(boards) == 0 {
		return nil
	}

	client, err := NewClientFromEnv()
	if err != nil {
		return err
	}
	for _, options := range boards {
		runID, err := BeginRun(ctx, db, options, AutoSyncBy)
		if errors.Is(err, ErrSyncRunning) {
			continue
		}
		if err != nil {
			return err
		}
		runCtx, cancel := context.WithTimeout(ctx, syncTimeout)
		err = Sync(runCtx, db, client, runID, options)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "portal auto sync %s (%s) failed: %v\n", options.BoardKey, runID, err)
		}
	}
	return nil
}

var errBoardNotFound = errors.New("board not found")

func saveSyncSettings(ctx context.Context, db *pgxpool.Pool, boardKey string, s SyncSettings, by string) (SyncSettings, error) {
	err := db.QueryRow(ctx, `
		INSERT INTO portal_sync_settings (board_key, auto_sync, category_id, days, updated_by_email)
		SELECT key, $2, $3, $4, $5 FROM boards WHERE key = $1
		ON CONFLICT (board_key) DO UPDATE SET
			auto_sync = EXCLUDED.auto_sync,
			category_id = EXCLUDED.category_id,
			days = EXCLUDED.days,
			updated_by_email = EXCLUDED.updated_by_email,
			updated_at = now()
		RETURNING auto_sync, category_id, days;
	`, boardKey, s.AutoSync, s.CategoryID, s.Days, by).Scan(&s.AutoSync, &s.CategoryID, &s.Days)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, errBoardNotFound
	}
	return s, err
}
