package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestParsePortalUser(t *testing.T) {
	wrapped, err := parsePortalUser([]byte(`{"data":{"id":"7","email":" Ann.Lee@Example.com ","name":"Ann","lastName":"Lee"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if wrapped != (portalUser{Email: "ann.lee@example.com", Name: "Ann Lee"}) {
		t.Errorf("wrapped: %+v", wrapped)
	}

	flat, err := parsePortalUser([]byte(`{"email":"bo@example.com","name":"","lastName":"Bo"}`))
	if err != nil {
		t.Fatal(err)
	}
	if flat != (portalUser{Email: "bo@example.com", Name: "Bo"}) {
		t.Errorf("flat: %+v", flat)
	}
}

func TestGatewayIdentityIsTrustedOnlyFromTheLoopback(t *testing.T) {
	cases := []struct {
		remote string
		header string
		want   bool
	}{
		{"127.0.0.1:51000", "681146", true},
		{"[::1]:51000", "681146", true},
		{"10.0.0.5:51000", "681146", false},
		{"127.0.0.1:51000", "", false},
		{"127.0.0.1:51000", "1 OR 1=1", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		r.RemoteAddr = c.remote
		r.Header.Set("X-Vibe-User-Id", c.header)
		if _, got := gatewayIdentity(r); got != c.want {
			t.Errorf("%s with %q: trusted %v, want %v", c.remote, c.header, got, c.want)
		}
	}
}

// gatewayRequest is a request as the gateway's tunnel delivers it.
func gatewayRequest(path string, portalUserID string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.RemoteAddr = "127.0.0.1:51000"
	r.Header.Set("X-Vibe-User-Id", portalUserID)
	return r
}

func useGatewayLookup(t *testing.T, users map[string]portalUser) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	lookupPortalUser = func(ctx context.Context, portalUserID string) (portalUser, error) {
		calls.Add(1)
		user, ok := users[portalUserID]
		if !ok {
			return portalUser{}, errors.New("not found")
		}
		return user, nil
	}
	gatewayLinks = &gatewayLinker{inFlight: map[string]chan struct{}{}, denied: map[string]time.Time{}}
	t.Cleanup(func() { lookupPortalUser = fetchPortalUser })
	return &calls
}

func TestGatewaySignInLinksByEmail(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	t.Setenv("AUTH_MODE", "vibe")
	t.Setenv("AUTH_ALLOWED_DOMAINS", "example.com")

	const (
		newEmail      = "gateway-new@example.com"
		knownEmail    = "gateway-known@elsewhere.test"
		outsiderEmail = "gateway-outsider@elsewhere.test"
	)
	for _, email := range []string{newEmail, knownEmail, outsiderEmail} {
		deleteTestUserByEmail(t, dbpool, email)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO users (email, role, name) VALUES ($1, 'admin', 'Kept Name');`, knownEmail); err != nil {
		t.Fatal(err)
	}
	calls := useGatewayLookup(t, map[string]portalUser{
		"9000001": {Email: newEmail, Name: "New Person"},
		"9000002": {Email: knownEmail, Name: "Portal Name"},
		"9000003": {Email: outsiderEmail, Name: "Out Sider"},
	})

	router := chi.NewRouter()
	router.Use(Middleware(dbpool))
	RegisterAuthRoutes(router, dbpool, nil)
	me := func(portalUserID string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, gatewayRequest("/api/auth/me", portalUserID))
		return response
	}

	// Someone new from an allowed domain becomes a reviewer on the first visit.
	if response := me("9000001"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), newEmail) {
		t.Fatalf("new user: %d %s", response.Code, response.Body.String())
	}
	var role, name string
	if err := dbpool.QueryRow(ctx, `SELECT role, name FROM users WHERE portal_user_id = '9000001';`).Scan(&role, &name); err != nil {
		t.Fatal(err)
	}
	if role != "reviewer" || name != "New Person" {
		t.Errorf("new user: role %q, name %q", role, name)
	}

	// A linked ID needs no lookup.
	before := calls.Load()
	if response := me("9000001"); response.Code != http.StatusOK {
		t.Fatalf("linked user: %d", response.Code)
	}
	if calls.Load() != before {
		t.Error("a linked user was looked up again")
	}

	// An existing user outside the allowed domains keeps their role and name.
	if response := me("9000002"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"role":"admin"`) {
		t.Fatalf("known user: %d %s", response.Code, response.Body.String())
	}
	if err := dbpool.QueryRow(ctx, `SELECT name FROM users WHERE email = $1;`, knownEmail).Scan(&name); err != nil || name != "Kept Name" {
		t.Errorf("known user's name: %q, %v", name, err)
	}

	// Someone unknown outside the allowed domains gets no access, and is not
	// looked up again on every request.
	if response := me("9000003"); response.Code != http.StatusForbidden {
		t.Fatalf("outsider: %d %s", response.Code, response.Body.String())
	}
	before = calls.Load()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, gatewayRequest("/api/boards", "9000003"))
	if response.Code != http.StatusForbidden || calls.Load() != before {
		t.Errorf("outsider again: %d, lookups %d → %d", response.Code, before, calls.Load())
	}

	// The same header from anywhere but the tunnel signs nobody in.
	request := gatewayRequest("/api/auth/me", "9000001")
	request.RemoteAddr = "10.0.0.5:51000"
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("from outside: %d", response.Code)
	}
}

func TestMergeKeepsThePortalAccount(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	const intoEmail, fromEmail = "gateway-merge-into@example.com", "gateway-merge-from@example.com"
	deleteTestUserByEmail(t, dbpool, intoEmail)
	deleteTestUserByEmail(t, dbpool, fromEmail)
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM user_email_aliases WHERE email = $1;`, fromEmail)
	})

	var intoID, fromID string
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ($1, 'admin') RETURNING id;`, intoEmail).Scan(&intoID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (email, role, portal_user_id) VALUES ($1, 'reviewer', '9000004') RETURNING id;`, fromEmail).Scan(&fromID); err != nil {
		t.Fatal(err)
	}

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := mergeUsers(ctx, tx, intoID, fromID, "test"); err != nil {
		t.Fatal(err)
	}
	var portalUserID *string
	if err := tx.QueryRow(ctx, `SELECT portal_user_id FROM users WHERE id = $1;`, intoID).Scan(&portalUserID); err != nil {
		t.Fatal(err)
	}
	if portalUserID == nil || *portalUserID != "9000004" {
		t.Errorf("portal_user_id after merge: %v", portalUserID)
	}
}
