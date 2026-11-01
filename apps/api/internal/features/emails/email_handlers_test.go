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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

func TestListEmailsIncludesOpenCommentCount(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)

	_, err := dbpool.Exec(context.Background(), `
		INSERT INTO comments (
			email_id,
			review_block,
			selected_text,
			start_offset,
			end_offset,
			body,
			status,
			severity
		)
		VALUES
			($1, 'body-001', 'open comment', 0, 12, 'Open comment body', 'open', 'issue'),
			($1, 'body-002', 'resolved comment', 0, 16, 'Resolved comment body', 'resolved', 'issue');
	`, emailID)
	if err != nil {
		t.Fatalf("failed to seed comments: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodGet, "/api/emails", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d: %s", response.Code, response.Body.String())
	}

	var emails []EmailListItem
	if err := json.NewDecoder(response.Body).Decode(&emails); err != nil {
		t.Fatalf("failed to decode emails: %v", err)
	}

	for _, email := range emails {
		if email.ID == emailID {
			if email.OpenCommentCount != 1 {
				t.Fatalf("expected open comment count 1, got %d", email.OpenCommentCount)
			}
			if email.ReviewStatus != "draft" {
				t.Fatalf("expected default review status draft, got %q", email.ReviewStatus)
			}
			return
		}
	}

	t.Fatalf("seed email %s not returned by list emails", emailID)
}

func TestListEmailsExcludesArchivedEmails(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET archived_at = now()
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to archive test email: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodGet, "/api/emails", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d: %s", response.Code, response.Body.String())
	}

	var emails []EmailListItem
	if err := json.NewDecoder(response.Body).Decode(&emails); err != nil {
		t.Fatalf("failed to decode emails: %v", err)
	}

	for _, email := range emails {
		if email.ID == emailID {
			t.Fatalf("archived email %s should not be returned by list emails", emailID)
		}
	}
}

func TestListEmailsFiltersByBoard(t *testing.T) {
	dbpool := testDBPool(t)
	firstEmailID := createTestEmail(t, dbpool)
	secondEmailID := createTestEmailWithSlug(t, dbpool, "comment-test-email-other-board")
	firstBoard := createTestBoard(t, dbpool, "first-board", []string{"test"})
	secondBoard := createTestBoard(t, dbpool, "second-board", []string{"test"})

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails SET sequence = $2 WHERE id = $1;
	`, firstEmailID, firstBoard)
	if err != nil {
		t.Fatalf("failed to assign first email board: %v", err)
	}
	_, err = dbpool.Exec(context.Background(), `
		UPDATE emails SET sequence = $2 WHERE id = $1;
	`, secondEmailID, secondBoard)
	if err != nil {
		t.Fatalf("failed to assign second email board: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodGet, "/api/emails?board="+firstBoard, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d: %s", response.Code, response.Body.String())
	}

	var emails []EmailListItem
	if err := json.NewDecoder(response.Body).Decode(&emails); err != nil {
		t.Fatalf("failed to decode emails: %v", err)
	}
	for _, email := range emails {
		if email.ID == secondEmailID {
			t.Fatalf("email from another board should not be returned: %#v", email)
		}
	}
	if !emailListContains(emails, firstEmailID) {
		t.Fatalf("expected filtered email %s in %#v", firstEmailID, emails)
	}
}

func TestCreateEmail(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")
	boardKey := createTestBoard(t, dbpool, "upload-board", []string{"uploaded"})

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	requestBody := []byte(`{
		"sequence": "upload-board",
		"title": "Uploaded Email",
		"subject": "Uploaded subject",
		"preheader": "Uploaded preheader",
		"send_timing": "day 3",
		"stage": "uploaded",
		"sort_order": 321,
		"language": "en",
		"variant": "candidate",
		"adaptation_label": "Promo",
		"original_html": "<html><body><p data-edit-text=\"intro_text\">Hello upload</p><a href=\"https://example.com\" data-edit-attr-href=\"cta_url\">Start</a></body></html>"
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/emails", bytes.NewReader(requestBody))
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", response.Code, response.Body.String())
	}

	var created EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created email: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected created email id")
	}
	if created.ReviewStatus != "draft" {
		t.Fatalf("expected default review status draft, got %q", created.ReviewStatus)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, created.ID)
	})
	if created.Slug != "upload-board-uploaded-uploaded-email-en-candidate--promo" {
		t.Fatalf("expected generated slug, got %q", created.Slug)
	}
	if created.Sequence != boardKey ||
		created.Title != "Uploaded Email" ||
		created.Stage != "uploaded" ||
		created.SortOrder != 321 ||
		created.Language != "en" ||
		created.Variant != "candidate" ||
		created.AdaptationKey != "promo" ||
		created.AdaptationLabel != "Promo" {
		t.Fatalf("unexpected created email: %#v", created)
	}
	if created.ReviewHTML == nil || !strings.Contains(*created.ReviewHTML, "data-review-block") {
		t.Fatalf("expected generated review html, got %#v", created.ReviewHTML)
	}

	var fields map[string]map[string]any
	if err := json.Unmarshal(created.EditableFields, &fields); err != nil {
		t.Fatalf("failed to decode editable fields: %v", err)
	}
	if fields["intro_text"]["value"] != "Hello upload" {
		t.Fatalf("expected extracted intro text, got %#v", fields)
	}
	if fields["cta_url"]["value"] != "https://example.com" {
		t.Fatalf("expected extracted cta url, got %#v", fields)
	}

	var actorEmail string
	err := dbpool.QueryRow(context.Background(), `
		SELECT actor_email
		FROM email_events
		WHERE email_id = $1
			AND action = 'email_created'
		LIMIT 1;
	`, created.ID).Scan(&actorEmail)
	if err != nil {
		t.Fatalf("failed to load created email event: %v", err)
	}
	if actorEmail != user.Email {
		t.Fatalf("expected actor %s, got %s", user.Email, actorEmail)
	}
}

func TestCreateEmailRejectsUnknownBoard(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	requestBody := []byte(`{
		"sequence": "unknown-board",
		"title": "Uploaded Email",
		"stage": "uploaded",
		"language": "en",
		"variant": "v1",
		"original_html": "<html><body><p>Hello upload</p></body></html>"
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/emails", bytes.NewReader(requestBody))
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected POST status 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateEmailAllowsReusingArchivedSlug(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")
	boardKey := createTestBoard(t, dbpool, "recreate-board", []string{"uploaded"})

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	requestBody := []byte(`{
		"sequence": "recreate-board",
		"title": "Recreated Email",
		"subject": "Uploaded subject",
		"preheader": "Uploaded preheader",
		"send_timing": "day 3",
		"stage": "uploaded",
		"sort_order": 321,
		"language": "es",
		"variant": "v1",
		"adaptation_label": "Default",
		"original_html": "<html><body><p data-edit-text=\"intro_text\">Hello upload</p></body></html>"
	}`)

	firstRequest := httptest.NewRequest(http.MethodPost, "/api/emails", bytes.NewReader(requestBody))
	firstRequest = withAuthUser(firstRequest, user)
	firstResponse := httptest.NewRecorder()
	router.ServeHTTP(firstResponse, firstRequest)
	if firstResponse.Code != http.StatusCreated {
		t.Fatalf("expected first POST status 201, got %d: %s", firstResponse.Code, firstResponse.Body.String())
	}

	var firstCreated EmailDetail
	if err := json.NewDecoder(firstResponse.Body).Decode(&firstCreated); err != nil {
		t.Fatalf("failed to decode first created email: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE sequence = $1 AND title = 'Recreated Email';`, boardKey)
	})

	archiveRequest := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+firstCreated.ID+"/archive",
		nil,
	)
	archiveRequest = withAuthUser(archiveRequest, user)
	archiveResponse := httptest.NewRecorder()
	router.ServeHTTP(archiveResponse, archiveRequest)
	if archiveResponse.Code != http.StatusNoContent {
		t.Fatalf("expected archive status 204, got %d: %s", archiveResponse.Code, archiveResponse.Body.String())
	}

	secondRequest := httptest.NewRequest(http.MethodPost, "/api/emails", bytes.NewReader(requestBody))
	secondRequest = withAuthUser(secondRequest, user)
	secondResponse := httptest.NewRecorder()
	router.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusCreated {
		t.Fatalf("expected second POST status 201, got %d: %s", secondResponse.Code, secondResponse.Body.String())
	}

	var secondCreated EmailDetail
	if err := json.NewDecoder(secondResponse.Body).Decode(&secondCreated); err != nil {
		t.Fatalf("failed to decode second created email: %v", err)
	}
	if secondCreated.Slug != firstCreated.Slug {
		t.Fatalf("expected recreated email slug %q, got %q", firstCreated.Slug, secondCreated.Slug)
	}

	var archivedSlug string
	err := dbpool.QueryRow(context.Background(), `
		SELECT slug
		FROM emails
		WHERE id = $1
			AND archived_at IS NOT NULL;
	`, firstCreated.ID).Scan(&archivedSlug)
	if err != nil {
		t.Fatalf("failed to load archived email slug: %v", err)
	}
	if archivedSlug == firstCreated.Slug {
		t.Fatalf("expected archived email slug to be released, got %q", archivedSlug)
	}
}

func TestGetEmailIncludesReviewStatus(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodGet, "/api/emails/"+emailID, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d: %s", response.Code, response.Body.String())
	}

	var email EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&email); err != nil {
		t.Fatalf("failed to decode email: %v", err)
	}
	if email.ReviewStatus != "draft" {
		t.Fatalf("expected default review status draft, got %q", email.ReviewStatus)
	}
}

func TestUpdateEmailReviewStatus(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/review-status",
		bytes.NewReader([]byte(`{"review_status":"ready"}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected PATCH status 200, got %d: %s", response.Code, response.Body.String())
	}

	var payload updateEmailReviewStatusResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode review status response: %v", err)
	}
	if payload.ReviewStatus != "ready" {
		t.Fatalf("expected approved status, got %q", payload.ReviewStatus)
	}

	var storedStatus string
	if err := dbpool.QueryRow(context.Background(), `
		SELECT review_status
		FROM emails
		WHERE id = $1;
	`, emailID).Scan(&storedStatus); err != nil {
		t.Fatalf("failed to load stored review status: %v", err)
	}
	if storedStatus != "ready" {
		t.Fatalf("expected stored approved status, got %q", storedStatus)
	}

	var changes []byte
	var metadata []byte
	if err := dbpool.QueryRow(context.Background(), `
		SELECT changes, metadata
		FROM email_events
		WHERE email_id = $1
			AND action = 'email_review_status_updated'
		ORDER BY created_at DESC
		LIMIT 1;
	`, emailID).Scan(&changes, &metadata); err != nil {
		t.Fatalf("failed to load review status event: %v", err)
	}
	if !strings.Contains(string(changes), `"before": "draft"`) ||
		!strings.Contains(string(changes), `"after": "ready"`) {
		t.Fatalf("expected review status changes, got %s", string(changes))
	}
	var approvalMetadata map[string]any
	if err := json.Unmarshal(metadata, &approvalMetadata); err != nil {
		t.Fatalf("failed to decode approval metadata: %v", err)
	}
	for _, key := range []string{
		"approved_content_hash",
		"approved_editable_fields_hash",
		"approved_template_hash",
	} {
		value, ok := approvalMetadata[key].(string)
		if !ok || len(value) != 64 {
			t.Fatalf("expected approval metadata %s hash, got %#v", key, approvalMetadata[key])
		}
	}
	if approvalMetadata["approval_snapshot_version"] != float64(1) {
		t.Fatalf("expected approval snapshot version 1, got %#v", approvalMetadata["approval_snapshot_version"])
	}
}

func TestUpdateEmailReviewStatusMarksReapprovalAfterStaleEdit(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET review_status = 'draft'
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to set email review status: %v", err)
	}
	_, err = dbpool.Exec(context.Background(), `
		INSERT INTO email_events (
			actor_user_id,
			actor_email,
			action,
			email_id,
			email_slug,
			email_title,
			metadata,
			changes
		)
		VALUES (
			$1,
			$2,
			'email_review_status_updated',
			$3,
			'comment-test-email',
			'Comment Test Email',
			'{"reason":"approval_stale_after_edit"}'::jsonb,
			'{"review_status":{"before":"approved","after":"draft"}}'::jsonb
		);
	`, user.ID, user.Email, emailID)
	if err != nil {
		t.Fatalf("failed to seed stale approval event: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/review-status",
		bytes.NewReader([]byte(`{"review_status":"ready"}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected PATCH status 200, got %d: %s", response.Code, response.Body.String())
	}

	var metadataJSON []byte
	var changesJSON []byte
	err = dbpool.QueryRow(context.Background(), `
		SELECT metadata, changes
		FROM email_events
		WHERE email_id = $1
			AND action = 'email_review_status_updated'
		ORDER BY created_at DESC, id DESC
		LIMIT 1;
	`, emailID).Scan(&metadataJSON, &changesJSON)
	if err != nil {
		t.Fatalf("failed to load reapproval event: %v", err)
	}
	if !strings.Contains(string(metadataJSON), `"reason": "reapproved_after_stale_edit"`) {
		t.Fatalf("expected reapproval metadata, got %s", string(metadataJSON))
	}
	if !strings.Contains(string(changesJSON), `"before": "draft"`) ||
		!strings.Contains(string(changesJSON), `"after": "ready"`) {
		t.Fatalf("expected reapproval status change, got %s", string(changesJSON))
	}
}

func TestUpdateEmailPlanningFields(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/planning-fields",
		bytes.NewReader([]byte(`{
			"owner_email": "owner@example.com",
			"reviewer_email": "reviewer@example.com",
			"due_date": "2026-06-15",
			"implementation_notes": "Implement after legal approval.",
			"send_timing": "Day 3",
			"adaptation_label": "UAE"
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected PATCH status 200, got %d: %s", response.Code, response.Body.String())
	}

	var payload updateEmailPlanningFieldsResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode planning fields response: %v", err)
	}
	if stringFromPointer(payload.OwnerEmail) != "owner@example.com" ||
		stringFromPointer(payload.ReviewerEmail) != "reviewer@example.com" ||
		stringFromPointer(payload.DueDate) != "2026-06-15" ||
		stringFromPointer(payload.ImplementationNotes) != "Implement after legal approval." ||
		stringFromPointer(payload.SendTiming) != "Day 3" ||
		payload.AdaptationLabel != "UAE" {
		t.Fatalf("unexpected planning response: %#v", payload)
	}

	var email EmailDetail
	getRequest := httptest.NewRequest(http.MethodGet, "/api/emails/"+emailID, nil)
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d: %s", getResponse.Code, getResponse.Body.String())
	}
	if err := json.NewDecoder(getResponse.Body).Decode(&email); err != nil {
		t.Fatalf("failed to decode email detail: %v", err)
	}
	if stringFromPointer(email.OwnerEmail) != "owner@example.com" ||
		stringFromPointer(email.ReviewerEmail) != "reviewer@example.com" ||
		stringFromPointer(email.DueDate) != "2026-06-15" ||
		stringFromPointer(email.ImplementationNotes) != "Implement after legal approval." ||
		stringFromPointer(email.SendTiming) != "Day 3" ||
		email.AdaptationLabel != "UAE" {
		t.Fatalf("expected planning fields on email detail, got %#v", email)
	}

	var actorEmail string
	var changesJSON []byte
	if err := dbpool.QueryRow(context.Background(), `
		SELECT actor_email, changes
		FROM email_events
		WHERE email_id = $1
			AND action = 'email_planning_updated'
		ORDER BY created_at DESC
		LIMIT 1;
	`, emailID).Scan(&actorEmail, &changesJSON); err != nil {
		t.Fatalf("failed to load planning event: %v", err)
	}
	if actorEmail != user.Email {
		t.Fatalf("expected planning event actor %s, got %s", user.Email, actorEmail)
	}
	if !strings.Contains(string(changesJSON), `"owner_email"`) ||
		!strings.Contains(string(changesJSON), `"due_date"`) ||
		!strings.Contains(string(changesJSON), `"implementation_notes"`) ||
		!strings.Contains(string(changesJSON), `"send_timing"`) ||
		!strings.Contains(string(changesJSON), `"adaptation_label"`) {
		t.Fatalf("expected planning changes, got %s", string(changesJSON))
	}
}

func TestUpdateEmailPlanningFieldsRejectsInvalidAdaptationLabel(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/planning-fields",
		bytes.NewReader([]byte(`{"adaptation_label":"日本"}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected PATCH status 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailPlanningFieldsRejectsInvalidDueDate(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/planning-fields",
		bytes.NewReader([]byte(`{"due_date":"06/15/2026"}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected PATCH status 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailPlanningFieldsRejectsReviewer(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "reviewer")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/planning-fields",
		bytes.NewReader([]byte(`{"owner_email":"owner@example.com"}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected PATCH status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailReviewStatusRejectsReviewer(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "reviewer")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/review-status",
		bytes.NewReader([]byte(`{"review_status":"ready"}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected PATCH status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailReviewStatusRejectsInvalidStatus(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/review-status",
		bytes.NewReader([]byte(`{"review_status":"approved"}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected PATCH status 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailReviewStatusAllowsReadyWithOpenComment(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	if _, err := dbpool.Exec(context.Background(), `
		INSERT INTO comments (
			email_id,
			review_block,
			selected_text,
			start_offset,
			end_offset,
			body,
			status,
			severity
		)
		VALUES ($1, 'body-001', 'selected text', 0, 13, 'Fix this', 'open', 'issue');
	`, emailID); err != nil {
		t.Fatalf("failed to seed comment: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/review-status",
		bytes.NewReader([]byte(`{"review_status":"ready"}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	// Comments do not gate the review status.
	if response.Code != http.StatusOK {
		t.Fatalf("expected PATCH status 200, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailReviewStatusRejectsArchivedEmail(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	if _, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET archived_at = now()
		WHERE id = $1;
	`, emailID); err != nil {
		t.Fatalf("failed to archive test email: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/review-status",
		bytes.NewReader([]byte(`{"review_status":"ready"}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected PATCH status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestInspectEmailHTML(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	requestBody := []byte(`{
		"original_html": "<html><body><p data-edit-text=\"intro_text\">Hello upload content that is long enough for review</p><a href=\"https://example.com\" data-edit-attr-href=\"cta_url\">Start now</a></body></html>"
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/emails/inspect-html", bytes.NewReader(requestBody))
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected POST status 200, got %d: %s", response.Code, response.Body.String())
	}

	var inspection emailHTMLInspection
	if err := json.NewDecoder(response.Body).Decode(&inspection); err != nil {
		t.Fatalf("failed to decode inspection: %v", err)
	}
	if inspection.ReviewBlockCount == 0 {
		t.Fatalf("expected generated review blocks, got %#v", inspection)
	}
	if inspection.OriginalReviewBlockCount != 0 {
		t.Fatalf("expected no original review blocks, got %d", inspection.OriginalReviewBlockCount)
	}
	if inspection.EditableFieldCount != 2 {
		t.Fatalf("expected 2 editable fields, got %#v", inspection.EditableFields)
	}
	if len(inspection.EditableFields) != 2 ||
		inspection.EditableFields[0].Key != "intro_text" ||
		inspection.EditableFields[1].Key != "cta_url" {
		t.Fatalf("expected ordered editable fields, got %#v", inspection.EditableFields)
	}
	if !strings.Contains(inspection.ReviewHTML, "data-review-block") {
		t.Fatalf("expected generated review html, got %q", inspection.ReviewHTML)
	}
	if !containsString(inspection.Warnings, "No data-review-block markers were found; review blocks will be generated automatically.") {
		t.Fatalf("expected warning about generated review blocks, got %#v", inspection.Warnings)
	}
}

func TestUpdateEmailEditableFields(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html>
			<body>
				<a
					href="https://example.com/old"
					style="display:block; width: 196px;"
					data-edit-text="primary_cta_text"
					data-edit-attr-href="primary_cta_url"
					data-edit-style-width-px="primary_cta_width_px"
				>Old CTA</a>
			</body>
		</html>
	`)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	requestBody := []byte(`{
		"title": "Updated Email",
		"subject": "Updated subject",
		"preheader": "Updated preheader",
		"editable_fields": {
			"primary_cta_text": { "type": "text", "value": "Start now" },
			"primary_cta_url": { "type": "url", "value": "https://example.com/start" },
			"primary_cta_width_px": { "type": "number", "value": 220 }
		}
	}`)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader(requestBody),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	fields := loadTestEmailEditableFields(t, dbpool, emailID)
	assertEditableField(t, fields, "primary_cta_text", "text", "Start now")
	assertEditableField(t, fields, "primary_cta_url", "url", "https://example.com/start")
	assertEditableField(t, fields, "primary_cta_width_px", "number", float64(220))

	var title string
	var subject *string
	var preheader *string
	var bodyText string
	var reviewHTML string
	err := dbpool.QueryRow(context.Background(), `
		SELECT title, subject, preheader, body_text, review_html
		FROM emails
		WHERE id = $1;
	`, emailID).Scan(&title, &subject, &preheader, &bodyText, &reviewHTML)
	if err != nil {
		t.Fatalf("failed to load updated metadata: %v", err)
	}
	if title != "Updated Email" {
		t.Fatalf("expected updated title, got %q", title)
	}
	if subject == nil || *subject != "Updated subject" {
		t.Fatalf("expected updated subject, got %#v", subject)
	}
	if preheader == nil || *preheader != "Updated preheader" {
		t.Fatalf("expected updated preheader, got %#v", preheader)
	}
	if !strings.Contains(reviewHTML, "Start now") {
		t.Fatalf("expected review html to include rendered editable text, got %s", reviewHTML)
	}
	if !strings.Contains(reviewHTML, "https://example.com/start") {
		t.Fatalf("expected review html to include rendered editable URL, got %s", reviewHTML)
	}
	if !strings.Contains(bodyText, "Start now") {
		t.Fatalf("expected body text to include rendered editable text, got %s", bodyText)
	}

	var actorEmail string
	var changesJSON []byte
	err = dbpool.QueryRow(context.Background(), `
		SELECT actor_email, changes
		FROM email_events
		WHERE email_id = $1
			AND action = 'email_updated'
		ORDER BY created_at DESC
		LIMIT 1;
	`, emailID).Scan(&actorEmail, &changesJSON)
	if err != nil {
		t.Fatalf("failed to load email update event: %v", err)
	}
	if actorEmail != user.Email {
		t.Fatalf("expected update event actor %s, got %s", user.Email, actorEmail)
	}
	var changes map[string]any
	if err := json.Unmarshal(changesJSON, &changes); err != nil {
		t.Fatalf("failed to decode update event changes: %v", err)
	}
	if _, ok := changes["title"]; !ok {
		t.Fatalf("expected title change, got %#v", changes)
	}
	editableFieldChanges, ok := changes["editable_fields"].(map[string]any)
	if !ok {
		t.Fatalf("expected editable_fields changes, got %#v", changes)
	}
	if _, ok := editableFieldChanges["primary_cta_text"]; !ok {
		t.Fatalf("expected primary_cta_text change, got %#v", editableFieldChanges)
	}
}

func TestUpdateEmailEditableFieldsCreatesVersionSnapshot(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html>
			<body>
				<p data-edit-text="intro_text">Old intro</p>
			</body>
		</html>
	`)
	insertTestEmailVersion(t, dbpool, emailID, "system", "initial")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{
			"title": "Versioned Email",
			"subject": "Versioned subject",
			"editable_fields": {
				"intro_text": { "type": "text", "value": "New intro" }
			}
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	var versionNumber int
	var actorEmail string
	var title string
	var subject *string
	var changedFieldCount int
	var changedMetadataCount int
	err := dbpool.QueryRow(context.Background(), `
		SELECT version_number, created_by_email, title, subject, changed_field_count, changed_metadata_count
		FROM email_versions
		WHERE email_id = $1
		ORDER BY version_number DESC
		LIMIT 1;
	`, emailID).Scan(&versionNumber, &actorEmail, &title, &subject, &changedFieldCount, &changedMetadataCount)
	if err != nil {
		t.Fatalf("failed to load email version: %v", err)
	}
	if versionNumber != 2 {
		t.Fatalf("expected version 2, got %d", versionNumber)
	}
	if actorEmail != user.Email {
		t.Fatalf("expected version actor %s, got %s", user.Email, actorEmail)
	}
	if title != "Versioned Email" || subject == nil || *subject != "Versioned subject" {
		t.Fatalf("expected saved snapshot title/subject, got %q %#v", title, subject)
	}
	if changedFieldCount != 1 || changedMetadataCount != 2 {
		t.Fatalf("expected field/metadata change counts 1/2, got %d/%d", changedFieldCount, changedMetadataCount)
	}
}

func TestUpdateEmailEditableFieldsNoopDoesNotCreateVersion(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html>
			<body>
				<p data-edit-text="intro_text">Same intro</p>
			</body>
		</html>
	`)
	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET editable_fields = '{"intro_text": { "type": "text", "value": "Same intro" }}'::jsonb
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to prepare noop email: %v", err)
	}
	insertTestEmailVersion(t, dbpool, emailID, "system", "initial")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{
			"title": "Comment Test Email",
			"editable_fields": {
				"intro_text": { "type": "text", "value": "Same intro" }
			}
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	var versionCount int
	err = dbpool.QueryRow(context.Background(), `
		SELECT count(*)::int
		FROM email_versions
		WHERE email_id = $1;
	`, emailID).Scan(&versionCount)
	if err != nil {
		t.Fatalf("failed to count email versions: %v", err)
	}
	if versionCount != 1 {
		t.Fatalf("expected one version after noop save, got %d", versionCount)
	}
}

func TestRestoreEmailVersionCreatesNewLatestVersion(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html>
			<body>
				<p data-edit-text="intro_text">Old intro</p>
			</body>
		</html>
	`)
	if _, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET editable_fields = '{"intro_text": { "type": "text", "value": "Old intro" }}'::jsonb
		WHERE id = $1;
	`, emailID); err != nil {
		t.Fatalf("failed to set initial editable fields: %v", err)
	}
	restoredVersionID := insertTestEmailVersion(t, dbpool, emailID, "system", "initial")
	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET
			title = 'Current title',
			editable_fields = '{"intro_text": { "type": "text", "value": "Current intro" }}'::jsonb,
			review_status = 'ready'
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to prepare restore email: %v", err)
	}
	insertTestEmailVersion(t, dbpool, emailID, user.Email, "manual")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/versions/"+restoredVersionID+"/restore",
		http.NoBody,
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected restore status 204, got %d: %s", response.Code, response.Body.String())
	}

	var title string
	var reviewStatus string
	var fieldsJSON []byte
	err = dbpool.QueryRow(context.Background(), `
		SELECT title, review_status, editable_fields
		FROM emails
		WHERE id = $1;
	`, emailID).Scan(&title, &reviewStatus, &fieldsJSON)
	if err != nil {
		t.Fatalf("failed to load restored email: %v", err)
	}
	if title != "Comment Test Email" {
		t.Fatalf("expected restored title, got %q", title)
	}
	if reviewStatus != "draft" {
		t.Fatalf("expected approved restore to become draft, got %q", reviewStatus)
	}
	if !strings.Contains(string(fieldsJSON), "Old intro") {
		t.Fatalf("expected restored editable fields, got %s", string(fieldsJSON))
	}

	var versionNumber int
	var source string
	var restoredFrom *string
	err = dbpool.QueryRow(context.Background(), `
		SELECT version_number, source, restored_from_version_id::text
		FROM email_versions
		WHERE email_id = $1
		ORDER BY version_number DESC
		LIMIT 1;
	`, emailID).Scan(&versionNumber, &source, &restoredFrom)
	if err != nil {
		t.Fatalf("failed to load latest restored version: %v", err)
	}
	if versionNumber != 3 || source != "restore" || restoredFrom == nil || *restoredFrom != restoredVersionID {
		t.Fatalf("expected restore version 3 from %s, got %d %s %#v", restoredVersionID, versionNumber, source, restoredFrom)
	}
}

func TestRestoreEmailVersionRejectsReviewer(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "reviewer")
	versionID := insertTestEmailVersion(t, dbpool, emailID, "system", "initial")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/versions/"+versionID+"/restore",
		http.NoBody,
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected restore status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailEditableFieldsRecordsChangedReviewBlocks(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET
			subject = 'Old subject',
			preheader = 'Old preheader',
			template_html = '
				<html>
					<body>
						<p data-review-block="intro" data-edit-text="intro_text">Keep intro</p>
						<a data-review-block="primary_cta" data-edit-text="primary_cta_text" data-edit-attr-href="primary_cta_url" href="https://example.com/old">Old CTA</a>
					</body>
				</html>
			',
			editable_fields = '{
				"intro_text": { "type": "text", "value": "Keep intro" },
				"primary_cta_text": { "type": "text", "value": "Old CTA" },
				"primary_cta_url": { "type": "url", "value": "https://example.com/old" }
			}'::jsonb
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to prepare changed review blocks test: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{
			"subject": "New subject",
			"preheader": "New preheader",
			"editable_fields": {
				"intro_text": { "type": "text", "value": "Keep intro" },
				"primary_cta_text": { "type": "text", "value": "New CTA" },
				"primary_cta_url": { "type": "url", "value": "https://example.com/new" }
			}
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	var metadataJSON []byte
	err = dbpool.QueryRow(context.Background(), `
		SELECT metadata
		FROM email_events
		WHERE email_id = $1
			AND action = 'email_updated'
		ORDER BY created_at DESC
		LIMIT 1;
	`, emailID).Scan(&metadataJSON)
	if err != nil {
		t.Fatalf("failed to load update event metadata: %v", err)
	}

	var metadata map[string]any
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		t.Fatalf("failed to decode update event metadata: %v", err)
	}
	changedBlocks, ok := metadata["changed_review_blocks"].([]any)
	if !ok {
		t.Fatalf("expected changed_review_blocks metadata, got %#v", metadata)
	}
	expectedBlocks := []string{"preheader", "primary_cta", "subject"}
	if len(changedBlocks) != len(expectedBlocks) {
		t.Fatalf("expected changed blocks %#v, got %#v", expectedBlocks, changedBlocks)
	}
	for index, expectedBlock := range expectedBlocks {
		if changedBlocks[index] != expectedBlock {
			t.Fatalf("expected changed blocks %#v, got %#v", expectedBlocks, changedBlocks)
		}
	}
}

func TestUpdateEmailEditableFieldsMarksApprovedEmailChangesRequested(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html>
			<body>
				<p data-edit-text="intro_text">Old intro</p>
			</body>
		</html>
	`)

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET review_status = 'ready'
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to approve test email: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{
			"editable_fields": {
				"intro_text": { "type": "text", "value": "New intro" }
			}
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	var storedStatus string
	if err := dbpool.QueryRow(context.Background(), `
		SELECT review_status
		FROM emails
		WHERE id = $1;
	`, emailID).Scan(&storedStatus); err != nil {
		t.Fatalf("failed to load review status: %v", err)
	}
	if storedStatus != "draft" {
		t.Fatalf("expected draft status, got %q", storedStatus)
	}
}

func TestUpdateEmailEditableFieldsPreservesNonApprovedReviewStatus(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html>
			<body>
				<p data-edit-text="intro_text">Old intro</p>
			</body>
		</html>
	`)

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET review_status = 'draft'
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to set draft review status: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{
			"editable_fields": {
				"intro_text": { "type": "text", "value": "New intro" }
			}
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	var storedStatus string
	if err := dbpool.QueryRow(context.Background(), `
		SELECT review_status
		FROM emails
		WHERE id = $1;
	`, emailID).Scan(&storedStatus); err != nil {
		t.Fatalf("failed to load review status: %v", err)
	}
	if storedStatus != "draft" {
		t.Fatalf("expected draft status, got %q", storedStatus)
	}
}

func TestUpdateEmailEditableFieldsRecordsStaleApprovalEvent(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html>
			<body>
				<p data-edit-text="intro_text">Old intro</p>
			</body>
		</html>
	`)

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET review_status = 'ready'
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to approve test email: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{
			"editable_fields": {
				"intro_text": { "type": "text", "value": "New intro" }
			}
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	var metadataJSON []byte
	var changesJSON []byte
	err = dbpool.QueryRow(context.Background(), `
		SELECT metadata, changes
		FROM email_events
		WHERE email_id = $1
			AND action = 'email_review_status_updated'
		ORDER BY created_at DESC
		LIMIT 1;
	`, emailID).Scan(&metadataJSON, &changesJSON)
	if err != nil {
		t.Fatalf("failed to load stale approval event: %v", err)
	}
	if !strings.Contains(string(metadataJSON), `"reason": "approval_stale_after_edit"`) {
		t.Fatalf("expected stale approval metadata, got %s", string(metadataJSON))
	}
	if !strings.Contains(string(changesJSON), `"before": "ready"`) ||
		!strings.Contains(string(changesJSON), `"after": "draft"`) {
		t.Fatalf("expected stale approval status change, got %s", string(changesJSON))
	}
}

func TestUpdateEmailEditableFieldsKeepsOpenCommentsOpen(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html>
			<body>
				<p data-review-block="intro" data-edit-text="intro_text">Old intro</p>
			</body>
		</html>
	`)

	_, err := dbpool.Exec(context.Background(), `
		INSERT INTO comments (
			email_id,
			review_block,
			selected_text,
			start_offset,
			end_offset,
			body,
			status
		)
		VALUES
			($1, 'intro', 'Old intro', 0, 9, 'Open comment body', 'open'),
			($1, 'intro', 'Old intro', 0, 9, 'Resolved comment body', 'resolved');
	`, emailID)
	if err != nil {
		t.Fatalf("failed to seed comments: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{
			"editable_fields": {
				"intro_text": { "type": "text", "value": "New intro" }
			}
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	var openCommentCount int
	var resolvedCommentCount int
	if err := dbpool.QueryRow(context.Background(), `
		SELECT
			count(*) FILTER (WHERE status = 'open')::int,
			count(*) FILTER (WHERE status = 'resolved')::int
		FROM comments
		WHERE email_id = $1;
	`, emailID).Scan(&openCommentCount, &resolvedCommentCount); err != nil {
		t.Fatalf("failed to count comments by status: %v", err)
	}
	if openCommentCount != 1 {
		t.Fatalf("expected one open comment after edit, got %d", openCommentCount)
	}
	if resolvedCommentCount != 1 {
		t.Fatalf("expected one resolved comment after edit, got %d", resolvedCommentCount)
	}
}

func TestUpdateEmailEditableFieldsResetsChangedTextCommentAnchors(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET
			subject = 'Old subject',
			preheader = 'Old preheader',
			template_html = '
				<html>
					<body>
						<p data-review-block="primary_cta" data-edit-text="primary_cta_text">Old CTA</p>
						<p data-review-block="secondary_cta" data-edit-text="secondary_cta_text">Keep going</p>
					</body>
				</html>
			',
			editable_fields = '{
				"primary_cta_text": { "type": "text", "value": "Old CTA" },
				"secondary_cta_text": { "type": "text", "value": "Keep going" }
			}'::jsonb
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to prepare editable comment anchor test: %v", err)
	}

	_, err = dbpool.Exec(context.Background(), `
		INSERT INTO comments (
			email_id,
			review_block,
			selected_text,
			start_offset,
			end_offset,
			body,
			status
		)
		VALUES
			($1, 'primary_cta', 'CTA', 4, 7, 'changed text fragment', 'open'),
			($1, 'primary_cta', 'Old', 0, 3, 'resolved text fragment', 'resolved'),
			($1, 'secondary_cta', 'Keep', 0, 4, 'unchanged text fragment', 'open'),
			($1, 'subject', 'Old', 0, 3, 'subject fragment', 'open'),
			($1, 'preheader', 'pre', 4, 7, 'preheader fragment', 'open');
	`, emailID)
	if err != nil {
		t.Fatalf("failed to seed comments: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	requestBody := []byte(`{
		"subject": "New subject",
		"preheader": "New preheader",
		"editable_fields": {
			"primary_cta_text": { "type": "text", "value": "Start now" },
			"secondary_cta_text": { "type": "text", "value": "Keep going" }
		}
	}`)
	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader(requestBody),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	anchors := loadTestCommentAnchorsByBody(t, dbpool, emailID)
	assertCommentAnchor(t, anchors, "changed text fragment", "primary_cta", "Start now", 0, 9, "open")
	assertCommentAnchor(t, anchors, "resolved text fragment", "primary_cta", "Start now", 0, 9, "resolved")
	assertCommentAnchor(t, anchors, "unchanged text fragment", "secondary_cta", "Keep", 0, 4, "open")
	assertCommentAnchor(t, anchors, "subject fragment", "subject", "New subject", 0, 11, "open")
	assertCommentAnchor(t, anchors, "preheader fragment", "preheader", "New preheader", 0, 13, "open")
}

func TestUpdateEmailEditableFieldsRejectsNullFields(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{ "editable_fields": null }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected PATCH status 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailEditableFieldsRejectsUnsafeURL(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html>
			<body>
				<a href="https://example.com/old" data-edit-attr-href="cta_url">CTA</a>
			</body>
		</html>
	`)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	requestBody := []byte(`{
		"editable_fields": {
			"cta_url": { "type": "url", "value": "javascript:alert(1)" }
		}
	}`)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader(requestBody),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected PATCH status 400, got %d: %s", response.Code, response.Body.String())
	}

	fields := loadTestEmailEditableFields(t, dbpool, emailID)
	if len(fields) != 0 {
		t.Fatalf("expected editable fields to remain empty, got %#v", fields)
	}
}

func TestUpdateEmailEditableFieldsNotFound(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/00000000-0000-0000-0000-000000000000/editable-fields",
		bytes.NewReader([]byte(`{ "editable_fields": {} }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected PATCH status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailEditableFieldsRejectsReviewer(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "reviewer")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{ "editable_fields": {} }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected PATCH status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailOriginalHTMLRequiresSuperAdmin(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{
			"editable_fields": {},
			"original_html": "<html><body><p>Updated HTML</p></body></html>"
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected PATCH status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUpdateEmailOriginalHTMLAsSuperAdmin(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "super_admin")
	setTestEmailTemplate(t, dbpool, emailID, `
		<html>
			<body>
				<a data-review-block="primary_cta" data-edit-text="primary_cta_text" href="https://example.com/old">Old CTA</a>
			</body>
		</html>
	`)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{
			"title": "HTML Edited Email",
			"editable_fields": {
				"primary_cta_text": { "type": "text", "value": "Preserved CTA" }
			},
			"original_html": "<html><body><section><a data-review-block=\"primary_cta\" data-edit-text=\"primary_cta_text\" href=\"https://example.com/new\">Template default</a><p data-edit-text=\"new_copy\">New copy</p></section></body></html>"
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	var title string
	var originalHTML string
	var templateHTML string
	var reviewHTML string
	err := dbpool.QueryRow(context.Background(), `
		SELECT title, original_html, template_html, review_html
		FROM emails
		WHERE id = $1;
	`, emailID).Scan(&title, &originalHTML, &templateHTML, &reviewHTML)
	if err != nil {
		t.Fatalf("failed to load updated original HTML: %v", err)
	}
	if title != "HTML Edited Email" {
		t.Fatalf("expected updated title, got %q", title)
	}
	if !strings.Contains(originalHTML, "<section>") || !strings.Contains(templateHTML, "<section>") {
		t.Fatalf("expected original/template HTML to include edited layout, got original=%q template=%q", originalHTML, templateHTML)
	}
	if !strings.Contains(reviewHTML, "Preserved CTA") || !strings.Contains(reviewHTML, "New copy") {
		t.Fatalf("expected review HTML to render merged editable fields, got %q", reviewHTML)
	}

	fields := loadTestEmailEditableFields(t, dbpool, emailID)
	assertEditableField(t, fields, "primary_cta_text", "text", "Preserved CTA")
	assertEditableField(t, fields, "new_copy", "text", "New copy")
}

func TestUpdateEmailOriginalHTMLAsSuperAdminUsesExtractedTextForSoftWrappedSource(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "super_admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{
			"editable_fields": {
				"intro_body": {
					"type": "text",
					"value": "Hi there,\n\nYour application to join the Bitrix24 Partner Program has been\nreceived.\n\nYour partner account is ready. Use the credentials below to\nlog in."
				}
			},
			"original_html": "<html><body><table><tr><td data-review-block=\"intro_body\" data-edit-text=\"intro_body\">Hi there,<br /><br />Your application to join the Bitrix24 Partner Program has been\nreceived.<br /><br />Your partner account is ready. Use the credentials below to\nlog in.</td></tr></table></body></html>"
		}`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	fields := loadTestEmailEditableFields(t, dbpool, emailID)
	assertEditableField(
		t,
		fields,
		"intro_body",
		"text",
		"Hi there,\n\nYour application to join the Bitrix24 Partner Program has been received.\n\nYour partner account is ready. Use the credentials below to log in.",
	)

	var reviewHTML string
	err := dbpool.QueryRow(context.Background(), `
		SELECT review_html
		FROM emails
		WHERE id = $1;
	`, emailID).Scan(&reviewHTML)
	if err != nil {
		t.Fatalf("failed to load review HTML: %v", err)
	}
	if strings.Contains(reviewHTML, "been<br/>received") || strings.Contains(reviewHTML, "to<br/>log in") {
		t.Fatalf("expected source soft wraps not to render as <br>, got %q", reviewHTML)
	}
	expectedHTML := "Hi there,<br/><br/>Your application to join the Bitrix24 Partner Program has been received.<br/><br/>Your partner account is ready. Use the credentials below to log in."
	if !strings.Contains(reviewHTML, expectedHTML) {
		t.Fatalf("expected review HTML to contain %q, got %q", expectedHTML, reviewHTML)
	}
}

func TestUpdateEmailEditableFieldsRejectsArchivedEmail(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET archived_at = now()
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to archive test email: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/editable-fields",
		bytes.NewReader([]byte(`{ "editable_fields": {} }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected PATCH status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestGetRenderedEmail(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET
			preheader = 'Rendered preheader',
			template_html = '<html><body><span data-email-preheader></span><a href="https://example.com/old" data-edit-text="primary_cta_text" data-edit-attr-href="primary_cta_url">Old CTA</a></body></html>',
			editable_fields = '{"primary_cta_text":{"type":"text","value":"Start now"},"primary_cta_url":{"type":"url","value":"https://example.com/start"}}'::jsonb
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to prepare rendered test email: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodGet, "/api/emails/"+emailID+"/rendered", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d: %s", response.Code, response.Body.String())
	}

	var rendered renderedEmailResponse
	if err := json.NewDecoder(response.Body).Decode(&rendered); err != nil {
		t.Fatalf("failed to decode rendered email: %v", err)
	}
	if !strings.Contains(rendered.HTML, "Rendered preheader") {
		t.Fatalf("expected rendered preheader, got %s", rendered.HTML)
	}
	if !strings.Contains(rendered.HTML, "Start now") {
		t.Fatalf("expected rendered CTA text, got %s", rendered.HTML)
	}
	if !strings.Contains(rendered.HTML, `href="https://example.com/start"`) {
		t.Fatalf("expected rendered CTA URL, got %s", rendered.HTML)
	}
	if rendered.ReviewStatus == "" {
		t.Fatalf("expected the review status with the rendered email")
	}
}

func TestGetRenderedEmailRejectsArchivedEmail(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET archived_at = now()
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to archive test email: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodGet, "/api/emails/"+emailID+"/rendered", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected GET status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestDuplicateEmail(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailForDuplicate(t, dbpool, emailID)

	_, err := dbpool.Exec(context.Background(), `
		INSERT INTO comments (
			email_id,
			review_block,
			selected_text,
			start_offset,
			end_offset,
			body,
			status
		)
		VALUES ($1, 'body-001', 'selected text', 0, 13, 'Source comment', 'open');
	`, emailID)
	if err != nil {
		t.Fatalf("failed to seed source comment: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	requestBody := []byte(`{
		"language": "ES",
		"variant": "new",
		"title": "Duplicated Email",
		"subject": "Duplicated subject",
		"preheader": "Duplicated preheader"
	}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/duplicate",
		bytes.NewReader(requestBody),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", response.Code, response.Body.String())
	}

	var created EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode duplicated email: %v", err)
	}
	if created.ID == "" || created.ID == emailID {
		t.Fatalf("expected duplicated email to have a new id, got %q", created.ID)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, created.ID)
	})
	if created.Slug != "comment-test-email-es" {
		t.Fatalf("expected duplicated slug comment-test-email-es, got %q", created.Slug)
	}
	if created.Language != "es" {
		t.Fatalf("expected duplicated language es, got %q", created.Language)
	}
	if created.Variant != "new" {
		t.Fatalf("expected duplicated variant new, got %q", created.Variant)
	}
	if created.ReviewStatus != "draft" {
		t.Fatalf("expected duplicated review status draft, got %q", created.ReviewStatus)
	}
	if created.Title != "Duplicated Email" {
		t.Fatalf("expected duplicated title override, got %q", created.Title)
	}
	if created.Subject == nil || *created.Subject != "Duplicated subject" {
		t.Fatalf("expected duplicated subject override, got %#v", created.Subject)
	}
	if created.Preheader == nil || *created.Preheader != "Duplicated preheader" {
		t.Fatalf("expected duplicated preheader override, got %#v", created.Preheader)
	}
	assertJSONContainsField(t, created.EditableFields, "primary_cta_text")

	var commentCount int
	err = dbpool.QueryRow(context.Background(), `
		SELECT count(*)::int
		FROM comments
		WHERE email_id = $1;
	`, created.ID).Scan(&commentCount)
	if err != nil {
		t.Fatalf("failed to count duplicated comments: %v", err)
	}
	if commentCount != 0 {
		t.Fatalf("expected duplicated email to have no comments, got %d", commentCount)
	}

	var actorEmail string
	var metadataJSON []byte
	err = dbpool.QueryRow(context.Background(), `
		SELECT actor_email, metadata
		FROM email_events
		WHERE email_id = $1
			AND action = 'email_duplicated'
		ORDER BY created_at DESC
		LIMIT 1;
	`, created.ID).Scan(&actorEmail, &metadataJSON)
	if err != nil {
		t.Fatalf("failed to load duplicate event: %v", err)
	}
	if actorEmail != user.Email {
		t.Fatalf("expected duplicate event actor %s, got %s", user.Email, actorEmail)
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		t.Fatalf("failed to decode duplicate event metadata: %v", err)
	}
	if metadata["source_email_id"] != emailID {
		t.Fatalf("expected source_email_id %s, got %#v", emailID, metadata["source_email_id"])
	}
	if metadata["created_email_id"] != created.ID {
		t.Fatalf("expected created_email_id %s, got %#v", created.ID, metadata["created_email_id"])
	}
	if metadata["language"] != "es" {
		t.Fatalf("expected duplicate event language es, got %#v", metadata["language"])
	}
}

func TestDuplicateEmailAsCreatesLanguageVersionAndAdaptation(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailForDuplicate(t, dbpool, emailID)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	requestBody := []byte(`{
		"language": "ES",
		"variant": "v2",
		"adaptation_label": "Legal",
		"title": "Legal ES v2",
		"subject": "Legal subject",
		"preheader": "Legal preheader"
	}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/duplicate-as",
		bytes.NewReader(requestBody),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", response.Code, response.Body.String())
	}

	var created EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode duplicated email: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, created.ID)
	})

	// A new title builds the slug from board, stage and title (see newEmailSlug),
	// so "Legal ES v2" plus language and version gives "es-v2" twice.
	if created.Slug != "onboarding-registered-legal-es-v2-es-v2--legal" {
		t.Fatalf("expected duplicate-as slug onboarding-registered-legal-es-v2-es-v2--legal, got %q", created.Slug)
	}
	if created.Language != "es" {
		t.Fatalf("expected language es, got %q", created.Language)
	}
	if created.Variant != "v2" {
		t.Fatalf("expected variant v2, got %q", created.Variant)
	}
	if created.AdaptationKey != "legal" {
		t.Fatalf("expected adaptation key legal, got %q", created.AdaptationKey)
	}
	if created.AdaptationLabel != "Legal" {
		t.Fatalf("expected adaptation label Legal, got %q", created.AdaptationLabel)
	}
	if created.Title != "Legal ES v2" {
		t.Fatalf("expected title override, got %q", created.Title)
	}
	if created.Subject == nil || *created.Subject != "Legal subject" {
		t.Fatalf("expected subject override, got %#v", created.Subject)
	}
	if created.Preheader == nil || *created.Preheader != "Legal preheader" {
		t.Fatalf("expected preheader override, got %#v", created.Preheader)
	}

	var metadataJSON []byte
	err := dbpool.QueryRow(context.Background(), `
		SELECT metadata
		FROM email_events
		WHERE email_id = $1
			AND action = 'email_duplicated'
		ORDER BY created_at DESC
		LIMIT 1;
	`, created.ID).Scan(&metadataJSON)
	if err != nil {
		t.Fatalf("failed to load duplicate-as event: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		t.Fatalf("failed to decode duplicate-as event metadata: %v", err)
	}
	if metadata["mode"] != "duplicate_as" {
		t.Fatalf("expected duplicate-as event mode, got %#v", metadata["mode"])
	}
	if metadata["adaptation_key"] != "legal" {
		t.Fatalf("expected adaptation_key legal, got %#v", metadata["adaptation_key"])
	}
}

func TestDuplicateEmailAsCanTargetBoardStageAndNewEvent(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailForDuplicate(t, dbpool, emailID)
	createTestBoard(t, dbpool, "target-board", []string{"qualified"})

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	requestBody := []byte(`{
		"sequence": "target-board",
		"stage": "qualified",
		"sort_order": 7,
		"language": "fr",
		"title": "New Target Event"
	}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/duplicate-as",
		bytes.NewReader(requestBody),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", response.Code, response.Body.String())
	}

	var created EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode duplicated email: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, created.ID)
	})

	if created.Sequence != "target-board" {
		t.Fatalf("expected target board, got %q", created.Sequence)
	}
	if created.Stage != "qualified" {
		t.Fatalf("expected target stage, got %q", created.Stage)
	}
	if created.SortOrder != 7 {
		t.Fatalf("expected target sort order 7, got %d", created.SortOrder)
	}
	if created.Title != "New Target Event" {
		t.Fatalf("expected target title, got %q", created.Title)
	}
	if created.Slug != "target-board-qualified-new-target-event-fr" {
		t.Fatalf("expected target slug, got %q", created.Slug)
	}
}

func TestDuplicateEmailConflict(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailForDuplicate(t, dbpool, emailID)
	conflictingID := createTestEmailWithSlug(t, dbpool, "comment-test-email-es")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/duplicate",
		bytes.NewReader([]byte(`{ "language": "es", "variant": "new" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("expected POST status 409, got %d: %s", response.Code, response.Body.String())
	}

	var stillExists bool
	err := dbpool.QueryRow(context.Background(), `
		SELECT EXISTS(SELECT 1 FROM emails WHERE id = $1);
	`, conflictingID).Scan(&stillExists)
	if err != nil {
		t.Fatalf("failed to check conflicting email: %v", err)
	}
	if !stillExists {
		t.Fatal("conflicting email should not be modified or deleted")
	}
}

func TestDuplicateEmailNotFound(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/00000000-0000-0000-0000-000000000000/duplicate",
		bytes.NewReader([]byte(`{ "language": "es" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected POST status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestDuplicateEmailRejectsMissingLanguage(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/duplicate",
		bytes.NewReader([]byte(`{ "variant": "new" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected POST status 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestDuplicateEmailAllowsCustomVersion(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/duplicate",
		bytes.NewReader([]byte(`{ "language": "es", "variant": "draft" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", response.Code, response.Body.String())
	}

	var created EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode duplicated email: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, created.ID)
	})
	if created.Variant != "draft" {
		t.Fatalf("expected duplicated variant draft, got %q", created.Variant)
	}
	if created.Slug != "comment-test-email-es-draft" {
		t.Fatalf("expected duplicated slug comment-test-email-es-draft, got %q", created.Slug)
	}
}

func TestDuplicateEmailPreservesAdaptation(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailForDuplicate(t, dbpool, emailID)
	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET slug = 'comment-test-email-en--uae',
			adaptation_key = 'uae',
			adaptation_label = 'UAE'
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to prepare adaptation source email: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/duplicate",
		bytes.NewReader([]byte(`{ "language": "es", "variant": "v2" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", response.Code, response.Body.String())
	}

	var created EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode duplicated email: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, created.ID)
	})
	if created.Slug != "comment-test-email-es-v2--uae" {
		t.Fatalf("expected duplicated adaptation slug comment-test-email-es-v2--uae, got %q", created.Slug)
	}
	if created.Language != "es" || created.Variant != "v2" {
		t.Fatalf("expected duplicated es/v2, got %s/%s", created.Language, created.Variant)
	}
	if created.AdaptationKey != "uae" || created.AdaptationLabel != "UAE" {
		t.Fatalf("expected duplicated UAE adaptation, got %s/%s", created.AdaptationKey, created.AdaptationLabel)
	}
}

func TestDuplicateEmailRejectsReviewer(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "reviewer")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/duplicate",
		bytes.NewReader([]byte(`{ "language": "es" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected POST status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateEmailAdaptation(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailForDuplicate(t, dbpool, emailID)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/emails/"+emailID+"/adaptations",
		bytes.NewReader([]byte(`{ "label": "UAE" }`)),
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", response.Code, response.Body.String())
	}

	var created EmailDetail
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode adaptation: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, created.ID)
	})
	if created.Slug != "comment-test-email-en--uae" {
		t.Fatalf("expected adaptation slug comment-test-email-en--uae, got %q", created.Slug)
	}
	if created.Language != "en" || created.Variant != "new" {
		t.Fatalf("expected adaptation to preserve en/new, got %s/%s", created.Language, created.Variant)
	}
	if created.AdaptationKey != "uae" || created.AdaptationLabel != "UAE" {
		t.Fatalf("expected uae/UAE adaptation, got %s/%s", created.AdaptationKey, created.AdaptationLabel)
	}
	if created.ReviewStatus != "draft" {
		t.Fatalf("expected adaptation review status draft, got %q", created.ReviewStatus)
	}
	assertJSONContainsField(t, created.EditableFields, "primary_cta_text")

	var commentCount int
	if err := dbpool.QueryRow(context.Background(), `
		SELECT count(*)::int FROM comments WHERE email_id = $1;
	`, created.ID).Scan(&commentCount); err != nil {
		t.Fatalf("failed to count adaptation comments: %v", err)
	}
	if commentCount != 0 {
		t.Fatalf("expected adaptation to have no comments, got %d", commentCount)
	}
}

func TestCreateEmailAdaptationConflict(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")
	setTestEmailForDuplicate(t, dbpool, emailID)
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE slug = 'comment-test-email-en--uae';`)
	})

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	for index := 0; index < 2; index++ {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/emails/"+emailID+"/adaptations",
			bytes.NewReader([]byte(`{ "label": "UAE" }`)),
		)
		request = withAuthUser(request, user)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if index == 0 && response.Code != http.StatusCreated {
			t.Fatalf("expected first POST status 201, got %d: %s", response.Code, response.Body.String())
		}
		if index == 1 && response.Code != http.StatusConflict {
			t.Fatalf("expected second POST status 409, got %d: %s", response.Code, response.Body.String())
		}
	}
}

func TestArchiveEmail(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/archive",
		nil,
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected PATCH status 204, got %d: %s", response.Code, response.Body.String())
	}

	var archivedBy string
	err := dbpool.QueryRow(context.Background(), `
		SELECT archived_by
		FROM emails
		WHERE id = $1
			AND archived_at IS NOT NULL;
	`, emailID).Scan(&archivedBy)
	if err != nil {
		t.Fatalf("expected email to be archived: %v", err)
	}
	if archivedBy != user.ID {
		t.Fatalf("expected archived_by %s, got %s", user.ID, archivedBy)
	}

	var actorEmail string
	var metadataJSON []byte
	err = dbpool.QueryRow(context.Background(), `
		SELECT actor_email, metadata
		FROM email_events
		WHERE email_id = $1
			AND action = 'email_archived'
		ORDER BY created_at DESC
		LIMIT 1;
	`, emailID).Scan(&actorEmail, &metadataJSON)
	if err != nil {
		t.Fatalf("failed to load archive event: %v", err)
	}
	if actorEmail != user.Email {
		t.Fatalf("expected archive event actor %s, got %s", user.Email, actorEmail)
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		t.Fatalf("failed to decode archive event metadata: %v", err)
	}
	if metadata["title"] != "Comment Test Email" {
		t.Fatalf("expected archive event title Comment Test Email, got %#v", metadata["title"])
	}
	if metadata["slug"] != "comment-test-email" {
		t.Fatalf("expected archive event slug comment-test-email, got %#v", metadata["slug"])
	}
}

func TestListEmailEventsRequiresSuperAdmin(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	admin := createTestUserWithRole(t, dbpool, "admin")
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")

	_, err := dbpool.Exec(context.Background(), `
		INSERT INTO email_events (
			actor_user_id,
			actor_email,
			action,
			email_id,
			email_slug,
			email_title,
			metadata,
			changes
		)
		VALUES ($1, $2, 'email_updated', $3, 'comment-test-email', 'Test Email', '{}'::jsonb, '{"title":{"before":"Old","after":"New"}}'::jsonb);
	`, admin.ID, admin.Email, emailID)
	if err != nil {
		t.Fatalf("failed to insert email event: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	adminRequest := httptest.NewRequest(http.MethodGet, "/api/admin/email-events", nil)
	adminRequest = withAuthUser(adminRequest, admin)
	adminResponse := httptest.NewRecorder()
	router.ServeHTTP(adminResponse, adminRequest)
	if adminResponse.Code != http.StatusForbidden {
		t.Fatalf("expected admin GET status 403, got %d: %s", adminResponse.Code, adminResponse.Body.String())
	}

	superRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/admin/email-events?action=email_updated&actor_email=email-handler-admin&email=Test&limit=50",
		nil,
	)
	superRequest = withAuthUser(superRequest, superAdmin)
	superResponse := httptest.NewRecorder()
	router.ServeHTTP(superResponse, superRequest)
	if superResponse.Code != http.StatusOK {
		t.Fatalf("expected super admin GET status 200, got %d: %s", superResponse.Code, superResponse.Body.String())
	}

	var events []EmailEventItem
	if err := json.NewDecoder(superResponse.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode email events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one email event")
	}
	if events[0].Action != "email_updated" {
		t.Fatalf("expected email_updated event, got %q", events[0].Action)
	}
	if events[0].ActorEmail != admin.Email {
		t.Fatalf("expected event actor %s, got %s", admin.Email, events[0].ActorEmail)
	}
}

func TestArchiveEmailRejectsReviewer(t *testing.T) {
	dbpool := testDBPool(t)
	emailID := createTestEmail(t, dbpool)
	user := createTestUserWithRole(t, dbpool, "reviewer")

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/emails/"+emailID+"/archive",
		nil,
	)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected PATCH status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func setTestEmailTemplate(t *testing.T, dbpool *pgxpool.Pool, emailID string, templateHTML string) {
	t.Helper()

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET template_html = $2
		WHERE id = $1;
	`, emailID, templateHTML)
	if err != nil {
		t.Fatalf("failed to update test email template: %v", err)
	}
}

type testCommentAnchor struct {
	ReviewBlock  string
	SelectedText string
	StartOffset  int
	EndOffset    int
	Status       string
}

func loadTestCommentAnchorsByBody(t *testing.T, dbpool *pgxpool.Pool, emailID string) map[string]testCommentAnchor {
	t.Helper()

	rows, err := dbpool.Query(context.Background(), `
		SELECT body, review_block, selected_text, start_offset, end_offset, status
		FROM comments
		WHERE email_id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to load comment anchors: %v", err)
	}
	defer rows.Close()

	anchors := map[string]testCommentAnchor{}
	for rows.Next() {
		var body string
		var anchor testCommentAnchor
		if err := rows.Scan(
			&body,
			&anchor.ReviewBlock,
			&anchor.SelectedText,
			&anchor.StartOffset,
			&anchor.EndOffset,
			&anchor.Status,
		); err != nil {
			t.Fatalf("failed to scan comment anchor: %v", err)
		}
		anchors[body] = anchor
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed to read comment anchors: %v", err)
	}

	return anchors
}

func assertCommentAnchor(
	t *testing.T,
	anchors map[string]testCommentAnchor,
	body string,
	reviewBlock string,
	selectedText string,
	startOffset int,
	endOffset int,
	status string,
) {
	t.Helper()

	anchor, ok := anchors[body]
	if !ok {
		t.Fatalf("expected comment %q, got %#v", body, anchors)
	}
	if anchor.ReviewBlock != reviewBlock ||
		anchor.SelectedText != selectedText ||
		anchor.StartOffset != startOffset ||
		anchor.EndOffset != endOffset ||
		anchor.Status != status {
		t.Fatalf(
			"unexpected anchor for %q: got %#v, want block=%q selected=%q start=%d end=%d status=%q",
			body,
			anchor,
			reviewBlock,
			selectedText,
			startOffset,
			endOffset,
			status,
		)
	}
}

func setTestEmailForDuplicate(t *testing.T, dbpool *pgxpool.Pool, emailID string) {
	t.Helper()

	_, err := dbpool.Exec(context.Background(), `
		UPDATE emails
		SET
			slug = 'comment-test-email-en',
			sequence = 'onboarding',
			title = 'Source Email',
			subject = 'Source subject',
			preheader = 'Source preheader',
			send_timing = 'day 1',
			stage = 'registered',
			sort_order = 10,
			language = 'en',
			variant = 'new',
			body_text = 'Source body text',
			content_parts = '{"subject":"Source subject","preheader":"Source preheader","body_text":"Source body text"}'::jsonb,
			original_html = '<html><body><a href="https://example.com">Old CTA</a></body></html>',
			review_html = '<html><body><a data-review-block="primary_cta" href="https://example.com">Old CTA</a></body></html>',
			template_html = '<html><body><a data-review-block="primary_cta" data-edit-text="primary_cta_text" href="https://example.com">Old CTA</a></body></html>',
			template_hash = 'test-template-hash',
			template_version = 'v1',
			editable_fields = '{"primary_cta_text":{"type":"text","value":"Old CTA"}}'::jsonb
		WHERE id = $1;
	`, emailID)
	if err != nil {
		t.Fatalf("failed to prepare duplicate source email: %v", err)
	}
}

func createTestEmailWithSlug(t *testing.T, dbpool *pgxpool.Pool, slug string) string {
	t.Helper()

	var emailID string
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO emails (
			slug,
			title,
			stage,
			sort_order,
			language,
			variant,
			original_html,
			review_html,
			template_html,
			editable_fields
		)
		VALUES (
			$1,
			'Conflicting Email',
			'test',
			9998,
			'es',
			'new',
			'<html><body>conflict</body></html>',
			'<html><body><p data-review-block="body-001">conflict</p></body></html>',
			'<html><body><p data-review-block="body-001">conflict</p></body></html>',
			'{}'::jsonb
		)
		RETURNING id;
	`, slug).Scan(&emailID)
	if err != nil {
		t.Fatalf("failed to create conflicting test email: %v", err)
	}

	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, emailID)
	})

	return emailID
}

func createTestUserWithRole(t *testing.T, dbpool *pgxpool.Pool, role string) AuthUser {
	t.Helper()

	var user AuthUser
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO users (email, role)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET
			role = EXCLUDED.role,
			updated_at = now()
		RETURNING id, email, role;
	`, "email-handler-"+role+"@example.com", role).Scan(&user.ID, &user.Email, &user.Role)
	if err != nil {
		t.Fatalf("failed to create %s test user: %v", role, err)
	}

	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM users WHERE id = $1;`, user.ID)
	})

	return user
}

func insertTestEmailVersion(t *testing.T, dbpool *pgxpool.Pool, emailID string, actorEmail string, source string) string {
	t.Helper()

	var versionID string
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO email_versions (
			email_id,
			version_number,
			created_by_email,
			source,
			title,
			subject,
			preheader,
			original_html,
			template_html,
			editable_fields
		)
		SELECT
			id,
			coalesce((SELECT max(version_number) FROM email_versions WHERE email_id = emails.id), 0) + 1,
			$2,
			$3,
			title,
			subject,
			preheader,
			original_html,
			template_html,
			editable_fields
		FROM emails
		WHERE id = $1
		RETURNING id;
	`, emailID, actorEmail, source).Scan(&versionID)
	if err != nil {
		t.Fatalf("failed to insert test email version: %v", err)
	}

	return versionID
}

func withAuthUser(request *http.Request, user AuthUser) *http.Request {
	return request.WithContext(auth.WithUser(request.Context(), user))
}

func loadTestEmailEditableFields(t *testing.T, dbpool *pgxpool.Pool, emailID string) map[string]struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
} {
	t.Helper()

	var fieldsJSON []byte
	err := dbpool.QueryRow(context.Background(), `
		SELECT editable_fields
		FROM emails
		WHERE id = $1;
	`, emailID).Scan(&fieldsJSON)
	if err != nil {
		t.Fatalf("failed to load editable fields: %v", err)
	}

	var fields map[string]struct {
		Type  string `json:"type"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		t.Fatalf("failed to decode editable fields: %v", err)
	}

	return fields
}

func emailListContains(emails []EmailListItem, emailID string) bool {
	for _, email := range emails {
		if email.ID == emailID {
			return true
		}
	}

	return false
}

func assertEditableField(
	t *testing.T,
	fields map[string]struct {
		Type  string `json:"type"`
		Value any    `json:"value"`
	},
	key string,
	fieldType string,
	value any,
) {
	t.Helper()

	field, ok := fields[key]
	if !ok {
		t.Fatalf("field %q is missing", key)
	}
	if field.Type != fieldType {
		t.Fatalf("field %q type = %q, want %q", key, field.Type, fieldType)
	}
	if field.Value != value {
		t.Fatalf("field %q value = %#v, want %#v", key, field.Value, value)
	}
}

func assertJSONContainsField(t *testing.T, raw json.RawMessage, key string) {
	t.Helper()

	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("failed to decode raw JSON fields: %v", err)
	}
	if _, ok := fields[key]; !ok {
		t.Fatalf("expected JSON fields to contain %q, got %#v", key, fields)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
