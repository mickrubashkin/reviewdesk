package comments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/b24bot"
)

type sentMessage struct {
	dialogID string
	text     string
	buttons  []b24bot.Button
}

type fakeMessenger struct {
	sent chan sentMessage
	err  error
}

func (m fakeMessenger) SendTo(_ context.Context, dialogID string, text string, buttons ...b24bot.Button) error {
	m.sent <- sentMessage{dialogID: dialogID, text: text, buttons: buttons}
	return m.err
}

func TestMentionsGoToBitrix24WhenTheIDIsKnown(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	author := createTestUserWithRole(t, dbpool, "bitrix-mention-author@example.com", "reviewer")
	linked := createTestUserWithRole(t, dbpool, "bitrix-mention-linked@example.com", "reviewer")
	emailID := createTestEmail(t, dbpool)
	if _, err := dbpool.Exec(ctx, `UPDATE users SET portal_user_id = '9100001' WHERE id = $1;`, linked.ID); err != nil {
		t.Fatal(err)
	}

	mailer := fakeMailer{sent: make(chan sentMail, 4)}
	messenger := fakeMessenger{sent: make(chan sentMessage, 4)}
	previousMailer, previousMessenger := Mailer, Messenger
	Mailer, Messenger = mailer, messenger
	t.Cleanup(func() { Mailer, Messenger = previousMailer, previousMessenger })
	t.Setenv("AUTH_APP_URL", "https://reviewdesk.example.com")

	router := chi.NewRouter()
	RegisterCommentRoutes(router, dbpool)
	mention := func(messengerErr error) string {
		Messenger = fakeMessenger{sent: messenger.sent, err: messengerErr}
		body, _ := json.Marshal(map[string]any{
			"review_block": "body-001", "selected_text": "selected text", "start_offset": 0, "end_offset": 13,
			"body": "@Linked look", "mention_user_ids": []string{linked.ID},
		})
		response := serveAs(router, http.MethodPost, "/api/emails/"+emailID+"/comments", body, author)
		if response.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
		}
		var comment EmailComment
		_ = json.NewDecoder(response.Body).Decode(&comment)
		return comment.ID
	}

	// A person with a Bitrix24 ID gets the message there, and no email.
	commentID := mention(nil)
	select {
	case message := <-messenger.sent:
		if message.dialogID != "9100001" || !strings.Contains(message.text, "@Linked look") {
			t.Fatalf("unexpected message %+v", message)
		}
		if len(message.buttons) != 1 || message.buttons[0].Link != "https://reviewdesk.example.com/emails/"+emailID+"/review?comment="+commentID {
			t.Fatalf("unexpected buttons %+v", message.buttons)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expected a Bitrix24 message")
	}
	waitForStatuses(t, linked.ID, commentID, "skipped", "sent")
	select {
	case mail := <-mailer.sent:
		t.Fatalf("expected no email, got one to %s", mail.to)
	default:
	}

	// When the bot fails, the email still goes.
	commentID = mention(errors.New("bitrix24 is down"))
	<-messenger.sent
	select {
	case mail := <-mailer.sent:
		if mail.to != linked.Email {
			t.Fatalf("expected the fallback email to %s, got %s", linked.Email, mail.to)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expected the fallback email")
	}
	waitForStatuses(t, linked.ID, commentID, "sent", "failed")
}

func waitForStatuses(t *testing.T, userID string, commentID string, wantEmail string, wantBitrix string) {
	t.Helper()
	dbpool := testDBPool(t)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var email, bitrix string
		err := dbpool.QueryRow(context.Background(), `
			SELECT email_status, bitrix_status FROM notifications WHERE user_id = $1 AND comment_id = $2;
		`, userID, commentID).Scan(&email, &bitrix)
		if err == nil && email == wantEmail && bitrix == wantBitrix {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected email %s and Bitrix24 %s, got %s and %s (%v)", wantEmail, wantBitrix, email, bitrix, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
