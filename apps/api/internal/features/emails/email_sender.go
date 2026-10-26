package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const resendEmailEndpoint = "https://api.resend.com/emails"

type EmailSender interface {
	SendLoginCode(ctx context.Context, to string, code string) error
}

type MultiEmailSender struct {
	Senders []EmailSender
}

func (sender MultiEmailSender) SendLoginCode(ctx context.Context, to string, code string) error {
	var sendErrors []error
	for _, child := range sender.Senders {
		if err := child.SendLoginCode(ctx, to, code); err != nil {
			sendErrors = append(sendErrors, err)
		}
	}

	if len(sendErrors) > 0 {
		return fmt.Errorf("failed to send login code: %w", errors.Join(sendErrors...))
	}

	return nil
}

type LogEmailSender struct {
	Writer io.Writer
}

func (sender LogEmailSender) SendLoginCode(_ context.Context, to string, code string) error {
	writer := sender.Writer
	if writer == nil {
		writer = os.Stdout
	}

	_, err := fmt.Fprintf(writer, "ReviewDesk login code for %s: %s\n", to, code)
	return err
}

// Send prints a notification email instead of sending it, for local runs
// without RESEND_API_KEY.
func (sender LogEmailSender) Send(_ context.Context, to string, subject string, text string, _ string) error {
	writer := sender.Writer
	if writer == nil {
		writer = os.Stdout
	}

	_, err := fmt.Fprintf(writer, "ReviewDesk email to %s: %s\n%s\n", to, subject, text)
	return err
}

type ResendEmailSender struct {
	APIKey   string
	From     string
	ReplyTo  string
	Endpoint string
	Client   *http.Client
}

func (sender ResendEmailSender) SendLoginCode(ctx context.Context, to string, code string) error {
	return sender.Send(ctx, to, "Your ReviewDesk sign-in code", loginCodeEmailText(code), loginCodeEmailHTML(code))
}

// Send delivers one email through Resend.
func (sender ResendEmailSender) Send(ctx context.Context, to string, subject string, text string, htmlBody string) error {
	payload := resendEmailPayload{
		From:    sender.From,
		To:      []string{to},
		ReplyTo: sender.ReplyTo,
		Subject: subject,
		Text:    text,
		HTML:    htmlBody,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := sender.Endpoint
	if endpoint == "" {
		endpoint = resendEmailEndpoint
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+sender.APIKey)
	request.Header.Set("Content-Type", "application/json")

	client := sender.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBytes, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			return readErr
		}

		return fmt.Errorf("resend returned %d: %s", response.StatusCode, string(responseBytes))
	}

	return nil
}

type resendEmailPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	ReplyTo string   `json:"reply_to,omitempty"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	Text    string   `json:"text"`
}

func NewLoginCodeEmailSender() EmailSender {
	return MultiEmailSender{Senders: newLoginCodeEmailSendersFromEnv()}
}

// NotificationSender sends notification emails, such as @mentions.
type NotificationSender interface {
	Send(ctx context.Context, to string, subject string, text string, htmlBody string) error
}

// NewNotificationSender sends through Resend when RESEND_API_KEY is set and
// prints the email otherwise. The public demo always prints: its users have
// made-up addresses, and mail to them would hurt the sender's reputation.
func NewNotificationSender() NotificationSender {
	apiKey := strings.TrimSpace(os.Getenv("RESEND_API_KEY"))
	if apiKey == "" || envBoolDefault("AUTH_DEMO_LOGIN_ENABLED", false) {
		return LogEmailSender{Writer: os.Stdout}
	}
	return ResendEmailSender{
		APIKey:  apiKey,
		From:    authFromEmail(),
		ReplyTo: authReplyToEmail(),
		Client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func newLoginCodeEmailSendersFromEnv() []EmailSender {
	apiKey := strings.TrimSpace(os.Getenv("RESEND_API_KEY"))
	shouldLog := apiKey == "" || envBoolDefault("AUTH_LOG_LOGIN_CODES", envBoolDefault("AUTH_LOG_MAGIC_LINKS", true))

	var senders []EmailSender
	if apiKey != "" {
		senders = append(senders, ResendEmailSender{
			APIKey:  apiKey,
			From:    authFromEmail(),
			ReplyTo: authReplyToEmail(),
			Client:  &http.Client{Timeout: 15 * time.Second},
		})
	}
	if shouldLog {
		senders = append(senders, LogEmailSender{Writer: os.Stdout})
	}

	return senders
}

func authFromEmail() string {
	fromEmail := strings.TrimSpace(os.Getenv("AUTH_FROM_EMAIL"))
	if fromEmail == "" {
		return "ReviewDesk <login@example.com>"
	}

	return fromEmail
}

func authReplyToEmail() string {
	return strings.TrimSpace(os.Getenv("AUTH_REPLY_TO_EMAIL"))
}

func envBoolDefault(name string, defaultValue bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}

	return strings.EqualFold(value, "true") || value == "1"
}

func loginCodeEmailText(code string) string {
	return fmt.Sprintf(`Sign in to ReviewDesk

Use this one-time code to sign in:
%s

This code expires in 10 minutes.

If you did not request this email, you can ignore it.`, code)
}

func loginCodeEmailHTML(code string) string {
	escapedCode := html.EscapeString(code)
	return fmt.Sprintf(`<!doctype html>
<html>
  <body style="font-family: Arial, sans-serif; color: #1f2937; line-height: 1.5;">
    <h1 style="font-size: 20px;">Sign in to ReviewDesk</h1>
    <p>Use this one-time code to sign in. It expires in 10 minutes.</p>
    <p style="font-size: 28px; font-weight: 700; letter-spacing: 4px; margin: 20px 0;">%s</p>
    <p>ReviewDesk never asks for your email password.</p>
    <p>If you did not request this email, you can ignore it.</p>
  </body>
</html>`, escapedCode)
}
