package boards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type createBoardRequest struct {
	Name           string  `json:"name"`
	Key            *string `json:"key"`
	SourceBoardKey *string `json:"source_board_key"`
	// CopyEmails also copies the source board's EN emails as drafts.
	CopyEmails bool `json:"copy_emails"`
}

type createBoardStageRequest struct {
	Name string  `json:"name"`
	Key  *string `json:"key"`
}

type updateBoardStageRequest struct {
	Name string `json:"name"`
}

type reorderBoardStagesRequest struct {
	Stages []string `json:"stages"`
}

var (
	errBoardNotFound     = errors.New("board not found")
	errStageExists       = errors.New("stage already exists")
	errStageNotFound     = errors.New("stage not found")
	errStageNotEmpty     = errors.New("stage is not empty")
	errInvalidStageOrder = errors.New("invalid stage order")
)

func RegisterBoardRoutes(r chi.Router, dbpool *pgxpool.Pool) {
	r.Get("/api/boards", listBoardsHandler(dbpool))
	r.Post("/api/boards", createBoardHandler(dbpool))
	r.Get("/api/boards/{boardKey}/regions", listRegionsHandler(dbpool))
	r.Put("/api/boards/{boardKey}/regions", replaceRegionsHandler(dbpool))
	r.Post("/api/boards/{boardKey}/stages", createBoardStageHandler(dbpool))
	r.Patch("/api/boards/{boardKey}/stages", reorderBoardStagesHandler(dbpool))
	r.Patch("/api/boards/{boardKey}/stages/{stage}", updateBoardStageHandler(dbpool))
	r.Delete("/api/boards/{boardKey}/stages/{stage}", deleteBoardStageHandler(dbpool))
}

func listBoardsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boards, err := listBoards(r.Context(), dbpool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list boards: %v\n", err)
			http.Error(w, "failed to load boards", http.StatusInternalServerError)
			return
		}
		if user, ok := auth.FromRequest(r); ok {
			visible, scoped, err := auth.ScopedBoards(r.Context(), dbpool, user)
			if err != nil {
				http.Error(w, "failed to load boards", http.StatusInternalServerError)
				return
			}
			if scoped {
				boards = slices.DeleteFunc(boards, func(board BoardItem) bool { return !slices.Contains(visible, board.Key) })
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(boards)
	}
}

func createBoardStageHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireBoardManager(w, r, dbpool)
		if !ok {
			return
		}

		var request createBoardStageRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(request.Name)
		keySource := name
		if request.Key != nil {
			keySource = *request.Key
		}
		stage := normalizeStageKey(keySource)
		if name == "" || stage == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}

		board, err := addBoardStage(r.Context(), dbpool, chi.URLParam(r, "boardKey"), stage, user)
		writeBoardStageResponse(w, err, board)
	}
}

func updateBoardStageHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireBoardManager(w, r, dbpool)
		if !ok {
			return
		}

		var request updateBoardStageRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		newStage := normalizeStageKey(request.Name)
		if newStage == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}

		board, err := renameBoardStage(r.Context(), dbpool, chi.URLParam(r, "boardKey"), chi.URLParam(r, "stage"), newStage, user)
		writeBoardStageResponse(w, err, board)
	}
}

func deleteBoardStageHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireBoardManager(w, r, dbpool)
		if !ok {
			return
		}

		board, err := deleteBoardStage(r.Context(), dbpool, chi.URLParam(r, "boardKey"), chi.URLParam(r, "stage"), user)
		writeBoardStageResponse(w, err, board)
	}
}

func reorderBoardStagesHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireBoardManager(w, r, dbpool)
		if !ok {
			return
		}

		var request reorderBoardStagesRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		stages := make([]string, 0, len(request.Stages))
		for _, stage := range request.Stages {
			normalizedStage := normalizeStageKey(stage)
			if normalizedStage == "" {
				http.Error(w, "stages must not contain empty values", http.StatusBadRequest)
				return
			}
			stages = append(stages, normalizedStage)
		}

		board, err := reorderBoardStages(r.Context(), dbpool, chi.URLParam(r, "boardKey"), stages, user)
		writeBoardStageResponse(w, err, board)
	}
}

func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	_, ok := requireAdminUser(w, r)
	return ok
}

func requireAdminUser(w http.ResponseWriter, r *http.Request) (AuthUser, bool) {
	user, ok := auth.FromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return AuthUser{}, false
	}
	if !auth.IsAdmin(user) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return AuthUser{}, false
	}
	return user, true
}

// requireBoardManager lets through admins and the managers of the board in
// the URL (Users → Manages).
func requireBoardManager(w http.ResponseWriter, r *http.Request, dbpool *pgxpool.Pool) (AuthUser, bool) {
	user, ok := auth.FromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return AuthUser{}, false
	}
	allowed, err := auth.CanManageBoard(r.Context(), dbpool, user, chi.URLParam(r, "boardKey"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to check board access: %v\n", err)
		http.Error(w, "failed to check access", http.StatusInternalServerError)
		return AuthUser{}, false
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return AuthUser{}, false
	}
	return user, true
}

func writeBoardStageResponse(w http.ResponseWriter, err error, board BoardItem) {
	if err != nil {
		switch {
		case errors.Is(err, errBoardNotFound):
			http.Error(w, "board not found", http.StatusNotFound)
		case errors.Is(err, errStageExists):
			http.Error(w, "stage already exists", http.StatusConflict)
		case errors.Is(err, errStageNotFound):
			http.Error(w, "stage not found", http.StatusNotFound)
		case errors.Is(err, errStageNotEmpty):
			http.Error(w, "stage is not empty", http.StatusConflict)
		case errors.Is(err, errInvalidStageOrder):
			http.Error(w, "stage order must contain the same stages exactly once", http.StatusBadRequest)
		default:
			fmt.Fprintf(os.Stderr, "failed to update board stages: %v\n", err)
			http.Error(w, "failed to update board stages", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(board)
}

func createBoardHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdminUser(w, r)
		if !ok {
			return
		}

		var request createBoardRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(request.Name)
		if name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}

		keySource := name
		if request.Key != nil {
			keySource = *request.Key
		}
		key := normalizeBoardKey(keySource)
		if key == "" {
			http.Error(w, "key is required", http.StatusBadRequest)
			return
		}

		sourceBoardKey := strings.TrimSpace(stringFromPointer(request.SourceBoardKey))
		if sourceBoardKey == "" {
			sourceBoardKey = "onboarding"
		}

		board, err := createBoardFromSource(r.Context(), dbpool, name, key, sourceBoardKey, request.CopyEmails, user)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "source board not found", http.StatusBadRequest)
				return
			}
			if isBoardKeyConflict(err) {
				http.Error(w, "board already exists", http.StatusConflict)
				return
			}
			fmt.Fprintf(os.Stderr, "failed to create board %s: %v\n", key, err)
			http.Error(w, "failed to create board", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(board)
	}
}

func createBoardFromSource(ctx context.Context, dbpool *pgxpool.Pool, name string, key string, sourceBoardKey string, copyEmails bool, actor AuthUser) (BoardItem, error) {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return BoardItem{}, err
	}
	defer tx.Rollback(ctx)

	var stages []byte
	err = tx.QueryRow(ctx, `
		SELECT stages
		FROM boards
		WHERE key = $1;
	`, sourceBoardKey).Scan(&stages)
	if err != nil {
		return BoardItem{}, err
	}

	var board BoardItem
	var createdStages []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO boards (key, name, stages)
		VALUES ($1, $2, $3::jsonb)
		RETURNING id, key, name, stages, created_at, updated_at;
	`, key, name, stages).Scan(
		&board.ID,
		&board.Key,
		&board.Name,
		&createdStages,
		&board.CreatedAt,
		&board.UpdatedAt,
	)
	if err != nil {
		return BoardItem{}, err
	}
	if err := json.Unmarshal(createdStages, &board.Stages); err != nil {
		return BoardItem{}, err
	}
	copiedEmailCount := 0
	if copyEmails {
		if CopyEmails == nil {
			return BoardItem{}, errors.New("copying emails is not configured")
		}
		copiedEmailCount, err = CopyEmails(ctx, tx, actor, sourceBoardKey, board.Key)
		if err != nil {
			return BoardItem{}, err
		}
		board.CopiedEmailCount = &copiedEmailCount
	}
	if err := insertBoardEvent(ctx, tx, actor, boardEventCreated, board, map[string]any{
		"board_key":          board.Key,
		"board_name":         board.Name,
		"source_board_key":   sourceBoardKey,
		"copied_email_count": copiedEmailCount,
	}, map[string]any{
		"board": map[string]any{
			"before": nil,
			"after": map[string]any{
				"key":    board.Key,
				"name":   board.Name,
				"stages": board.Stages,
			},
		},
	}); err != nil {
		return BoardItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BoardItem{}, err
	}

	return board, nil
}

func listBoards(ctx context.Context, dbpool *pgxpool.Pool) ([]BoardItem, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT id, key, name, stages, created_at, updated_at,
			(
				SELECT max(finished_at) FROM portal_sync_runs r
				WHERE r.board_key = boards.key AND r.status = 'done'
			)
		FROM boards
		ORDER BY created_at, name;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	boards := []BoardItem{}
	for rows.Next() {
		var board BoardItem
		var stages []byte
		if err := rows.Scan(
			&board.ID,
			&board.Key,
			&board.Name,
			&stages,
			&board.CreatedAt,
			&board.UpdatedAt,
			&board.PortalSyncedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(stages, &board.Stages); err != nil {
			return nil, err
		}
		boards = append(boards, board)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return boards, nil
}

func Exists(ctx context.Context, dbpool *pgxpool.Pool, boardKey string) (bool, error) {
	var exists bool
	err := dbpool.QueryRow(ctx, `SELECT exists(SELECT 1 FROM boards WHERE key = $1)`, boardKey).Scan(&exists)
	return exists, err
}

func addBoardStage(ctx context.Context, dbpool *pgxpool.Pool, boardKey string, stage string, actor AuthUser) (BoardItem, error) {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return BoardItem{}, err
	}
	defer tx.Rollback(ctx)

	board, err := getBoardForUpdate(ctx, tx, boardKey)
	if err != nil {
		return BoardItem{}, err
	}
	if containsStage(board.Stages, stage) {
		return BoardItem{}, errStageExists
	}

	beforeStages := copyStringSlice(board.Stages)
	board.Stages = append(board.Stages, stage)
	updated, err := updateBoardStages(ctx, tx, board.Key, board.Stages)
	if err != nil {
		return BoardItem{}, err
	}
	if err := insertBoardEvent(ctx, tx, actor, boardEventStageCreated, updated, map[string]any{
		"board_key":  updated.Key,
		"board_name": updated.Name,
		"stage":      stage,
	}, map[string]any{
		"stage": map[string]any{
			"before": nil,
			"after":  stage,
		},
		"stages": map[string]any{
			"before": beforeStages,
			"after":  updated.Stages,
		},
	}); err != nil {
		return BoardItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BoardItem{}, err
	}

	return updated, nil
}

func renameBoardStage(ctx context.Context, dbpool *pgxpool.Pool, boardKey string, oldStage string, newStage string, actor AuthUser) (BoardItem, error) {
	oldStage = normalizeStageKey(oldStage)
	if oldStage == "" {
		return BoardItem{}, errStageNotFound
	}

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return BoardItem{}, err
	}
	defer tx.Rollback(ctx)

	board, err := getBoardForUpdate(ctx, tx, boardKey)
	if err != nil {
		return BoardItem{}, err
	}

	stageIndex := indexOfStage(board.Stages, oldStage)
	if stageIndex == -1 {
		return BoardItem{}, errStageNotFound
	}
	if oldStage != newStage && containsStage(board.Stages, newStage) {
		return BoardItem{}, errStageExists
	}

	beforeStages := copyStringSlice(board.Stages)
	board.Stages[stageIndex] = newStage
	updated, err := updateBoardStages(ctx, tx, board.Key, board.Stages)
	if err != nil {
		return BoardItem{}, err
	}
	affectedEmailCount := int64(0)
	if oldStage != newStage {
		commandTag, err := tx.Exec(ctx, `
			UPDATE emails
			SET stage = $1, updated_at = now()
			WHERE sequence = $2 AND stage = $3;
		`, newStage, board.Key, oldStage)
		if err != nil {
			return BoardItem{}, err
		}
		affectedEmailCount = commandTag.RowsAffected()
	}
	if err := insertBoardEvent(ctx, tx, actor, boardEventStageRenamed, updated, map[string]any{
		"board_key":            updated.Key,
		"board_name":           updated.Name,
		"stage":                newStage,
		"affected_email_count": affectedEmailCount,
	}, map[string]any{
		"stage": map[string]any{
			"before": oldStage,
			"after":  newStage,
		},
		"stages": map[string]any{
			"before": beforeStages,
			"after":  updated.Stages,
		},
	}); err != nil {
		return BoardItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BoardItem{}, err
	}

	return updated, nil
}

func deleteBoardStage(ctx context.Context, dbpool *pgxpool.Pool, boardKey string, stage string, actor AuthUser) (BoardItem, error) {
	stage = normalizeStageKey(stage)
	if stage == "" {
		return BoardItem{}, errStageNotFound
	}

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return BoardItem{}, err
	}
	defer tx.Rollback(ctx)

	board, err := getBoardForUpdate(ctx, tx, boardKey)
	if err != nil {
		return BoardItem{}, err
	}

	stageIndex := indexOfStage(board.Stages, stage)
	if stageIndex == -1 {
		return BoardItem{}, errStageNotFound
	}

	var emailCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM emails
		WHERE sequence = $1 AND stage = $2 AND archived_at IS NULL;
	`, board.Key, stage).Scan(&emailCount); err != nil {
		return BoardItem{}, err
	}
	if emailCount > 0 {
		return BoardItem{}, errStageNotEmpty
	}

	beforeStages := copyStringSlice(board.Stages)
	board.Stages = append(board.Stages[:stageIndex], board.Stages[stageIndex+1:]...)
	updated, err := updateBoardStages(ctx, tx, board.Key, board.Stages)
	if err != nil {
		return BoardItem{}, err
	}
	if err := insertBoardEvent(ctx, tx, actor, boardEventStageDeleted, updated, map[string]any{
		"board_key":  updated.Key,
		"board_name": updated.Name,
		"stage":      stage,
	}, map[string]any{
		"stage": map[string]any{
			"before": stage,
			"after":  nil,
		},
		"stages": map[string]any{
			"before": beforeStages,
			"after":  updated.Stages,
		},
	}); err != nil {
		return BoardItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BoardItem{}, err
	}

	return updated, nil
}

func reorderBoardStages(ctx context.Context, dbpool *pgxpool.Pool, boardKey string, stages []string, actor AuthUser) (BoardItem, error) {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return BoardItem{}, err
	}
	defer tx.Rollback(ctx)

	board, err := getBoardForUpdate(ctx, tx, boardKey)
	if err != nil {
		return BoardItem{}, err
	}
	if !sameStageSet(board.Stages, stages) {
		return BoardItem{}, errInvalidStageOrder
	}

	beforeStages := copyStringSlice(board.Stages)
	updated, err := updateBoardStages(ctx, tx, board.Key, stages)
	if err != nil {
		return BoardItem{}, err
	}
	if err := insertBoardEvent(ctx, tx, actor, boardEventStagesReordered, updated, map[string]any{
		"board_key":  updated.Key,
		"board_name": updated.Name,
	}, map[string]any{
		"stages": map[string]any{
			"before": beforeStages,
			"after":  updated.Stages,
		},
	}); err != nil {
		return BoardItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BoardItem{}, err
	}

	return updated, nil
}

func getBoardForUpdate(ctx context.Context, tx pgx.Tx, boardKey string) (BoardItem, error) {
	var board BoardItem
	var stages []byte
	err := tx.QueryRow(ctx, `
		SELECT id, key, name, stages, created_at, updated_at
		FROM boards
		WHERE key = $1
		FOR UPDATE;
	`, boardKey).Scan(
		&board.ID,
		&board.Key,
		&board.Name,
		&stages,
		&board.CreatedAt,
		&board.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BoardItem{}, errBoardNotFound
	}
	if err != nil {
		return BoardItem{}, err
	}
	if err := json.Unmarshal(stages, &board.Stages); err != nil {
		return BoardItem{}, err
	}

	return board, nil
}

func insertBoardEvent(ctx context.Context, db EmailEventExecutor, actor AuthUser, action string, board BoardItem, metadata map[string]any, changes map[string]any) error {
	if Logger == nil {
		return nil
	}
	return Logger.LogEvent(ctx, db, actor, action, board, metadata, changes)
}

func updateBoardStages(ctx context.Context, tx pgx.Tx, boardKey string, stages []string) (BoardItem, error) {
	stagesJSON, err := json.Marshal(stages)
	if err != nil {
		return BoardItem{}, err
	}

	var board BoardItem
	var updatedStages []byte
	err = tx.QueryRow(ctx, `
		UPDATE boards
		SET stages = $2::jsonb, updated_at = now()
		WHERE key = $1
		RETURNING id, key, name, stages, created_at, updated_at;
	`, boardKey, stagesJSON).Scan(
		&board.ID,
		&board.Key,
		&board.Name,
		&updatedStages,
		&board.CreatedAt,
		&board.UpdatedAt,
	)
	if err != nil {
		return BoardItem{}, err
	}
	if err := json.Unmarshal(updatedStages, &board.Stages); err != nil {
		return BoardItem{}, err
	}

	return board, nil
}

func boardExists(ctx context.Context, db interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, key string) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM boards WHERE key = $1);
	`, key).Scan(&exists)
	return exists, err
}

func normalizeBoardKey(value string) string {
	return normalizeAdaptationKey(value)
}

func normalizeStageKey(value string) string {
	return normalizeAdaptationKey(value)
}

func containsStage(stages []string, stage string) bool {
	return indexOfStage(stages, stage) != -1
}

func indexOfStage(stages []string, stage string) int {
	for index, candidate := range stages {
		if candidate == stage {
			return index
		}
	}

	return -1
}

func sameStageSet(first []string, second []string) bool {
	if len(first) != len(second) {
		return false
	}

	counts := make(map[string]int, len(first))
	for _, stage := range first {
		counts[stage]++
	}
	for _, stage := range second {
		if counts[stage] == 0 {
			return false
		}
		counts[stage]--
	}

	return true
}

func copyStringSlice(values []string) []string {
	copied := make([]string, len(values))
	copy(copied, values)
	return copied
}

func isBoardKeyConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "boards_key_key"
}
