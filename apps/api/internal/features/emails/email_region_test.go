package emails

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/boards"
)

func TestEmailRegion(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	admin := createTestUserWithRole(t, dbpool, "admin")
	board := createTestBoard(t, dbpool, "email-region-test", []string{"registered"})
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO board_regions (board_key, key, name, language) VALUES ($1, 'mena', 'MENA', 'en');
	`, board); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE email_id IN (SELECT id FROM emails WHERE sequence = $1);`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE sequence = $1;`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM board_regions WHERE board_key = $1;`, board)
	})
	var id string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, original_html, template_html)
		VALUES ('email-region-test-en', 'Welcome', $1, 'registered', 101, 'en', 'new', '<p>x</p>', '<p>x</p>')
		RETURNING id;
	`, board).Scan(&id); err != nil {
		t.Fatal(err)
	}

	put := func(region string) int {
		return blockRequest(t, dbpool, admin, http.MethodPut, "/api/emails/"+id+"/region", updateEmailRegionRequest{RegionKey: region}).Code
	}
	if code := put("nope"); code != http.StatusBadRequest {
		t.Fatalf("a region of another board or none must be refused, got %d", code)
	}
	if code := put("mena"); code != http.StatusNoContent {
		t.Fatalf("set: %d", code)
	}
	response := blockRequest(t, dbpool, admin, http.MethodGet, "/api/emails/"+id, nil)
	var email EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&email); err != nil {
		t.Fatal(err)
	}
	if email.RegionKey == nil || *email.RegionKey != "mena" {
		t.Fatalf("the email is in MENA: %v", email.RegionKey)
	}
	var events int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM email_events WHERE email_id = $1 AND changes ? 'region';`, id).Scan(&events); err != nil || events != 1 {
		t.Fatalf("the change is in the history: %d %v", events, err)
	}
	if code := put(""); code != http.StatusNoContent {
		t.Fatalf("clear: %d", code)
	}
}

func TestEmailRegionSetsAdaptation(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	admin := createTestUserWithRole(t, dbpool, "admin")
	board := createTestBoard(t, dbpool, "region-adaptation-test", []string{"registered"})
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO board_regions (board_key, key, name, language, reference, sort_order) VALUES
			($1, 'reference', 'Reference', 'en', true, 0),
			($1, 'mena', 'MENA', 'en', false, 1),
			($1, 'pl', 'PL', 'pl', false, 2);
	`, board); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE email_id IN (SELECT id FROM emails WHERE sequence = $1);`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE sequence = $1;`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM board_regions WHERE board_key = $1;`, board)
	})
	create := func(slug, language, adaptation string) string {
		var id string
		if err := dbpool.QueryRow(ctx, `
			INSERT INTO emails (slug, title, sequence, stage, sort_order, language, variant, adaptation_key, adaptation_label, original_html, template_html)
			VALUES ($1, $1, $2, 'registered', 105, $3, 'new', $4, $4, '<p>x</p>', '<p>x</p>') RETURNING id;
		`, slug, board, language, adaptation).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	adaptation := func(id string) string {
		var key, label string
		if err := dbpool.QueryRow(ctx, `SELECT adaptation_key, adaptation_label FROM emails WHERE id = $1;`, id).Scan(&key, &label); err != nil {
			t.Fatal(err)
		}
		return key + "/" + label
	}
	put := func(id, region string) int {
		return blockRequest(t, dbpool, admin, http.MethodPut, "/api/emails/"+id+"/region", updateEmailRegionRequest{RegionKey: region}).Code
	}

	mena := create("region-adaptation-mena", "en", "mena-new-onboarding")
	other := create("region-adaptation-other", "en", "default")
	pl := create("region-adaptation-pl", "pl", "pl-new-onboarding")
	if code := put(mena, "mena"); code != http.StatusNoContent || adaptation(mena) != "mena/MENA" {
		t.Fatalf("an email of another region is its adaptation: %d %s", code, adaptation(mena))
	}
	if code := put(pl, "pl"); code != http.StatusNoContent || adaptation(pl) != "default/Default" {
		t.Fatalf("the main region of a language is the default version: %d %s", code, adaptation(pl))
	}
	if code := put(other, "mena"); code != http.StatusConflict {
		t.Fatalf("two EN emails of MENA in one slot must be refused, got %d", code)
	}
	if code := put(other, "reference"); code != http.StatusNoContent || adaptation(other) != "default/Default" {
		t.Fatalf("the reference is the default version: %d %s", code, adaptation(other))
	}

	// Duplicate as: a copy put straight into a region takes its version and
	// adapts the slot's reference email.
	region := "eu"
	if _, err := dbpool.Exec(ctx, `INSERT INTO board_regions (board_key, key, name, language, sort_order) VALUES ($1, 'eu', 'EU', 'en', 3);`, board); err != nil {
		t.Fatal(err)
	}
	if code := blockRequest(t, dbpool, admin, http.MethodPost, "/api/emails/"+other+"/duplicate-as", duplicateEmailAsRequest{RegionKey: &region}).Code; code != http.StatusCreated {
		t.Fatalf("duplicate as EU: %d", code)
	}
	var copyKey, copyRegion string
	var copyBasedOn *string
	if err := dbpool.QueryRow(ctx, `
		SELECT adaptation_key, coalesce(region_key, ''), based_on::text FROM emails WHERE sequence = $1 AND id NOT IN ($2, $3, $4);
	`, board, mena, other, pl).Scan(&copyKey, &copyRegion, &copyBasedOn); err != nil {
		t.Fatal(err)
	}
	if copyKey != "eu" || copyRegion != "eu" || copyBasedOn == nil || *copyBasedOn != other {
		t.Fatalf("the copy is EU's version based on the reference: %s %s %v", copyKey, copyRegion, copyBasedOn)
	}
	region = "mena"
	if code := blockRequest(t, dbpool, admin, http.MethodPost, "/api/emails/"+other+"/duplicate-as", duplicateEmailAsRequest{RegionKey: &region}).Code; code != http.StatusConflict {
		t.Fatalf("MENA already has its EN email in the slot, got %d", code)
	}

	// A new email can go straight into a region too.
	region = "mena"
	response := blockRequest(t, dbpool, admin, http.MethodPost, "/api/emails", createEmailRequest{
		Sequence: &board, Title: "New in MENA", Stage: "registered", SortOrder: 106, Language: "en",
		RegionKey: &region, OriginalHTML: `<p data-review-block="intro">Hi</p>`,
	})
	var created EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil || response.Code != http.StatusCreated {
		t.Fatalf("create in MENA: %d %v", response.Code, err)
	}
	if created.AdaptationKey != "mena" || created.RegionKey == nil || *created.RegionKey != "mena" {
		t.Fatalf("the new email is MENA's version: %s %v", created.AdaptationKey, created.RegionKey)
	}

	// Renaming a region renames its emails' version.
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE board_regions SET name = 'MENA + TR' WHERE board_key = $1 AND key = 'mena';`, board); err != nil {
		t.Fatal(err)
	}
	changes, err := boards.SyncRegionAdaptations(ctx, tx, board, "")
	if err != nil || len(changes) != 2 || changes[0].After != "MENA + TR" || changes[1].After != "MENA + TR" {
		t.Fatalf("rename: %+v %v", changes, err)
	}
}
