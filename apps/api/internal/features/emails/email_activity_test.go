package emails

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestEmailActivityIncludesReviewAndAnalysisEvents(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUser(t, dbpool)
	emailID := createTestEmail(t, dbpool)
	otherEmailID := createTestEmailWithSlug(t, dbpool, "activity-other-email")

	var commentID string
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO comments (
			email_id,
			user_id,
			review_block,
			selected_text,
			start_offset,
			end_offset,
			body,
			status,
			severity,
			created_at,
			resolved_at,
			resolved_by
		)
		VALUES (
			$1,
			$2,
			'body-001',
			'selected text',
			0,
			13,
			'Please clarify.',
			'resolved',
			'blocking',
			now() - interval '5 minutes',
			now() - interval '3 minutes',
			$2
		)
		RETURNING id;
	`, emailID, user.ID).Scan(&commentID)
	if err != nil {
		t.Fatalf("failed to seed comment: %v", err)
	}

	_, err = dbpool.Exec(context.Background(), `
		INSERT INTO comment_messages (
			comment_id,
			user_id,
			body,
			created_at,
			updated_at
		)
		VALUES
			($1, $2, 'Please clarify.', now() - interval '5 minutes', now() - interval '5 minutes'),
			($1, $2, 'Clarified in copy.', now() - interval '4 minutes', now() - interval '4 minutes');
	`, commentID, user.ID)
	if err != nil {
		t.Fatalf("failed to seed comment messages: %v", err)
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
			changes,
			created_at
		)
		VALUES
			($1, $2, 'email_review_status_updated', $3, 'comment-test-email', 'Comment Test Email', '{}'::jsonb, '{"review_status":{"before":"in_review","after":"approved"}}'::jsonb, now() - interval '2 minutes'),
			($1, $2, 'email_review_status_updated', $4, 'activity-other-email', 'Other Email', '{}'::jsonb, '{"review_status":{"before":"in_review","after":"approved"}}'::jsonb, now() - interval '1 minute');
	`, user.ID, user.Email, emailID, otherEmailID)
	if err != nil {
		t.Fatalf("failed to seed email events: %v", err)
	}

	var analysisLogID string
	err = dbpool.QueryRow(context.Background(), `
		INSERT INTO ai_analysis_logs (
			email_id,
			user_id,
			user_email,
			model,
			status,
			latency_ms,
			force_refresh,
			created_at
		)
		VALUES ($1, $2, $3, 'gpt-test', 'success', 123, false, now() - interval '1 minute')
		RETURNING id;
	`, emailID, user.ID, user.Email).Scan(&analysisLogID)
	if err != nil {
		t.Fatalf("failed to seed ai analysis log: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM ai_analysis_logs WHERE id = $1;`, analysisLogID)
	})

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodGet, "/api/emails/"+emailID+"/activity", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected activity status 200, got %d: %s", response.Code, response.Body.String())
	}

	var activities []EmailActivityItem
	if err := json.NewDecoder(response.Body).Decode(&activities); err != nil {
		t.Fatalf("failed to decode activity response: %v", err)
	}

	activityTypes := make([]string, 0, len(activities))
	for _, activity := range activities {
		activityTypes = append(activityTypes, activity.Type)
		if activity.ID == "" || activity.Summary == "" {
			t.Fatalf("expected activity id and summary, got %#v", activity)
		}
	}

	assertContainsActivityType(t, activityTypes, "comment_created")
	assertContainsActivityType(t, activityTypes, "comment_replied")
	assertContainsActivityType(t, activityTypes, "comment_resolved")
	assertContainsActivityType(t, activityTypes, "email_review_status_updated")
	assertContainsActivityType(t, activityTypes, "ai_analysis_run")

	if activityTypes[0] != "ai_analysis_run" {
		t.Fatalf("expected newest activity to be ai analysis, got %q in %#v", activityTypes[0], activityTypes)
	}
	if countActivityType(activityTypes, "email_review_status_updated") != 1 {
		t.Fatalf("expected unrelated email event to be excluded, got %#v", activityTypes)
	}
}

func TestEmailActivityReturnsNotFoundForMissingEmail(t *testing.T) {
	dbpool := testDBPool(t)

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)

	request := httptest.NewRequest(http.MethodGet, "/api/emails/00000000-0000-0000-0000-000000000000/activity", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected missing activity status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestEmailEventActivitySummaryMarksStaleApprovalAfterEdit(t *testing.T) {
	summary := emailEventActivitySummary(
		emailEventReviewStatusUpdated,
		[]byte(`{"review_status":{"before":"approved","after":"changes_requested"}}`),
		[]byte(`{"reason":"approval_stale_after_edit"}`),
	)

	if summary != "Marked approval stale after edit" {
		t.Fatalf("expected stale approval summary, got %q", summary)
	}
}

func TestEmailEventActivitySummaryMarksReapprovalAfterStaleEdit(t *testing.T) {
	summary := emailEventActivitySummary(
		emailEventReviewStatusUpdated,
		[]byte(`{"review_status":{"before":"changes_requested","after":"approved"}}`),
		[]byte(`{"reason":"reapproved_after_stale_edit"}`),
	)

	if summary != "Re-approved after stale edit" {
		t.Fatalf("expected reapproval summary, got %q", summary)
	}
}

func assertContainsActivityType(t *testing.T, activityTypes []string, expected string) {
	t.Helper()
	if countActivityType(activityTypes, expected) == 0 {
		t.Fatalf("expected activity type %q in %#v", expected, activityTypes)
	}
}

func countActivityType(activityTypes []string, expected string) int {
	count := 0
	for _, activityType := range activityTypes {
		if activityType == expected {
			count++
		}
	}
	return count
}
