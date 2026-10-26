package emails

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

func TestEmailChecks(t *testing.T) {
	dbpool := testDBPool(t)
	resetEmailBlocks(t, dbpool)
	admin := createTestUserWithRole(t, dbpool, "admin")
	id := createLayoutTestEmail(t, dbpool, "checks-test-en", "en", "Hello Alice", "")

	request := func(method, path string, body any) (int, string) {
		response := blockRequest(t, dbpool, admin, method, "/api/emails/"+id+path, body)
		return response.Code, strings.TrimSpace(response.Body.String())
	}
	putLayout := func(layout emailblocks.Layout) (int, string) {
		return request(http.MethodPut, "/layout", updateEmailLayoutRequest{Layout: layout})
	}
	saveFields := func(update updateEmailEditableFieldsRequest) (int, string) {
		if update.EditableFields == nil {
			update.EditableFields = emailedit.EditableFields{}
		}
		return request(http.MethodPatch, "/editable-fields", update)
	}
	markReady := func() (int, string) {
		return request(http.MethodPatch, "/review-status", updateEmailReviewStatusRequest{ReviewStatus: "ready"})
	}
	checks := func() []emailblocks.Issue {
		code, body := request(http.MethodGet, "", nil)
		if code != http.StatusOK {
			t.Fatalf("get: %d %s", code, body)
		}
		var email EmailDetail
		if err := json.Unmarshal([]byte(body), &email); err != nil {
			t.Fatal(err)
		}
		return email.Checks
	}

	// Switching to blocks without a button brings in an error.
	if code, body := putLayout(emailblocks.Layout{{Block: "intro_body"}, {Block: "footer"}}); code != http.StatusBadRequest ||
		!strings.Contains(body, "no button") {
		t.Fatalf("a layout without a button must be refused: %d %s", code, body)
	}
	layout := emailblocks.Layout{{Block: "intro_body"}, {Block: "primary_cta"}, {Block: "footer"}}
	if code, body := putLayout(layout); code != http.StatusNoContent {
		t.Fatalf("switch to blocks: %d %s", code, body)
	}
	if code, body := putLayout(append(layout, emailblocks.LayoutItem{Block: "primary_cta", Suffix: "2"})); code != http.StatusBadRequest ||
		!strings.Contains(body, "2 buttons") {
		t.Fatalf("a second button must be refused: %d %s", code, body)
	}
	if issues := checks(); len(issues) != 0 {
		t.Fatalf("the email has no issues yet: %+v", issues)
	}

	// Emptying a required field is refused; a long subject is a warning:
	// it saves, but the email cannot be marked ready.
	if code, body := saveFields(updateEmailEditableFieldsRequest{EditableFields: emailedit.EditableFields{
		"primary_cta_text": {Type: "text", Value: " "},
	}}); code != http.StatusBadRequest || body != "Button: Button text is empty" {
		t.Fatalf("an empty button text must be refused: %d %s", code, body)
	}
	long := strings.Repeat("s", emailblocks.SubjectMaxLength+5)
	if code, body := saveFields(updateEmailEditableFieldsRequest{Subject: &long}); code != http.StatusNoContent {
		t.Fatalf("a long subject is only a warning: %d %s", code, body)
	}
	issues := checks()
	if len(issues) != 1 || issues[0].Level != emailblocks.IssueWarning || issues[0].Field != "subject" {
		t.Fatalf("expected the subject warning: %+v", issues)
	}
	if code, body := markReady(); code != http.StatusConflict || !strings.Contains(body, "Subject is 95 characters") {
		t.Fatalf("a warning must keep the email from ready: %d %s", code, body)
	}

	short := "Welcome"
	if code, body := saveFields(updateEmailEditableFieldsRequest{Subject: &short}); code != http.StatusNoContent {
		t.Fatalf("fix the subject: %d %s", code, body)
	}
	if code, body := markReady(); code != http.StatusOK {
		t.Fatalf("a clean email can be marked ready: %d %s", code, body)
	}
}

func TestEmailChecksKeepOldErrors(t *testing.T) {
	dbpool := testDBPool(t)
	resetEmailBlocks(t, dbpool)
	admin := createTestUserWithRole(t, dbpool, "admin")
	id := createLayoutTestEmail(t, dbpool, "checks-old-en", "en", "Hello Alice", "")

	// The converter puts an email on blocks without the checks; one without
	// a button stays editable, its text can still be saved.
	if _, err := dbpool.Exec(t.Context(), `UPDATE emails SET layout = '[{"block":"intro_body"}]' WHERE id = $1;`, id); err != nil {
		t.Fatal(err)
	}
	response := blockRequest(t, dbpool, admin, http.MethodPatch, "/api/emails/"+id+"/editable-fields",
		updateEmailEditableFieldsRequest{EditableFields: emailedit.EditableFields{"intro_body": {Type: "text", Value: "Hi Alice"}}})
	if response.Code != http.StatusNoContent {
		t.Fatalf("an error the email already had must not block a save: %d %s", response.Code, response.Body.String())
	}
}
