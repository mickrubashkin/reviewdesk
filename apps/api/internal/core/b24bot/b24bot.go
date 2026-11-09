// Package b24bot sends messages to one Bitrix24 dialog through a bot of the
// VibeCode platform; on VibeCode it takes the place of the Telegram bot.
package b24bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const apiBase = "https://vibecode.bitrix24.tech/v1"

// Bot posts as the bot BotID to DialogID: a user's ID for a private chat, or
// chatN for a group chat.
type Bot struct {
	APIKey   string
	BotID    string
	DialogID string
	BaseURL  string
	Client   *http.Client
}

// FromEnv returns the bot of BITRIX_BOT_ID and BITRIX_BOT_DIALOG_ID, with the
// platform key BITRIX_API_KEY and the optional BITRIX_API_BASE_URL, or false
// when one of them is unset.
func FromEnv() (*Bot, bool) {
	bot := &Bot{
		APIKey:   strings.TrimSpace(os.Getenv("BITRIX_API_KEY")),
		BotID:    strings.TrimSpace(os.Getenv("BITRIX_BOT_ID")),
		DialogID: strings.TrimSpace(os.Getenv("BITRIX_BOT_DIALOG_ID")),
		BaseURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("BITRIX_API_BASE_URL")), "/"),
	}
	if bot.APIKey == "" || bot.BotID == "" || bot.DialogID == "" {
		return nil, false
	}
	return bot, true
}

// Send posts text as it is, without link previews. Bitrix24 reads BB codes
// such as [b] in it, which our messages don't use.
func (b *Bot) Send(ctx context.Context, text string) error {
	body, err := json.Marshal(map[string]any{
		"dialogId": b.DialogID,
		"fields":   map[string]any{"message": text, "urlPreview": false},
	})
	if err != nil {
		return err
	}
	base := b.BaseURL
	if base == "" {
		base = apiBase
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bots/"+url.PathEscape(b.BotID)+"/messages", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("bitrix24 bot: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Api-Key", b.APIKey)
	client := b.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("bitrix24 bot: %w", err)
	}
	defer response.Body.Close()
	detail, _ := io.ReadAll(io.LimitReader(response.Body, 512))
	var answer struct {
		Success bool `json:"success"`
	}
	if response.StatusCode != http.StatusOK || json.Unmarshal(detail, &answer) != nil || !answer.Success {
		return fmt.Errorf("bitrix24 bot: %s: %s", response.Status, strings.TrimSpace(string(detail)))
	}
	return nil
}
