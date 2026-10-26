package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMergeUsersMovesWorkAndSignsInThroughAlias(t *testing.T) {
	t.Setenv("AUTH_DEV_LOGIN_ENABLED", "true")
	t.Setenv("AUTH_ALLOWED_DOMAINS", "acme.example")
	dbpool := testDBPool(t)
	ctx := context.Background()
	admin := createTestUserWithRole(t, dbpool, "super_admin")

	keepEmail, dropEmail := "merge-keep@acme.example", "merge-drop@acme.example"
	deleteTestUserByEmail(t, dbpool, keepEmail)
	deleteTestUserByEmail(t, dbpool, dropEmail)
	t.Cleanup(func() {
		_, _ = dbpool.Exec(ctx, `DELETE FROM user_email_aliases WHERE email = $1;`, dropEmail)
	})

	var keepID, dropID string
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (email, role, languages) VALUES ($1, 'admin', '{de}') RETURNING id;`, keepEmail).Scan(&keepID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (email, role, languages, name) VALUES ($1, 'reviewer', '{es}', 'Merge Drop') RETURNING id;`, dropEmail).Scan(&dropID); err != nil {
		t.Fatal(err)
	}
	var emailID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, editable_fields, owner_email)
		VALUES ('merge-test-email', 'Merge Test', 'onboarding', 'test', 9998, 'es', 'new', '<html></html>', '<html></html>', '{}', $1)
		ON CONFLICT (slug) DO UPDATE SET owner_email = EXCLUDED.owner_email
		RETURNING id;
	`, dropEmail).Scan(&emailID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM emails WHERE id = $1;`, emailID) })

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+keepID+"/merge", strings.NewReader(`{"from_user_id":"`+dropID+`"}`))
	request = withAuthUser(request, admin)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected merge 200, got %d: %s", response.Code, response.Body.String())
	}

	var owner string
	if err := dbpool.QueryRow(ctx, `SELECT owner_email FROM emails WHERE id = $1;`, emailID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != keepEmail {
		t.Fatalf("expected the email to move to %s, got %s", keepEmail, owner)
	}
	var languages []string
	var role, name string
	if err := dbpool.QueryRow(ctx, `SELECT languages, role, name FROM users WHERE id = $1;`, keepID).Scan(&languages, &role, &name); err != nil {
		t.Fatal(err)
	}
	if strings.Join(languages, ",") != "de,es" || role != "admin" {
		t.Fatalf("expected languages de,es and role admin, got %v %s", languages, role)
	}
	if name != "Merge Drop" {
		t.Fatalf("expected the unnamed user to take the merged name, got %q", name)
	}
	var dropped int
	_ = dbpool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1;`, dropID).Scan(&dropped)
	if dropped != 0 {
		t.Fatal("expected the merged user to be deleted")
	}

	// Signing in with the old address opens the merged account.
	login := httptest.NewRequest(http.MethodPost, "/api/auth/dev-login", strings.NewReader(`{"email":"`+dropEmail+`"}`))
	loginResponse := httptest.NewRecorder()
	devLoginHandler(dbpool).ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("expected alias login 200, got %d", loginResponse.Code)
	}
	var users int
	_ = dbpool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email = $1;`, dropEmail).Scan(&users)
	if users != 0 {
		t.Fatal("expected alias login not to create a new user")
	}
	var sessionUser string
	if err := dbpool.QueryRow(ctx, `SELECT user_id FROM sessions ORDER BY created_at DESC LIMIT 1;`).Scan(&sessionUser); err != nil {
		t.Fatal(err)
	}
	if sessionUser != keepID {
		t.Fatalf("expected the session to belong to %s, got %s", keepID, sessionUser)
	}
}

func TestMergeUsersRejectsMergingYourself(t *testing.T) {
	dbpool := testDBPool(t)
	admin := createTestUserWithRole(t, dbpool, "super_admin")
	other := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+other.ID+"/merge", strings.NewReader(`{"from_user_id":"`+admin.ID+`"}`))
	request = withAuthUser(request, admin)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when merging your own account away, got %d", response.Code)
	}
}
