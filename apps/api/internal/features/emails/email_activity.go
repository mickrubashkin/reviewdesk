package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultEmailActivityLimit = 100
const maxEmailActivityLimit = 500

func listEmailActivityHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		emailID := chi.URLParam(r, "id")
		limit := parseEmailActivityLimit(r.URL.Query().Get("limit"))

		exists, err := emailExists(r.Context(), dbpool, emailID)
		if err != nil {
			http.Error(w, "failed to load email activity", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		items, err := listEmailActivity(r.Context(), dbpool, emailID, limit)
		if err != nil {
			http.Error(w, "failed to load email activity", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	}
}

func parseEmailActivityLimit(value string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || limit <= 0 {
		return defaultEmailActivityLimit
	}
	if limit > maxEmailActivityLimit {
		return maxEmailActivityLimit
	}
	return limit
}

func emailExists(ctx context.Context, dbpool *pgxpool.Pool, emailID string) (bool, error) {
	var exists bool
	err := dbpool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM emails WHERE id = $1
		);
	`, emailID).Scan(&exists)
	return exists, err
}

func listEmailActivity(ctx context.Context, dbpool *pgxpool.Pool, emailID string, limit int) ([]EmailActivityItem, error) {
	items := []EmailActivityItem{}

	appendItems := func(next []EmailActivityItem, err error) error {
		if err != nil {
			return err
		}
		items = append(items, next...)
		return nil
	}

	if err := appendItems(listCommentCreatedActivity(ctx, dbpool, emailID)); err != nil {
		return nil, err
	}
	if err := appendItems(listCommentReplyActivity(ctx, dbpool, emailID)); err != nil {
		return nil, err
	}
	if err := appendItems(listCommentResolvedActivity(ctx, dbpool, emailID)); err != nil {
		return nil, err
	}
	if err := appendItems(listEmailEventActivity(ctx, dbpool, emailID)); err != nil {
		return nil, err
	}
	if err := appendItems(listAIAnalysisActivity(ctx, dbpool, emailID)); err != nil {
		return nil, err
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	if len(items) > limit {
		items = items[:limit]
	}

	return items, nil
}

func listCommentCreatedActivity(ctx context.Context, dbpool *pgxpool.Pool, emailID string) ([]EmailActivityItem, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT
			comments.id,
			authors.email,
			comments.created_at,
			comments.review_block,
			comments.selected_text,
			comments.severity
		FROM comments
		LEFT JOIN users AS authors ON authors.id = comments.user_id
		WHERE comments.email_id = $1;
	`, emailID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []EmailActivityItem{}
	for rows.Next() {
		var id string
		var actorEmail *string
		var createdAt time.Time
		var reviewBlock string
		var selectedText string
		var severity string
		if err := rows.Scan(&id, &actorEmail, &createdAt, &reviewBlock, &selectedText, &severity); err != nil {
			return nil, err
		}
		metadata := map[string]any{
			"comment_id":    id,
			"review_block":  reviewBlock,
			"selected_text": selectedText,
			"severity":      severity,
		}
		items = append(items, emailActivityItem("comment:"+id, emailEventCommentCreated, actorEmail, createdAt, fmt.Sprintf("Added a %s comment", severity), metadata, nil))
	}

	return items, rows.Err()
}

func listCommentReplyActivity(ctx context.Context, dbpool *pgxpool.Pool, emailID string) ([]EmailActivityItem, error) {
	rows, err := dbpool.Query(ctx, `
		WITH ranked_messages AS (
			SELECT
				comment_messages.id,
				comment_messages.comment_id,
				comment_messages.user_id,
				comment_messages.body,
				comment_messages.created_at,
				row_number() OVER (
					PARTITION BY comment_messages.comment_id
					ORDER BY comment_messages.created_at, comment_messages.id
				) AS message_index
			FROM comment_messages
			INNER JOIN comments ON comments.id = comment_messages.comment_id
			WHERE comments.email_id = $1
		)
		SELECT
			ranked_messages.id,
			ranked_messages.comment_id,
			authors.email,
			ranked_messages.created_at,
			comments.review_block,
			ranked_messages.body
		FROM ranked_messages
		INNER JOIN comments ON comments.id = ranked_messages.comment_id
		LEFT JOIN users AS authors ON authors.id = ranked_messages.user_id
		WHERE ranked_messages.message_index > 1;
	`, emailID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []EmailActivityItem{}
	for rows.Next() {
		var id string
		var commentID string
		var actorEmail *string
		var createdAt time.Time
		var reviewBlock string
		var body string
		if err := rows.Scan(&id, &commentID, &actorEmail, &createdAt, &reviewBlock, &body); err != nil {
			return nil, err
		}
		metadata := map[string]any{
			"comment_id":   commentID,
			"message_id":   id,
			"review_block": reviewBlock,
			"body":         body,
		}
		items = append(items, emailActivityItem("comment-message:"+id, emailEventCommentReplied, actorEmail, createdAt, "Replied to a comment", metadata, nil))
	}

	return items, rows.Err()
}

func listCommentResolvedActivity(ctx context.Context, dbpool *pgxpool.Pool, emailID string) ([]EmailActivityItem, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT
			comments.id,
			resolvers.email,
			comments.resolved_at,
			comments.review_block,
			comments.severity
		FROM comments
		LEFT JOIN users AS resolvers ON resolvers.id = comments.resolved_by
		WHERE comments.email_id = $1
			AND comments.resolved_at IS NOT NULL;
	`, emailID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []EmailActivityItem{}
	for rows.Next() {
		var id string
		var actorEmail *string
		var resolvedAt time.Time
		var reviewBlock string
		var severity string
		if err := rows.Scan(&id, &actorEmail, &resolvedAt, &reviewBlock, &severity); err != nil {
			return nil, err
		}
		metadata := map[string]any{
			"comment_id":   id,
			"review_block": reviewBlock,
			"severity":     severity,
		}
		items = append(items, emailActivityItem("comment-resolved:"+id, emailEventCommentResolved, actorEmail, resolvedAt, "Resolved a comment", metadata, nil))
	}

	return items, rows.Err()
}

func listEmailEventActivity(ctx context.Context, dbpool *pgxpool.Pool, emailID string) ([]EmailActivityItem, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT
			id,
			action,
			actor_email,
			created_at,
			metadata,
			changes
		FROM email_events
		WHERE email_id = $1
			AND action = ANY($2)
	`, emailID, []string{
		emailEventCreated,
		emailEventUpdated,
		emailEventPlanningUpdated,
		emailEventReviewStatusUpdated,
		emailEventDuplicated,
		emailEventAdaptationCreated,
		emailEventArchived,
		emailEventLiveMarked,
		emailEventLiveUnmarked,
		emailEventTranslationConfirmed,
		emailEventPlaced,
		emailEventTranslationLinked,
	})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []EmailActivityItem{}
	for rows.Next() {
		var id string
		var action string
		var actorEmail string
		var createdAt time.Time
		var metadata []byte
		var changes []byte
		if err := rows.Scan(&id, &action, &actorEmail, &createdAt, &metadata, &changes); err != nil {
			return nil, err
		}
		items = append(items, emailActivityItem("email-event:"+id, action, &actorEmail, createdAt, emailEventActivitySummary(action, changes, metadata), json.RawMessage(metadata), json.RawMessage(changes)))
	}

	return items, rows.Err()
}

func listAIAnalysisActivity(ctx context.Context, dbpool *pgxpool.Pool, emailID string) ([]EmailActivityItem, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT
			id,
			user_email,
			created_at,
			model,
			status,
			CASE
				WHEN cached_tokens IS NULL THEN 'unknown'
				WHEN cached_tokens > 0 THEN 'hit'
				ELSE 'miss'
			END AS cache_status,
			latency_ms,
			force_refresh,
			error_message
		FROM ai_analysis_logs
		WHERE email_id = $1;
	`, emailID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []EmailActivityItem{}
	for rows.Next() {
		var id string
		var actorEmail *string
		var createdAt time.Time
		var model string
		var status string
		var cacheStatus string
		var latencyMS int
		var forceRefresh bool
		var errorMessage *string
		if err := rows.Scan(&id, &actorEmail, &createdAt, &model, &status, &cacheStatus, &latencyMS, &forceRefresh, &errorMessage); err != nil {
			return nil, err
		}
		metadata := map[string]any{
			"analysis_log_id": id,
			"model":           model,
			"status":          status,
			"cache_status":    cacheStatus,
			"latency_ms":      latencyMS,
			"force_refresh":   forceRefresh,
		}
		if errorMessage != nil {
			metadata["error_message"] = *errorMessage
		}
		items = append(items, emailActivityItem("ai-analysis:"+id, "ai_analysis_run", actorEmail, createdAt, aiAnalysisActivitySummary(status, forceRefresh), metadata, nil))
	}

	return items, rows.Err()
}

func emailActivityItem(id string, itemType string, actorEmail *string, createdAt time.Time, summary string, metadata any, changes any) EmailActivityItem {
	metadataJSON, _ := json.Marshal(emptyMapIfNil(metadata))
	changesJSON, _ := json.Marshal(emptyMapIfNil(changes))
	return EmailActivityItem{
		ID:         id,
		Type:       itemType,
		ActorEmail: actorEmail,
		CreatedAt:  createdAt,
		Summary:    summary,
		Metadata:   json.RawMessage(metadataJSON),
		Changes:    json.RawMessage(changesJSON),
	}
}

func emailEventActivitySummary(action string, changes []byte, metadata []byte) string {
	switch action {
	case emailEventCreated:
		return "Created email"
	case emailEventUpdated:
		return "Updated email content"
	case emailEventPlanningUpdated:
		return "Updated planning fields"
	case emailEventReviewStatusUpdated:
		reason := metadataString(metadata, "reason")
		if reason == "approval_stale_after_edit" {
			return "Marked approval stale after edit"
		}
		if reason == "reapproved_after_stale_edit" {
			return "Re-approved after stale edit"
		}
		if reason == "reapproved" {
			return "Re-approved email"
		}
		nextStatus := changedFieldAfter(changes, "review_status")
		if nextStatus != "" {
			return "Changed review status to " + nextStatus
		}
		return "Changed review status"
	case emailEventAreaApprovalUpdated:
		areaName := metadataString(metadata, "area_name")
		if areaName == "" {
			areaName = metadataString(metadata, "area")
		}
		reason := metadataString(metadata, "reason")
		if reason == "approval_stale_after_edit" {
			return "Marked " + areaName + " approval stale after edit"
		}
		if reason == "approval_stale_after_restore" {
			return "Marked " + areaName + " approval stale after restore"
		}
		if reason == "reapproved_after_stale_edit" {
			return "Re-approved " + areaName + " after stale edit"
		}
		nextStatus := changedFieldAfter(changes, "area_approval_status")
		if nextStatus != "" {
			return "Changed " + areaName + " approval to " + nextStatus
		}
		return "Changed " + areaName + " approval"
	case emailEventDuplicated:
		return "Duplicated email"
	case emailEventAdaptationCreated:
		label := metadataString(metadata, "adaptation_label")
		if label != "" {
			return "Created adaptation " + label
		}
		return "Created adaptation"
	case emailEventArchived:
		return "Archived email"
	case emailEventLiveMarked:
		if note := metadataString(metadata, "note"); note != "" {
			return "Marked live by hand: " + note
		}
		return "Marked live by hand"
	case emailEventLiveUnmarked:
		return "Removed the live mark"
	case emailEventTranslationConfirmed:
		return "Marked the translation up to date with EN"
	case emailEventPlaced:
		return "Moved to a slot of the sequence"
	case emailEventTranslationLinked:
		return "Changed which EN email this translates"
	default:
		return "Changed email"
	}
}

func aiAnalysisActivitySummary(status string, forceRefresh bool) string {
	if forceRefresh {
		return "Re-ran AI analysis with refresh"
	}
	if status == "error" {
		return "AI analysis failed"
	}
	return "Ran AI analysis"
}

func changedFieldAfter(changes []byte, key string) string {
	var decoded map[string]map[string]any
	if err := json.Unmarshal(changes, &decoded); err != nil {
		return ""
	}
	value, ok := decoded[key]
	if !ok {
		return ""
	}
	after, ok := value["after"].(string)
	if !ok {
		return ""
	}
	return after
}

func metadataString(metadata []byte, key string) string {
	var decoded map[string]any
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		return ""
	}
	value, ok := decoded[key].(string)
	if !ok {
		return ""
	}
	return value
}

func loadEmailEventTarget(ctx context.Context, db emailEventExecutor, emailID string) (slug string, title string, err error) {
	err = db.QueryRow(ctx, `
		SELECT slug, title
		FROM emails
		WHERE id = $1;
	`, emailID).Scan(&slug, &title)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	return slug, title, err
}
