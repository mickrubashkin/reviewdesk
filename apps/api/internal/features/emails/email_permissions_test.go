package emails

import (
	"context"
	"testing"
)

func TestCanEditEmailLetsTranslatorsEditTheirLanguage(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	emailID := createTestEmail(t, dbpool) // language en
	admin := createTestUserWithRole(t, dbpool, "admin")
	translator := createTestUserWithRole(t, dbpool, "reviewer")

	check := func(user AuthUser) bool {
		allowed, err := CanEditEmail(ctx, dbpool, user, emailID)
		if err != nil {
			t.Fatal(err)
		}
		return allowed
	}
	if !check(admin) {
		t.Fatal("expected an admin to edit any email")
	}
	if check(translator) {
		t.Fatal("expected a reviewer without the language to be refused")
	}
	if _, err := dbpool.Exec(ctx, `UPDATE users SET languages = '{en}' WHERE id = $1;`, translator.ID); err != nil {
		t.Fatal(err)
	}
	if !check(translator) {
		t.Fatal("expected a reviewer who translates the language to edit the email")
	}
}
