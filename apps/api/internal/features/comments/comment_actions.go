package comments

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

// A reply and a resolve come from the app and from the Bitrix24 bot's chat;
// both go through these.
var (
	errCommentNotFound  = errors.New("comment not found")
	errCommentResolved  = errors.New("comment is resolved")
	errEmailArchived    = errors.New(errArchivedEmail)
	errCannotResolve    = errors.New("only the comment author or a board admin can resolve this comment")
	errMentionsNotSaved = errors.New("failed to save mentions")
	errEventNotRecorded = errors.New("failed to record comment event")
	// errReplyPosted: the chat message was posted to the thread before.
	errReplyPosted = errors.New("reply already posted")
)

// addCommentReply adds user's message to the comment's thread and returns it
// with the notifications for the people it mentions, to deliver after.
// bitrixMessageID is the Bitrix24 chat message the reply came from, or "".
func addCommentReply(ctx context.Context, dbpool *pgxpool.Pool, user AuthUser, commentID string, body string, mentionIDs []string, bitrixMessageID string) (EmailCommentMessage, []string, error) {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return EmailCommentMessage{}, nil, err
	}
	defer tx.Rollback(ctx)

	var status, emailID, emailSlug, emailTitle, reviewBlock string
	var archived bool
	err = tx.QueryRow(ctx, `
		SELECT comments.status, comments.email_id, emails.slug, emails.title, comments.review_block,
			emails.archived_at IS NOT NULL
		FROM comments
		INNER JOIN emails ON emails.id = comments.email_id
		WHERE comments.id = $1;
	`, commentID).Scan(&status, &emailID, &emailSlug, &emailTitle, &reviewBlock, &archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return EmailCommentMessage{}, nil, errCommentNotFound
	}
	if err != nil {
		return EmailCommentMessage{}, nil, err
	}
	if status != "open" {
		return EmailCommentMessage{}, nil, errCommentResolved
	}
	if archived {
		return EmailCommentMessage{}, nil, errEmailArchived
	}

	var message EmailCommentMessage
	err = tx.QueryRow(ctx, `
		INSERT INTO comment_messages (
			comment_id,
			user_id,
			body,
			bitrix_message_id
		)
			VALUES ($1, $2, $3, NULLIF($4, ''))
			ON CONFLICT (bitrix_message_id) DO NOTHING
			RETURNING id, comment_id, user_id, body, created_at, updated_at;
	`, commentID, user.ID, body, bitrixMessageID).Scan(
		&message.ID,
		&message.CommentID,
		&message.UserID,
		&message.Body,
		&message.CreatedAt,
		&message.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return EmailCommentMessage{}, nil, errReplyPosted
	}
	if err != nil {
		return EmailCommentMessage{}, nil, err
	}

	mentionEmails, notificationIDs, err := saveMentions(ctx, tx, user, emailID, commentID, message.ID, message.Body, mentionIDs)
	if err != nil {
		return EmailCommentMessage{}, nil, fmt.Errorf("%w: %v", errMentionsNotSaved, err)
	}
	message.MentionEmails = mentionEmails

	if Logger != nil {
		if err := Logger.LogEvent(ctx, tx, user, "comment_replied", emailID, emailSlug, emailTitle, map[string]any{
			"comment_id":   commentID,
			"message_id":   message.ID,
			"review_block": reviewBlock,
			"body":         message.Body,
			"mentions":     mentionEmails,
		}); err != nil {
			return EmailCommentMessage{}, nil, fmt.Errorf("%w: %v", errEventNotRecorded, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return EmailCommentMessage{}, nil, err
	}
	return message, notificationIDs, nil
}

// resolveComment resolves the comment's thread as user, when user may: the
// comment's author, an admin or a manager of its board.
func resolveComment(ctx context.Context, dbpool *pgxpool.Pool, user AuthUser, commentID string) (EmailComment, error) {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return EmailComment{}, err
	}
	defer tx.Rollback(ctx)

	var authorID *string
	var emailID string
	err = tx.QueryRow(ctx, `
		SELECT user_id, email_id::text
		FROM comments
		WHERE id = $1
		FOR UPDATE;
	`, commentID).Scan(&authorID, &emailID)
	if errors.Is(err, pgx.ErrNoRows) {
		return EmailComment{}, errCommentNotFound
	}
	if err != nil {
		return EmailComment{}, err
	}
	allowed, err := mayResolveComment(ctx, tx, user, authorID, emailID)
	if err != nil {
		return EmailComment{}, err
	}
	if !allowed {
		return EmailComment{}, errCannotResolve
	}

	var comment EmailComment
	var emailSlug string
	var emailTitle string
	err = tx.QueryRow(ctx, `
		WITH updated_comment AS (
			UPDATE comments
			SET
				status = 'resolved',
				resolved_at = now(),
				resolved_by = $2
			WHERE id = $1
			RETURNING *
		)
		SELECT
			updated_comment.id,
			updated_comment.email_id,
			updated_comment.user_id,
			authors.email AS author_email,
			updated_comment.review_block,
			updated_comment.selected_text,
			updated_comment.start_offset,
			updated_comment.end_offset,
			updated_comment.body,
			updated_comment.status,
			updated_comment.severity,
			updated_comment.created_at,
			updated_comment.resolved_at,
			updated_comment.resolved_by,
			resolvers.email AS resolved_by_email,
			emails.slug,
			emails.title
		FROM updated_comment
		INNER JOIN emails ON emails.id = updated_comment.email_id
		LEFT JOIN users AS authors ON authors.id = updated_comment.user_id
		LEFT JOIN users AS resolvers ON resolvers.id = updated_comment.resolved_by;
	`, commentID, user.ID).Scan(
		&comment.ID,
		&comment.EmailID,
		&comment.UserID,
		&comment.AuthorEmail,
		&comment.ReviewBlock,
		&comment.SelectedText,
		&comment.StartOffset,
		&comment.EndOffset,
		&comment.Body,
		&comment.Status,
		&comment.Severity,
		&comment.CreatedAt,
		&comment.ResolvedAt,
		&comment.ResolvedBy,
		&comment.ResolvedByEmail,
		&emailSlug,
		&emailTitle,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return EmailComment{}, errCommentNotFound
	}
	if err != nil {
		return EmailComment{}, err
	}

	if Logger != nil {
		if err := Logger.LogEvent(ctx, tx, user, "comment_resolved", comment.EmailID, emailSlug, emailTitle, map[string]any{
			"comment_id":   comment.ID,
			"review_block": comment.ReviewBlock,
			"severity":     comment.Severity,
		}); err != nil {
			return EmailComment{}, fmt.Errorf("%w: %v", errEventNotRecorded, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return EmailComment{}, err
	}
	return comment, nil
}

// mayResolveComment: the comment's author, an admin or a manager of the
// email's board.
func mayResolveComment(ctx context.Context, db EmailEventExecutor, user AuthUser, authorID *string, emailID string) (bool, error) {
	if canResolveComment(user, authorID) {
		return true, nil
	}
	return coreauth.CanManageEmail(ctx, db, user, emailID)
}
