package emails

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func createReferenceTestEmail(t *testing.T, dbpool *pgxpool.Pool, board, slug, language, adaptation, region, subject, fields, layout, timing string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO emails (slug, title, subject, sequence, stage, sort_order, language, variant, adaptation_key,
			region_key, send_timing, original_html, template_html, editable_fields, layout)
		VALUES ($1, $1, $2, $3, 'registered', 101, $4, 'new', $5, nullif($6, ''), $7, '<p>x</p>', '<p>x</p>',
			$8::jsonb, nullif($9, '')::jsonb)
		RETURNING id;
	`, slug, subject, board, language, adaptation, region, timing, fields, layout).Scan(&id); err != nil {
		t.Fatalf("create %s: %v", slug, err)
	}
	if err := insertEmailVersion(ctx, dbpool, emailVersionSnapshot{
		EmailID: id, CreatedByEmail: "test", Source: "initial", Title: slug, Subject: &subject,
		OriginalHTML: "<p>x</p>", TemplateHTML: "<p>x</p>", EditableFieldsJSON: []byte(fields),
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestEmailBasedOnReference(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	admin := createTestUserWithRole(t, dbpool, "admin")
	board := createTestBoard(t, dbpool, "email-based-on-test", []string{"registered"})
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO board_regions (board_key, key, name, language, reference) VALUES
			($1, 'reference', 'Reference', 'en', true),
			($1, 'mena', 'MENA', 'en', false),
			($1, 'br', 'BR', 'pt-br', false);
	`, board); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_events WHERE email_id IN (SELECT id FROM emails WHERE sequence = $1);`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE sequence = $1;`, board)
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM board_regions WHERE board_key = $1;`, board)
	})

	const fields = `{"intro": {"type": "text", "value": "Hello", "order": 1}, "cta_url": {"type": "url", "value": "https://example.com", "order": 2}}`
	const menaFields = `{"intro": {"type": "text", "value": "Hello", "order": 1}, "cta_url": {"type": "url", "value": "https://example.ae", "order": 2}}`
	reference := createReferenceTestEmail(t, dbpool, board, "based-on-reference", "en", "default", "reference", "Welcome", fields,
		`[{"block": "hero"}, {"block": "text"}]`, "1 day")
	mena := createReferenceTestEmail(t, dbpool, board, "based-on-mena", "en", "mena", "", "Welcome", menaFields,
		`[{"block": "hero"}, {"block": "text"}, {"block": "button"}]`, "1 day")
	br := createReferenceTestEmail(t, dbpool, board, "based-on-br", "pt-br", "default", "", "Bem-vindo", fields,
		`[{"block": "hero"}]`, "2 days")

	put := func(id, region string) {
		t.Helper()
		if code := blockRequest(t, dbpool, admin, http.MethodPut, "/api/emails/"+id+"/region", updateEmailRegionRequest{RegionKey: region}).Code; code != http.StatusNoContent {
			t.Fatalf("region %s: %d", region, code)
		}
	}
	status := func(id string) (basedOnStatus, int) {
		t.Helper()
		response := blockRequest(t, dbpool, admin, http.MethodGet, "/api/emails/"+id+"/based-on", nil)
		var s basedOnStatus
		if response.Code == http.StatusOK {
			if err := json.NewDecoder(response.Body).Decode(&s); err != nil {
				t.Fatal(err)
			}
		}
		return s, response.Code
	}

	// Putting an email in a region links it to the slot's reference email.
	put(mena, "mena")
	put(br, "br")
	menaStatus, code := status(mena)
	if code != http.StatusOK || menaStatus.ReferenceID != reference || menaStatus.Stale {
		t.Fatalf("MENA is based on the reference and up to date: %d %+v", code, menaStatus)
	}
	if len(menaStatus.FieldDifferences) != 1 || menaStatus.FieldDifferences[0].Key != "cta_url" || menaStatus.FieldDifferences[0].Own != "https://example.ae" {
		t.Fatalf("MENA differs in its link: %+v", menaStatus.FieldDifferences)
	}
	if len(menaStatus.BlockDifferences) != 1 || menaStatus.BlockDifferences[0].Action != "added" || menaStatus.BlockDifferences[0].Block != "button" {
		t.Fatalf("MENA has its own button: %+v", menaStatus.BlockDifferences)
	}
	if menaStatus.TimingDifference != nil {
		t.Fatalf("MENA has the reference's timing: %+v", menaStatus.TimingDifference)
	}
	brStatus, _ := status(br)
	if len(brStatus.FieldDifferences) != 0 {
		t.Fatalf("another language is not compared by fields: %+v", brStatus.FieldDifferences)
	}
	if len(brStatus.BlockDifferences) != 1 || brStatus.BlockDifferences[0].Action != "removed" || brStatus.BlockDifferences[0].Block != "text" {
		t.Fatalf("BR has no text block: %+v", brStatus.BlockDifferences)
	}
	if brStatus.TimingDifference == nil || brStatus.TimingDifference.Own != "2 days" {
		t.Fatalf("BR has its own timing: %+v", brStatus.TimingDifference)
	}

	// The board's map of regions gets the same, for all emails at once.
	response := blockRequest(t, dbpool, admin, http.MethodGet, "/api/reference-differences?board="+board, nil)
	var differences struct {
		Emails map[string]referenceDifference `json:"emails"`
	}
	if err := json.NewDecoder(response.Body).Decode(&differences); err != nil {
		t.Fatal(err)
	}
	if d := differences.Emails[mena]; d.Fields != 1 || len(d.Blocks) != 1 || d.Timing != nil {
		t.Fatalf("MENA on the map: %+v", d)
	}
	if d := differences.Emails[br]; d.Fields != 0 || len(d.Blocks) != 1 || d.Timing == nil {
		t.Fatalf("BR on the map: %+v", d)
	}
	if _, ok := differences.Emails[reference]; ok || len(differences.Emails) != 2 {
		t.Fatalf("only emails based on the reference are on the map: %+v", differences.Emails)
	}

	// The reference changes: its regions see it in the list and the status.
	newSubject := "Welcome aboard"
	if err := insertEmailVersion(ctx, dbpool, emailVersionSnapshot{
		EmailID: reference, CreatedByEmail: "test", Source: "manual", Title: "based-on-reference", Subject: &newSubject,
		OriginalHTML: "<p>x</p>", TemplateHTML: "<p>x</p>", EditableFieldsJSON: []byte(fields),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `UPDATE emails SET subject = $2 WHERE id = $1;`, reference, newSubject); err != nil {
		t.Fatal(err)
	}
	menaStatus, _ = status(mena)
	if !menaStatus.Stale || len(menaStatus.Changes) != 1 || menaStatus.Changes[0].After != newSubject {
		t.Fatalf("the reference changed since: %+v", menaStatus)
	}
	var listStale bool
	if err := listEmail(t, dbpool, admin, board, mena, &listStale); err != nil {
		t.Fatal(err)
	}
	if !listStale {
		t.Fatal("the board shows that the reference changed")
	}
	if code := blockRequest(t, dbpool, admin, http.MethodPost, "/api/emails/"+mena+"/based-on/confirm", nil).Code; code != http.StatusOK {
		t.Fatalf("confirm: %d", code)
	}
	if menaStatus, _ = status(mena); menaStatus.Stale || menaStatus.BasedOnVersion != 2 {
		t.Fatalf("after confirming the email matches version 2: %+v", menaStatus)
	}

	// By hand: a reference email only, and unlinking leaves the region's own email.
	if code := blockRequest(t, dbpool, admin, http.MethodPatch, "/api/emails/"+br+"/based-on", basedOnLinkRequest{ReferenceID: &mena}).Code; code != http.StatusBadRequest {
		t.Fatalf("an email outside the reference region cannot be a reference, got %d", code)
	}
	if code := blockRequest(t, dbpool, admin, http.MethodPatch, "/api/emails/"+br+"/based-on", basedOnLinkRequest{}).Code; code != http.StatusNoContent {
		t.Fatalf("unlink: %d", code)
	}
	if _, code := status(br); code != http.StatusNoContent {
		t.Fatalf("BR is its own email now, got %d", code)
	}
	if code := blockRequest(t, dbpool, admin, http.MethodPatch, "/api/emails/"+reference+"/based-on", basedOnLinkRequest{ReferenceID: &reference}).Code; code != http.StatusBadRequest {
		t.Fatalf("an email of the reference region is based on nothing, got %d", code)
	}

	// Back in the reference region or in none, the email adapts nothing.
	put(mena, "")
	if _, code := status(mena); code != http.StatusNoContent {
		t.Fatalf("without a region the link is gone, got %d", code)
	}
}

// listEmail reads based_on_stale of one email from the board's list.
func listEmail(t *testing.T, dbpool *pgxpool.Pool, user AuthUser, board, id string, stale *bool) error {
	t.Helper()
	response := blockRequest(t, dbpool, user, http.MethodGet, "/api/emails?board="+board, nil)
	var emails []EmailListItem
	if err := json.NewDecoder(response.Body).Decode(&emails); err != nil {
		return err
	}
	for _, email := range emails {
		if email.ID == id {
			*stale = email.BasedOnStale
			return nil
		}
	}
	t.Fatalf("email %s is not in the list", id)
	return nil
}
