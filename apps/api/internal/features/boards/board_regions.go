package boards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Region is a version of a board's content: the reference chain or a market
// whose managers adapt it, each manager with a robot in Bitrix24.
type Region struct {
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	Language  string    `json:"language"`
	Managers  []string  `json:"managers"`
	Reference bool      `json:"reference"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
	// Emails is how many active emails of the board belong to the region.
	Emails int `json:"emails"`
}

var regionKeyPattern = regexp.MustCompile(`[^a-z0-9]+`)

// RegionKey makes a key from a region name: "EU + SSA" → "eu-ssa".
func RegionKey(name string) string {
	return strings.Trim(regionKeyPattern.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// ListRegions returns the regions of a board in order.
func ListRegions(ctx context.Context, db rowsQuerier, boardKey string) ([]Region, error) {
	rows, err := db.Query(ctx, `
		SELECT r.key, r.name, r.language, r.managers, r.reference, r.updated_by_email, r.updated_at,
			(SELECT count(*)::int FROM emails e
				WHERE e.sequence = r.board_key AND e.region_key = r.key AND e.archived_at IS NULL)
		FROM board_regions r
		WHERE r.board_key = $1
		ORDER BY r.sort_order, r.key;
	`, boardKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	regions := []Region{}
	for rows.Next() {
		var region Region
		if err := rows.Scan(&region.Key, &region.Name, &region.Language, &region.Managers, &region.Reference,
			&region.UpdatedBy, &region.UpdatedAt, &region.Emails); err != nil {
			return nil, err
		}
		regions = append(regions, region)
	}
	return regions, rows.Err()
}

type rowsQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// normalizeRegions checks a board's region list before it replaces the old
// one: names and languages filled in, keys unique, at most one reference.
func normalizeRegions(regions []Region) ([]Region, error) {
	seen := map[string]bool{}
	references := 0
	out := make([]Region, 0, len(regions))
	for _, region := range regions {
		region.Name = strings.TrimSpace(region.Name)
		region.Language = strings.ToLower(strings.TrimSpace(region.Language))
		region.Key = RegionKey(region.Key)
		if region.Key == "" {
			region.Key = RegionKey(region.Name)
		}
		switch {
		case region.Name == "" || region.Key == "":
			return nil, fmt.Errorf("every region needs a name")
		case region.Language == "":
			return nil, fmt.Errorf("region %s: the language is required", region.Name)
		case seen[region.Key]:
			return nil, fmt.Errorf("region %s is in the list twice", region.Name)
		}
		seen[region.Key] = true
		managers := []string{}
		for _, manager := range region.Managers {
			if manager = strings.TrimSpace(manager); manager != "" {
				managers = append(managers, manager)
			}
		}
		region.Managers = managers
		if region.Reference {
			references++
		}
		out = append(out, region)
	}
	if references > 1 {
		return nil, fmt.Errorf("a board has one reference region")
	}
	return out, nil
}

// replaceRegions makes regions the board's list, in their order. A region
// left out is deleted and its emails are left without a region.
func replaceRegions(ctx context.Context, tx pgx.Tx, boardKey string, regions []Region, userEmail string) error {
	keys := make([]string, len(regions))
	for i, region := range regions {
		keys[i] = region.Key
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM board_regions WHERE board_key = $1 AND NOT (key = ANY($2));
	`, boardKey, keys); err != nil {
		return err
	}
	// The reference moves in two steps: the unique index allows one.
	if _, err := tx.Exec(ctx, `UPDATE board_regions SET reference = false WHERE board_key = $1;`, boardKey); err != nil {
		return err
	}
	for i, region := range regions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO board_regions (board_key, key, name, language, managers, reference, sort_order, updated_by_email)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (board_key, key) DO UPDATE SET
				name = EXCLUDED.name,
				language = EXCLUDED.language,
				managers = EXCLUDED.managers,
				reference = EXCLUDED.reference,
				sort_order = EXCLUDED.sort_order,
				updated_by_email = CASE
					WHEN (board_regions.name, board_regions.language, board_regions.managers, board_regions.reference)
						IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.language, EXCLUDED.managers, EXCLUDED.reference)
					THEN EXCLUDED.updated_by_email ELSE board_regions.updated_by_email END,
				updated_at = CASE
					WHEN (board_regions.name, board_regions.language, board_regions.managers, board_regions.reference)
						IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.language, EXCLUDED.managers, EXCLUDED.reference)
					THEN now() ELSE board_regions.updated_at END;
		`, boardKey, region.Key, region.Name, region.Language, region.Managers, region.Reference, i, userEmail); err != nil {
			return err
		}
	}
	return nil
}

func listRegionsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		regions, err := ListRegions(r.Context(), dbpool, chi.URLParam(r, "boardKey"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list regions: %v\n", err)
			http.Error(w, "failed to load regions", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"regions": regions})
	}
}

type replaceRegionsRequest struct {
	Regions []Region `json:"regions"`
}

// replaceRegionsHandler saves the whole region list of a board; managers of
// the board may.
func replaceRegionsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireBoardManager(w, r, dbpool)
		if !ok {
			return
		}
		boardKey := chi.URLParam(r, "boardKey")
		var request replaceRegionsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		regions, err := normalizeRegions(request.Regions)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to save regions", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())
		var exists bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM boards WHERE key = $1);`, boardKey).Scan(&exists); err != nil || !exists {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}
		if err := replaceRegions(r.Context(), tx, boardKey, regions, user.Email); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save regions of %s: %v\n", boardKey, err)
			http.Error(w, "failed to save regions", http.StatusInternalServerError)
			return
		}
		// A renamed region or a new reference renames the emails' versions.
		if _, err := SyncRegionAdaptations(r.Context(), tx, boardKey, ""); err != nil {
			if errors.Is(err, ErrRegionSlotTaken) {
				http.Error(w, "two emails of a slot would be the same language and version of one region: "+
					"move one of them to another region first", http.StatusConflict)
				return
			}
			fmt.Fprintf(os.Stderr, "failed to sync the versions of %s with its regions: %v\n", boardKey, err)
			http.Error(w, "failed to save regions", http.StatusInternalServerError)
			return
		}
		saved, err := ListRegions(r.Context(), tx, boardKey)
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			http.Error(w, "failed to save regions", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"regions": saved})
	}
}
