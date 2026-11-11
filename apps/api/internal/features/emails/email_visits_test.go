package emails

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func listedEmail(t *testing.T, router http.Handler, user AuthUser, id string) EmailListItem {
	t.Helper()
	response := serveVisitRequest(router, user, http.MethodGet, "/api/emails?board=visit-test")
	if response.Code != http.StatusOK {
		t.Fatalf("list: %d %s", response.Code, response.Body.String())
	}
	var emails []EmailListItem
	if err := json.NewDecoder(response.Body).Decode(&emails); err != nil {
		t.Fatal(err)
	}
	for _, email := range emails {
		if email.ID == id {
			return email
		}
	}
	t.Fatalf("email %s is not in the list", id)
	return EmailListItem{}
}

func TestChangedSinceMyVisit(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	router := visitRouter(dbpool)
	viewer := createTestUserWithRole(t, dbpool, "reviewer")
	editor := createTestUserWithRole(t, dbpool, "admin")
	emailID := createTestEmailWithSlug(t, dbpool, "visit-test-email")
	t.Cleanup(func() { _, _ = dbpool.Exec(ctx, `DELETE FROM email_events WHERE email_id = $1;`, emailID) })
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := dbpool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	event := func(actor AuthUser, action, changes string, minutesAgo int) {
		t.Helper()
		exec(`INSERT INTO email_events (actor_user_id, actor_email, action, email_id, changes, created_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, now() - make_interval(mins => $6));`,
			actor.ID, actor.Email, action, emailID, changes, minutesAgo)
	}
	exec(`UPDATE emails SET sequence = 'visit-test' WHERE id = $1;`, emailID)
	exec(`UPDATE users SET changes_tracked_since = now() - interval '3 hours' WHERE id = $1;`, viewer.ID)

	// An edit before marks started is not new; the viewer's own edit never is.
	event(editor, "email_updated", "{}", 300)
	event(viewer, "email_updated", "{}", 60)
	if listedEmail(t, router, viewer, emailID).ChangedSinceMyVisit {
		t.Fatal("expected no mark for an old edit and the viewer's own")
	}

	// Someone else's edit after marks started shows on an email never opened.
	event(editor, "email_updated", "{}", 120)
	if !listedEmail(t, router, viewer, emailID).ChangedSinceMyVisit {
		t.Fatal("expected a mark for someone else's edit")
	}

	// Opening the email clears it; the visit before is kept across a reopen
	// within 30 minutes.
	exec(`INSERT INTO email_views (user_id, email_id, viewed_at) VALUES ($1, $2, now() - interval '2 hours');`, viewer.ID, emailID)
	if code := serveVisitRequest(router, viewer, http.MethodPost, "/api/emails/"+emailID+"/visit").Code; code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", code)
	}
	serveVisitRequest(router, viewer, http.MethodPost, "/api/emails/"+emailID+"/visit")
	listed := listedEmail(t, router, viewer, emailID)
	if listed.ChangedSinceMyVisit || listed.MyPreviousViewedAt == nil || listed.MyViewedAt == nil {
		t.Fatalf("expected the mark gone and the previous visit kept, got %+v", listed)
	}

	// Being made owner by someone else is recorded.
	event(editor, "email_planning_updated", `{"owner_email": {"before": null, "after": "`+viewer.Email+`"}}`, 5)
	if listedEmail(t, router, viewer, emailID).AssignedToMeAt == nil {
		t.Fatal("expected assigned_to_me_at")
	}

	if code := serveVisitRequest(router, viewer, http.MethodPost, "/api/emails/00000000-0000-0000-0000-000000000000/visit").Code; code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing email, got %d", code)
	}
}

func visitRouter(dbpool *pgxpool.Pool) http.Handler {
	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	return router
}

func serveVisitRequest(router http.Handler, user AuthUser, method, path string) *httptest.ResponseRecorder {
	request := withAuthUser(httptest.NewRequest(method, path, nil), user)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
