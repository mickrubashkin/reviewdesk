package comments

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type AuthUser = auth.AuthUser

type EmailComment struct {
	ID              string                `json:"id"`
	EmailID         string                `json:"email_id"`
	UserID          *string               `json:"user_id"`
	AuthorEmail     *string               `json:"author_email"`
	ReviewBlock     string                `json:"review_block"`
	SelectedText    string                `json:"selected_text"`
	StartOffset     int                   `json:"start_offset"`
	EndOffset       int                   `json:"end_offset"`
	Body            string                `json:"body"`
	Status          string                `json:"status"`
	Severity        string                `json:"severity"`
	CreatedAt       time.Time             `json:"created_at"`
	ResolvedAt      *time.Time            `json:"resolved_at"`
	ResolvedBy      *string               `json:"resolved_by"`
	ResolvedByEmail *string               `json:"resolved_by_email"`
	Messages        []EmailCommentMessage `json:"messages"`
}

type EmailCommentMessage struct {
	ID          string    `json:"id"`
	CommentID   string    `json:"comment_id"`
	UserID      *string   `json:"user_id"`
	AuthorEmail *string   `json:"author_email"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type EventLogger interface {
	LogEvent(ctx context.Context, db EmailEventExecutor, actor AuthUser, action string, emailID string, emailSlug string, emailTitle string, metadata map[string]any) error
}

type EmailEventExecutor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

var Logger EventLogger
