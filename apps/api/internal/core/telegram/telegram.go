// Package telegram sends messages to one Telegram chat through a bot.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.telegram.org"

// Bot posts to the chat ChatID as the bot whose token is Token, in the topic
// ThreadID of a group with topics when it is not zero.
type Bot struct {
	Token    string
	ChatID   string
	ThreadID int64
	BaseURL  string
	Client   *http.Client
}

// FromEnv returns the bot of TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID, with
// the optional topic TELEGRAM_THREAD_ID, or false when the token or the chat
// is unset.
func FromEnv() (*Bot, bool) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	chatID := strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	if token == "" || chatID == "" {
		return nil, false
	}
	bot := &Bot{Token: token, ChatID: chatID}
	if thread := strings.TrimSpace(os.Getenv("TELEGRAM_THREAD_ID")); thread != "" {
		id, err := strconv.ParseInt(thread, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "TELEGRAM_THREAD_ID %q is not a number; posting to the whole chat\n", thread)
		}
		bot.ThreadID = id
	}
	return bot, true
}

// Send posts text as a plain message, without link previews.
func (b *Bot) Send(ctx context.Context, text string) error {
	message := map[string]any{
		"chat_id":                  b.ChatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if b.ThreadID != 0 {
		message["message_thread_id"] = b.ThreadID
	}
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	base := b.BaseURL
	if base == "" {
		base = apiBase
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+b.Token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return errors.New("telegram: invalid request")
	}
	request.Header.Set("Content-Type", "application/json")
	client := b.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		// The URL holds the token: keep it out of the error.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("telegram: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("telegram: %s: %s", response.Status, strings.TrimSpace(string(detail)))
	}
	return nil
}
