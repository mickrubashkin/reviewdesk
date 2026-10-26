package emails

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestArchiveHistoryAndRestore(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	emailID := createTestEmail(t, dbpool)

	var admin AuthUser
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO users (email, role) VALUES ('restore-test-admin@example.com', 'admin')
		ON CONFLICT (email) DO UPDATE SET role = 'admin' RETURNING id, email, role;
	`).Scan(&admin.ID, &admin.Email, &admin.Role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM users WHERE id = $1;`, admin.ID) })
	reviewer := createTestUser(t, dbpool)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	call := func(method, path string, user *AuthUser) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		if user != nil {
			request = withAuthUser(request, *user)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	list := func(query string) []EmailListItem {
		response := call(http.MethodGet, "/api/emails?board=onboarding"+query, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("list returned %d: %s", response.Code, response.Body.String())
		}
		var emails []EmailListItem
		if err := json.NewDecoder(response.Body).Decode(&emails); err != nil {
			t.Fatal(err)
		}
		return emails
	}
	find := func(emails []EmailListItem) *EmailListItem {
		for i := range emails {
			if emails[i].ID == emailID {
				return &emails[i]
			}
		}
		return nil
	}

	if response := call(http.MethodPatch, "/api/emails/"+emailID+"/archive", &admin); response.Code != http.StatusNoContent {
		t.Fatalf("archive returned %d: %s", response.Code, response.Body.String())
	}
	if find(list("")) != nil {
		t.Fatal("archived email must not be on the active board")
	}
	archived := find(list("&archived=only"))
	if archived == nil || archived.ArchivedAt == nil || archived.ArchivedByEmail == nil || *archived.ArchivedByEmail != admin.Email {
		t.Fatalf("expected the email in the board history with who archived it, got %+v", archived)
	}

	if response := call(http.MethodPatch, "/api/emails/"+emailID+"/restore", &reviewer); response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a reviewer, got %d", response.Code)
	}

	// Another active email took the place meanwhile: restoring must refuse.
	var blockerID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, editable_fields)
		VALUES ('restore-test-blocker', 'Blocker', 'onboarding', 'test', 9999, 'en', 'new', '<html></html>', '<html></html>', '{}'::jsonb)
		RETURNING id;
	`).Scan(&blockerID); err != nil {
		t.Fatal(err)
	}
	if response := call(http.MethodPatch, "/api/emails/"+emailID+"/restore", &admin); response.Code != http.StatusConflict {
		t.Fatalf("expected 409 while the place is taken, got %d: %s", response.Code, response.Body.String())
	}
	if _, err := dbpool.Exec(ctx, `DELETE FROM emails WHERE id = $1;`, blockerID); err != nil {
		t.Fatal(err)
	}

	if response := call(http.MethodPatch, "/api/emails/"+emailID+"/restore", &admin); response.Code != http.StatusNoContent {
		t.Fatalf("restore returned %d: %s", response.Code, response.Body.String())
	}
	var slug string
	var archivedAt *string
	if err := dbpool.QueryRow(ctx, `SELECT slug, archived_at::text FROM emails WHERE id = $1;`, emailID).Scan(&slug, &archivedAt); err != nil {
		t.Fatal(err)
	}
	if slug != "comment-test-email" || archivedAt != nil {
		t.Fatalf("expected the original slug back and no archive mark, got %q %v", slug, archivedAt)
	}
	if find(list("")) == nil || find(list("&archived=only")) != nil {
		t.Fatal("expected the restored email back on the active board only")
	}
	if response := call(http.MethodPatch, "/api/emails/"+emailID+"/restore", &admin); response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when restoring an active email, got %d", response.Code)
	}

	var events int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM email_events WHERE email_id = $1 AND action = 'email_restored';`, emailID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("expected one restore event, got %d", events)
	}
}

func TestRestoredEmailSlug(t *testing.T) {
	id := "1b4e28ba-2fa1-11d2-883f-0016d3cca427"
	if got := restoredEmailSlug(archivedEmailSlug("welcome-en", id), id); got != "welcome-en" {
		t.Fatalf("unexpected slug %q", got)
	}
}
