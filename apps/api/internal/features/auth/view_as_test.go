package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

// viewAsRouter serves the auth routes plus a GET and a POST that answer with
// the user the middleware put in the context.
func viewAsRouter(dbpool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()
	r.Use(Middleware(dbpool))
	r.Post("/api/auth/view-as", startViewAsHandler(dbpool))
	r.Delete("/api/auth/view-as", stopViewAsHandler())
	r.Get("/api/auth/me", meHandler(dbpool))
	whoami := func(w http.ResponseWriter, r *http.Request) {
		user, _ := coreauth.FromRequest(r)
		_, _ = w.Write([]byte(user.Email))
	}
	r.Get("/api/whoami", whoami)
	r.Post("/api/whoami", whoami)
	r.Get("/api/emails/{id}/ai-analysis-stream", whoami)
	return r
}

func sessionCookieFor(t *testing.T, dbpool *pgxpool.Pool, user AuthUser) *http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	if err := createSessionCookie(context.Background(), dbpool, recorder, user); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	return recorder.Result().Cookies()[0]
}

func serveWithCookies(router http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func startViewAs(t *testing.T, router http.Handler, session *http.Cookie, target AuthUser) *httptest.ResponseRecorder {
	t.Helper()
	return serveWithCookies(router, http.MethodPost, "/api/auth/view-as", `{"user_id":"`+target.ID+`"}`, session)
}

func viewAsCookieFrom(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == viewAsCookieName {
			return cookie
		}
	}
	t.Fatalf("expected a view-as cookie, got %#v", response.Result().Cookies())
	return nil
}

func TestSuperAdminViewsAsReviewerReadOnly(t *testing.T) {
	dbpool := testDBPool(t)
	router := viewAsRouter(dbpool)
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	reviewer := createTestUserWithRole(t, dbpool, "reviewer")
	session := sessionCookieFor(t, dbpool, superAdmin)
	lastSeen := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	if _, err := dbpool.Exec(context.Background(), `UPDATE users SET last_seen_at = $2 WHERE id = $1;`, reviewer.ID, lastSeen); err != nil {
		t.Fatal(err)
	}

	start := startViewAs(t, router, session, reviewer)
	if start.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", start.Code, start.Body.String())
	}
	viewAs := viewAsCookieFrom(t, start)

	if got := serveWithCookies(router, http.MethodGet, "/api/whoami", "", session, viewAs); got.Body.String() != reviewer.Email {
		t.Fatalf("expected GET as %s, got %d %q", reviewer.Email, got.Code, got.Body.String())
	}
	if got := serveWithCookies(router, http.MethodPost, "/api/whoami", "", session, viewAs); got.Code != http.StatusForbidden {
		t.Fatalf("expected POST to be refused while viewing, got %d", got.Code)
	}
	if got := serveWithCookies(router, http.MethodGet, "/api/emails/1/ai-analysis-stream", "", session, viewAs); got.Code != http.StatusForbidden {
		t.Fatalf("expected the AI analysis to be refused while viewing, got %d", got.Code)
	}

	me := serveWithCookies(router, http.MethodGet, "/api/auth/me", "", session, viewAs)
	var body meResponse
	if err := json.NewDecoder(me.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Email != reviewer.Email || body.ViewedBy != superAdmin.Email {
		t.Fatalf("expected me as %s viewed by %s, got %+v", reviewer.Email, superAdmin.Email, body)
	}

	var seen time.Time
	if err := dbpool.QueryRow(context.Background(), `SELECT last_seen_at FROM users WHERE id = $1;`, reviewer.ID).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if !seen.Equal(lastSeen) {
		t.Fatalf("expected the viewed user's last_seen_at to stay %v, got %v", lastSeen, seen)
	}

	stop := serveWithCookies(router, http.MethodDelete, "/api/auth/view-as", "", session, viewAs)
	if cookie := viewAsCookieFrom(t, stop); cookie.MaxAge >= 0 {
		t.Fatalf("expected the view-as cookie to be removed, got %#v", cookie)
	}
	if got := serveWithCookies(router, http.MethodPost, "/api/whoami", "", session); got.Body.String() != superAdmin.Email {
		t.Fatalf("expected POST as %s after stopping, got %d %q", superAdmin.Email, got.Code, got.Body.String())
	}
}

func TestOnlySuperAdminCanViewAs(t *testing.T) {
	dbpool := testDBPool(t)
	router := viewAsRouter(dbpool)
	admin := createTestUserWithRole(t, dbpool, "admin")
	reviewer := createTestUserWithRole(t, dbpool, "reviewer")
	session := sessionCookieFor(t, dbpool, admin)

	if got := startViewAs(t, router, session, reviewer); got.Code != http.StatusForbidden {
		t.Fatalf("expected an admin to be refused, got %d", got.Code)
	}

	// A cookie made by hand does not work for anyone but a super admin.
	forged := viewAsCookie(reviewer.ID, 0)
	if got := serveWithCookies(router, http.MethodPost, "/api/whoami", "", session, forged); got.Body.String() != admin.Email {
		t.Fatalf("expected the forged cookie to be ignored, got %d %q", got.Code, got.Body.String())
	}
}

func TestCannotViewAsSuperAdmin(t *testing.T) {
	dbpool := testDBPool(t)
	router := viewAsRouter(dbpool)
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	session := sessionCookieFor(t, dbpool, superAdmin)

	if got := startViewAs(t, router, session, superAdmin); got.Code != http.StatusBadRequest {
		t.Fatalf("expected viewing as a super admin to be refused, got %d", got.Code)
	}
}
