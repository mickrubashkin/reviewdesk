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

func TestPastedFormattingSurvivesSavingAnotherField(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")
	createTestBoard(t, dbpool, "pasted-markdown", []string{"uploaded"})
	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	pasted := `<html><body><table><tr>` +
		`<td><div data-edit-text="body">Read <strong>this</strong>, <em>then</em> <a href="https://example.com/guide" style="color: #ff0000;">the guide</a>.</div></td>` +
		`<td data-edit-text="other">Other</td>` +
		`</tr></table></body></html>`
	body, err := json.Marshal(map[string]any{
		"sequence": "pasted-markdown", "title": "Pasted", "stage": "uploaded", "language": "en", "original_html": pasted,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, withAuthUser(httptest.NewRequest(http.MethodPost, "/api/emails", bytes.NewReader(body)), user))
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	var created EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, created.ID)
	})
	var fields emailedit.EditableFields
	if err := json.Unmarshal(created.EditableFields, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["body"].Format != emailedit.FormatMarkdown || fields["body"].Value != "Read **this**, *then* [the guide](https://example.com/guide)." {
		t.Fatalf("body field: %+v", fields["body"])
	}

	fields["other"] = emailedit.EditableField{Type: emailedit.FieldTypeText, Value: "Other, edited"}
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, withAuthUser(httptest.NewRequest(http.MethodPatch, "/api/emails/"+created.ID+"/editable-fields",
		bytes.NewReader([]byte(`{"title": "Pasted", "editable_fields": `+string(fieldsJSON)+`}`))), user))
	if response.Code != http.StatusNoContent {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	var reviewHTML string
	if err := dbpool.QueryRow(context.Background(), `SELECT review_html FROM emails WHERE id = $1`, created.ID).Scan(&reviewHTML); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Read <strong>this</strong>, <em>then</em> ",
		`<a href="https://example.com/guide" target="_blank" style="color: #ff0000;"`,
		"Other, edited",
	} {
		if !strings.Contains(reviewHTML, want) {
			t.Errorf("review html has no %s: %s", want, reviewHTML)
		}
	}
}
