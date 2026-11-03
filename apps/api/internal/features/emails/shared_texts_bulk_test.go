package emails

import (
	"context"
	"testing"
)

func TestApplySharedTextsChangeSetsRelinksAndDrops(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	createSharedTestText(t, dbpool, "en", "© 2026 Acme")
	createSharedTestText(t, dbpool, "de", "© 2026 Acme DE")

	linked := createSharedTestEmail(t, dbpool, "bulk-test-linked", 1, "en", "ready", "© 2026 Acme", true, "")
	own := createSharedTestEmail(t, dbpool, "bulk-test-own", 2, "en", "ready", "© Acme Inc", false, "")
	de := createSharedTestEmail(t, dbpool, "bulk-test-de", 1, "de", "draft", "© 2026 Acme DE", true, linked)

	versions := func(id string) int {
		var n int
		if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM email_versions WHERE email_id = $1;`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplySharedTextsChange(ctx, tx, superAdmin, SharedTextsChange{
		Set: []SharedTextValue{
			{Key: "test_shared_footer", Language: "de", Value: "© {{=date('Y')}} Acme DE"},
			{Key: "test_shared_footer", Language: "en", Value: "© {{=date('Y')}} Acme"},
		},
		Relink: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if result.Set != 2 {
		t.Errorf("expected 2 texts set, got %d", result.Set)
	}
	for _, id := range []string{linked, own} {
		if value, shared := emailFieldValue(t, dbpool, id, "test_shared_footer"); value != "© {{=date('Y')}} Acme" || !shared {
			t.Errorf("email %s: value %q shared %v", id, value, shared)
		}
		if n := versions(id); n != 2 {
			t.Errorf("email %s: expected one new version, got %d versions", id, n)
		}
		var status string
		if err := dbpool.QueryRow(ctx, `SELECT review_status FROM emails WHERE id = $1;`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "ready" {
			t.Errorf("email %s must stay ready, got %q", id, status)
		}
	}
	if value, _ := emailFieldValue(t, dbpool, de, "test_shared_footer"); value != "© {{=date('Y')}} Acme DE" {
		t.Errorf("DE email: got %q", value)
	}
	texts, err := listSharedTexts(ctx, dbpool)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range texts {
		if text.Key == "test_shared_footer" && text.ENChanged {
			t.Errorf("%s set after EN must not show EN changed", text.Language)
		}
	}

	tx, err = dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err = ApplySharedTextsChange(ctx, tx, superAdmin, SharedTextsChange{Drop: []string{"test_shared_footer"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if result.Dropped != 2 || result.Unlinked < 3 {
		t.Errorf("unexpected drop result: %+v", result)
	}
	if value, shared := emailFieldValue(t, dbpool, own, "test_shared_footer"); value != "© {{=date('Y')}} Acme" || shared {
		t.Errorf("a dropped text must leave the value and unlink it, got %q shared %v", value, shared)
	}
	if n := versions(own); n != 2 {
		t.Errorf("dropping changes no HTML and writes no version, got %d versions", n)
	}
}
