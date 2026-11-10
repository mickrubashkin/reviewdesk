package comments

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/b24bot"
)

// fakeInbox records the bot's answers.
type fakeInbox struct{ answers []sentMessage }

func (f *fakeInbox) Events(context.Context) ([]b24bot.Event, bool, error) { return nil, false, nil }

func (f *fakeInbox) Post(_ context.Context, dialogID string, text string, buttons ...b24bot.Button) (string, error) {
	f.answers = append(f.answers, sentMessage{dialogID: dialogID, text: text, buttons: buttons})
	return "answer", nil
}

func (f *fakeInbox) last(t *testing.T) sentMessage {
	t.Helper()
	if len(f.answers) == 0 {
		t.Fatal("expected an answer from the bot")
	}
	answer := f.answers[len(f.answers)-1]
	f.answers = nil
	return answer
}

func chatMessage(portalUserID string, messageID string, quotedID string, text string) b24bot.Event {
	var event b24bot.Event
	event.Type = "ONIMBOTV2MESSAGEADD"
	event.Data.Bot.ID = "42"
	event.Data.Message.ID = b24bot.ID(messageID)
	event.Data.Message.AuthorID = b24bot.ID(portalUserID)
	event.Data.Message.Text = text
	event.Data.Message.Params.ReplyID = b24bot.ID(quotedID)
	event.Data.Chat.DialogID = portalUserID
	event.Data.Chat.Type = "private"
	event.Data.User.ID = b24bot.ID(portalUserID)
	return event
}

func resolvePress(portalUserID string, commentID string) b24bot.Event {
	var event b24bot.Event
	event.Type = "ONIMBOTV2COMMANDADD"
	event.Data.Bot.ID = "42"
	event.Data.Chat.DialogID = portalUserID
	event.Data.User.ID = b24bot.ID(portalUserID)
	event.Data.Command.Command = ResolveCommand
	event.Data.Command.Params = commentID
	return event
}

func TestBotRepliesAndResolve(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	author := createTestUserWithRole(t, dbpool, "bot-reply-author@example.com", "reviewer")
	linked := createTestUserWithRole(t, dbpool, "bot-reply-linked@example.com", "reviewer")
	admin := createTestUserWithRole(t, dbpool, "bot-reply-admin@example.com", "admin")
	emailID := createTestEmail(t, dbpool)
	for user, id := range map[string]string{author.ID: "9200001", linked.ID: "9200002", admin.ID: "9200003"} {
		if _, err := dbpool.Exec(ctx, `UPDATE users SET portal_user_id = $2 WHERE id = $1;`, user, id); err != nil {
			t.Fatal(err)
		}
	}

	messenger := fakeMessenger{sent: make(chan sentMessage, 8)}
	previousMailer, previousMessenger := Mailer, Messenger
	Mailer, Messenger = nil, messenger
	t.Cleanup(func() { Mailer, Messenger = previousMailer, previousMessenger })
	t.Setenv("AUTH_APP_URL", "https://reviewdesk.example.com")

	router := chi.NewRouter()
	RegisterCommentRoutes(router, dbpool)
	mention := func(userIDs ...string) string {
		body, _ := json.Marshal(map[string]any{
			"review_block": "body-001", "selected_text": "selected text", "start_offset": 0, "end_offset": 13,
			"body": "@Someone look", "mention_user_ids": userIDs,
		})
		response := serveAs(router, http.MethodPost, "/api/emails/"+emailID+"/comments", body, author)
		if response.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
		}
		var comment EmailComment
		_ = json.NewDecoder(response.Body).Decode(&comment)
		for range userIDs {
			select {
			case <-messenger.sent:
			case <-time.After(5 * time.Second):
				t.Fatal("expected a Bitrix24 message")
			}
		}
		waitForStatuses(t, linked.ID, comment.ID, "skipped", "sent")
		return comment.ID
	}
	// Each mention message gets its own ID, as in Bitrix24.
	messageID := func(commentID string) string {
		id := "m-" + commentID
		if _, err := dbpool.Exec(ctx, `UPDATE notifications SET bitrix_message_id = $2 WHERE user_id = $1 AND comment_id = $3;`, linked.ID, id, commentID); err != nil {
			t.Fatal(err)
		}
		return id
	}
	// The thread starts with the comment itself.
	thread := func(commentID string) []string {
		rows, err := dbpool.Query(ctx, `SELECT body FROM comment_messages WHERE comment_id = $1 ORDER BY created_at;`, commentID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		bodies := []string{}
		for rows.Next() {
			var body string
			_ = rows.Scan(&body)
			bodies = append(bodies, body)
		}
		return bodies
	}

	first := mention(linked.ID)
	firstMessage := messageID(first)
	second := mention(linked.ID)
	messageID(second)
	inbox := &fakeInbox{}

	// A quoted reply goes to the quoted mention's thread, as its author.
	handleBotEvent(ctx, dbpool, inbox, chatMessage("9200002", "501", firstMessage, "[b]Fixed[/b] it"))
	if got := thread(first); len(got) != 2 || got[1] != "Fixed it" {
		t.Fatalf("thread %v", got)
	}
	var replier string
	_ = dbpool.QueryRow(ctx, `SELECT user_id FROM comment_messages WHERE bitrix_message_id = '501';`).Scan(&replier)
	if answer := inbox.last(t); replier != linked.ID || answer.dialogID != "9200002" || !strings.HasPrefix(answer.text, "Added to the thread on") ||
		len(answer.buttons) != 1 || !strings.Contains(answer.buttons[0].Link, "comment="+first) {
		t.Fatalf("replier %s, answer %+v", replier, answer)
	}

	// The same event again is not posted twice, and the bot keeps quiet.
	handleBotEvent(ctx, dbpool, inbox, chatMessage("9200002", "501", firstMessage, "Fixed it"))
	if got := thread(first); len(got) != 2 || len(inbox.answers) != 0 {
		t.Fatalf("thread %v, answers %+v", got, inbox.answers)
	}

	// An unquoted message goes to the latest mention.
	handleBotEvent(ctx, dbpool, inbox, chatMessage("9200002", "502", "", "And this one"))
	if got := thread(second); len(got) != 2 || got[1] != "And this one" {
		t.Fatalf("thread %v", got)
	}
	inbox.last(t)

	// Someone ReviewDesk doesn't know is told so; the bot's own messages and
	// group chats are skipped.
	handleBotEvent(ctx, dbpool, inbox, chatMessage("9299999", "503", "", "Hi"))
	if answer := inbox.last(t); !strings.Contains(answer.text, "don't know you") {
		t.Fatalf("answer %+v", answer)
	}
	own := chatMessage("42", "504", "", "Hi")
	group := chatMessage("9200002", "505", "", "Hi")
	group.Data.Chat.Type = "chat"
	handleBotEvent(ctx, dbpool, inbox, own)
	handleBotEvent(ctx, dbpool, inbox, group)
	if len(inbox.answers) != 0 {
		t.Fatalf("answers %+v", inbox.answers)
	}

	// Only the author, an admin or a board manager resolves.
	handleBotEvent(ctx, dbpool, inbox, resolvePress("9200002", first))
	if answer := inbox.last(t); !strings.HasPrefix(answer.text, "Only the comment's author") {
		t.Fatalf("answer %+v", answer)
	}
	handleBotEvent(ctx, dbpool, inbox, resolvePress("9200001", first))
	var status string
	_ = dbpool.QueryRow(ctx, `SELECT status FROM comments WHERE id = $1;`, first).Scan(&status)
	if answer := inbox.last(t); status != "resolved" || !strings.HasPrefix(answer.text, "Resolved the thread") {
		t.Fatalf("status %s, answer %+v", status, answer)
	}
	handleBotEvent(ctx, dbpool, inbox, resolvePress("9200001", first))
	if answer := inbox.last(t); !strings.Contains(answer.text, "already resolved") {
		t.Fatalf("answer %+v", answer)
	}

	// A reply to a resolved thread is not posted.
	handleBotEvent(ctx, dbpool, inbox, chatMessage("9200002", "506", firstMessage, "One more"))
	if got := thread(first); len(got) != 2 {
		t.Fatalf("thread %v", got)
	}
	if answer := inbox.last(t); !strings.Contains(answer.text, "is resolved") {
		t.Fatalf("answer %+v", answer)
	}

	// The mention message offers "Resolve" only to someone who may.
	body, _ := json.Marshal(map[string]any{
		"review_block": "body-001", "selected_text": "selected text", "start_offset": 0, "end_offset": 13,
		"body": "@Both look", "mention_user_ids": []string{linked.ID, admin.ID},
	})
	if response := serveAs(router, http.MethodPost, "/api/emails/"+emailID+"/comments", body, author); response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", response.Code)
	}
	for range 2 {
		select {
		case message := <-messenger.sent:
			hasResolve := false
			for _, button := range message.buttons {
				hasResolve = hasResolve || button.Command == ResolveCommand
			}
			if want := message.dialogID == "9200003"; hasResolve != want {
				t.Errorf("message to %s: buttons %+v", message.dialogID, message.buttons)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("expected a Bitrix24 message")
		}
	}
}
