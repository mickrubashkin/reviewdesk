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

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

func TestEditedTextFieldsBecomeMarkdown(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html><body><table><tr>
			<td data-review-block="body" data-edit-text="body">Old body</td>
			<td data-review-block="other" data-edit-text="other">Other</td>
			<td><a href="https://example.com" data-review-block="cta" data-edit-text="cta">Go</a></td>
		</tr></table></body></html>
	`)
	if _, err := dbpool.Exec(context.Background(), `
		UPDATE emails SET editable_fields = '{
			"body": {"type": "text", "value": "Old body"},
			"other": {"type": "text", "value": "Other"},
			"cta": {"type": "text", "value": "Go"}
		}' WHERE id = $1;
	`, emailID); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	patch := func(fields string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPatch, "/api/emails/"+emailID+"/editable-fields",
			bytes.NewReader([]byte(`{"title": "Markdown", "editable_fields": `+fields+`}`)))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, withAuthUser(request, user))
		return response
	}

	response := patch(`{
		"body": {"type": "text", "value": "Do **this** and [book a call]({{Responsible person > Booking link}})"},
		"other": {"type": "text", "value": "Other"},
		"cta": {"type": "text", "value": "Go now"}
	}`)
	if response.Code != http.StatusNoContent {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	var fieldsJSON []byte
	if err := dbpool.QueryRow(context.Background(), `SELECT editable_fields FROM emails WHERE id = $1`, emailID).Scan(&fieldsJSON); err != nil {
		t.Fatal(err)
	}
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		t.Fatal(err)
	}
	formats := map[string]string{}
	for key, field := range fields {
		formats[key] = field.Format
	}
	if formats["body"] != emailedit.FormatMarkdown || formats["other"] != "" || formats["cta"] != "" {
		t.Errorf("formats: %v", formats)
	}
	var reviewHTML string
	if err := dbpool.QueryRow(context.Background(), `SELECT review_html FROM emails WHERE id = $1`, emailID).Scan(&reviewHTML); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<strong>this</strong>", `href="{{Responsible person > Booking link}}"`} {
		if !strings.Contains(reviewHTML, want) {
			t.Errorf("review html has no %s: %s", want, reviewHTML)
		}
	}

	response = patch(`{
		"body": {"type": "text", "value": "[click](javascript:alert(1))", "format": "markdown"},
		"other": {"type": "text", "value": "Other"},
		"cta": {"type": "text", "value": "Go now"}
	}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "link") {
		t.Errorf("unsafe link: %d %s", response.Code, response.Body.String())
	}
}
