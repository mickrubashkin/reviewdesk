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
}

// deliverMentionEmails emails each notification and records the outcome. It
// runs after the comment is committed, so a slow or failing mail service
// never blocks or undoes the comment.
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
			notifications.excerpt
		FROM notifications
		INNER JOIN users AS recipients ON recipients.id = notifications.user_id
		INNER JOIN emails ON emails.id = notifications.email_id
		LEFT JOIN users AS actors ON actors.id = notifications.actor_user_id
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
		if err := rows.Scan(&d.ID, &d.RecipientEmail, &d.ActorEmail, &d.ActorName, &d.EmailID, &d.EmailTitle, &d.EmailLanguage, &d.CommentID, &d.Excerpt); err != nil {
			rows.Close()
			fmt.Fprintf(os.Stderr, "failed to read mention notification: %v\n", err)
			return
		}
		deliveries = append(deliveries, d)
	}
	rows.Close()

	for _, delivery := range deliveries {
		status := "skipped"
		if Mailer != nil {
			subject, text, htmlBody := mentionEmailContent(delivery, appURL())
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
				emailed_at = CASE WHEN $2 = 'sent' THEN now() ELSE emailed_at END
			WHERE id = $1;
		`, delivery.ID, status); err != nil {
			fmt.Fprintf(os.Stderr, "failed to record mention email status %s: %v\n", delivery.ID, err)
		}
	}
}

// appURL is where links in notification emails point: AUTH_APP_URL, or the
// first allowed origin.
func appURL() string {
	if value := strings.TrimSpace(os.Getenv("AUTH_APP_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	origins := strings.TrimSpace(os.Getenv("AUTH_ALLOWED_ORIGINS"))
	if origins == "" {
		origins = strings.TrimSpace(os.Getenv("CORS_ORIGIN"))
	}
	for _, origin := range strings.Split(origins, ",") {
		if origin = strings.TrimRight(strings.TrimSpace(origin), "/"); origin != "" {
			return origin
		}
	}
	return ""
}

// mentionReviewPath opens the email with the comment thread in focus.
func mentionReviewPath(emailID string, commentID *string) string {
	path := "/emails/" + url.PathEscape(emailID) + "/review"
	if commentID != nil {
		path += "?comment=" + url.QueryEscape(*commentID)
	}
	return path
}

func mentionEmailContent(delivery mentionDelivery, baseURL string) (subject string, text string, htmlBody string) {
	actor := delivery.ActorName
	if actor == "" {
		actor = delivery.ActorEmail
	}
	if actor == "" {
		actor = "Someone"
	}
	emailLabel := fmt.Sprintf("%s (%s)", delivery.EmailTitle, strings.ToUpper(delivery.EmailLanguage))
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
