package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/go-chi/chi/v5"
)

func intPointer(value int) *int { return &value }

func TestPlanSlotReorder(t *testing.T) {
	orders := []int{301, 302, 303, 304}
	from := func(order int) slotMove { return slotMove{FromStage: "approved", FromSortOrder: order} }
	move := func(fromOrder, toOrder int) slotMove { return slotMove{"approved", fromOrder, "approved", toOrder} }

	cases := []struct {
		name   string
		from   int
		before *int
		want   []slotMove
	}{
		{"up", 304, intPointer(302), []slotMove{move(304, 302), move(302, 303), move(303, 304)}},
		{"down to the end", 301, nil, []slotMove{move(302, 301), move(303, 302), move(304, 303), move(301, 304)}},
		{"in place", 302, intPointer(303), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := planSlotReorder(orders, from(tc.from), tc.before)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}

	if _, err := planSlotReorder(orders, from(301), intPointer(999)); err != errSlotStale {
		t.Fatalf("expected errSlotStale for an unknown target, got %v", err)
	}
}

func TestPlanSlotInsert(t *testing.T) {
	from := slotMove{FromStage: "registered", FromSortOrder: 105}
	cases := []struct {
		name   string
		orders []int
		before *int
		want   []slotMove
	}{
		{"before the first, shift until a gap", []int{301, 302, 304}, intPointer(301), []slotMove{
			{"registered", 105, "approved", 301}, {"approved", 301, "approved", 302}, {"approved", 302, "approved", 303},
		}},
		{"at the end", []int{301, 302}, nil, []slotMove{{"registered", 105, "approved", 303}}},
		{"empty stage", nil, nil, []slotMove{{"registered", 105, "approved", 301}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := planSlotInsert(tc.orders, from, "approved", tc.before, 301)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSlotEditAndMove(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	board := createTestBoard(t, dbpool, "slot-edit-test", []string{"first", "second"})

	insert := func(stage string, sortOrder int, language string, archived bool) string {
		t.Helper()
		var id string
		if err := dbpool.QueryRow(ctx, `
			INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, editable_fields, send_timing, archived_at)
			VALUES ($1, 'Slot ' || $3::int, $2, $4, $3::int, $5, 'v1', '<html></html>', '<html></html>', '{}'::jsonb, 'immediately', CASE WHEN $6 THEN now() END)
			RETURNING id;
		`, fmt.Sprintf("slot-edit-%s-%d-%s-%t", stage, sortOrder, language, archived), board, sortOrder, stage, language, archived).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = dbpool.Exec(ctx, `DELETE FROM email_events WHERE email_id = $1;`, id)
			_, _ = dbpool.Exec(ctx, `DELETE FROM emails WHERE id = $1;`, id)
		})
		return id
	}
	a := insert("first", 101, "en", false)
	aDE := insert("first", 101, "de", false)
	aOld := insert("first", 101, "de", true)
	b := insert("first", 102, "en", false)
	c := insert("first", 103, "en", false)

	var admin AuthUser
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO users (email, role) VALUES ('slot-edit-test-admin@example.com', 'admin')
		ON CONFLICT (email) DO UPDATE SET role = 'admin' RETURNING id, email, role;
	`).Scan(&admin.ID, &admin.Email, &admin.Role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM users WHERE id = $1;`, admin.ID) })
	reviewer := createTestUser(t, dbpool)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	call := func(method, path string, user AuthUser, body any) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		request := withAuthUser(httptest.NewRequest(method, path, bytes.NewReader(payload)), user)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	place := func(id string) (stage string, sortOrder int, title string, timing *string) {
		t.Helper()
		if err := dbpool.QueryRow(ctx, `SELECT stage, sort_order, title, send_timing FROM emails WHERE id = $1;`, id).Scan(&stage, &sortOrder, &title, &timing); err != nil {
			t.Fatal(err)
		}
		return
	}

	if response := call(http.MethodPatch, "/api/slots", reviewer, map[string]any{"sequence": board, "stage": "first", "sort_order": 101, "title": "X"}); response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a reviewer, got %d", response.Code)
	}

	// Edit the whole slot: name and timing for every language, then move it.
	response := call(http.MethodPatch, "/api/slots", admin, map[string]any{
		"sequence": board, "stage": "first", "sort_order": 101,
		"title": "Welcome", "send_timing": "1 day after previous email", "send_condition": "",
		"to_stage": "second",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("edit returned %d: %s", response.Code, response.Body.String())
	}
	for _, id := range []string{a, aDE} {
		stage, sortOrder, title, timing := place(id)
		if stage != "second" || sortOrder != 201 || title != "Welcome" || timing == nil || *timing != "1 day after previous email" {
			t.Fatalf("email %s: got %s #%d %q %v", id, stage, sortOrder, title, timing)
		}
	}
	if stage, sortOrder, title, _ := place(aOld); stage != "second" || sortOrder != 201 || title == "Welcome" {
		t.Fatalf("archived email must follow its slot without being edited, got %s #%d %q", stage, sortOrder, title)
	}
	var events int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM email_events WHERE action = 'slot_updated' AND email_id IN ($1, $2);`, a, aDE).Scan(&events); err != nil || events != 2 {
		t.Fatalf("expected one slot_updated event per language, got %d (%v)", events, err)
	}

	// A taken position is refused.
	if response := call(http.MethodPatch, "/api/slots", admin, map[string]any{"sequence": board, "stage": "first", "sort_order": 102, "to_sort_order": 103}); response.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a taken position, got %d", response.Code)
	}

	// Drag 103 above 102 within the stage: they swap.
	if response := call(http.MethodPost, "/api/slots/move", admin, map[string]any{"sequence": board, "stage": "first", "sort_order": 103, "to_stage": "first", "before_sort_order": 102}); response.Code != http.StatusOK {
		t.Fatalf("move returned %d: %s", response.Code, response.Body.String())
	}
	if _, sortOrder, _, _ := place(c); sortOrder != 102 {
		t.Fatalf("dragged slot should take #102, got #%d", sortOrder)
	}
	if _, sortOrder, _, _ := place(b); sortOrder != 103 {
		t.Fatalf("the other slot should shift to #103, got #%d", sortOrder)
	}

	// Drag it into the other stage before the slot there.
	if response := call(http.MethodPost, "/api/slots/move", admin, map[string]any{"sequence": board, "stage": "first", "sort_order": 102, "to_stage": "second", "before_sort_order": 201}); response.Code != http.StatusOK {
		t.Fatalf("move returned %d: %s", response.Code, response.Body.String())
	}
	if stage, sortOrder, _, _ := place(c); stage != "second" || sortOrder != 201 {
		t.Fatalf("dragged slot should be second #201, got %s #%d", stage, sortOrder)
	}
	if stage, sortOrder, _, _ := place(a); stage != "second" || sortOrder != 202 {
		t.Fatalf("Welcome should shift to second #202, got %s #%d", stage, sortOrder)
	}

	if response := call(http.MethodPost, "/api/slots/move", admin, map[string]any{"sequence": board, "stage": "first", "sort_order": 103, "to_stage": "second", "before_sort_order": 999}); response.Code != http.StatusConflict {
		t.Fatalf("expected 409 when the target slot is gone, got %d", response.Code)
	}
}
