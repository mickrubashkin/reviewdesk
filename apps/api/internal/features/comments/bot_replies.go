package comments

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/b24bot"
)

// ResolveCommand is the bot's command behind the "Resolve" button of a
// mention message; its parameter is the comment's ID.
const ResolveCommand = "resolve"

const (
	botPollInterval = 3 * time.Second
	botPollMaxPause = time.Minute
)

// BotInbox is the Bitrix24 bot that sends mentions and reads what people
// write back.
type BotInbox interface {
	Events(ctx context.Context) ([]b24bot.Event, bool, error)
	Post(ctx context.Context, dialogID string, text string, buttons ...b24bot.Button) (string, error)
}

// RunBotReplies reads the bot's events until ctx ends (#188): a message in a
// person's chat with the bot is a reply in the thread of the mention it quotes,
// or of the latest mention sent to them; the "Resolve" button resolves the
// thread.
func RunBotReplies(ctx context.Context, dbpool *pgxpool.Pool, bot BotInbox) {
	pause := botPollInterval
	for {
		events, more, err := bot.Events(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			fmt.Fprintf(os.Stderr, "failed to read the Bitrix24 bot's events: %v\n", err)
			pause = min(pause*2, botPollMaxPause)
		} else {
			pause = botPollInterval
			for _, event := range events {
				handleBotEvent(ctx, dbpool, bot, event)
			}
		}
		if more && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
	}
}

func handleBotEvent(ctx context.Context, dbpool *pgxpool.Pool, bot BotInbox, event b24bot.Event) {
	data := event.Data
	switch event.Type {
	case "ONIMBOTV2MESSAGEADD":
		// Only people's messages in their own chat with the bot.
		if data.Message.IsSystem || data.Message.AuthorID == data.Bot.ID || data.Chat.Type != "private" {
			return
		}
		handleBotMessage(ctx, dbpool, bot, event)
	case "ONIMBOTV2COMMANDADD":
		if strings.TrimPrefix(data.Command.Command, "/") == ResolveCommand {
			handleBotResolve(ctx, dbpool, bot, event)
		}
	}
}

// botThread is the comment thread a chat message answers.
type botThread struct {
	CommentID string
	EmailID   string
	Label     string
}

func handleBotMessage(ctx context.Context, dbpool *pgxpool.Pool, bot BotInbox, event b24bot.Event) {
	data := event.Data
	answer := func(text string, buttons ...b24bot.Button) { botAnswer(ctx, bot, data.Chat.DialogID, text, buttons...) }

	user, found, err := botUser(ctx, dbpool, string(data.User.ID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find the author of Bitrix24 message %s: %v\n", data.Message.ID, err)
		answer("Something went wrong, so your message was not added to ReviewDesk. Please try again later.")
		return
	}
	if !found {
		answer("I only take replies from people in ReviewDesk, and I don't know you there yet. Open ReviewDesk once from the portal, then reply again.")
		return
	}
	body := botMessageText(data.Message.Text)
	if body == "" {
		answer("I can only add text to a thread.")
		return
	}
	thread, found, err := botReplyThread(ctx, dbpool, user.ID, string(data.Message.Params.ReplyID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find the thread of Bitrix24 message %s: %v\n", data.Message.ID, err)
		answer("Something went wrong, so your reply was not added. Please try again later.")
		return
	}
	if !found {
		answer("I add replies to comment threads you were mentioned in, and there are none yet.")
		return
	}

	_, notificationIDs, err := addCommentReply(ctx, dbpool, user, thread.CommentID, body, nil, string(data.Message.ID))
	open := threadButton(thread)
	switch {
	case errors.Is(err, errReplyPosted):
		// The same event came twice.
	case errors.Is(err, errCommentNotFound):
		answer("The comment on " + thread.Label + " was deleted, so your reply was not added.")
	case errors.Is(err, errCommentResolved):
		answer("The thread on "+thread.Label+" is resolved, so your reply was not added.", open...)
	case errors.Is(err, errEmailArchived):
		answer(thread.Label+" is archived, so your reply was not added.", open...)
	case err != nil:
		fmt.Fprintf(os.Stderr, "failed to add Bitrix24 message %s to comment %s: %v\n", data.Message.ID, thread.CommentID, err)
		answer("Something went wrong, so your reply was not added. Please try again later.")
	default:
		if len(notificationIDs) > 0 {
			go deliverMentionEmails(context.WithoutCancel(ctx), dbpool, notificationIDs)
		}
		answer("Added to the thread on "+thread.Label+".", open...)
	}
}

func handleBotResolve(ctx context.Context, dbpool *pgxpool.Pool, bot BotInbox, event b24bot.Event) {
	data := event.Data
	answer := func(text string, buttons ...b24bot.Button) { botAnswer(ctx, bot, data.Chat.DialogID, text, buttons...) }

	user, found, err := botUser(ctx, dbpool, string(data.User.ID))
	if err != nil || !found {
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to find who pressed Resolve: %v\n", err)
		}
		answer("I couldn't resolve the thread. Please resolve it in ReviewDesk.")
		return
	}
	commentID := strings.TrimSpace(data.Command.Params)
	var thread botThread
	var status, title, language string
	err = dbpool.QueryRow(ctx, `
		SELECT comments.id, comments.email_id, comments.status, emails.title, emails.language
		FROM comments
		INNER JOIN emails ON emails.id = comments.email_id
		WHERE comments.id::text = $1;
	`, commentID).Scan(&thread.CommentID, &thread.EmailID, &status, &title, &language)
	if errors.Is(err, pgx.ErrNoRows) {
		answer("That comment was deleted.")
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load comment %s to resolve: %v\n", commentID, err)
		answer("Something went wrong, so the thread was not resolved. Please try again later.")
		return
	}
	thread.Label = emailLabel(title, language)
	open := threadButton(thread)
	if status != "open" {
		answer("The thread on "+thread.Label+" is already resolved.", open...)
		return
	}
	_, err = resolveComment(ctx, dbpool, user, thread.CommentID)
	switch {
	case errors.Is(err, errCommentNotFound):
		answer("That comment was deleted.")
	case errors.Is(err, errCannotResolve):
		answer("Only the comment's author or a board admin can resolve the thread on "+thread.Label+".", open...)
	case err != nil:
		fmt.Fprintf(os.Stderr, "failed to resolve comment %s from Bitrix24: %v\n", thread.CommentID, err)
		answer("Something went wrong, so the thread was not resolved. Please try again later.")
	default:
		answer("Resolved the thread on "+thread.Label+".", open...)
	}
}

// botUser is the ReviewDesk user with this Bitrix24 ID.
func botUser(ctx context.Context, dbpool *pgxpool.Pool, portalUserID string) (AuthUser, bool, error) {
	var user AuthUser
	err := dbpool.QueryRow(ctx, `
		SELECT id, email, role FROM users WHERE portal_user_id = $1;
	`, portalUserID).Scan(&user.ID, &user.Email, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) || portalUserID == "" {
		return AuthUser{}, false, nil
	}
	return user, err == nil, err
}

// botReplyThread is the thread of the mention message quotedID, or, when the
// reply quotes none of them, of the latest mention sent to the user's chat.
func botReplyThread(ctx context.Context, dbpool *pgxpool.Pool, userID string, quotedID string) (botThread, bool, error) {
	if quotedID != "" {
		if thread, found, err := mentionThread(ctx, dbpool, userID, quotedID); found || err != nil {
			return thread, found, err
		}
	}
	return mentionThread(ctx, dbpool, userID, "")
}

// mentionThread is the thread of the user's mention message messageID, or of
// their latest one when messageID is "".
func mentionThread(ctx context.Context, dbpool *pgxpool.Pool, userID string, messageID string) (botThread, bool, error) {
	var thread botThread
	var title, language string
	err := dbpool.QueryRow(ctx, `
		SELECT notifications.comment_id, notifications.email_id, emails.title, emails.language
		FROM notifications
		INNER JOIN emails ON emails.id = notifications.email_id
		WHERE notifications.user_id = $1
			AND notifications.comment_id IS NOT NULL
			AND notifications.bitrix_status = 'sent'
			AND ($2 = '' OR notifications.bitrix_message_id = $2)
		ORDER BY notifications.created_at DESC
		LIMIT 1;
	`, userID, messageID).Scan(&thread.CommentID, &thread.EmailID, &title, &language)
	if errors.Is(err, pgx.ErrNoRows) {
		return botThread{}, false, nil
	}
	if err != nil {
		return botThread{}, false, err
	}
	thread.Label = emailLabel(title, language)
	return thread, true, nil
}

// bbCode matches the Bitrix24 formatting a chat message can carry, such as
// [b]…[/b] or [USER=5]Name[/USER]; the text inside stays.
var bbCode = regexp.MustCompile(`(?i)\[/?(b|i|u|s|url|user|chat|color|size|quote|code|icon)(=[^\]]*)?\]`)

func botMessageText(text string) string {
	return strings.TrimSpace(bbCode.ReplaceAllString(text, ""))
}

func threadButton(thread botThread) []b24bot.Button {
	baseURL := appURL()
	if baseURL == "" {
		return nil
	}
	commentID := thread.CommentID
	return []b24bot.Button{{Text: "Open the thread", Link: baseURL + mentionReviewPath(thread.EmailID, &commentID)}}
}

func botAnswer(ctx context.Context, bot BotInbox, dialogID string, text string, buttons ...b24bot.Button) {
	if _, err := bot.Post(ctx, dialogID, text, buttons...); err != nil {
		fmt.Fprintf(os.Stderr, "failed to answer in Bitrix24 chat %s: %v\n", dialogID, err)
	}
}
