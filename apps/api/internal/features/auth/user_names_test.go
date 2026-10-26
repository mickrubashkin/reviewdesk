package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestUpdateUserNameIsSuperAdminOnlyAndTidiesSpaces(t *testing.T) {
	dbpool := testDBPool(t)
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	admin := createTestUserWithRole(t, dbpool, "admin")
	target := createTestUserWithRole(t, dbpool, "reviewer")

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, nil)
	rename := func(actor AuthUser, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+target.ID+"/name", strings.NewReader(body))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, withAuthUser(request, actor))
		return response
	}

	if response := rename(admin, `{"name":"Nope"}`); response.Code != http.StatusForbidden {
		t.Fatalf("expected admin to be forbidden, got %d", response.Code)
	}
	if response := rename(superAdmin, `{"name":"`+strings.Repeat("x", maxUserNameLength+1)+`"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("expected a too long name to be rejected, got %d", response.Code)
	}

	response := rename(superAdmin, `{"name":"  Jane Doe "}`)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var updated UserAdminItem
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Jane Doe" || updated.Aliases == nil {
		t.Fatalf("expected tidied name and aliases list, got %q %v", updated.Name, updated.Aliases)
	}
	var stored string
	if err := dbpool.QueryRow(context.Background(), `SELECT name FROM users WHERE id = $1;`, target.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "Jane Doe" {
		t.Fatalf("expected the name to be stored, got %q", stored)
	}
}

func TestListUserNamesCoversAliases(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	reviewer := createTestUserWithRole(t, dbpool, "reviewer")
	named := createTestUserWithRole(t, dbpool, "admin")
	alias := "names-alias@acme.example"
	if _, err := dbpool.Exec(ctx, `UPDATE users SET name = 'Named Person' WHERE id = $1;`, named.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO user_email_aliases (email, user_id) VALUES ($1, $2) ON CONFLICT (email) DO UPDATE SET user_id = EXCLUDED.user_id;`, alias, named.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM user_email_aliases WHERE email = $1;`, alias) })

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, withAuthUser(httptest.NewRequest(http.MethodGet, "/api/users/names", nil), reviewer))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var names map[string]string
	if err := json.NewDecoder(response.Body).Decode(&names); err != nil {
		t.Fatal(err)
	}
	if names[strings.ToLower(named.Email)] != "Named Person" || names[alias] != "Named Person" {
		t.Fatalf("expected the address and alias to map to the name, got %v", names)
	}
	if _, ok := names[strings.ToLower(reviewer.Email)]; ok {
		t.Fatal("expected unnamed users to be left out")
	}
}
