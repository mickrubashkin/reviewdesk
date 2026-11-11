package auth

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestChangesTrackedFromFirstLoadNotWhileViewing(t *testing.T) {
	dbpool := testDBPool(t)
	router := viewAsRouter(dbpool)
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	reviewer := createTestUserWithRole(t, dbpool, "reviewer")
	session := sessionCookieFor(t, dbpool, superAdmin)
	trackedSince := func(user AuthUser) *time.Time {
		t.Helper()
		var at *time.Time
		if err := dbpool.QueryRow(context.Background(), `SELECT changes_tracked_since FROM users WHERE id = $1;`, user.ID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}

	viewAs := viewAsCookieFrom(t, startViewAs(t, router, session, reviewer))
	serveWithCookies(router, http.MethodGet, "/api/auth/me", "", session, viewAs)
	if trackedSince(reviewer) != nil || trackedSince(superAdmin) != nil {
		t.Fatal("expected no start while viewing as someone")
	}

	serveWithCookies(router, http.MethodGet, "/api/auth/me", "", session)
	first := trackedSince(superAdmin)
	if first == nil {
		t.Fatal("expected marks to start on the first load")
	}
	serveWithCookies(router, http.MethodGet, "/api/auth/me", "", session)
	if again := trackedSince(superAdmin); again == nil || !again.Equal(*first) {
		t.Fatalf("expected the start to stay %v, got %v", first, again)
	}
}

func TestMyTasksVisitKeepsThePreviousVisit(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	user := createTestUserWithRole(t, dbpool, "reviewer")
	visit := func() {
		t.Helper()
		response := serveWithCookies(withUserRouter(user, recordMyTasksVisitHandler(dbpool)), http.MethodPost, "/", "")
		if response.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", response.Code)
		}
	}
	seen := func() (current, previous *time.Time) {
		t.Helper()
		if err := dbpool.QueryRow(ctx, `SELECT my_tasks_seen_at, my_tasks_previous_seen_at FROM users WHERE id = $1;`, user.ID).Scan(&current, &previous); err != nil {
			t.Fatal(err)
		}
		return current, previous
	}

	lastVisit := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	if _, err := dbpool.Exec(ctx, `UPDATE users SET my_tasks_seen_at = $2 WHERE id = $1;`, user.ID, lastVisit); err != nil {
		t.Fatal(err)
	}
	visit()
	visit() // a reopen within the same visit
	current, previous := seen()
	if previous == nil || !previous.Equal(lastVisit) {
		t.Fatalf("expected the previous visit %v, got %v", lastVisit, previous)
	}
	if current == nil || time.Since(*current) > time.Minute {
		t.Fatalf("expected the current visit now, got %v", current)
	}
}

// withUserRouter serves handler at / for user, as the auth middleware would.
func withUserRouter(user AuthUser, handler http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, withAuthUser(r, user))
	})
}
