package boards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)


type createBoardApprovalAreaRequest struct {
	Name     string  `json:"name"`
	Key      *string `json:"key"`
	Required *bool   `json:"required"`
}

type updateBoardApprovalAreaRequest struct {
	Name     *string `json:"name"`
	Required *bool   `json:"required"`
	Archived *bool   `json:"archived"`
}

type reorderBoardApprovalAreasRequest struct {
	Areas []string `json:"areas"`
}

func listBoardApprovalAreasHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boardKey := chi.URLParam(r, "boardKey")
		areas, err := listBoardApprovalAreas(r.Context(), dbpool, boardKey, false)
		if err != nil {
			writeBoardApprovalAreaError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(areas)
	}
}

func createBoardApprovalAreaHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdminUser(w, r)
		if !ok {
			return
		}

		var request createBoardApprovalAreaRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(request.Name)
		keySource := name
		if request.Key != nil {
			keySource = *request.Key
		}
		key := NormalizeApprovalAreaKey(keySource)
		if name == "" || key == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		required := true
		if request.Required != nil {
			required = *request.Required
		}

		area, err := createBoardApprovalArea(r.Context(), dbpool, chi.URLParam(r, "boardKey"), key, name, required, user)
		if err != nil {
			writeBoardApprovalAreaError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(area)
	}
}

func updateBoardApprovalAreaHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdminUser(w, r)
		if !ok {
			return
		}

		var request updateBoardApprovalAreaRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		area, err := updateBoardApprovalArea(r.Context(), dbpool, chi.URLParam(r, "boardKey"), chi.URLParam(r, "areaKey"), request, user)
		if err != nil {
			writeBoardApprovalAreaError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(area)
	}
}

func deleteBoardApprovalAreaHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdminUser(w, r)
		if !ok {
			return
		}

		area, err := archiveBoardApprovalArea(r.Context(), dbpool, chi.URLParam(r, "boardKey"), chi.URLParam(r, "areaKey"), user)
		if err != nil {
			writeBoardApprovalAreaError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(area)
	}
}

func reorderBoardApprovalAreasHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdminUser(w, r)
		if !ok {
			return
		}

		var request reorderBoardApprovalAreasRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		areaKeys := make([]string, 0, len(request.Areas))
		for _, area := range request.Areas {
			areaKey := NormalizeApprovalAreaKey(area)
			if areaKey == "" {
				http.Error(w, "areas must not contain empty values", http.StatusBadRequest)
				return
			}
			areaKeys = append(areaKeys, areaKey)
		}

		areas, err := reorderBoardApprovalAreas(r.Context(), dbpool, chi.URLParam(r, "boardKey"), areaKeys, user)
		if err != nil {
			writeBoardApprovalAreaError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(areas)
	}
}

func listBoardApprovalAreas(ctx context.Context, db pgxExecutor, boardKey string, includeArchived bool) ([]BoardApprovalAreaItem, error) {
	archivedFilter := "AND board_approval_areas.archived_at IS NULL"
	if includeArchived {
		archivedFilter = ""
	}
	rows, err := db.Query(ctx, `
		SELECT
			board_approval_areas.id,
			approval_areas.key,
			board_approval_areas.name,
			board_approval_areas.required,
			board_approval_areas.sort_order,
			board_approval_areas.archived_at
		FROM board_approval_areas
		JOIN boards ON boards.id = board_approval_areas.board_id
		JOIN approval_areas ON approval_areas.id = board_approval_areas.approval_area_id
		WHERE boards.key = $1
		`+archivedFilter+`
		ORDER BY board_approval_areas.sort_order, board_approval_areas.created_at;
	`, boardKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	areas := []BoardApprovalAreaItem{}
	for rows.Next() {
		var area BoardApprovalAreaItem
		if err := rows.Scan(&area.ID, &area.Key, &area.Name, &area.Required, &area.SortOrder, &area.ArchivedAt); err != nil {
			return nil, err
		}
		areas = append(areas, area)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	exists, err := boardExists(ctx, db, boardKey)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errBoardNotFound
	}

	return areas, nil
}

func createBoardApprovalArea(ctx context.Context, dbpool *pgxpool.Pool, boardKey string, areaKey string, name string, required bool, actor AuthUser) (BoardApprovalAreaItem, error) {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return BoardApprovalAreaItem{}, err
	}
	defer tx.Rollback(ctx)

	board, err := getBoardForUpdate(ctx, tx, boardKey)
	if err != nil {
		return BoardApprovalAreaItem{}, err
	}

	var approvalAreaID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO approval_areas (key, default_name)
		VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET updated_at = now()
		RETURNING id;
	`, areaKey, name).Scan(&approvalAreaID); err != nil {
		return BoardApprovalAreaItem{}, err
	}

	nextSortOrder, err := nextBoardApprovalAreaSortOrder(ctx, tx, board.ID)
	if err != nil {
		return BoardApprovalAreaItem{}, err
	}

	var area BoardApprovalAreaItem
	if err := tx.QueryRow(ctx, `
		INSERT INTO board_approval_areas (
			board_id,
			approval_area_id,
			name,
			required,
			sort_order,
			archived_at
		)
		VALUES ($1, $2, $3, $4, $5, NULL)
		ON CONFLICT (board_id, approval_area_id) DO UPDATE SET
			name = EXCLUDED.name,
			required = EXCLUDED.required,
			sort_order = CASE
				WHEN board_approval_areas.archived_at IS NULL THEN board_approval_areas.sort_order
				ELSE EXCLUDED.sort_order
			END,
			archived_at = NULL,
			updated_at = now()
		RETURNING id, $6::text, name, required, sort_order, archived_at;
	`, board.ID, approvalAreaID, name, required, nextSortOrder, areaKey).Scan(
		&area.ID,
		&area.Key,
		&area.Name,
		&area.Required,
		&area.SortOrder,
		&area.ArchivedAt,
	); err != nil {
		return BoardApprovalAreaItem{}, err
	}

	if err := insertBoardEvent(ctx, tx, actor, boardEventApprovalAreaCreated, board, map[string]any{
		"area_key":  area.Key,
		"area_name": area.Name,
		"required":  area.Required,
	}, map[string]any{
		"approval_area": map[string]any{
			"before": nil,
			"after":  area,
		},
	}); err != nil {
		return BoardApprovalAreaItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BoardApprovalAreaItem{}, err
	}

	return area, nil
}

func updateBoardApprovalArea(ctx context.Context, dbpool *pgxpool.Pool, boardKey string, areaKey string, request updateBoardApprovalAreaRequest, actor AuthUser) (BoardApprovalAreaItem, error) {
	areaKey = NormalizeApprovalAreaKey(areaKey)
	if areaKey == "" {
		return BoardApprovalAreaItem{}, errApprovalAreaNotFound
	}
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return BoardApprovalAreaItem{}, err
	}
	defer tx.Rollback(ctx)

	board, err := getBoardForUpdate(ctx, tx, boardKey)
	if err != nil {
		return BoardApprovalAreaItem{}, err
	}
	current, err := getBoardApprovalAreaForUpdate(ctx, tx, board.ID, areaKey)
	if err != nil {
		return BoardApprovalAreaItem{}, err
	}

	name := current.Name
	if request.Name != nil {
		name = strings.TrimSpace(*request.Name)
		if name == "" {
			return BoardApprovalAreaItem{}, errInvalidApprovalArea
		}
	}
	required := current.Required
	if request.Required != nil {
		required = *request.Required
	}
	archiveExpression := "archived_at"
	if request.Archived != nil {
		if *request.Archived {
			archiveExpression = "now()"
		} else {
			archiveExpression = "NULL"
		}
	}

	var updated BoardApprovalAreaItem
	if err := tx.QueryRow(ctx, `
		UPDATE board_approval_areas
		SET name = $3,
			required = $4,
			archived_at = `+archiveExpression+`,
			updated_at = now()
		FROM approval_areas
		WHERE board_approval_areas.id = $1
			AND board_approval_areas.board_id = $2
			AND approval_areas.id = board_approval_areas.approval_area_id
		RETURNING board_approval_areas.id, approval_areas.key, board_approval_areas.name, board_approval_areas.required, board_approval_areas.sort_order, board_approval_areas.archived_at;
	`, current.ID, board.ID, name, required).Scan(
		&updated.ID,
		&updated.Key,
		&updated.Name,
		&updated.Required,
		&updated.SortOrder,
		&updated.ArchivedAt,
	); err != nil {
		return BoardApprovalAreaItem{}, err
	}

	eventAction := boardEventApprovalAreaUpdated
	if request.Archived != nil && *request.Archived {
		eventAction = boardEventApprovalAreaDeleted
	}
	if err := insertBoardEvent(ctx, tx, actor, eventAction, board, map[string]any{
		"area_key":  updated.Key,
		"area_name": updated.Name,
		"required":  updated.Required,
	}, map[string]any{
		"name": map[string]any{
			"before": current.Name,
			"after":  updated.Name,
		},
		"required": map[string]any{
			"before": current.Required,
			"after":  updated.Required,
		},
		"archived": map[string]any{
			"before": current.ArchivedAt != nil,
			"after":  updated.ArchivedAt != nil,
		},
	}); err != nil {
		return BoardApprovalAreaItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BoardApprovalAreaItem{}, err
	}

	return updated, nil
}

func archiveBoardApprovalArea(ctx context.Context, dbpool *pgxpool.Pool, boardKey string, areaKey string, actor AuthUser) (BoardApprovalAreaItem, error) {
	archived := true
	return updateBoardApprovalArea(ctx, dbpool, boardKey, areaKey, updateBoardApprovalAreaRequest{
		Archived: &archived,
	}, actor)
}

func reorderBoardApprovalAreas(ctx context.Context, dbpool *pgxpool.Pool, boardKey string, areaKeys []string, actor AuthUser) ([]BoardApprovalAreaItem, error) {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	board, err := getBoardForUpdate(ctx, tx, boardKey)
	if err != nil {
		return nil, err
	}

	current, err := listBoardApprovalAreas(ctx, tx, boardKey, false)
	if err != nil {
		return nil, err
	}
	if !sameApprovalAreaSet(current, areaKeys) {
		return nil, errInvalidApprovalAreaOrder
	}

	before := make([]string, 0, len(current))
	for _, area := range current {
		before = append(before, area.Key)
	}
	for index, areaKey := range areaKeys {
		if _, err := tx.Exec(ctx, `
			UPDATE board_approval_areas
			SET sort_order = $3,
				updated_at = now()
			FROM approval_areas
			WHERE board_approval_areas.board_id = $1
				AND board_approval_areas.approval_area_id = approval_areas.id
				AND approval_areas.key = $2;
		`, board.ID, areaKey, (index+1)*10); err != nil {
			return nil, err
		}
	}
	updated, err := listBoardApprovalAreas(ctx, tx, boardKey, false)
	if err != nil {
		return nil, err
	}
	if err := insertBoardEvent(ctx, tx, actor, boardEventApprovalAreasReordered, board, nil, map[string]any{
		"approval_areas": map[string]any{
			"before": before,
			"after":  areaKeys,
		},
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return updated, nil
}

func copyBoardApprovalAreas(ctx context.Context, tx pgx.Tx, sourceBoardKey string, targetBoardID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO board_approval_areas (
			board_id,
			approval_area_id,
			name,
			required,
			sort_order
		)
		SELECT
			$2,
			board_approval_areas.approval_area_id,
			board_approval_areas.name,
			board_approval_areas.required,
			board_approval_areas.sort_order
		FROM board_approval_areas
		JOIN boards ON boards.id = board_approval_areas.board_id
		WHERE boards.key = $1
			AND board_approval_areas.archived_at IS NULL
		ON CONFLICT (board_id, approval_area_id) DO NOTHING;
	`, sourceBoardKey, targetBoardID)
	return err
}

func nextBoardApprovalAreaSortOrder(ctx context.Context, db pgxExecutor, boardID string) (int, error) {
	var sortOrder int
	err := db.QueryRow(ctx, `
		SELECT coalesce(max(sort_order), 0) + 10
		FROM board_approval_areas
		WHERE board_id = $1;
	`, boardID).Scan(&sortOrder)
	return sortOrder, err
}

func getBoardApprovalAreaForUpdate(ctx context.Context, tx pgx.Tx, boardID string, areaKey string) (BoardApprovalAreaItem, error) {
	var area BoardApprovalAreaItem
	err := tx.QueryRow(ctx, `
		SELECT
			board_approval_areas.id,
			approval_areas.key,
			board_approval_areas.name,
			board_approval_areas.required,
			board_approval_areas.sort_order,
			board_approval_areas.archived_at
		FROM board_approval_areas
		JOIN approval_areas ON approval_areas.id = board_approval_areas.approval_area_id
		WHERE board_approval_areas.board_id = $1
			AND approval_areas.key = $2
		FOR UPDATE OF board_approval_areas;
	`, boardID, areaKey).Scan(
		&area.ID,
		&area.Key,
		&area.Name,
		&area.Required,
		&area.SortOrder,
		&area.ArchivedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BoardApprovalAreaItem{}, errApprovalAreaNotFound
	}
	if err != nil {
		return BoardApprovalAreaItem{}, err
	}

	return area, nil
}

func sameApprovalAreaSet(current []BoardApprovalAreaItem, next []string) bool {
	if len(current) != len(next) {
		return false
	}
	counts := map[string]int{}
	for _, area := range current {
		counts[area.Key]++
	}
	for _, areaKey := range next {
		if counts[areaKey] == 0 {
			return false
		}
		counts[areaKey]--
	}
	return true
}

func NormalizeApprovalAreaKey(value string) string {
	return normalizeAdaptationKey(value)
}

var (
	errApprovalAreaNotFound     = errors.New("approval area not found")
	errInvalidApprovalArea      = errors.New("invalid approval area")
	errInvalidApprovalAreaOrder = errors.New("invalid approval area order")
)

type pgxExecutor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func writeBoardApprovalAreaError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBoardNotFound):
		http.Error(w, "board not found", http.StatusNotFound)
	case errors.Is(err, errApprovalAreaNotFound):
		http.Error(w, "approval area not found", http.StatusNotFound)
	case errors.Is(err, errInvalidApprovalArea):
		http.Error(w, "name is required", http.StatusBadRequest)
	case errors.Is(err, errInvalidApprovalAreaOrder):
		http.Error(w, "area order must contain the same active areas exactly once", http.StatusBadRequest)
	default:
		fmt.Fprintf(os.Stderr, "failed to update board approval areas: %v\n", err)
		http.Error(w, "failed to update board approval areas", http.StatusInternalServerError)
	}
}
