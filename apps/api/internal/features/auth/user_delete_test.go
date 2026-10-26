package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestDeleteUserUnassignsWorkAndDropsAliases(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	admin := createTestUserWithRole(t, dbpool, "admin")
	target := createTestUserWithRole(t, dbpool, "reviewer")
	alias := "delete-alias@acme.example"
	if _, err := dbpool.Exec(ctx, `INSERT INTO user_email_aliases (email, user_id) VALUES ($1, $2) ON CONFLICT (email) DO UPDATE SET user_id = EXCLUDED.user_id;`, alias, target.ID); err != nil {
		t.Fatal(err)
	}
	var emailID string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html, editable_fields, owner_email, reviewer_email)
		VALUES ('delete-user-test-email', 'Delete Test', 'onboarding', 'test', 9997, 'es', 'new', '<html></html>', '<html></html>', '{}', $1, $2)
		ON CONFLICT (slug) DO UPDATE SET owner_email = EXCLUDED.owner_email, reviewer_email = EXCLUDED.reviewer_email
		RETURNING id;
	`, target.Email, alias).Scan(&emailID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM emails WHERE id = $1;`, emailID) })

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, nil)
	remove := func(actor AuthUser, userID string) int {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, withAuthUser(httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+userID, nil), actor))
		return response.Code
	}

	if code := remove(admin, target.ID); code != http.StatusForbidden {
		t.Fatalf("expected admin to be forbidden, got %d", code)
	}
	if code := remove(superAdmin, superAdmin.ID); code != http.StatusBadRequest {
		t.Fatalf("expected deleting yourself to be refused, got %d", code)
	}
	if code := remove(superAdmin, target.ID); code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", code)
	}

	var users, aliases int
	_ = dbpool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1;`, target.ID).Scan(&users)
	_ = dbpool.QueryRow(ctx, `SELECT count(*) FROM user_email_aliases WHERE email = $1;`, alias).Scan(&aliases)
	if users != 0 || aliases != 0 {
		t.Fatalf("expected the user and alias to be gone, got %d users, %d aliases", users, aliases)
	}
	var owner, reviewer *string
	if err := dbpool.QueryRow(ctx, `SELECT owner_email, reviewer_email FROM emails WHERE id = $1;`, emailID).Scan(&owner, &reviewer); err != nil {
		t.Fatal(err)
	}
	if owner != nil || reviewer != nil {
		t.Fatalf("expected the email to be unassigned, got %v %v", owner, reviewer)
	}
	if code := remove(superAdmin, target.ID); code != http.StatusNotFound {
		t.Fatalf("expected a second delete to be 404, got %d", code)
	}
}
