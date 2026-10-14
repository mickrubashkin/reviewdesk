package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
)

type AuthUser = auth.AuthUser

type EmailContentParts = emailtext.ContentParts

type EmailDetail struct {
	ID                       string          `json:"id"`
	Slug                     string          `json:"slug"`
	Sequence                 string          `json:"sequence"`
	Title                    string          `json:"title"`
	Subject                  *string         `json:"subject"`
	Preheader                *string         `json:"preheader"`
	SendTiming               *string         `json:"send_timing"`
	Stage                    string          `json:"stage"`
	SortOrder                int             `json:"sort_order"`
	Language                 string          `json:"language"`
	Variant                  string          `json:"variant"`
	AdaptationKey            string          `json:"adaptation_key"`
	AdaptationLabel          string          `json:"adaptation_label"`
	ReviewStatus             string          `json:"review_status"`
	OwnerEmail               *string         `json:"owner_email"`
	ReviewerEmail            *string         `json:"reviewer_email"`
	DueDate                  *string         `json:"due_date"`
	ImplementationNotes      *string         `json:"implementation_notes"`
	OpenCommentCount         int             `json:"open_comment_count"`
	OpenBlockingCommentCount int             `json:"open_blocking_comment_count"`
	BodyText                 *string         `json:"-"`
	ContentParts             *string         `json:"-"`
	UpdatedAt                time.Time       `json:"-"`
	OriginalHTML             string          `json:"original_html"`
	ReviewHTML               *string         `json:"review_html"`
	TemplateHTML             string          `json:"template_html"`
	TemplateHash             *string         `json:"template_hash"`
	EditableFields           json.RawMessage `json:"editable_fields"`
}

type EmailAnalysis struct {
	Summary         string                `json:"summary"`
	Score           int                   `json:"score"`
	Verdict         string                `json:"verdict"`
	Checks          EmailAnalysisChecks   `json:"checks"`
	Recommendations []EmailRecommendation `json:"recommendations"`
}

type EmailAnalysisChecks struct {
	Subject        string `json:"subject"`
	Preheader      string `json:"preheader"`
	Focus          string `json:"focus"`
	CTA            string `json:"cta"`
	StageAlignment string `json:"stage_alignment"`
	Readability    string `json:"readability"`
}

type EmailRecommendation struct {
	Priority string `json:"priority"`
	Title    string `json:"title"`
	Details  string `json:"details"`
}

type AIAnalysisResult struct {
	Analysis EmailAnalysis
	Metrics  AIAnalysisMetrics
}

type AIAnalysisMetrics struct {
	Model        string
	Status       string
	LatencyMS    int
	InputTokens  *int
	OutputTokens *int
	TotalTokens  *int
	CachedTokens *int
	ErrorMessage *string
}

type AIAnalysisLogItem struct {
	ID           string    `json:"id"`
	EmailID      *string   `json:"email_id"`
	EmailTitle   *string   `json:"email_title"`
	EmailSlug    *string   `json:"email_slug"`
	Language     *string   `json:"language"`
	Variant      *string   `json:"variant"`
	UserID       *string   `json:"user_id"`
	UserEmail    *string   `json:"user_email"`
	Model        string    `json:"model"`
	Status       string    `json:"status"`
	CacheStatus  string    `json:"cache_status"`
	LatencyMS    int       `json:"latency_ms"`
	InputTokens  *int      `json:"input_tokens"`
	OutputTokens *int      `json:"output_tokens"`
	TotalTokens  *int      `json:"total_tokens"`
	CachedTokens *int      `json:"cached_tokens"`
	ErrorMessage *string   `json:"error_message"`
	CreatedAt    time.Time `json:"created_at"`
}

type EventLogger interface {
	LogEvent(ctx context.Context, db EmailEventExecutor, actor AuthUser, action string, emailID string, emailSlug string, emailTitle string, metadata map[string]any) error
}

type EmailEventExecutor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type OpsLogger interface {
	LogError(ctx context.Context, r *http.Request, db *pgxpool.Pool, eventType, message, emailID string, err error)
}

var Logger EventLogger
var SystemLogger OpsLogger
