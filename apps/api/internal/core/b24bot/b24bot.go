// Package b24bot sends messages to one Bitrix24 dialog through a bot of the
// VibeCode platform, and reads what people write to it; on VibeCode it takes
// the place of the Telegram bot.
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

// Bot posts as the bot BotID. Send posts to DialogID, the owner's chat: a
// user's ID for a private chat, or chatN for a group chat. SendTo posts to any
// dialog, such as a person's private chat.
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
	bot, ok := MessengerFromEnv()
	if !ok || bot.DialogID == "" {
		return nil, false
	}
	return bot, true
}

// MessengerFromEnv is FromEnv for writing to people: it needs no owner's chat.
func MessengerFromEnv() (*Bot, bool) {
	bot := &Bot{
		APIKey:   strings.TrimSpace(os.Getenv("BITRIX_API_KEY")),
		BotID:    strings.TrimSpace(os.Getenv("BITRIX_BOT_ID")),
		DialogID: strings.TrimSpace(os.Getenv("BITRIX_BOT_DIALOG_ID")),
		BaseURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("BITRIX_API_BASE_URL")), "/"),
	}
	if bot.APIKey == "" || bot.BotID == "" {
		return nil, false
	}
	return bot, true
}

// Button is a button under a message: a link, or a command of the bot that
// comes back as an ONIMBOTV2COMMANDADD event with CommandParams.
type Button struct {
	Text          string
	Link          string
	Command       string
	CommandParams string
}

// Send posts text to the owner's chat.
func (b *Bot) Send(ctx context.Context, text string) error {
	return b.SendTo(ctx, b.DialogID, text)
}

// SendWithLink posts text to the owner's chat with a button that opens link.
func (b *Bot) SendWithLink(ctx context.Context, text string, label string, link string) error {
	return b.SendTo(ctx, b.DialogID, text, Button{Text: label, Link: link})
}

// SendTo posts text to dialogID; see Post.
func (b *Bot) SendTo(ctx context.Context, dialogID string, text string, buttons ...Button) error {
	_, err := b.Post(ctx, dialogID, text, buttons...)
	return err
}

// Post posts text to dialogID as it is, without link previews, with the
// buttons under it, and returns the message's ID. Bitrix24 reads BB codes
// such as [b] in the text, which our messages don't use.
func (b *Bot) Post(ctx context.Context, dialogID string, text string, buttons ...Button) (string, error) {
	fields := map[string]any{"message": text, "urlPreview": false}
	if len(buttons) > 0 {
		keyboard := make([]map[string]any, 0, len(buttons))
		for _, button := range buttons {
			key := map[string]any{"TEXT": button.Text, "BG_COLOR_TOKEN": "primary", "DISPLAY": "LINE"}
			if button.Command != "" {
				// Pressed once: the button turns grey.
				key["COMMAND"], key["COMMAND_PARAMS"], key["BLOCK"] = button.Command, button.CommandParams, "Y"
				key["BG_COLOR_TOKEN"] = "secondary"
			} else {
				key["LINK"] = button.Link
			}
			keyboard = append(keyboard, key)
		}
		fields["keyboard"] = keyboard
	}
	var answer struct {
		ID ID `json:"id"`
	}
	if err := b.call(ctx, http.MethodPost, "/messages", map[string]any{"dialogId": dialogID, "fields": fields}, &answer); err != nil {
		return "", err
	}
	return string(answer.ID), nil
}

// Event is what the bot receives: ONIMBOTV2MESSAGEADD for a message in its
// chat, ONIMBOTV2COMMANDADD for a command or a command button, and others we
// skip.
type Event struct {
	ID   ID     `json:"eventId"`
	Type string `json:"type"`
	Data struct {
		Bot struct {
			ID ID `json:"id"`
		} `json:"bot"`
		Message struct {
			ID       ID     `json:"id"`
			AuthorID ID     `json:"authorId"`
			Text     string `json:"text"`
			IsSystem bool   `json:"isSystem"`
			Params   MessageParams `json:"params"`
		} `json:"message"`
		Chat struct {
			DialogID string `json:"dialogId"`
			Type     string `json:"type"`
		} `json:"chat"`
		User struct {
			ID ID `json:"id"`
		} `json:"user"`
		Command struct {
			Command string `json:"command"`
			Params  string `json:"params"`
		} `json:"command"`
	} `json:"data"`
}

// MessageParams are a message's extras. Bitrix24 sends [] when there are none.
type MessageParams struct {
	// ReplyID is the quoted message of a reply.
	ReplyID ID `json:"REPLY_ID"`
}

func (p *MessageParams) UnmarshalJSON(data []byte) error {
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		*p = MessageParams{}
		return nil
	}
	type plain MessageParams
	return json.Unmarshal(data, (*plain)(p))
}

// Events takes the bot's next events. The platform keeps the position and
// counts the events read when it answers, so each comes once, unless the
// answer is lost on the way. more is true when others are waiting.
func (b *Bot) Events(ctx context.Context) (events []Event, more bool, err error) {
	var answer struct {
		Events  []Event `json:"events"`
		HasMore bool    `json:"hasMore"`
	}
	if err := b.call(ctx, http.MethodGet, "/events", nil, &answer); err != nil {
		return nil, false, err
	}
	return answer.Events, answer.HasMore, nil
}

// EnsureCommand registers the hidden command, without its slash, unless the
// bot has it already. A command button works only with a registered command.
func (b *Bot) EnsureCommand(ctx context.Context, command string, title string) error {
	var list struct {
		Commands []struct {
			BotID   ID     `json:"botId"`
			Command string `json:"command"`
		} `json:"commands"`
	}
	// The list holds every command of the portal and can take long.
	slow := *b
	if slow.Client == nil {
		slow.Client = &http.Client{Timeout: 3 * time.Minute}
	}
	if err := slow.call(ctx, http.MethodGet, "/commands", nil, &list); err != nil {
		return err
	}
	for _, existing := range list.Commands {
		if string(existing.BotID) == b.BotID && strings.TrimPrefix(existing.Command, "/") == command {
			return nil
		}
	}
	return b.call(ctx, http.MethodPost, "/commands", map[string]any{"command": command, "title": title, "hidden": "Y"}, nil)
}

// call sends payload, when there is one, to the bot's path and reads the
// answer's data into out.
func (b *Bot) call(ctx context.Context, method string, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	base := b.BaseURL
	if base == "" {
		base = apiBase
	}
	request, err := http.NewRequestWithContext(ctx, method, base+"/bots/"+url.PathEscape(b.BotID)+path, body)
	if err != nil {
		return fmt.Errorf("bitrix24 bot: %w", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Api-Key", b.APIKey)
	client := b.Client
	if client == nil {
		// The platform can take long under load; a request cut short may
		// still be delivered.
		client = &http.Client{Timeout: 30 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("bitrix24 bot: %w", err)
	}
	defer response.Body.Close()
	detail, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	var answer struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if response.StatusCode/100 != 2 || json.Unmarshal(detail, &answer) != nil || !answer.Success {
		return fmt.Errorf("bitrix24 bot: %s: %s", response.Status, strings.TrimSpace(string(detail[:min(len(detail), 512)])))
	}
	if out != nil && len(answer.Data) > 0 {
		if err := json.Unmarshal(answer.Data, out); err != nil {
			return fmt.Errorf("bitrix24 bot: %w", err)
		}
	}
	return nil
}

// ID is an ID that Bitrix24 sends as a number or as a string.
type ID string

func (id *ID) UnmarshalJSON(data []byte) error {
	var text string
	if json.Unmarshal(data, &text) == nil {
		*id = ID(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*id = ID(number.String())
	return nil
}
