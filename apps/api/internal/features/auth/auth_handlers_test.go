package auth

import (
	"bytes"
	"context"

	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

type recordingEmailSender struct {
	calls int
}

func (sender *recordingEmailSender) SendLoginCode(_ context.Context, _ string, _ string) error {
	sender.calls++
	return nil
}

func TestRequestOTPCodeDisallowedDomainDoesNotSendEmail(t *testing.T) {
	t.Setenv("AUTH_ALLOWED_DOMAIN", "acme.example")

	emailSender := &recordingEmailSender{}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/request-code",
		strings.NewReader(`{"email":"user@example.com"}`),
	)
	response := httptest.NewRecorder()

	requestOTPCodeHandler(nil, emailSender, newOTPRequestLimiter()).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected generic success, got %d", response.Code)
	}
	if emailSender.calls != 0 {
		t.Fatalf("expected email sender not to be called, got %d calls", emailSender.calls)
	}
}

func TestIsAllowedAuthEmailUsesDomainList(t *testing.T) {
	t.Setenv("AUTH_ALLOWED_DOMAIN", "")
	t.Setenv("AUTH_ALLOWED_DOMAINS", "acme.example, @bitrix24.com, bitrix24.com.br")

	for _, email := range []string{
		"user@acme.example",
		"user@bitrix24.com",
		"user@bitrix24.com.br",
	} {
		if !isAllowedAuthEmail(email) {
			t.Fatalf("expected %s to be allowed", email)
		}
	}

	if isAllowedAuthEmail("user@example.com") {
		t.Fatalf("expected example.com to be disallowed")
	}
}

func TestIsAllowedAuthEmailFallsBackToSingleDomain(t *testing.T) {
	t.Setenv("AUTH_ALLOWED_DOMAIN", "acme.example")
	t.Setenv("AUTH_ALLOWED_DOMAINS", "")

	if !isAllowedAuthEmail("user@acme.example") {
		t.Fatalf("expected configured domain to be allowed")
	}
	if isAllowedAuthEmail("user@bitrix24.com") {
		t.Fatalf("expected unconfigured domain to be disallowed")
	}
}

func TestIsValidOTPCodeFormat(t *testing.T) {
	for _, code := range []string{"123456", "000001"} {
		if !isValidOTPCodeFormat(code) {
			t.Fatalf("expected %s to be valid", code)
		}
	}

	for _, code := range []string{"", "12345", "1234567", "abc123", "12 456"} {
		if isValidOTPCodeFormat(code) {
			t.Fatalf("expected %q to be invalid", code)
		}
	}
}

func TestDevLoginDisabledByDefault(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/dev-login",
		strings.NewReader(`{"email":"user@acme.example"}`),
	)
	response := httptest.NewRecorder()

	devLoginHandler(nil).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.Code)
	}
}

func TestDevLoginCreatesSessionWhenEnabled(t *testing.T) {
	t.Setenv("AUTH_DEV_LOGIN_ENABLED", "true")
	t.Setenv("AUTH_ALLOWED_DOMAINS", "acme.example")
	t.Setenv("AUTH_BOOTSTRAP_SUPER_ADMIN_EMAIL", "dev@acme.example")
	dbpool := testDBPool(t)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/dev-login",
		strings.NewReader(`{"email":"dev@acme.example"}`),
	)
	response := httptest.NewRecorder()

	devLoginHandler(dbpool).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName {
		t.Fatalf("expected one session cookie, got %#v", cookies)
	}

	var role string
	err := dbpool.QueryRow(
		context.Background(),
		`SELECT role FROM users WHERE email = $1`,
		"dev@acme.example",
	).Scan(&role)
	if err != nil {
		t.Fatalf("expected dev user to be created: %v", err)
	}
	if role != "super_admin" {
		t.Fatalf("expected bootstrap dev user to be super_admin, got %q", role)
	}
}

func TestDemoLoginDisabledByDefault(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/demo-login",
		nil,
	)
	response := httptest.NewRecorder()

	demoLoginHandler(nil, newDemoLoginLimiter()).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.Code)
	}
}

func TestDemoLoginCreatesSandboxPerVisitor(t *testing.T) {
	t.Setenv("AUTH_DEMO_LOGIN_ENABLED", "true")
	dbpool := testDBPool(t)
	ctx := context.Background()
	handler := demoLoginHandler(dbpool, newDemoLoginLimiter())

	visit := func() string {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/demo-login", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
		}
		cookies := response.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != sessionCookieName {
			t.Fatalf("expected one session cookie, got %#v", cookies)
		}
		if until := time.Until(cookies[0].Expires); until > 25*time.Hour {
			t.Fatalf("expected the session to end with the sandbox, ends in %s", until)
		}
		var email string
		if err := dbpool.QueryRow(ctx, `
			SELECT u.email FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.session_hash = $1;
		`, hashToken(cookies[0].Value)).Scan(&email); err != nil {
			t.Fatal(err)
		}
		return email
	}
	first, second := visit(), visit()
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `
			DELETE FROM email_events WHERE email_id IN (SELECT id FROM emails WHERE sequence IN (SELECT unnest(managed_boards) FROM users WHERE email = ANY($1)));
			`, []string{first, second})
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE sequence IN (SELECT unnest(managed_boards) FROM users WHERE email = ANY($1));`, []string{first, second})
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM boards WHERE key IN (SELECT unnest(managed_boards) FROM users WHERE email = ANY($1));`, []string{first, second})
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM auth_events WHERE email = ANY($1);`, []string{first, second})
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM users WHERE email = ANY($1);`, []string{first, second})
	})
	if first == second {
		t.Fatalf("expected each visitor to get their own user, both got %s", first)
	}

	var role string
	var scoped bool
	var boards []string
	var emails int
	if err := dbpool.QueryRow(ctx, `
		SELECT role, board_scoped, managed_boards,
			(SELECT count(*) FROM emails WHERE sequence = ANY(users.managed_boards))
		FROM users WHERE email = $1;
	`, first).Scan(&role, &scoped, &boards, &emails); err != nil {
		t.Fatal(err)
	}
	if role != "reviewer" || !scoped || len(boards) != 1 || emails == 0 {
		t.Fatalf("expected a scoped reviewer with one seeded board, got %s scoped=%v boards=%v emails=%d", role, scoped, boards, emails)
	}
}

func TestIsAllowedRequestOrigin(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "")
	t.Setenv("AUTH_ALLOWED_ORIGINS", "https://reviewdesk.example.com")

	tests := []struct {
		name         string
		host         string
		origin       string
		secFetchSite string
		want         bool
	}{
		{
			name: "rejects requests without origin",
			host: "localhost:8080",
			want: false,
		},
		{
			name:         "rejects cross site request without origin",
			host:         "localhost:8080",
			secFetchSite: "cross-site",
			want:         false,
		},
		{
			name:         "allows same origin fetch metadata without origin",
			host:         "localhost:8080",
			secFetchSite: "same-origin",
			want:         true,
		},
		{
			name:   "rejects opaque null origin",
			host:   "localhost:8080",
			origin: "null",
			want:   false,
		},
		{
			name:   "allows same host origin",
			host:   "localhost:5173",
			origin: "http://localhost:5173",
			want:   true,
		},
		{
			name:   "allows configured deployment origin",
			host:   "api.example.com",
			origin: "https://reviewdesk.example.com",
			want:   true,
		},
		{
			name:   "rejects cross origin request",
			host:   "reviewdesk.example.com",
			origin: "https://attacker.example.com",
			want:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/emails", nil)
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.secFetchSite != "" {
				request.Header.Set("Sec-Fetch-Site", test.secFetchSite)
			}

			if got := isAllowedRequestOrigin(request); got != test.want {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestIsAllowedRequestOriginFallsBackToCORSOrigin(t *testing.T) {
	t.Setenv("AUTH_ALLOWED_ORIGINS", "")
	t.Setenv("CORS_ORIGIN", "http://localhost:5173")

	request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	request.Host = "localhost:8080"
	request.Header.Set("Origin", "http://localhost:5173")

	if !isAllowedRequestOrigin(request) {
		t.Fatal("expected CORS_ORIGIN to allow local Vite proxy logout")
	}
}

func TestIsRequestCanceledError(t *testing.T) {
	if !IsRequestCanceledError(context.Canceled) {
		t.Fatal("expected context.Canceled to be treated as request cancellation")
	}
	if !IsRequestCanceledError(context.DeadlineExceeded) {
		t.Fatal("expected context.DeadlineExceeded to be treated as request cancellation")
	}
	if !IsRequestCanceledError(errors.Join(errors.New("wrapped"), context.Canceled)) {
		t.Fatal("expected wrapped context.Canceled to be treated as request cancellation")
	}
	if IsRequestCanceledError(errors.New("database unavailable")) {
		t.Fatal("expected unrelated errors to be logged")
	}
}

func TestListAdminUsersRequiresSuperAdmin(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "reviewer")

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, &recordingEmailSender{})

	request := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected GET status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAdminCanListUsers(t *testing.T) {
	dbpool := testDBPool(t)
	user := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, &recordingEmailSender{})

	request := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	request = withAuthUser(request, user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d: %s", response.Code, response.Body.String())
	}
}

func TestSuperAdminCanListAndUpdateUsers(t *testing.T) {
	dbpool := testDBPool(t)
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")
	reviewer := createTestUserWithRole(t, dbpool, "reviewer")

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, &recordingEmailSender{})

	listRequest := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	listRequest = withAuthUser(listRequest, superAdmin)
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)

	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d: %s", listResponse.Code, listResponse.Body.String())
	}

	var users []UserAdminItem
	if err := json.NewDecoder(listResponse.Body).Decode(&users); err != nil {
		t.Fatalf("failed to decode users: %v", err)
	}
	if len(users) == 0 {
		t.Fatal("expected users list to include test users")
	}

	updateRequest := httptest.NewRequest(
		http.MethodPatch,
		"/api/admin/users/"+reviewer.ID+"/role",
		bytes.NewReader([]byte(`{ "role": "admin" }`)),
	)
	updateRequest = withAuthUser(updateRequest, superAdmin)
	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, updateRequest)

	if updateResponse.Code != http.StatusOK {
		t.Fatalf("expected PATCH status 200, got %d: %s", updateResponse.Code, updateResponse.Body.String())
	}

	var updated UserAdminItem
	if err := json.NewDecoder(updateResponse.Body).Decode(&updated); err != nil {
		t.Fatalf("failed to decode updated user: %v", err)
	}
	if updated.ID != reviewer.ID || updated.Role != "admin" {
		t.Fatalf("expected reviewer to become admin, got %#v", updated)
	}
}

func TestAdminCanCreateReviewerUser(t *testing.T) {
	dbpool := testDBPool(t)
	admin := createTestUserWithRole(t, dbpool, "admin")

	deleteTestUserByEmail(t, dbpool, "new.reviewer@example.com")

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, &recordingEmailSender{})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users",
		bytes.NewReader([]byte(`{ "email": "New.Reviewer@Example.com", "role": "reviewer" }`)),
	)
	request = withAuthUser(request, admin)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", response.Code, response.Body.String())
	}

	var created UserAdminItem
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created user: %v", err)
	}
	if created.Email != "new.reviewer@example.com" || created.Role != "reviewer" {
		t.Fatalf("expected normalized reviewer user, got %#v", created)
	}

	var eventCount int
	if err := dbpool.QueryRow(context.Background(), `
		SELECT count(*)::int
		FROM auth_events
		WHERE user_id = $1
			AND email = $2
			AND event_type = 'admin_user_created'
			AND success = true;
	`, created.ID, created.Email).Scan(&eventCount); err != nil {
		t.Fatalf("failed to count created user auth events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("expected one admin_user_created event, got %d", eventCount)
	}
}

func TestSuperAdminCanCreateAdminUser(t *testing.T) {
	dbpool := testDBPool(t)
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")

	deleteTestUserByEmail(t, dbpool, "new-admin@example.com")

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, &recordingEmailSender{})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users",
		bytes.NewReader([]byte(`{ "email": "new-admin@example.com", "role": "admin" }`)),
	)
	request = withAuthUser(request, superAdmin)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected POST status 201, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAdminCannotCreateAdminUser(t *testing.T) {
	dbpool := testDBPool(t)
	admin := createTestUserWithRole(t, dbpool, "admin")

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, &recordingEmailSender{})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users",
		bytes.NewReader([]byte(`{ "email": "blocked-admin@example.com", "role": "admin" }`)),
	)
	request = withAuthUser(request, admin)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected POST status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateAdminUserRejectsDuplicateEmail(t *testing.T) {
	dbpool := testDBPool(t)
	admin := createTestUserWithRole(t, dbpool, "admin")
	existing := createTestUserWithRole(t, dbpool, "reviewer")

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, &recordingEmailSender{})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users",
		bytes.NewReader([]byte(fmt.Sprintf(`{ "email": %q, "role": "reviewer" }`, existing.Email))),
	)
	request = withAuthUser(request, admin)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("expected POST status 409, got %d: %s", response.Code, response.Body.String())
	}
}

func TestSuperAdminCannotDemoteSelf(t *testing.T) {
	dbpool := testDBPool(t)
	superAdmin := createTestUserWithRole(t, dbpool, "super_admin")

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, &recordingEmailSender{})

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/admin/users/"+superAdmin.ID+"/role",
		bytes.NewReader([]byte(`{ "role": "admin" }`)),
	)
	request = withAuthUser(request, superAdmin)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected PATCH status 400, got %d: %s", response.Code, response.Body.String())
	}
}
