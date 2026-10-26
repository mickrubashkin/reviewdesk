package boards

import (
	"time"
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

const (
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

func stringFromPointer(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeAdaptationKey(label string) string {
	key := strings.ToLower(strings.TrimSpace(label))
	key = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == ' ' || r == '_' || r == '-':
			return '-'
		default:
			return -1
		}
	}, key)
	return strings.Trim(strings.Join(strings.FieldsFunc(key, func(r rune) bool { return r == '-' }), "-"), "-")
}

type AuthUser = auth.AuthUser

type EventLogger interface {
	LogEvent(ctx context.Context, db EmailEventExecutor, actor AuthUser, action string, board BoardItem, metadata map[string]any, changes map[string]any) error
}

type EmailEventExecutor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

var Logger EventLogger

// CopyEmails copies the source board's EN emails into a new board inside
// the board's creation transaction; set by the server (the emails package
// imports this one).
var CopyEmails func(ctx context.Context, tx pgx.Tx, actor AuthUser, sourceBoardKey, targetBoardKey string) (int, error)


type BoardItem struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	Stages    []string  `json:"stages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// PortalSyncedAt is the end of the last successful portal sync, so the
	// board can tell "not sent in the period" apart from "never checked".
	PortalSyncedAt *time.Time `json:"portal_synced_at"`
	// CopiedEmailCount is set only in the response to creating a board.
	CopiedEmailCount *int `json:"copied_email_count,omitempty"`
}

type BoardApprovalAreaItem struct {
	ID         string     `json:"id"`
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	Required   bool       `json:"required"`
	SortOrder  int        `json:"sort_order"`
	ArchivedAt *time.Time `json:"archived_at"`
}





