package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	emailEventDuplicated             = "email_duplicated"
	emailEventAdaptationCreated      = "email_adaptation_created"
	emailEventCreated                = "email_created"
	emailEventLiveMarked             = "email_live_marked"
	emailEventLiveUnmarked           = "email_live_unmarked"
	emailEventArchived               = "email_archived"
	emailEventUpdated                = "email_updated"
	emailEventPlanningUpdated        = "email_planning_updated"
	emailEventReviewStatusUpdated    = "email_review_status_updated"
	emailEventAreaApprovalUpdated    = "email_area_approval_updated"
	emailEventCommentCreated         = "comment_created"
	emailEventCommentReplied         = "comment_replied"
	emailEventCommentResolved        = "comment_resolved"
	boardEventCreated                = "board_created"
	boardEventStageCreated           = "board_stage_created"
	boardEventStageRenamed           = "board_stage_renamed"
	boardEventStageDeleted           = "board_stage_deleted"
	boardEventStagesReordered        = "board_stages_reordered"
	boardEventApprovalAreaCreated    = "board_approval_area_created"
	boardEventApprovalAreaUpdated    = "board_approval_area_updated"
	boardEventApprovalAreaDeleted    = "board_approval_area_deleted"
	boardEventApprovalAreasReordered = "board_approval_areas_reordered"
)

type emailEventFilters struct {
	ActorEmail string
	Action     string
	Email      string
	Limit      int
}

type EmailEventParam struct {
	ActorUserID string
	ActorEmail  string
	Action      string
	EmailID     *string
	EmailSlug   *string
	EmailTitle  *string
	Metadata    any
	Changes     any
}

type emailEventExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func InsertEmailEvent(ctx context.Context, db emailEventExecutor, event EmailEventParam) error {
	metadataJSON, err := json.Marshal(emptyMapIfNil(event.Metadata))
	if err != nil {
		return err
	}
	changesJSON, err := json.Marshal(emptyMapIfNil(event.Changes))
	if err != nil {
		return err
	}

	_, err = db.Exec(ctx, `
		INSERT INTO email_events (
			actor_user_id,
			actor_email,
			action,
			email_id,
			email_slug,
			email_title,
			metadata,
			changes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb);
	`,
		event.ActorUserID,
		event.ActorEmail,
		event.Action,
		event.EmailID,
		event.EmailSlug,
		event.EmailTitle,
		metadataJSON,
		changesJSON,
	)

	return err
}

func listEmailEvents(ctx context.Context, dbpool *pgxpool.Pool, filters emailEventFilters) ([]EmailEventItem, error) {
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	where := []string{}
	args := []any{}

	if filters.ActorEmail != "" {
		args = append(args, "%"+strings.ToLower(filters.ActorEmail)+"%")
		where = append(where, fmt.Sprintf("lower(actor_email) LIKE $%d", len(args)))
	}
	if filters.Action != "" {
		args = append(args, filters.Action)
		where = append(where, fmt.Sprintf("action = $%d", len(args)))
	}
	if filters.Email != "" {
		args = append(args, "%"+strings.ToLower(filters.Email)+"%")
		where = append(where, fmt.Sprintf(`(
			lower(coalesce(email_title, '')) LIKE $%d OR
			lower(coalesce(email_slug, '')) LIKE $%d OR
			lower(coalesce(metadata->>'board_name', '')) LIKE $%d OR
			lower(coalesce(metadata->>'board_key', '')) LIKE $%d
		)`, len(args), len(args), len(args), len(args)))
	}

	query := `
		SELECT
			id,
			actor_user_id,
			actor_email,
			action,
			email_id,
			email_slug,
			email_title,
			metadata,
			changes,
			created_at
		FROM email_events
	`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d;", len(args))

	rows, err := dbpool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []EmailEventItem{}
	for rows.Next() {
		var event EmailEventItem
		var metadata []byte
		var changes []byte
		if err := rows.Scan(
			&event.ID,
			&event.ActorUserID,
			&event.ActorEmail,
			&event.Action,
			&event.EmailID,
			&event.EmailSlug,
			&event.EmailTitle,
			&metadata,
			&changes,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		event.Metadata = json.RawMessage(metadata)
		event.Changes = json.RawMessage(changes)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func emptyMapIfNil(value any) any {
	if value == nil {
		return map[string]any{}
	}

	return value
}
