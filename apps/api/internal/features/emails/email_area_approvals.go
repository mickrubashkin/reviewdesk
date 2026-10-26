package emails

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/boards"
)

type emailAreaApprovalItem struct {
	BoardApprovalAreaID string     `json:"board_approval_area_id"`
	Area                string     `json:"area"`
	Name                string     `json:"name"`
	Required            bool       `json:"required"`
	Status              string     `json:"status"`
	DecisionNote        *string    `json:"decision_note"`
	DecidedByEmail      *string    `json:"decided_by_email"`
	ContentSnapshotHash *string    `json:"content_snapshot_hash"`
	DecidedAt           *time.Time `json:"decided_at"`
}

type updateEmailAreaApprovalRequest struct {
	Status       string  `json:"status"`
	DecisionNote *string `json:"decision_note"`
}

type staleEmailAreaApproval struct {
	ID                  string
	Area                string
	Name                string
	ContentSnapshotHash *string
}

func listEmailAreaApprovalsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var exists bool
		if err := dbpool.QueryRow(r.Context(), `
			SELECT EXISTS (
				SELECT 1
				FROM emails
				WHERE id = $1
					AND archived_at IS NULL
			);
		`, id).Scan(&exists); err != nil {
			http.Error(w, "failed to load area approvals", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		rows, err := dbpool.Query(r.Context(), `
			SELECT
				board_approval_areas.id,
				approval_areas.key,
				board_approval_areas.name,
				board_approval_areas.required,
				coalesce(email_area_approvals.status, 'pending'),
				email_area_approvals.decision_note,
				email_area_approvals.decided_by_email,
				email_area_approvals.content_snapshot_hash,
				email_area_approvals.decided_at
			FROM emails
			JOIN boards ON boards.key = coalesce(nullif(trim(emails.sequence), ''), 'onboarding')
			JOIN board_approval_areas ON board_approval_areas.board_id = boards.id
				AND board_approval_areas.archived_at IS NULL
			JOIN approval_areas ON approval_areas.id = board_approval_areas.approval_area_id
			LEFT JOIN email_area_approvals ON email_area_approvals.email_id = emails.id
				AND email_area_approvals.board_approval_area_id = board_approval_areas.id
			WHERE emails.id = $1
				AND emails.archived_at IS NULL
			ORDER BY board_approval_areas.sort_order, board_approval_areas.created_at;
		`, id)
		if err != nil {
			http.Error(w, "failed to load area approvals", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := []emailAreaApprovalItem{}
		for rows.Next() {
			var item emailAreaApprovalItem
			if err := rows.Scan(
				&item.BoardApprovalAreaID,
				&item.Area,
				&item.Name,
				&item.Required,
				&item.Status,
				&item.DecisionNote,
				&item.DecidedByEmail,
				&item.ContentSnapshotHash,
				&item.DecidedAt,
			); err != nil {
				http.Error(w, "failed to load area approvals", http.StatusInternalServerError)
				return
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "failed to load area approvals", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	}
}

func updateEmailAreaApprovalHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		area := strings.TrimSpace(chi.URLParam(r, "area"))
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !requireManageEmail(w, r, dbpool, user, id) {
			return
		}
		areaKey := boards.NormalizeApprovalAreaKey(area)
		if areaKey == "" {
			http.Error(w, "invalid approval area", http.StatusBadRequest)
			return
		}

		var request updateEmailAreaApprovalRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		nextStatus := strings.TrimSpace(request.Status)
		if !isValidEmailAreaApprovalUpdateStatus(nextStatus) {
			http.Error(w, "invalid approval status", http.StatusBadRequest)
			return
		}
		decisionNote := trimmedOptionalString(request.DecisionNote)

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to update area approval", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		var slug string
		var title string
		var currentSubject *string
		var currentPreheader *string
		var currentTemplateHash *string
		var currentTemplateHTML string
		var currentEditableFieldsText string
		var boardApprovalAreaID string
		var areaName string
		var areaRequired bool
		err = tx.QueryRow(r.Context(), `
			SELECT
				emails.slug,
				emails.title,
				emails.subject,
				emails.preheader,
				emails.template_hash,
				emails.template_html,
				emails.editable_fields::text,
				board_approval_areas.id,
				board_approval_areas.name,
				board_approval_areas.required
			FROM emails
			JOIN boards ON boards.key = coalesce(nullif(trim(emails.sequence), ''), 'onboarding')
			JOIN board_approval_areas ON board_approval_areas.board_id = boards.id
				AND board_approval_areas.archived_at IS NULL
			JOIN approval_areas ON approval_areas.id = board_approval_areas.approval_area_id
				AND approval_areas.key = $2
			WHERE emails.id = $1
				AND emails.archived_at IS NULL
			FOR UPDATE OF emails, board_approval_areas;
		`, id, area).Scan(
			&slug,
			&title,
			&currentSubject,
			&currentPreheader,
			&currentTemplateHash,
			&currentTemplateHTML,
			&currentEditableFieldsText,
			&boardApprovalAreaID,
			&areaName,
			&areaRequired,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				http.Error(w, "email not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to update area approval", http.StatusInternalServerError)
			return
		}

		var previousStatus sql.NullString
		var previousNote sql.NullString
		err = tx.QueryRow(r.Context(), `
			SELECT status, decision_note
			FROM email_area_approvals
			WHERE email_id = $1
				AND board_approval_area_id = $2
			FOR UPDATE;
		`, id, boardApprovalAreaID).Scan(&previousStatus, &previousNote)
		if err != nil && err != pgx.ErrNoRows {
			http.Error(w, "failed to update area approval", http.StatusInternalServerError)
			return
		}

		var contentSnapshotHash *string
		if nextStatus == "approved" {
			snapshot := approvalContentSnapshot(
				title,
				currentSubject,
				currentPreheader,
				currentTemplateHash,
				currentTemplateHTML,
				currentEditableFieldsText,
			)
			if hash, ok := snapshot["approved_content_hash"].(string); ok {
				contentSnapshotHash = &hash
			}
		}

		var item emailAreaApprovalItem
		err = tx.QueryRow(r.Context(), `
			INSERT INTO email_area_approvals (
				email_id,
				board_approval_area_id,
				status,
				decided_by_user_id,
				decided_by_email,
				decision_note,
				content_snapshot_hash,
				decided_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now())
			ON CONFLICT (email_id, board_approval_area_id) DO UPDATE SET
				status = EXCLUDED.status,
				decided_by_user_id = EXCLUDED.decided_by_user_id,
				decided_by_email = EXCLUDED.decided_by_email,
				decision_note = EXCLUDED.decision_note,
				content_snapshot_hash = EXCLUDED.content_snapshot_hash,
				decided_at = EXCLUDED.decided_at,
				updated_at = now()
			RETURNING status, decision_note, decided_by_email, content_snapshot_hash, decided_at;
		`, id, boardApprovalAreaID, nextStatus, user.ID, user.Email, decisionNote, contentSnapshotHash).Scan(
			&item.Status,
			&item.DecisionNote,
			&item.DecidedByEmail,
			&item.ContentSnapshotHash,
			&item.DecidedAt,
		)
		if err != nil {
			http.Error(w, "failed to update area approval", http.StatusInternalServerError)
			return
		}
		item.BoardApprovalAreaID = boardApprovalAreaID
		item.Area = area
		item.Name = areaName
		item.Required = areaRequired

		previousStatusValue := "pending"
		if previousStatus.Valid {
			previousStatusValue = previousStatus.String
		}
		var previousNoteValue *string
		if previousNote.Valid {
			previousNoteValue = &previousNote.String
		}
		eventMetadata := map[string]any{
			"area":                  area,
			"area_name":             areaName,
			"status":                nextStatus,
			"decision_note":         decisionNote,
			"content_snapshot_hash": contentSnapshotHash,
		}
		if previousStatusValue == "stale" && nextStatus == "approved" {
			eventMetadata["reason"] = "reapproved_after_stale_edit"
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventAreaApprovalUpdated,
			EmailID:     &id,
			EmailSlug:   &slug,
			EmailTitle:  &title,
			Metadata:    eventMetadata,
			Changes: map[string]any{
				"area_approval_status": map[string]any{
					"before": previousStatusValue,
					"after":  nextStatus,
				},
				"decision_note": map[string]any{
					"before": previousNoteValue,
					"after":  decisionNote,
				},
			},
		}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to insert area approval event for %s: %v\n", id, err)
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update area approval", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(item)
	}
}

func isValidEmailAreaApprovalUpdateStatus(status string) bool {
	switch status {
	case "approved", "changes_requested":
		return true
	default:
		return false
	}
}

func markStaleAreaApprovalsAfterContentChange(
	ctx context.Context,
	db emailEventExecutor,
	emailID string,
	actor AuthUser,
	reason string,
) error {
	var slug string
	var title string
	var subject *string
	var preheader *string
	var templateHash *string
	var templateHTML string
	var editableFieldsText string
	err := db.QueryRow(ctx, `
		SELECT slug, title, subject, preheader, template_hash, template_html, editable_fields::text
		FROM emails
		WHERE id = $1
			AND archived_at IS NULL;
	`, emailID).Scan(
		&slug,
		&title,
		&subject,
		&preheader,
		&templateHash,
		&templateHTML,
		&editableFieldsText,
	)
	if err != nil {
		return err
	}

	snapshot := approvalContentSnapshot(
		title,
		subject,
		preheader,
		templateHash,
		templateHTML,
		editableFieldsText,
	)
	currentContentHash, ok := snapshot["approved_content_hash"].(string)
	if !ok || currentContentHash == "" {
		return fmt.Errorf("failed to calculate approval content hash")
	}

	rows, err := db.Query(ctx, `
		SELECT
			email_area_approvals.id,
			approval_areas.key,
			board_approval_areas.name,
			email_area_approvals.content_snapshot_hash
		FROM email_area_approvals
		JOIN board_approval_areas ON board_approval_areas.id = email_area_approvals.board_approval_area_id
		JOIN approval_areas ON approval_areas.id = board_approval_areas.approval_area_id
		WHERE email_area_approvals.email_id = $1
			AND email_area_approvals.status = 'approved'
			AND email_area_approvals.content_snapshot_hash IS DISTINCT FROM $2
		ORDER BY board_approval_areas.sort_order, board_approval_areas.created_at
		FOR UPDATE OF email_area_approvals;
	`, emailID, currentContentHash)
	if err != nil {
		return err
	}
	defer rows.Close()

	staleApprovals := []staleEmailAreaApproval{}
	for rows.Next() {
		var approval staleEmailAreaApproval
		if err := rows.Scan(
			&approval.ID,
			&approval.Area,
			&approval.Name,
			&approval.ContentSnapshotHash,
		); err != nil {
			return err
		}
		staleApprovals = append(staleApprovals, approval)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, approval := range staleApprovals {
		if _, err := db.Exec(ctx, `
			UPDATE email_area_approvals
			SET status = 'stale',
				updated_at = now()
			WHERE id = $1;
		`, approval.ID); err != nil {
			return err
		}

		if err := InsertEmailEvent(ctx, db, EmailEventParam{
			ActorUserID: actor.ID,
			ActorEmail:  actor.Email,
			Action:      emailEventAreaApprovalUpdated,
			EmailID:     &emailID,
			EmailSlug:   &slug,
			EmailTitle:  &title,
			Metadata: map[string]any{
				"area":                          approval.Area,
				"area_name":                     approval.Name,
				"status":                        "stale",
				"reason":                        reason,
				"content_snapshot_hash":         approval.ContentSnapshotHash,
				"current_content_snapshot_hash": currentContentHash,
			},
			Changes: map[string]any{
				"area_approval_status": map[string]any{
					"before": "approved",
					"after":  "stale",
				},
			},
		}); err != nil {
			return err
		}
	}

	return nil
}
