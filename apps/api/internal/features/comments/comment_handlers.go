package comments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type createEmailCommentRequest struct {
	ReviewBlock  string `json:"review_block"`
	SelectedText string `json:"selected_text"`
	StartOffset  int    `json:"start_offset"`
	EndOffset    int    `json:"end_offset"`
	Body         string `json:"body"`
	Severity     string `json:"severity"`
	// MentionUserIDs are the people @mentioned in body.
	MentionUserIDs []string `json:"mention_user_ids"`
}

type createCommentMessageRequest struct {
	Body           string   `json:"body"`
	MentionUserIDs []string `json:"mention_user_ids"`
}

func RegisterCommentRoutes(r chi.Router, dbpool *pgxpool.Pool) {
	r.Get("/api/emails/{id}/comments", listEmailCommentsHandler(dbpool))
	r.Post("/api/emails/{id}/comments", createEmailCommentHandler(dbpool))
	r.Post("/api/comments/{id}/messages", createCommentMessageHandler(dbpool))
	r.Patch("/api/comments/{id}/resolve", resolveCommentHandler(dbpool))
	registerNotificationRoutes(r, dbpool)
}

func listEmailCommentsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		emailID := chi.URLParam(r, "id")
		rows, err := dbpool.Query(r.Context(), `
			SELECT
				comments.id,
				comments.email_id,
				comments.user_id,
				authors.email AS author_email,
				comments.review_block,
				comments.selected_text,
				comments.start_offset,
				comments.end_offset,
				comments.body,
				comments.status,
				comments.severity,
				comments.created_at,
				comments.resolved_at,
				comments.resolved_by,
				resolvers.email AS resolved_by_email
			FROM comments
			LEFT JOIN users AS authors ON authors.id = comments.user_id
			LEFT JOIN users AS resolvers ON resolvers.id = comments.resolved_by
			WHERE comments.email_id = $1
			ORDER BY comments.created_at;
		`, emailID)
		if err != nil {
			http.Error(w, "failed to load comments", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		comments := []EmailComment{}

		for rows.Next() {
			var comment EmailComment

			err := rows.Scan(
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
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to scan comment row for email %s: %v\n", emailID, err)
				http.Error(w, "failed to read email comments", http.StatusInternalServerError)
				return
			}

			comments = append(comments, comment)
		}

		if err := rows.Err(); err != nil {
			http.Error(w, "failed to read email comments", http.StatusInternalServerError)
			return
		}

		if err := attachCommentMessages(r.Context(), dbpool, comments); err != nil {
			http.Error(w, "failed to read email comment messages", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(comments)
	}
}

func createEmailCommentHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		emailID := chi.URLParam(r, "id")
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var payload createEmailCommentRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid comment payload", http.StatusBadRequest)
			return
		}

		payload.ReviewBlock = strings.TrimSpace(payload.ReviewBlock)
		payload.SelectedText = strings.TrimSpace(payload.SelectedText)
		payload.Body = strings.TrimSpace(payload.Body)
		payload.Severity = normalizeCommentSeverity(payload.Severity)

		if payload.ReviewBlock == "" ||
			payload.SelectedText == "" ||
			payload.Body == "" ||
			payload.Severity == "" ||
			payload.StartOffset < 0 ||
			payload.EndOffset <= payload.StartOffset {
			http.Error(w, "invalid comment payload", http.StatusBadRequest)
			return
		}
		mentionIDs, err := normalizeMentionIDs(payload.MentionUserIDs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to create comment", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		emailSlug, emailTitle, archived, err := loadCommentTarget(r.Context(), tx, emailID)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to create comment", http.StatusInternalServerError)
			return
		}
		if archived {
			http.Error(w, errArchivedEmail, http.StatusConflict)
			return
		}

		var comment EmailComment
		err = tx.QueryRow(r.Context(), `
			INSERT INTO comments (
				email_id,
				user_id,
				review_block,
				selected_text,
				start_offset,
				end_offset,
				body,
				severity
			)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				RETURNING
					id,
					email_id,
					user_id,
					review_block,
					selected_text,
					start_offset,
					end_offset,
					body,
					status,
					severity,
					created_at,
					resolved_at;
		`, emailID,
			user.ID,
			payload.ReviewBlock,
			payload.SelectedText,
			payload.StartOffset,
			payload.EndOffset,
			payload.Body,
			payload.Severity).Scan(
			&comment.ID,
			&comment.EmailID,
			&comment.UserID,
			&comment.ReviewBlock,
			&comment.SelectedText,
			&comment.StartOffset,
			&comment.EndOffset,
			&comment.Body,
			&comment.Status,
			&comment.Severity,
			&comment.CreatedAt,
			&comment.ResolvedAt,
		)
		if err != nil {
			http.Error(w, "failed to create comment", http.StatusInternalServerError)
			return
		}

		var message EmailCommentMessage
		err = tx.QueryRow(r.Context(), `
			INSERT INTO comment_messages (
				comment_id,
				user_id,
				body
			)
				VALUES ($1, $2, $3)
				RETURNING id, comment_id, user_id, body, created_at, updated_at;
		`, comment.ID, user.ID, payload.Body).Scan(
			&message.ID,
			&message.CommentID,
			&message.UserID,
			&message.Body,
			&message.CreatedAt,
			&message.UpdatedAt,
		)
		if err != nil {
			http.Error(w, "failed to create comment", http.StatusInternalServerError)
			return
		}

		mentionEmails, notificationIDs, err := saveMentions(r.Context(), tx, user, emailID, comment.ID, message.ID, message.Body, mentionIDs)
		if err != nil {
			http.Error(w, "failed to save mentions", http.StatusInternalServerError)
			return
		}
		message.MentionEmails = mentionEmails

		if Logger != nil {
			if err := Logger.LogEvent(r.Context(), tx, user, "comment_created", emailID, emailSlug, emailTitle, map[string]any{
				"comment_id":    comment.ID,
				"review_block":  comment.ReviewBlock,
				"selected_text": comment.SelectedText,
				"severity":      comment.Severity,
				"mentions":      mentionEmails,
			}); err != nil {
				http.Error(w, "failed to record comment event", http.StatusInternalServerError)
				return
			}
		}

		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to create comment", http.StatusInternalServerError)
			return
		}

		sendMentionEmails(dbpool, r, notificationIDs)

		comment.AuthorEmail = &user.Email
		message.AuthorEmail = &user.Email
		comment.Messages = []EmailCommentMessage{message}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(comment)
	}
}

func createCommentMessageHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commentID := chi.URLParam(r, "id")
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var payload createCommentMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid comment message payload", http.StatusBadRequest)
			return
		}

		payload.Body = strings.TrimSpace(payload.Body)
		if payload.Body == "" {
			http.Error(w, "invalid comment message payload", http.StatusBadRequest)
			return
		}
		mentionIDs, err := normalizeMentionIDs(payload.MentionUserIDs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to create comment message", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		var status string
		var emailID string
		var emailSlug string
		var emailTitle string
		var reviewBlock string
		var archived bool
		err = tx.QueryRow(r.Context(), `
			SELECT comments.status, comments.email_id, emails.slug, emails.title, comments.review_block,
				emails.archived_at IS NOT NULL
			FROM comments
			INNER JOIN emails ON emails.id = comments.email_id
			WHERE comments.id = $1;
		`, commentID).Scan(&status, &emailID, &emailSlug, &emailTitle, &reviewBlock, &archived)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "comment not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to create comment message", http.StatusInternalServerError)
			return
		}
		if status != "open" {
			http.Error(w, "comment is resolved", http.StatusConflict)
			return
		}
		if archived {
			http.Error(w, errArchivedEmail, http.StatusConflict)
			return
		}

		var message EmailCommentMessage
		err = tx.QueryRow(r.Context(), `
			INSERT INTO comment_messages (
				comment_id,
				user_id,
				body
			)
				VALUES ($1, $2, $3)
				RETURNING id, comment_id, user_id, body, created_at, updated_at;
		`, commentID, user.ID, payload.Body).Scan(
			&message.ID,
			&message.CommentID,
			&message.UserID,
			&message.Body,
			&message.CreatedAt,
			&message.UpdatedAt,
		)
		if err != nil {
			http.Error(w, "failed to create comment message", http.StatusInternalServerError)
			return
		}

		mentionEmails, notificationIDs, err := saveMentions(r.Context(), tx, user, emailID, commentID, message.ID, message.Body, mentionIDs)
		if err != nil {
			http.Error(w, "failed to save mentions", http.StatusInternalServerError)
			return
		}
		message.MentionEmails = mentionEmails

		if Logger != nil {
			if err := Logger.LogEvent(r.Context(), tx, user, "comment_replied", emailID, emailSlug, emailTitle, map[string]any{
				"comment_id":   commentID,
				"message_id":   message.ID,
				"review_block": reviewBlock,
				"body":         message.Body,
				"mentions":     mentionEmails,
			}); err != nil {
				http.Error(w, "failed to record comment event", http.StatusInternalServerError)
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to create comment message", http.StatusInternalServerError)
			return
		}

		sendMentionEmails(dbpool, r, notificationIDs)

		message.AuthorEmail = &user.Email
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(message)
	}
}

func resolveCommentHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commentID := chi.URLParam(r, "id")

		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to resolve comment", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		var authorID *string
		var emailID string
		err = tx.QueryRow(r.Context(), `
			SELECT user_id, email_id::text
			FROM comments
			WHERE id = $1
			FOR UPDATE;
		`, commentID).Scan(&authorID, &emailID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "comment not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to resolve comment", http.StatusInternalServerError)
			return
		}
		allowed := canResolveComment(user, authorID)
		if !allowed {
			allowed, err = coreauth.CanManageEmail(r.Context(), tx, user, emailID)
			if err != nil {
				http.Error(w, "failed to resolve comment", http.StatusInternalServerError)
				return
			}
		}
		if !allowed {
			http.Error(w, "only the comment author or a board admin can resolve this comment", http.StatusForbidden)
			return
		}

		var comment EmailComment
		var emailSlug string
		var emailTitle string
		err = tx.QueryRow(r.Context(), `
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
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "comment not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to resolve comment", http.StatusInternalServerError)
			return
		}

		if Logger != nil {
			if err := Logger.LogEvent(r.Context(), tx, user, "comment_resolved", comment.EmailID, emailSlug, emailTitle, map[string]any{
				"comment_id":   comment.ID,
				"review_block": comment.ReviewBlock,
				"severity":     comment.Severity,
			}); err != nil {
				http.Error(w, "failed to record comment event", http.StatusInternalServerError)
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to resolve comment", http.StatusInternalServerError)
			return
		}

		comments := []EmailComment{comment}
		if err := attachCommentMessages(r.Context(), dbpool, comments); err != nil {
			http.Error(w, "failed to read comment messages", http.StatusInternalServerError)
			return
		}
		comment = comments[0]

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(comment)
	}
}

// canResolveComment keeps reviewers from closing other people's comments.
func canResolveComment(user coreauth.AuthUser, authorID *string) bool {
	if coreauth.IsAdmin(user) {
		return true
	}
	return authorID != nil && *authorID == user.ID
}

func normalizeCommentSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "issue"
	case "suggestion", "issue":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func attachCommentMessages(ctx context.Context, dbpool *pgxpool.Pool, comments []EmailComment) error {
	if len(comments) == 0 {
		return nil
	}

	commentIDs := make([]string, 0, len(comments))
	commentIndexByID := make(map[string]int, len(comments))
	for index := range comments {
		commentIDs = append(commentIDs, comments[index].ID)
		commentIndexByID[comments[index].ID] = index
		comments[index].Messages = []EmailCommentMessage{}
	}

	rows, err := dbpool.Query(ctx, `
		SELECT
			comment_messages.id,
			comment_messages.comment_id,
			comment_messages.user_id,
			authors.email AS author_email,
			comment_messages.body,
			comment_messages.created_at,
			comment_messages.updated_at,
			COALESCE((
				SELECT array_agg(lower(mentioned.email) ORDER BY lower(mentioned.email))
				FROM comment_mentions
				INNER JOIN users AS mentioned ON mentioned.id = comment_mentions.user_id
				WHERE comment_mentions.message_id = comment_messages.id
			), '{}') AS mention_emails
		FROM comment_messages
		LEFT JOIN users AS authors ON authors.id = comment_messages.user_id
		WHERE comment_messages.comment_id::text = ANY($1)
		ORDER BY comment_messages.created_at, comment_messages.id;
	`, commentIDs)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var message EmailCommentMessage
		if err := rows.Scan(
			&message.ID,
			&message.CommentID,
			&message.UserID,
			&message.AuthorEmail,
			&message.Body,
			&message.CreatedAt,
			&message.UpdatedAt,
			&message.MentionEmails,
		); err != nil {
			return err
		}

		index, ok := commentIndexByID[message.CommentID]
		if ok {
			comments[index].Messages = append(comments[index].Messages, message)
		}
	}

	return rows.Err()
}

// errArchivedEmail refuses a new comment or reply on an archived email: it
// is history, read-only until restored.
const errArchivedEmail = "the email is archived: restore it to comment"

// loadCommentTarget reads what the comment events name the email by, and
// whether it is archived.
func loadCommentTarget(ctx context.Context, db EmailEventExecutor, emailID string) (slug string, title string, archived bool, err error) {
	err = db.QueryRow(ctx, `
		SELECT slug, title, archived_at IS NOT NULL
		FROM emails
		WHERE id = $1;
	`, emailID).Scan(&slug, &title, &archived)
	return slug, title, archived, err
}

// sendMentionEmails emails the new mention notifications in the background,
// after the request's transaction is committed.
func sendMentionEmails(dbpool *pgxpool.Pool, r *http.Request, notificationIDs []string) {
	if len(notificationIDs) == 0 {
		return
	}
	go deliverMentionEmails(context.WithoutCancel(r.Context()), dbpool, notificationIDs)
}
