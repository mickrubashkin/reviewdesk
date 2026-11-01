package boards

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestListBoards(t *testing.T) {
	dbpool := testDBPool(t)

	router := chi.NewRouter()
	RegisterBoardRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodGet, "/api/boards", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d: %s", response.Code, response.Body.String())
	}

	var boards []BoardItem
	if err := json.NewDecoder(response.Body).Decode(&boards); err != nil {
		t.Fatalf("failed to decode boards: %v", err)
	}
	if len(boards) == 0 {
		t.Fatal("expected at least one board")
	}
}

func TestCreateBoardCopiesStages(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")
	sourceKey := createTestBoard(t, dbpool, "source-board", []string{"registered", "qualified"})

	router := chi.NewRouter()
	RegisterBoardRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/boards",
		bytes.NewReader([]byte(`{ "name": "Launch Board", "source_board_key": "`+sourceKey+`" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", response.Code, response.Body.String())
	}

	var created BoardItem
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created board: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE metadata->>'board_key' = $1;`, created.Key)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM boards WHERE id = $1;`, created.ID)
	})
	if created.Key != "launch-board" || created.Name != "Launch Board" {
		t.Fatalf("unexpected created board: %#v", created)
	}
	if len(created.Stages) != 2 || created.Stages[0] != "registered" || created.Stages[1] != "qualified" {
		t.Fatalf("expected copied stages, got %#v", created.Stages)
	}

	event := loadBoardEvent(t, dbpool, created.Key, "board_created")
	if event.ActorEmail != user.Email {
		t.Fatalf("expected board event actor %s, got %s", user.Email, event.ActorEmail)
	}
	if event.Metadata["source_board_key"] != sourceKey {
		t.Fatalf("expected source board key %s in event metadata, got %#v", sourceKey, event.Metadata["source_board_key"])
	}
}

func TestCreateBoardConflict(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")
	createTestBoard(t, dbpool, "duplicate-board", []string{"registered"})

	router := chi.NewRouter()
	RegisterBoardRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/boards",
		bytes.NewReader([]byte(`{ "name": "Duplicate Board", "key": "duplicate-board", "source_board_key": "duplicate-board" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("expected POST status 409, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateBoardRejectsReviewer(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "reviewer")
	sourceKey := createTestBoard(t, dbpool, "reviewer-source-board", []string{"registered"})

	router := chi.NewRouter()
	RegisterBoardRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/boards",
		bytes.NewReader([]byte(`{ "name": "Reviewer Board", "source_board_key": "`+sourceKey+`" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected POST status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateBoardStage(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")
	boardKey := createTestBoard(t, dbpool, "stage-create-board", []string{"registered"})

	router := chi.NewRouter()
	RegisterBoardRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/boards/"+boardKey+"/stages",
		bytes.NewReader([]byte(`{ "name": "Ready for QA" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected stage create status 200, got %d: %s", response.Code, response.Body.String())
	}

	var board BoardItem
	if err := json.NewDecoder(response.Body).Decode(&board); err != nil {
		t.Fatalf("failed to decode board: %v", err)
	}
	if len(board.Stages) != 2 || board.Stages[1] != "ready-for-qa" {
		t.Fatalf("expected appended stage, got %#v", board.Stages)
	}

	event := loadBoardEvent(t, dbpool, boardKey, "board_stage_created")
	if event.ActorEmail != user.Email {
		t.Fatalf("expected stage create event actor %s, got %s", user.Email, event.ActorEmail)
	}
	if event.Metadata["stage"] != "ready-for-qa" {
		t.Fatalf("expected created stage in event metadata, got %#v", event.Metadata["stage"])
	}
}

func TestRenameBoardStageUpdatesEmails(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")
	boardKey := createTestBoard(t, dbpool, "stage-rename-board", []string{"registered", "qualified"})
	emailID := createBoardTestEmail(t, dbpool, boardKey, "registered")

	router := chi.NewRouter()
	RegisterBoardRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/boards/"+boardKey+"/stages/registered",
		bytes.NewReader([]byte(`{ "name": "Signup" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected stage rename status 200, got %d: %s", response.Code, response.Body.String())
	}

	var stage string
	if err := dbpool.QueryRow(context.Background(), `SELECT stage FROM emails WHERE id = $1;`, emailID).Scan(&stage); err != nil {
		t.Fatalf("failed to load updated email stage: %v", err)
	}
	if stage != "signup" {
		t.Fatalf("expected email stage signup, got %s", stage)
	}

	event := loadBoardEvent(t, dbpool, boardKey, "board_stage_renamed")
	stageChange, ok := event.Changes["stage"].(map[string]any)
	if !ok {
		t.Fatalf("expected stage change event, got %#v", event.Changes)
	}
	if stageChange["before"] != "registered" || stageChange["after"] != "signup" {
		t.Fatalf("unexpected stage change: %#v", stageChange)
	}
	if event.Metadata["affected_email_count"] != float64(1) {
		t.Fatalf("expected affected email count 1, got %#v", event.Metadata["affected_email_count"])
	}
}

func TestDeleteBoardStageWritesEvent(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")
	boardKey := createTestBoard(t, dbpool, "stage-delete-board", []string{"registered", "empty-stage"})

	router := chi.NewRouter()
	RegisterBoardRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodDelete, "/api/boards/"+boardKey+"/stages/empty-stage", nil)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected delete status 200, got %d: %s", response.Code, response.Body.String())
	}

	event := loadBoardEvent(t, dbpool, boardKey, "board_stage_deleted")
	if event.ActorEmail != user.Email {
		t.Fatalf("expected stage delete event actor %s, got %s", user.Email, event.ActorEmail)
	}
	if event.Metadata["stage"] != "empty-stage" {
		t.Fatalf("expected deleted stage in event metadata, got %#v", event.Metadata["stage"])
	}
}

func TestDeleteBoardStageRejectsNonEmptyStage(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")
	boardKey := createTestBoard(t, dbpool, "stage-delete-non-empty-board", []string{"registered"})
	createBoardTestEmail(t, dbpool, boardKey, "registered")

	router := chi.NewRouter()
	RegisterBoardRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodDelete, "/api/boards/"+boardKey+"/stages/registered", nil)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("expected delete status 409, got %d: %s", response.Code, response.Body.String())
	}
}

func TestReorderBoardStages(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")
	boardKey := createTestBoard(t, dbpool, "stage-reorder-board", []string{"registered", "qualified", "approved"})

	router := chi.NewRouter()
	RegisterBoardRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/boards/"+boardKey+"/stages",
		bytes.NewReader([]byte(`{ "stages": ["approved", "registered", "qualified"] }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected reorder status 200, got %d: %s", response.Code, response.Body.String())
	}

	var board BoardItem
	if err := json.NewDecoder(response.Body).Decode(&board); err != nil {
		t.Fatalf("failed to decode board: %v", err)
	}
	if len(board.Stages) != 3 || board.Stages[0] != "approved" || board.Stages[1] != "registered" {
		t.Fatalf("unexpected stage order: %#v", board.Stages)
	}

	event := loadBoardEvent(t, dbpool, boardKey, "board_stages_reordered")
	stagesChange, ok := event.Changes["stages"].(map[string]any)
	if !ok {
		t.Fatalf("expected stages change event, got %#v", event.Changes)
	}
	after, ok := stagesChange["after"].([]any)
	if !ok || len(after) != 3 || after[0] != "approved" {
		t.Fatalf("unexpected reordered stages in event: %#v", stagesChange["after"])
	}
}

func createTestBoard(t *testing.T, dbpool *pgxpool.Pool, key string, stages []string) string {
	t.Helper()
	stagesJSON, err := json.Marshal(stages)
	if err != nil {
		t.Fatalf("failed to encode test board stages: %v", err)
	}

	_, err = dbpool.Exec(context.Background(), `
		INSERT INTO boards (key, name, stages)
		VALUES ($1, $2, $3::jsonb)
		ON CONFLICT (key) DO UPDATE SET
			name = EXCLUDED.name,
			stages = EXCLUDED.stages,
			updated_at = now();
	`, key, key, stagesJSON)
	if err != nil {
		t.Fatalf("failed to create test board: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE metadata->>'board_key' = $1;`, key)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM boards WHERE key = $1;`, key)
	})

	return key
}

func createBoardTestEmail(t *testing.T, dbpool *pgxpool.Pool, sequence string, stage string) string {
	t.Helper()

	var emailID string
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO emails (
			slug,
			sequence,
			title,
			stage,
			sort_order,
			language,
			variant,
			original_html,
			review_html,
			template_html,
			editable_fields
		)
		VALUES (
			$1,
			$2,
			'Board Test Email',
			$3,
			9999,
			'en',
			'new',
			'<html><body>board test</body></html>',
			'<html><body><p data-review-block="body-001">board test</p></body></html>',
			'<html><body><p data-review-block="body-001">board test</p></body></html>',
			'{}'::jsonb
		)
		RETURNING id;
	`, sequence+"-"+stage+"-board-test-email", sequence, stage).Scan(&emailID)
	if err != nil {
		t.Fatalf("failed to create board test email: %v", err)
	}

	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, emailID)
	})

	return emailID
}

type boardEventRecord struct {
	ActorEmail string
	Metadata   map[string]any
	Changes    map[string]any
}

func loadBoardEvent(t *testing.T, dbpool *pgxpool.Pool, boardKey string, action string) boardEventRecord {
	t.Helper()

	var event boardEventRecord
	var metadata []byte
	var changes []byte
	err := dbpool.QueryRow(context.Background(), `
		SELECT actor_email, metadata, changes
		FROM email_events
		WHERE action = $1 AND metadata->>'board_key' = $2
		ORDER BY created_at DESC
		LIMIT 1;
	`, action, boardKey).Scan(&event.ActorEmail, &metadata, &changes)
	if err != nil {
		t.Fatalf("failed to load board event %s for %s: %v", action, boardKey, err)
	}
	if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
		t.Fatalf("failed to decode board event metadata: %v", err)
	}
	if err := json.Unmarshal(changes, &event.Changes); err != nil {
		t.Fatalf("failed to decode board event changes: %v", err)
	}

	return event
}

