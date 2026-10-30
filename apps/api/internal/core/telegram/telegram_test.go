package telegram

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
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	bot := &Bot{Token: "123:abc", ChatID: "-100", BaseURL: server.URL}
	if err := bot.Send(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if path != "/bot123:abc/sendMessage" || got["chat_id"] != "-100" || got["text"] != "hello" {
		t.Fatalf("unexpected request %s %v", path, got)
	}
	if _, ok := got["message_thread_id"]; ok {
		t.Fatalf("expected no topic, got %v", got)
	}

	bot.ThreadID = 42
	if err := bot.Send(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if got["message_thread_id"] != float64(42) {
		t.Fatalf("expected topic 42, got %v", got)
	}
}

func TestSendErrorsKeepTheTokenOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"ok":false,"description":"chat not found"}`, http.StatusBadRequest)
	}))
	bot := &Bot{Token: "123:secret", ChatID: "-100", BaseURL: server.URL}
	err := bot.Send(context.Background(), "hello")
	if err == nil || !strings.Contains(err.Error(), "chat not found") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unexpected error %v", err)
	}

	server.Close()
	err = bot.Send(context.Background(), "hello")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "")
	if _, ok := FromEnv(); ok {
		t.Fatal("expected no bot without a chat")
	}
	t.Setenv("TELEGRAM_CHAT_ID", "-100")
	if bot, ok := FromEnv(); !ok || bot.ChatID != "-100" || bot.ThreadID != 0 {
		t.Fatalf("unexpected bot %+v", bot)
	}
	t.Setenv("TELEGRAM_THREAD_ID", "7")
	if bot, ok := FromEnv(); !ok || bot.ThreadID != 7 {
		t.Fatalf("unexpected bot %+v", bot)
	}
	t.Setenv("TELEGRAM_THREAD_ID", "general")
	if bot, ok := FromEnv(); !ok || bot.ThreadID != 0 {
		t.Fatalf("unexpected bot %+v", bot)
	}
}
