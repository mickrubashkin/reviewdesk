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
