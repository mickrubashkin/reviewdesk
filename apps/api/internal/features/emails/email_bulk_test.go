package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestBulkEmailActions(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	board := createTestBoard(t, dbpool, "bulk-test", []string{"first", "second"})

	insert := func(stage string, sortOrder int, language string) string {
		t.Helper()
		var id string
		if err := dbpool.QueryRow(ctx, `
			INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, editable_fields, send_timing)
			VALUES ($1, 'Slot ' || $3::int, $2, $4, $3::int, $5, 'v1', '<html></html>', '<html></html>', '{}'::jsonb, 'immediately')
			RETURNING id;
		`, fmt.Sprintf("bulk-%s-%d-%s", stage, sortOrder, language), board, sortOrder, stage, language).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = dbpool.Exec(ctx, `DELETE FROM comments WHERE email_id = $1;`, id)
			_, _ = dbpool.Exec(ctx, `DELETE FROM email_events WHERE email_id = $1;`, id)
			_, _ = dbpool.Exec(ctx, `DELETE FROM emails WHERE id = $1;`, id)
		})
		return id
	}
	a := insert("first", 101, "en")
	aDE := insert("first", 101, "de")
	b := insert("first", 102, "en")
	bDE := insert("first", 102, "de")
	c := insert("second", 201, "es")

	var admin AuthUser
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO users (email, role) VALUES ('bulk-test-admin@example.com', 'admin')
		ON CONFLICT (email) DO UPDATE SET role = 'admin' RETURNING id, email, role;
	`).Scan(&admin.ID, &admin.Email, &admin.Role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM users WHERE id = $1;`, admin.ID) })
	reviewer := createTestUser(t, dbpool)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	call := func(user AuthUser, body map[string]any) (*httptest.ResponseRecorder, bulkEmailResponse) {
		t.Helper()
		body["sequence"] = board
		payload, _ := json.Marshal(body)
		request := withAuthUser(httptest.NewRequest(http.MethodPost, "/api/emails/bulk", bytes.NewReader(payload)), user)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var decoded bulkEmailResponse
		_ = json.Unmarshal(response.Body.Bytes(), &decoded)
		return response, decoded
	}
	column := func(id, name string) *string {
		t.Helper()
		var value *string
		if err := dbpool.QueryRow(ctx, fmt.Sprintf(`SELECT %s::text FROM emails WHERE id = $1;`, name), id).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	if response, _ := call(reviewer, map[string]any{"action": "archive", "email_ids": []string{a}}); response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a reviewer, got %d", response.Code)
	}

	// Owner: set on the selection, unknown users refused.
	if response, result := call(admin, map[string]any{"action": "owner", "email_ids": []string{a, aDE}, "owner_email": admin.Email}); response.Code != http.StatusOK || result.Updated != 2 {
		t.Fatalf("owner returned %d: %s", response.Code, response.Body.String())
	}
	if owner := column(aDE, "owner_email"); owner == nil || *owner != admin.Email {
		t.Fatalf("expected the owner to be set, got %v", owner)
	}
	if response, _ := call(admin, map[string]any{"action": "owner", "email_ids": []string{a}, "owner_email": "nobody@example.com"}); response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown owner, got %d", response.Code)
	}

	// Status: an email that fails the checks of form (no button) is skipped,
	// the rest become ready; an open comment holds nothing back.
	if _, err := dbpool.Exec(ctx, `UPDATE emails SET layout = '[{"block":"intro_body"}]' WHERE id = $1;`, aDE); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO comments (email_id, review_block, selected_text, start_offset, end_offset, body, status, severity)
		VALUES ($1, 'body-001', 'x', 0, 1, 'Fix this', 'open', 'issue');
	`, a); err != nil {
		t.Fatal(err)
	}
	response, result := call(admin, map[string]any{"action": "status", "email_ids": []string{a, aDE}, "review_status": "ready"})
	if response.Code != http.StatusOK || result.Updated != 1 || len(result.Skipped) != 1 || result.Skipped[0].ID != aDE {
		t.Fatalf("status returned %d: %s", response.Code, response.Body.String())
	}
	if status := column(a, "review_status"); *status != "ready" {
		t.Fatalf("expected ready, got %s", *status)
	}

	// Timing applies to the whole slot, the unselected language included.
	if response, result := call(admin, map[string]any{"action": "timing", "email_ids": []string{a}, "send_timing": "2 days"}); response.Code != http.StatusOK || result.Slots != 1 {
		t.Fatalf("timing returned %d: %s", response.Code, response.Body.String())
	}
	if timing := column(aDE, "send_timing"); timing == nil || *timing != "2 days" {
		t.Fatalf("expected the whole slot to get the timing, got %v", timing)
	}

	// Moving a language into a slot that already has it is refused as a whole.
	if response, _ := call(admin, map[string]any{"action": "slot", "email_ids": []string{bDE, c}, "stage": "first", "sort_order": 101}); response.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a taken language, got %d: %s", response.Code, response.Body.String())
	}
	if stage := column(c, "stage"); *stage != "second" {
		t.Fatalf("a refused move must change nothing, got %s", *stage)
	}

	// Into an existing slot: ES joins slot 101 and takes its timing.
	if response, result := call(admin, map[string]any{"action": "slot", "email_ids": []string{c}, "stage": "first", "sort_order": 101}); response.Code != http.StatusOK || result.SortOrder == nil || *result.SortOrder != 101 {
		t.Fatalf("move returned %d: %s", response.Code, response.Body.String())
	}
	if timing := column(c, "send_timing"); timing == nil || *timing != "2 days" {
		t.Fatalf("a moved email takes the slot's timing, got %v", timing)
	}

	// Into a new slot at the end of a stage; DE links to the moved EN.
	if response, result := call(admin, map[string]any{"action": "slot", "email_ids": []string{bDE, b}, "stage": "second"}); response.Code != http.StatusOK || result.SortOrder == nil || *result.SortOrder != 201 {
		t.Fatalf("move to a new slot returned %d: %s", response.Code, response.Body.String())
	}
	if master := column(bDE, "translation_of"); master == nil || *master != b {
		t.Fatalf("DE should translate the EN moved with it, got %v", master)
	}

	// Archive, then the archived email is no longer on the board.
	if response, result := call(admin, map[string]any{"action": "archive", "email_ids": []string{b, bDE}}); response.Code != http.StatusOK || result.Updated != 2 {
		t.Fatalf("archive returned %d: %s", response.Code, response.Body.String())
	}
	if archived := column(bDE, "archived_at"); archived == nil {
		t.Fatal("expected the email to be archived")
	}
	if response, _ := call(admin, map[string]any{"action": "owner", "email_ids": []string{a, b}, "owner_email": ""}); response.Code != http.StatusConflict {
		t.Fatalf("expected 409 when a selected email is gone, got %d", response.Code)
	}
	if owner := column(a, "owner_email"); owner == nil {
		t.Fatal("a refused bulk change must not touch the other emails")
	}
	if response, _ := call(admin, map[string]any{"action": "owner", "email_ids": []string{"not-a-uuid"}}); response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a bad id, got %d", response.Code)
	}
}
