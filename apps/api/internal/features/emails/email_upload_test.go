package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMarkEmailUploadedChecksCopiedVersion(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	if _, err := dbpool.Exec(context.Background(), `UPDATE emails SET review_status = 'ready' WHERE id = $1;`, emailID); err != nil {
		t.Fatal(err)
	}
	insertTestEmailVersion(t, dbpool, emailID, superAdmin.Email, "manual")
	insertTestEmailVersion(t, dbpool, emailID, superAdmin.Email, "manual")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	mark := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPatch, "/api/emails/"+emailID+"/uploaded", bytes.NewReader([]byte(body)))
		request = withAuthUser(request, superAdmin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	uploadedVersion := func() *int {
		var value *int
		if err := dbpool.QueryRow(context.Background(), `SELECT uploaded_version FROM email_upload_summary WHERE email_id = $1;`, emailID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	if response := mark(`{"uploaded":true,"version":1}`); response.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a copy of an older version, got %d", response.Code)
	}
	if uploadedVersion() != nil {
		t.Fatalf("expected no upload mark after the conflict, got %d", *uploadedVersion())
	}

	if response := mark(`{"uploaded":true,"version":2}`); response.Code != http.StatusOK {
		t.Fatalf("expected 200 for the copied latest version, got %d: %s", response.Code, response.Body.String())
	}
	if version := uploadedVersion(); version == nil || *version != 2 {
		t.Fatalf("expected uploaded version 2, got %v", version)
	}

	if response := mark(`{"uploaded":true}`); response.Code != http.StatusOK {
		t.Fatalf("expected 200 without a version, got %d: %s", response.Code, response.Body.String())
	}
}

func TestMarkEmailUploadedPerRobot(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	board := createTestBoard(t, dbpool, "email-upload-robots-test", []string{"registered"})
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO board_regions (board_key, key, name, language, managers) VALUES ($1, 'br', 'BR', 'br', '{Pedro,Diego}');
	`, board); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE email_id IN (SELECT id FROM emails WHERE sequence = $1);`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE sequence = $1;`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM board_regions WHERE board_key = $1;`, board)
	})
	var emailID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, region_key, review_status)
		VALUES ('email-upload-robots-test', 'Welcome', $1, 'registered', 101, 'br', 'new', '<p>x</p>', '<p>x</p>', 'br', 'ready')
		RETURNING id;
	`, board).Scan(&emailID); err != nil {
		t.Fatal(err)
	}
	insertTestEmailVersion(t, dbpool, emailID, superAdmin.Email, "manual")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	mark := func(body string) (int, markEmailUploadedResponse) {
		request := httptest.NewRequest(http.MethodPatch, "/api/emails/"+emailID+"/uploaded", bytes.NewReader([]byte(body)))
		request = withAuthUser(request, superAdmin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var decoded markEmailUploadedResponse
		if response.Code == http.StatusOK {
			if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
		}
		return response.Code, decoded
	}
	versions := func(response markEmailUploadedResponse) map[string]int {
		got := map[string]int{}
		for _, robot := range response.UploadRobots {
			if robot.Version != nil {
				got[robot.Robot] = *robot.Version
			}
		}
		return got
	}

	if code, _ := mark(`{"uploaded":true,"robots":["Alice"]}`); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a robot outside the region, got %d", code)
	}

	code, response := mark(`{"uploaded":true,"version":1,"robots":["Diego"]}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if len(response.UploadRobots) != 2 || response.UploadRobots[0].Robot != "Pedro" || response.UploadRobots[1].Robot != "Diego" {
		t.Fatalf("expected both robots in the region's order, got %+v", response.UploadRobots)
	}
	if got := versions(response); len(got) != 1 || got["Diego"] != 1 {
		t.Fatalf("expected only Diego's robot marked, got %v", got)
	}

	// Without robots the mark goes into every robot of the region.
	insertTestEmailVersion(t, dbpool, emailID, superAdmin.Email, "manual")
	code, response = mark(`{"uploaded":true,"version":2}`)
	if got := versions(response); code != http.StatusOK || got["Pedro"] != 2 || got["Diego"] != 2 {
		t.Fatalf("expected both robots on version 2, got %d %v", code, got)
	}

	code, response = mark(`{"uploaded":false,"robots":["Pedro"]}`)
	if got := versions(response); code != http.StatusOK || len(got) != 1 || got["Diego"] != 2 {
		t.Fatalf("expected only Pedro's mark removed, got %d %v", code, got)
	}
	if response.UploadedVersion == nil || *response.UploadedVersion != 2 {
		t.Fatalf("expected uploaded version 2 from Diego's robot, got %v", response.UploadedVersion)
	}
	var robots string
	if err := dbpool.QueryRow(ctx, `
		SELECT metadata->>'robots' FROM email_events WHERE email_id = $1 ORDER BY created_at DESC LIMIT 1;
	`, emailID).Scan(&robots); err != nil || robots != "Pedro" {
		t.Fatalf("expected the event to name Pedro, got %q (%v)", robots, err)
	}
}
