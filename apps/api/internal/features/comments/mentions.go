package comments

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/appurl"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/b24bot"
)

const (
	maxMentionsPerMessage = 20
	mentionExcerptLength  = 280
	mentionEmailTimeout   = 20 * time.Second
)

// NotificationMailer sends the email copy of a notification.
type NotificationMailer interface {
	Send(ctx context.Context, to string, subject string, text string, html string) error
}

// Mailer delivers mention emails; without one they are marked skipped.
var Mailer NotificationMailer

// NotificationMessenger sends a notification as a private Bitrix24 message
// and returns the message's ID, which a reply in that chat quotes.
type NotificationMessenger interface {
	Post(ctx context.Context, dialogID string, text string, buttons ...b24bot.Button) (string, error)
}

// Messenger, on VibeCode, sends a notification to the Bitrix24 chat of a
// person whose Bitrix24 ID ReviewDesk knows, instead of the email.
var Messenger NotificationMessenger

// normalizeMentionIDs drops blanks and repeats, keeping the order people were
// mentioned in.
func normalizeMentionIDs(ids []string) ([]string, error) {
	seen := map[string]bool{}
	result := []string{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	if len(result) > maxMentionsPerMessage {
		return nil, fmt.Errorf("mention at most %d people in one message", maxMentionsPerMessage)
	}
	return result, nil
}

// saveMentions records who a message mentions and creates a notification for
// everyone mentioned except the author. Ids of people who do not exist are
// dropped. It returns the mentioned addresses and the new notification ids.
func saveMentions(
	ctx context.Context,
	db EmailEventExecutor,
	author AuthUser,
	emailID string,
	commentID string,
	messageID string,
	body string,
	userIDs []string,
) ([]string, []string, error) {
	mentionEmails := []string{}
	if len(userIDs) == 0 {
		return mentionEmails, nil, nil
	}

	rows, err := db.Query(ctx, `
		WITH inserted AS (
			INSERT INTO comment_mentions (message_id, user_id)
			SELECT $1, users.id
			FROM users
			WHERE users.id::text = ANY($2)
			ON CONFLICT DO NOTHING
			RETURNING user_id
		)
		SELECT lower(users.email)
		FROM inserted
		INNER JOIN users ON users.id = inserted.user_id
		ORDER BY lower(users.email);
	`, messageID, userIDs)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			rows.Close()
			return nil, nil, err
		}
		mentionEmails = append(mentionEmails, email)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	rows, err = db.Query(ctx, `
		INSERT INTO notifications (user_id, kind, actor_user_id, email_id, comment_id, message_id, excerpt)
		SELECT user_id, 'mention', $2, $3, $4, $1, $5
		FROM comment_mentions
		WHERE message_id = $1
			AND user_id <> $2
		RETURNING id;
	`, messageID, author.ID, emailID, commentID, mentionExcerpt(body))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	notificationIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, nil, err
		}
		notificationIDs = append(notificationIDs, id)
	}
	return mentionEmails, notificationIDs, rows.Err()
}

func mentionExcerpt(body string) string {
	body = strings.Join(strings.Fields(body), " ")
	if utf8.RuneCountInString(body) <= mentionExcerptLength {
		return body
	}
	runes := []rune(body)
	return strings.TrimSpace(string(runes[:mentionExcerptLength])) + "…"
}

type mentionDelivery struct {
	ID             string
	RecipientEmail string
	ActorEmail     string
	ActorName      string
	EmailID        string
	EmailTitle     string
	EmailLanguage  string
	CommentID      *string
	Excerpt        string
	PortalUserID   string
	// Recipient and CommentAuthorID decide whether the message offers to
	// resolve the thread.
	Recipient       AuthUser
	CommentAuthorID *string
}

// deliverMentionEmails sends each notification and records the outcome: to
// the person's Bitrix24 chat when it can, else by email. It runs after the
// comment is committed, so a slow or failing service never blocks or undoes
// the comment.
func deliverMentionEmails(ctx context.Context, dbpool *pgxpool.Pool, notificationIDs []string) {
	ctx, cancel := context.WithTimeout(ctx, mentionEmailTimeout*time.Duration(max(1, len(notificationIDs))))
	defer cancel()

	rows, err := dbpool.Query(ctx, `
		SELECT
			notifications.id,
			recipients.email,
			COALESCE(actors.email, ''),
			COALESCE(actors.name, ''),
			emails.id,
			emails.title,
			emails.language,
			notifications.comment_id,
			notifications.excerpt,
			COALESCE(recipients.portal_user_id, ''),
			recipients.id,
			recipients.role,
			comments.user_id
		FROM notifications
		INNER JOIN users AS recipients ON recipients.id = notifications.user_id
		INNER JOIN emails ON emails.id = notifications.email_id
		LEFT JOIN users AS actors ON actors.id = notifications.actor_user_id
		LEFT JOIN comments ON comments.id = notifications.comment_id
		WHERE notifications.id::text = ANY($1)
			AND notifications.email_status = 'pending';
	`, notificationIDs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load mention notifications: %v\n", err)
		return
	}
	deliveries := []mentionDelivery{}
	for rows.Next() {
		var d mentionDelivery
		if err := rows.Scan(&d.ID, &d.RecipientEmail, &d.ActorEmail, &d.ActorName, &d.EmailID, &d.EmailTitle, &d.EmailLanguage, &d.CommentID, &d.Excerpt, &d.PortalUserID, &d.Recipient.ID, &d.Recipient.Role, &d.CommentAuthorID); err != nil {
			rows.Close()
			fmt.Fprintf(os.Stderr, "failed to read mention notification: %v\n", err)
			return
		}
		deliveries = append(deliveries, d)
	}
	rows.Close()

	for _, delivery := range deliveries {
		bitrixStatus, bitrixMessageID := "skipped", ""
		if Messenger != nil && delivery.PortalUserID != "" {
			delivery.Recipient.Email = delivery.RecipientEmail
			canResolve := false
			if delivery.CommentID != nil {
				var err error
				if canResolve, err = mayResolveComment(ctx, dbpool, delivery.Recipient, delivery.CommentAuthorID, delivery.EmailID); err != nil {
					fmt.Fprintf(os.Stderr, "failed to check who may resolve mention %s: %v\n", delivery.ID, err)
				}
			}
			text, buttons := mentionMessageContent(delivery, appurl.Get(), canResolve)
			if id, err := Messenger.Post(ctx, delivery.PortalUserID, text, buttons...); err != nil {
				fmt.Fprintf(os.Stderr, "failed to message mention %s to Bitrix24: %v\n", delivery.ID, err)
				bitrixStatus = "failed"
			} else {
				bitrixStatus, bitrixMessageID = "sent", id
			}
		}

		status := "skipped"
		if Mailer != nil && bitrixStatus != "sent" {
			subject, text, htmlBody := mentionEmailContent(delivery, appurl.Get())
			if err := Mailer.Send(ctx, delivery.RecipientEmail, subject, text, htmlBody); err != nil {
				fmt.Fprintf(os.Stderr, "failed to email mention %s: %v\n", delivery.ID, err)
				status = "failed"
			} else {
				status = "sent"
			}
		}
		if _, err := dbpool.Exec(ctx, `
			UPDATE notifications
			SET email_status = $2,
				emailed_at = CASE WHEN $2 = 'sent' THEN now() ELSE emailed_at END,
				bitrix_status = $3,
				bitrix_sent_at = CASE WHEN $3 = 'sent' THEN now() ELSE bitrix_sent_at END,
				bitrix_message_id = NULLIF($4, '')
			WHERE id = $1;
		`, delivery.ID, status, bitrixStatus, bitrixMessageID); err != nil {
			fmt.Fprintf(os.Stderr, "failed to record mention email status %s: %v\n", delivery.ID, err)
		}
	}
}

// mentionReviewPath opens the email with the comment thread in focus.
func mentionReviewPath(emailID string, commentID *string) string {
	path := "/emails/" + url.PathEscape(emailID) + "/review"
	if commentID != nil {
		path += "?comment=" + url.QueryEscape(*commentID)
	}
	return path
}

// mentionMessageContent is the Bitrix24 message: the same words as the
// email, with the link as a button, and a button that resolves the thread for
// someone who may. A reply in the chat goes to the thread.
func mentionMessageContent(delivery mentionDelivery, baseURL string, canResolve bool) (string, []b24bot.Button) {
	actor, emailLabel := mentionActor(delivery), mentionEmailLabel(delivery)
	text := fmt.Sprintf("%s mentioned you in a comment on %s:\n\n%s", actor, emailLabel, delivery.Excerpt)
	buttons := []b24bot.Button{}
	if baseURL != "" {
		buttons = append(buttons, b24bot.Button{Text: "Open the comment", Link: baseURL + mentionReviewPath(delivery.EmailID, delivery.CommentID)})
	}
	if delivery.CommentID != nil {
		text += "\n\nReply here to answer in the thread."
		if canResolve {
			buttons = append(buttons, b24bot.Button{Text: "Resolve", Command: ResolveCommand, CommandParams: *delivery.CommentID})
		}
	}
	return text, buttons
}

func mentionActor(delivery mentionDelivery) string {
	switch {
	case delivery.ActorName != "":
		return delivery.ActorName
	case delivery.ActorEmail != "":
		return delivery.ActorEmail
	}
	return "Someone"
}

func mentionEmailLabel(delivery mentionDelivery) string {
	return emailLabel(delivery.EmailTitle, delivery.EmailLanguage)
}

func emailLabel(title string, language string) string {
	return fmt.Sprintf("%s (%s)", title, strings.ToUpper(language))
}

func mentionEmailContent(delivery mentionDelivery, baseURL string) (subject string, text string, htmlBody string) {
	actor, emailLabel := mentionActor(delivery), mentionEmailLabel(delivery)
	subject = fmt.Sprintf("%s mentioned you on %s", actor, emailLabel)

	link := ""
	if baseURL != "" {
		link = baseURL + mentionReviewPath(delivery.EmailID, delivery.CommentID)
	}

	text = fmt.Sprintf("%s mentioned you in a comment on %s:\n\n%s\n", actor, emailLabel, delivery.Excerpt)
	if link != "" {
		text += "\nOpen the comment: " + link + "\n"
	}
	text += "\nYou get this email because someone mentioned you in ReviewDesk.\n"

	linkHTML := ""
	if link != "" {
		linkHTML = fmt.Sprintf(`<p><a href="%s" style="color: #1c7ed6;">Open the comment</a></p>`, html.EscapeString(link))
	}
	htmlBody = fmt.Sprintf(`<!doctype html>
<html>
  <body style="font-family: Arial, sans-serif; color: #1f2937; line-height: 1.5;">
    <p><strong>%s</strong> mentioned you in a comment on <strong>%s</strong>:</p>
    <blockquote style="margin: 12px 0; padding: 8px 12px; border-left: 3px solid #d0d7de; color: #374151; white-space: pre-wrap;">%s</blockquote>
    %s
    <p style="color: #6b7280; font-size: 12px;">You get this email because someone mentioned you in ReviewDesk.</p>
  </body>
</html>`, html.EscapeString(actor), html.EscapeString(emailLabel), html.EscapeString(delivery.Excerpt), linkHTML)
	return subject, text, htmlBody
}
