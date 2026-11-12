package emails

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

// A region in the reference's language with no email of its own in a slot
// sends the reference email from its managers' robots; a region in another
// language without the slot sends nothing there (#216).
func TestReferenceEmailRobotsIncludeRegionsWithoutTheSlot(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	board := createTestBoard(t, dbpool, "email-reference-robots-test", []string{"registered"})
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO board_regions (board_key, key, name, language, managers, reference, sort_order) VALUES
			($1, 'ref', 'Reference', 'en', '{Ann,Ben}', true, 0),
			($1, 'mena', 'MENA', 'en', '{Omar}', false, 1),
			($1, 'eu', 'EU', 'en', '{Eva,Pia}', false, 2),
			($1, 'pl', 'PL', 'pl', '{Pia}', false, 3);
	`, board); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE sequence = $1;`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM board_regions WHERE board_key = $1;`, board)
	})
	email := func(slug, region, language string, sortOrder int, archived bool) string {
		var id string
		if err := dbpool.QueryRow(ctx, `
			INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, adaptation_key, original_html, template_html, region_key, archived_at)
			VALUES ($1, $1, $2, 'registered', $3, $4, 'new', $5, '<p>x</p>', '<p>x</p>', $5, CASE WHEN $6 THEN now() END)
			RETURNING id;
		`, slug, board, sortOrder, language, region, archived).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	robots := func(emailID string) []string {
		tx, err := dbpool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		got, err := emailRobots(ctx, tx, emailID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// Slot 101: MENA and EU have their own emails, PL has none.
	ref101 := email("ref-robots-101", "ref", "en", 101, false)
	mena101 := email("mena-robots-101", "mena", "en", 101, false)
	email("eu-robots-101", "eu", "en", 101, false)
	// Slot 102: only the reference, and an archived EU email.
	ref102 := email("ref-robots-102", "ref", "en", 102, false)
	email("eu-robots-102-gone", "eu", "en", 102, true)

	if got := strings.Join(robots(ref101), ","); got != "Ann,Ben" {
		t.Errorf("reference email of a slot every English region has: %s", got)
	}
	if got := strings.Join(robots(mena101), ","); got != "Omar" {
		t.Errorf("MENA's own email: %s", got)
	}
	// Pia manages EU and PL: the EU robot counts once, PL's language differs.
	if got := strings.Join(robots(ref102), ","); got != "Ann,Ben,Omar,Eva,Pia" {
		t.Errorf("reference email of a slot only the reference has: %s", got)
	}
}
