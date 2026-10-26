package comments

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type sentMail struct {
	to      string
	subject string
	text    string
}

type fakeMailer struct {
	sent chan sentMail
}

func (m fakeMailer) Send(_ context.Context, to string, subject string, text string, _ string) error {
	m.sent <- sentMail{to: to, subject: subject, text: text}
	return nil
}

func TestMentionsNotifyAndEmailMentionedPeople(t *testing.T) {
	dbpool := testDBPool(t)
	author := createTestUserWithRole(t, dbpool, "mention-author@example.com", "reviewer")
	mentioned := createTestUserWithRole(t, dbpool, "mention-target@example.com", "reviewer")
	emailID := createTestEmail(t, dbpool)

	mailer := fakeMailer{sent: make(chan sentMail, 4)}
	previous := Mailer
	Mailer = mailer
	t.Cleanup(func() { Mailer = previous })
	t.Setenv("AUTH_APP_URL", "https://reviewdesk.example.com/")

	router := chi.NewRouter()
	RegisterCommentRoutes(router, dbpool)

	// The author mentions themselves too and a person who does not exist:
	// both stay out of the notifications.
	body, _ := json.Marshal(map[string]any{
		"review_block":     "body-001",
		"selected_text":    "selected text",
		"start_offset":     0,
		"end_offset":       13,
		"body":             "@Target please check, cc @Author",
		"mention_user_ids": []string{mentioned.ID, author.ID, "00000000-0000-0000-0000-000000000000", mentioned.ID},
	})
	response := serveAs(router, http.MethodPost, "/api/emails/"+emailID+"/comments", body, author)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var comment EmailComment
	if err := json.NewDecoder(response.Body).Decode(&comment); err != nil {
		t.Fatal(err)
	}
	if got := comment.Messages[0].MentionEmails; len(got) != 2 {
		t.Fatalf("expected the author and the target as mentions, got %v", got)
	}

	select {
	case mail := <-mailer.sent:
		if mail.to != mentioned.Email {
			t.Fatalf("expected the mention email to go to %s, got %s", mentioned.Email, mail.to)
		}
		if !strings.Contains(mail.text, "https://reviewdesk.example.com/emails/"+emailID+"/review?comment="+comment.ID) {
			t.Fatalf("expected a link to the comment, got %q", mail.text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expected a mention email")
	}
	select {
	case mail := <-mailer.sent:
		t.Fatalf("expected one email, also got one to %s", mail.to)
	case <-time.After(200 * time.Millisecond):
	}

	notifications := listNotificationsAs(t, router, mentioned)
	if len(notifications) != 1 || notifications[0].CommentID == nil || *notifications[0].CommentID != comment.ID {
		t.Fatalf("expected one notification for the comment, got %+v", notifications)
	}
	if notifications[0].ActorEmail == nil || *notifications[0].ActorEmail != author.Email {
		t.Fatalf("expected the author as actor, got %+v", notifications[0].ActorEmail)
	}
	if len(listNotificationsAs(t, router, author)) != 0 {
		t.Fatal("expected no notification for mentioning yourself")
	}
	waitForEmailStatus(t, notifications[0].ID, "sent")

	// A reply can mention too, and the list shows mentions per message.
	replyBody, _ := json.Marshal(map[string]any{"body": "@Target one more", "mention_user_ids": []string{mentioned.ID}})
	response = serveAs(router, http.MethodPost, "/api/comments/"+comment.ID+"/messages", replyBody, author)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	<-mailer.sent
	if got := listNotificationsAs(t, router, mentioned); len(got) != 2 {
		t.Fatalf("expected two unread notifications, got %d", len(got))
	}

	// Marking someone else's notification does nothing; marking all clears mine.
	readBody, _ := json.Marshal(map[string]any{"ids": []string{notifications[0].ID}})
	if code := serveAs(router, http.MethodPost, "/api/notifications/read", readBody, author).Code; code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", code)
	}
	if got := listNotificationsAs(t, router, mentioned); len(got) != 2 {
		t.Fatalf("expected another person's read to change nothing, got %d", len(got))
	}
	if code := serveAs(router, http.MethodPost, "/api/notifications/read", readBody, mentioned).Code; code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", code)
	}
	if got := listNotificationsAs(t, router, mentioned); len(got) != 1 {
		t.Fatalf("expected one unread notification left, got %d", len(got))
	}
	if code := serveAs(router, http.MethodPost, "/api/notifications/read", []byte(`{}`), mentioned).Code; code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", code)
	}
	if got := listNotificationsAs(t, router, mentioned); len(got) != 0 {
		t.Fatalf("expected all read, got %d", len(got))
	}

	listResponse := serveAs(router, http.MethodGet, "/api/emails/"+emailID+"/comments", nil, author)
	var listed []EmailComment
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || len(listed[0].Messages) != 2 ||
		len(listed[0].Messages[1].MentionEmails) != 1 || listed[0].Messages[1].MentionEmails[0] != mentioned.Email {
		t.Fatalf("expected the reply's mention in the list, got %+v", listed)
	}
}

func TestTooManyMentionsAreRejected(t *testing.T) {
	ids := make([]string, maxMentionsPerMessage+1)
	for i := range ids {
		ids[i] = strings.Repeat("a", i+1)
	}
	if _, err := normalizeMentionIDs(ids); err == nil {
		t.Fatal("expected an error")
	}
	got, err := normalizeMentionIDs([]string{" a ", "a", "", "b"})
	if err != nil || len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected [a b], got %v (%v)", got, err)
	}
}

func TestMentionEmailContent(t *testing.T) {
	commentID := "c-1"
	subject, text, html := mentionEmailContent(mentionDelivery{
		ActorName:     "Ann <Admin>",
		EmailID:       "e-1",
		EmailTitle:    "Welcome",
		EmailLanguage: "de",
		CommentID:     &commentID,
		Excerpt:       "@Bob look <here>",
	}, "https://rd.example.com")
	if subject != "Ann <Admin> mentioned you on Welcome (DE)" {
		t.Fatalf("unexpected subject %q", subject)
	}
	if !strings.Contains(text, "https://rd.example.com/emails/e-1/review?comment=c-1") {
		t.Fatalf("expected the link in the text, got %q", text)
	}
	if strings.Contains(html, "<here>") || !strings.Contains(html, "Ann &lt;Admin&gt;") {
		t.Fatalf("expected escaped HTML, got %q", html)
	}

	_, text, html = mentionEmailContent(mentionDelivery{EmailTitle: "Welcome", EmailLanguage: "en"}, "")
	if strings.Contains(text, "Open the comment") || strings.Contains(html, "href") {
		t.Fatal("expected no link without an app URL")
	}
}

func TestMentionExcerptShortensLongMessages(t *testing.T) {
	long := strings.Repeat("word ", 100)
	got := mentionExcerpt(long)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) > mentionExcerptLength+1 {
		t.Fatalf("expected a shortened excerpt, got %d runes", len([]rune(got)))
	}
	if mentionExcerpt("a\n\n b") != "a b" {
		t.Fatal("expected whitespace to collapse")
	}
}

func serveAs(router chi.Router, method string, path string, body []byte, user AuthUser) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request = request.WithContext(auth.WithUser(request.Context(), user))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func listNotificationsAs(t *testing.T, router chi.Router, user AuthUser) []Notification {
	t.Helper()
	response := serveAs(router, http.MethodGet, "/api/notifications", nil, user)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var notifications []Notification
	if err := json.NewDecoder(response.Body).Decode(&notifications); err != nil {
		t.Fatal(err)
	}
	return notifications
}

func waitForEmailStatus(t *testing.T, notificationID string, want string) {
	t.Helper()
	dbpool := testDBPool(t)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var status string
		if err := dbpool.QueryRow(context.Background(), `SELECT email_status FROM notifications WHERE id = $1;`, notificationID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected email status %s, got %s", want, status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
