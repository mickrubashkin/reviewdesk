package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
)

type recordingSender struct {
	mu       sync.Mutex
	messages []string
	sent     chan struct{}
}

func (s *recordingSender) Send(ctx context.Context, text string) error {
	s.mu.Lock()
	s.messages = append(s.messages, text)
	s.mu.Unlock()
	s.sent <- struct{}{}
	return nil
}

func TestPossibleDuplicatesAreShownAnnouncedAndDismissed(t *testing.T) {
	t.Setenv("AUTH_DEV_LOGIN_ENABLED", "true")
	t.Setenv("AUTH_ALLOWED_DOMAINS", "acme.example, acme.test")
	t.Setenv("AUTH_APP_URL", "https://reviewdesk.example.com")
	dbpool := testDBPool(t)
	ctx := context.Background()
	admin := createTestUserWithRole(t, dbpool, "super_admin")

	olderEmail, newerEmail := "dup.person@acme.example", "dup.person@acme.test"
	salesA, salesB := "sales@acme.example", "sales@acme.test"
	for _, email := range []string{olderEmail, newerEmail, salesA, salesB} {
		deleteTestUserByEmail(t, dbpool, email)
	}
	var olderID string
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ($1, 'admin') RETURNING id;`, olderEmail).Scan(&olderID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO users (email, role) VALUES ($1, 'reviewer'), ($2, 'reviewer');`, salesA, salesB); err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{sent: make(chan struct{}, 1)}
	Notifier = sender
	t.Cleanup(func() { Notifier = nil })

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, nil)

	// A first sign-in with the same name on another domain is announced.
	login := httptest.NewRequest(http.MethodPost, "/api/auth/dev-login", strings.NewReader(`{"email":"`+newerEmail+`"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, login)
	if response.Code != http.StatusOK {
		t.Fatalf("expected dev login 200, got %d: %s", response.Code, response.Body.String())
	}
	<-sender.sent
	if len(sender.messages) != 1 || !strings.Contains(sender.messages[0], newerEmail) ||
		!strings.Contains(sender.messages[0], olderEmail) ||
		!strings.Contains(sender.messages[0], "https://reviewdesk.example.com/admin/users") {
		t.Fatalf("unexpected announcement: %v", sender.messages)
	}

	var newerID string
	if err := dbpool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1;`, newerEmail).Scan(&newerID); err != nil {
		t.Fatal(err)
	}
	duplicates, err := loadPossibleDuplicates(ctx, dbpool, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := duplicates[newerID]; len(got) != 1 || got[0] != olderID {
		t.Fatalf("expected %s to be a possible duplicate of %s, got %v", newerEmail, olderEmail, got)
	}
	if len(duplicates[olderID]) != 0 {
		t.Fatalf("expected the hint on the newer user only, got %v", duplicates[olderID])
	}
	for id, list := range duplicates {
		if len(list) > 0 && id != newerID {
			var email string
			_ = dbpool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1;`, id).Scan(&email)
			if strings.HasPrefix(email, "sales@") {
				t.Fatalf("expected shared mailboxes not to be duplicates, got %s", email)
			}
		}
	}

	// Signing in again is not announced again.
	again := httptest.NewRequest(http.MethodPost, "/api/auth/dev-login", strings.NewReader(`{"email":"`+newerEmail+`"}`))
	router.ServeHTTP(httptest.NewRecorder(), again)

	dismiss := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+newerID+"/not-duplicate", strings.NewReader(`{"other_user_id":"`+olderID+`"}`))
	dismiss = withAuthUser(dismiss, admin)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, dismiss)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected dismiss 204, got %d: %s", response.Code, response.Body.String())
	}
	duplicates, err = loadPossibleDuplicates(ctx, dbpool, newerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(duplicates[newerID]) != 0 {
		t.Fatalf("expected no hint after dismissing, got %v", duplicates[newerID])
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one announcement, got %v", sender.messages)
	}
}

func TestOnlySuperAdminsDismissDuplicates(t *testing.T) {
	dbpool := testDBPool(t)
	admin := createTestUserWithRole(t, dbpool, "admin")
	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+admin.ID+"/not-duplicate", strings.NewReader(`{"other_user_id":"x"}`))
	request = withAuthUser(request, admin)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

type linkRecordingSender struct {
	text, label, link string
}

func (s *linkRecordingSender) Send(_ context.Context, text string) error {
	s.text = text
	return nil
}

func (s *linkRecordingSender) SendWithLink(_ context.Context, text string, label string, link string) error {
	s.text, s.label, s.link = text, label, link
	return nil
}

func TestNotifyOwnerPutsTheLinkUnderTheMessage(t *testing.T) {
	t.Setenv("AUTH_APP_URL", "https://reviewdesk.example.com/")
	t.Cleanup(func() { Notifier = nil })

	// A chat with buttons gets the link as one.
	buttons := &linkRecordingSender{}
	Notifier = buttons
	if err := notifyOwner(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if buttons.text != "hello" || buttons.label != "Open Settings → Users" || buttons.link != "https://reviewdesk.example.com/admin/users" {
		t.Errorf("with buttons: %+v", buttons)
	}

	// Any other chat gets it on a line of its own.
	plain := &recordingSender{sent: make(chan struct{}, 1)}
	Notifier = plain
	if err := notifyOwner(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if len(plain.messages) != 1 || plain.messages[0] != "hello\nhttps://reviewdesk.example.com/admin/users" {
		t.Errorf("plain: %v", plain.messages)
	}
}
