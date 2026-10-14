package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AIAnalysisLogFilters struct {
	Status      string
	CacheStatus string
	EmailID     string
	Model       string
	Limit       int
}

func insertAIAnalysisLog(ctx context.Context, dbpool *pgxpool.Pool, emailID string, user *AuthUser, metrics AIAnalysisMetrics, forceRefresh bool) error {
	var userID *string
	var userEmail *string
	if user != nil {
		userID = &user.ID
		userEmail = &user.Email
	}

	_, err := dbpool.Exec(ctx, `
		INSERT INTO ai_analysis_logs (
			email_id,
			user_id,
			user_email,
			model,
			status,
			latency_ms,
			input_tokens,
			output_tokens,
			total_tokens,
			cached_tokens,
			force_refresh,
			error_message
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);
	`,
		emailID,
		userID,
		userEmail,
		metrics.Model,
		metrics.Status,
		metrics.LatencyMS,
		metrics.InputTokens,
		metrics.OutputTokens,
		metrics.TotalTokens,
		metrics.CachedTokens,
		forceRefresh,
		metrics.ErrorMessage,
	)

	return err
}

func countDailyAIAnalysisRefreshes(ctx context.Context, dbpool *pgxpool.Pool, emailID string, userID string) (int, error) {
	var count int
	err := dbpool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM ai_analysis_logs
		WHERE email_id = $1
			AND user_id = $2
			AND force_refresh = TRUE
			AND created_at >= date_trunc('day', now());
	`, emailID, userID).Scan(&count)
	return count, err
}

func listAIAnalysisLogs(ctx context.Context, dbpool *pgxpool.Pool, filters AIAnalysisLogFilters) ([]AIAnalysisLogItem, error) {
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	where := []string{}
	args := []any{}

	if filters.Status != "" {
		args = append(args, filters.Status)
		where = append(where, fmt.Sprintf("l.status = $%d", len(args)))
	}
	if filters.CacheStatus != "" {
		args = append(args, filters.CacheStatus)
		where = append(where, fmt.Sprintf(`
			CASE
				WHEN l.cached_tokens IS NULL THEN 'unknown'
				WHEN l.cached_tokens > 0 THEN 'hit'
				ELSE 'miss'
			END = $%d
		`, len(args)))
	}
	if filters.EmailID != "" {
		args = append(args, filters.EmailID)
		where = append(where, fmt.Sprintf("l.email_id = $%d", len(args)))
	}
	if filters.Model != "" {
		args = append(args, filters.Model)
		where = append(where, fmt.Sprintf("l.model = $%d", len(args)))
	}

	query := `
		SELECT
			l.id,
			l.email_id,
			e.title,
			e.slug,
			e.language,
			e.variant,
			l.user_id,
			l.user_email,
			l.model,
			l.status,
			CASE
				WHEN l.cached_tokens IS NULL THEN 'unknown'
				WHEN l.cached_tokens > 0 THEN 'hit'
				ELSE 'miss'
			END AS cache_status,
			l.latency_ms,
			l.input_tokens,
			l.output_tokens,
			l.total_tokens,
			l.cached_tokens,
			l.error_message,
			l.created_at
		FROM ai_analysis_logs l
		LEFT JOIN emails e ON e.id = l.email_id
	`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY l.created_at DESC LIMIT $%d;", len(args))

	rows, err := dbpool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []AIAnalysisLogItem{}
	for rows.Next() {
		var log AIAnalysisLogItem
		if err := rows.Scan(
			&log.ID,
			&log.EmailID,
			&log.EmailTitle,
			&log.EmailSlug,
			&log.Language,
			&log.Variant,
			&log.UserID,
			&log.UserEmail,
			&log.Model,
			&log.Status,
			&log.CacheStatus,
			&log.LatencyMS,
			&log.InputTokens,
			&log.OutputTokens,
			&log.TotalTokens,
			&log.CachedTokens,
			&log.ErrorMessage,
			&log.CreatedAt,
		); err != nil {
			return nil, err
		}

		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return logs, nil
}
