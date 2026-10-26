package boards

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestBoardRegions(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	admin := createTestUserWithRole(t, dbpool, "admin")
	reviewer := createTestUserWithRole(t, dbpool, "reviewer")
	boardKey := createTestBoard(t, dbpool, "regions-board", []string{"registered"})

	router := chi.NewRouter()
	RegisterBoardRoutes(router, dbpool)
	put := func(user AuthUser, body string) *httptest.ResponseRecorder {
		request := withAuthUser(httptest.NewRequest(http.MethodPut, "/api/boards/"+boardKey+"/regions", bytes.NewReader([]byte(body))), user)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	regions := `{"regions": [
		{"name": "Reference", "language": "EN", "managers": ["Alice", " Max ", ""], "reference": true},
		{"name": "EU + SSA", "language": "en", "managers": ["Jane"]},
		{"name": "BR", "language": "br", "managers": ["Pedro", "Diego"]}
	]}`
	if response := put(reviewer, regions); response.Code != http.StatusForbidden {
		t.Fatalf("only managers of the board edit regions, got %d", response.Code)
	}
	for body, why := range map[string]string{
		`{"regions": [{"name": "A", "language": "en", "reference": true}, {"name": "B", "language": "en", "reference": true}]}`: "one reference",
		`{"regions": [{"name": "A", "language": "en"}, {"name": "a", "language": "de"}]}`:                                      "twice",
		`{"regions": [{"name": "A", "language": ""}]}`:                                                                         "language",
	} {
		if response := put(admin, body); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), why) {
			t.Fatalf("expected a refusal about %q, got %d %s", why, response.Code, response.Body.String())
		}
	}

	response := put(admin, regions)
	if response.Code != http.StatusOK {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	var saved struct{ Regions []Region }
	if err := json.NewDecoder(response.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Regions) != 3 || saved.Regions[1].Key != "eu-ssa" || saved.Regions[0].Language != "en" ||
		strings.Join(saved.Regions[0].Managers, ",") != "Alice,Max" || !saved.Regions[0].Reference {
		t.Fatalf("unexpected regions: %+v", saved.Regions)
	}

	// An email in a region loses it when the region goes.
	var emailID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, region_key)
		VALUES ('regions-board-email', 'Welcome', $1, 'registered', 101, 'br', 'new', '<p>x</p>', '<p>x</p>', 'br')
		RETURNING id;
	`, boardKey).Scan(&emailID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, emailID) })

	// The reference moves to another region in one save; BR goes.
	response = put(admin, `{"regions": [
		{"key": "reference", "name": "Reference", "language": "en"},
		{"key": "eu-ssa", "name": "EU + SSA", "language": "en", "reference": true}
	]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("second save: %d %s", response.Code, response.Body.String())
	}
	var region *string
	if err := dbpool.QueryRow(ctx, `SELECT region_key FROM emails WHERE id = $1;`, emailID).Scan(&region); err != nil {
		t.Fatal(err)
	}
	if region != nil {
		t.Fatalf("the email of a deleted region has none, got %q", *region)
	}
	listed, err := ListRegions(ctx, dbpool, boardKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Reference || !listed[1].Reference {
		t.Fatalf("the reference moved: %+v", listed)
	}
}
