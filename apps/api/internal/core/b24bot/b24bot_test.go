package b24bot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSend(t *testing.T) {
	var got map[string]any
	var path, key string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, key = r.URL.Path, r.Header.Get("X-Api-Key")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":1}}`))
	}))
	defer server.Close()

	bot := &Bot{APIKey: "vibe-test", BotID: "42", DialogID: "681146", BaseURL: server.URL}
	if err := bot.Send(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	fields, _ := got["fields"].(map[string]any)
	if path != "/bots/42/messages" || key != "vibe-test" || got["dialogId"] != "681146" || fields["message"] != "hello" || fields["urlPreview"] != false {
		t.Fatalf("unexpected request %s %s %v", path, key, got)
	}
}

func TestSendToWithAButton(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":1}}`))
	}))
	defer server.Close()

	bot := &Bot{APIKey: "k", BotID: "42", BaseURL: server.URL}
	if err := bot.SendTo(context.Background(), "681146", "hi", Button{Text: "Open", Link: "https://app.test/x"}); err != nil {
		t.Fatal(err)
	}
	fields, _ := got["fields"].(map[string]any)
	keyboard, _ := fields["keyboard"].([]any)
	if got["dialogId"] != "681146" || len(keyboard) != 1 {
		t.Fatalf("unexpected request %v", got)
	}
	if button, _ := keyboard[0].(map[string]any); button["TEXT"] != "Open" || button["LINK"] != "https://app.test/x" {
		t.Errorf("button %v", button)
	}
}

func TestSendReportsRefusals(t *testing.T) {
	for _, answer := range []struct {
		status int
		body   string
	}{
		{http.StatusForbidden, `{"success":false,"error":{"code":"SCOPE_REQUIRED"}}`},
		{http.StatusOK, `{"success":false,"error":{"code":"BOT_NOT_FOUND"}}`},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(answer.status)
			_, _ = w.Write([]byte(answer.body))
		}))
		err := (&Bot{APIKey: "k", BotID: "42", DialogID: "1", BaseURL: server.URL}).Send(context.Background(), "hi")
		server.Close()
		if err == nil || !strings.Contains(err.Error(), "success\":false") {
			t.Errorf("%d %s: err %v", answer.status, answer.body, err)
		}
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("BITRIX_API_KEY", "k")
	t.Setenv("BITRIX_BOT_ID", "42")
	t.Setenv("BITRIX_BOT_DIALOG_ID", "")
	if _, ok := FromEnv(); ok {
		t.Error("a bot without a dialog")
	}
	if _, ok := MessengerFromEnv(); !ok {
		t.Error("a messenger needs no owner's chat")
	}
	t.Setenv("BITRIX_BOT_DIALOG_ID", "chat7")
	t.Setenv("BITRIX_API_BASE_URL", "https://platform.test/v1/")
	bot, ok := FromEnv()
	if !ok || bot.DialogID != "chat7" || bot.BaseURL != "https://platform.test/v1" {
		t.Errorf("bot %+v, ok %v", bot, ok)
	}
}

func TestPostReturnsTheMessageIDAndSendsCommandButtons(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":36357,"uuidMap":[]}}`))
	}))
	defer server.Close()

	bot := &Bot{APIKey: "k", BotID: "42", BaseURL: server.URL}
	id, err := bot.Post(context.Background(), "681146", "hi", Button{Text: "Resolve", Command: "resolve", CommandParams: "c-1"})
	if err != nil || id != "36357" {
		t.Fatalf("id %q, err %v", id, err)
	}
	fields, _ := got["fields"].(map[string]any)
	keyboard, _ := fields["keyboard"].([]any)
	button, _ := keyboard[0].(map[string]any)
	if button["COMMAND"] != "resolve" || button["COMMAND_PARAMS"] != "c-1" || button["BLOCK"] != "Y" || button["LINK"] != nil {
		t.Errorf("button %v", button)
	}
}

func TestEvents(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{"success":true,"data":{"events":[
			{"eventId":35,"type":"ONIMBOTV2MESSAGEADD","data":{"bot":{"id":42},
				"message":{"id":1501,"authorId":7,"text":"ok","isSystem":false,"params":{"REPLY_ID":"1498"}},
				"chat":{"id":87,"dialogId":"7","type":"private"},"user":{"id":7}}},
			{"eventId":36,"type":"ONIMBOTV2MESSAGEADD","data":{"bot":{"id":42},
				"message":{"id":1502,"authorId":7,"text":"no quote","params":[]},"chat":{"dialogId":"7","type":"private"},"user":{"id":7}}},
			{"eventId":37,"type":"ONIMBOTV2COMMANDADD","data":{"bot":{"id":42},"message":{"id":1510,"text":"/resolve c-1"},
				"chat":{"dialogId":"7"},"user":{"id":7},"command":{"id":7,"command":"resolve","params":"c-1","context":"keyboard"}}}
		],"nextOffset":38,"hasMore":true}}`))
	}))
	defer server.Close()

	events, more, err := (&Bot{APIKey: "k", BotID: "42", BaseURL: server.URL}).Events(context.Background())
	if err != nil || !more || len(events) != 3 || path != "/bots/42/events" {
		t.Fatalf("events %+v, more %v, err %v, path %s", events, more, err, path)
	}
	reply := events[0].Data
	if reply.Message.ID != "1501" || reply.Message.Params.ReplyID != "1498" || reply.User.ID != "7" || reply.Chat.Type != "private" || reply.Bot.ID != "42" {
		t.Errorf("reply %+v", reply)
	}
	if events[1].Data.Message.Params.ReplyID != "" {
		t.Errorf("params [] %+v", events[1].Data.Message.Params)
	}
	if command := events[2].Data.Command; events[2].Type != "ONIMBOTV2COMMANDADD" || command.Command != "resolve" || command.Params != "c-1" {
		t.Errorf("command %+v", events[2])
	}
}

func TestEnsureCommand(t *testing.T) {
	for _, existing := range []string{`{"id":229,"botId":42,"command":"/resolve"}`, `{"id":230,"botId":43,"command":"/resolve"}`} {
		registered := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				var got map[string]any
				_ = json.NewDecoder(r.Body).Decode(&got)
				registered = got["command"] == "resolve" && got["hidden"] == "Y"
				_, _ = w.Write([]byte(`{"success":true,"data":{"command":{"id":231}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"commands":[{"id":"def0","botId":0,"command":"/me"},` + existing + `]}}`))
		}))
		err := (&Bot{APIKey: "k", BotID: "42", BaseURL: server.URL}).EnsureCommand(context.Background(), "resolve", "Resolve")
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.Contains(existing, `"botId":43`); registered != want {
			t.Errorf("%s: registered %v", existing, registered)
		}
	}
}
