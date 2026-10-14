package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailreview"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
)

type emailVersionSnapshot struct {
	EmailID               string
	CreatedByUserID       string
	CreatedByEmail        string
	Source                string
	RestoredFromVersionID *string
	Title                 string
	Subject               *string
	Preheader             *string
	OriginalHTML          string
	TemplateHTML          string
	EditableFieldsJSON    []byte
	ChangedFieldCount     int
	ChangedMetadataCount  int
	HTMLChanged           bool
}

func listEmailVersionsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.IsAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		rows, err := dbpool.Query(r.Context(), `
			SELECT
				email_versions.id,
				email_versions.email_id,
				email_versions.version_number,
				email_versions.created_at,
				email_versions.created_by_user_id,
				email_versions.created_by_email,
				email_versions.source,
				email_versions.restored_from_version_id,
				restored.version_number,
				email_versions.changed_field_count,
				email_versions.changed_metadata_count,
				email_versions.html_changed
			FROM email_versions
			JOIN emails ON emails.id = email_versions.email_id
				AND emails.archived_at IS NULL
			LEFT JOIN email_versions restored ON restored.id = email_versions.restored_from_version_id
			WHERE email_versions.email_id = $1
			ORDER BY email_versions.version_number DESC;
		`, id)
		if err != nil {
			http.Error(w, "failed to load email versions", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		versions := []EmailVersionListItem{}
		for rows.Next() {
			var version EmailVersionListItem
			if err := rows.Scan(
				&version.ID,
				&version.EmailID,
				&version.VersionNumber,
				&version.CreatedAt,
				&version.CreatedByUserID,
				&version.CreatedByEmail,
				&version.Source,
				&version.RestoredFromVersionID,
				&version.RestoredFromVersionNumber,
				&version.ChangedFieldCount,
				&version.ChangedMetadataCount,
				&version.HTMLChanged,
			); err != nil {
				http.Error(w, "failed to load email versions", http.StatusInternalServerError)
				return
			}
			versions = append(versions, version)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "failed to load email versions", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(versions)
	}
}

func getEmailVersionHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		versionID := chi.URLParam(r, "versionId")
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.IsAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		version, err := loadEmailVersionDetail(r, dbpool, id, versionID)
		if err != nil {
			http.Error(w, "email version not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(version)
	}
}

func restoreEmailVersionHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		versionID := chi.URLParam(r, "versionId")
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.IsAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to restore email version", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		var slug string
		var currentTitle string
		var currentSubject *string
		var currentPreheader *string
		var currentOriginalHTML string
		var currentTemplateHTML string
		var currentEditableFieldsJSON []byte
		var currentReviewStatus string
		err = tx.QueryRow(r.Context(), `
			SELECT slug, title, subject, preheader, original_html, template_html, editable_fields, review_status
			FROM emails
			WHERE id = $1
				AND archived_at IS NULL
			FOR UPDATE;
		`, id).Scan(&slug, &currentTitle, &currentSubject, &currentPreheader, &currentOriginalHTML, &currentTemplateHTML, &currentEditableFieldsJSON, &currentReviewStatus)
		if err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		var snapshotVersionNumber int
		var title string
		var subject *string
		var preheader *string
		var originalHTML string
		var templateHTML string
		var editableFieldsJSON []byte
		err = tx.QueryRow(r.Context(), `
			SELECT version_number, title, subject, preheader, original_html, template_html, editable_fields
			FROM email_versions
			WHERE id = $1
				AND email_id = $2;
		`, versionID, id).Scan(&snapshotVersionNumber, &title, &subject, &preheader, &originalHTML, &templateHTML, &editableFieldsJSON)
		if err != nil {
			http.Error(w, "email version not found", http.StatusNotFound)
			return
		}
		htmlChanged := originalHTML != currentOriginalHTML || templateHTML != currentTemplateHTML
		if htmlChanged && !auth.IsSuperAdmin(user) {
			http.Error(w, "restoring HTML changes requires super admin", http.StatusForbidden)
			return
		}

		var editableFields emailedit.EditableFields
		if err := json.Unmarshal(editableFieldsJSON, &editableFields); err != nil {
			http.Error(w, "invalid email version", http.StatusInternalServerError)
			return
		}
		renderedHTML, err := emailedit.RenderEditableHTMLWithMetadata(templateHTML, editableFields, emailedit.RenderMetadata{
			Preheader: stringFromPointer(preheader),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid editable fields for email version %s: %v\n", versionID, err)
			http.Error(w, "invalid email version", http.StatusBadRequest)
			return
		}
		reviewHTML, err := emailreview.AddReviewBlocks(renderedHTML)
		if err != nil {
			http.Error(w, "invalid email version HTML", http.StatusBadRequest)
			return
		}
		renderedBodyText := emailtext.HTMLToText(renderedHTML)
		contentParts := emailtext.ExtractContentParts(
			renderedHTML,
			stringFromPointer(subject),
			stringFromPointer(preheader),
			renderedBodyText,
		)
		contentPartsJSON, err := json.Marshal(contentParts)
		if err != nil {
			http.Error(w, "failed to restore email version", http.StatusInternalServerError)
			return
		}

		changes, err := buildEmailUpdateChanges(
			currentTitle,
			currentSubject,
			currentPreheader,
			currentEditableFieldsJSON,
			title,
			subject,
			preheader,
			editableFields,
		)
		if err != nil {
			http.Error(w, "failed to restore email version", http.StatusInternalServerError)
			return
		}
		if htmlChanged {
			changes["original_html"] = map[string]any{
				"before": currentOriginalHTML,
				"after":  originalHTML,
			}
		}
		if len(changes) == 0 {
			if err := insertEmailVersion(r.Context(), tx, emailVersionSnapshot{
				EmailID:               id,
				CreatedByUserID:       user.ID,
				CreatedByEmail:        user.Email,
				Source:                "restore",
				RestoredFromVersionID: &versionID,
				Title:                 title,
				Subject:               subject,
				Preheader:             preheader,
				OriginalHTML:          originalHTML,
				TemplateHTML:          templateHTML,
				EditableFieldsJSON:    editableFieldsJSON,
			}); err != nil {
				http.Error(w, "failed to record email version", http.StatusInternalServerError)
				return
			}
			if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
				ActorUserID: user.ID,
				ActorEmail:  user.Email,
				Action:      emailEventUpdated,
				EmailID:     &id,
				EmailSlug:   &slug,
				EmailTitle:  &title,
				Metadata: map[string]any{
					"restored_from_version_id":     versionID,
					"restored_from_version_number": snapshotVersionNumber,
				},
				Changes: changes,
			}); err != nil {
				http.Error(w, "failed to record email event", http.StatusInternalServerError)
				return
			}
			if err := tx.Commit(r.Context()); err != nil {
				http.Error(w, "failed to restore email version", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		changedReviewBlocks, err := changedReviewBlocksForEmailUpdate(currentTemplateHTML, templateHTML, changes)
		if err != nil {
			http.Error(w, "failed to restore email version", http.StatusInternalServerError)
			return
		}
		commentAnchorTexts, err := changedCommentAnchorTexts(
			templateHTML,
			renderedHTML,
			currentTitle,
			currentSubject,
			currentPreheader,
			currentEditableFieldsJSON,
			title,
			subject,
			preheader,
			editableFields,
		)
		if err != nil {
			http.Error(w, "failed to restore email version", http.StatusInternalServerError)
			return
		}
		nextReviewStatus := currentReviewStatus
		approvalBecameStale := isApprovedEmailReviewStatus(currentReviewStatus)
		if approvalBecameStale {
			nextReviewStatus = "changes_requested"
		}

		result, err := tx.Exec(r.Context(), `
			UPDATE emails
			SET title = $2,
				subject = $3,
				preheader = $4,
				editable_fields = $5::jsonb,
				content_parts = $6::jsonb,
				review_html = $7,
				body_text = $8,
				original_html = $9,
				template_html = $10,
				review_status = $11,
				updated_at = now()
			WHERE id = $1
				AND archived_at IS NULL;
		`, id, title, subject, preheader, editableFieldsJSON, contentPartsJSON, reviewHTML, renderedBodyText, originalHTML, templateHTML, nextReviewStatus)
		if err != nil {
			http.Error(w, "failed to restore email version", http.StatusInternalServerError)
			return
		}
		if result.RowsAffected() == 0 {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}
		if err := insertEmailVersion(r.Context(), tx, emailVersionSnapshot{
			EmailID:               id,
			CreatedByUserID:       user.ID,
			CreatedByEmail:        user.Email,
			Source:                "restore",
			RestoredFromVersionID: &versionID,
			Title:                 title,
			Subject:               subject,
			Preheader:             preheader,
			OriginalHTML:          originalHTML,
			TemplateHTML:          templateHTML,
			EditableFieldsJSON:    editableFieldsJSON,
			ChangedFieldCount:     countEditableFieldChanges(changes),
			ChangedMetadataCount:  countMetadataChanges(changes),
			HTMLChanged:           htmlChanged,
		}); err != nil {
			http.Error(w, "failed to record email version", http.StatusInternalServerError)
			return
		}
		if err := resetCommentAnchorsForReviewBlocks(r.Context(), tx, id, commentAnchorTexts); err != nil {
			http.Error(w, "failed to restore email version", http.StatusInternalServerError)
			return
		}
		metadata := map[string]any{
			"restored_from_version_id":     versionID,
			"restored_from_version_number": snapshotVersionNumber,
		}
		if len(changedReviewBlocks) > 0 {
			metadata["changed_review_blocks"] = changedReviewBlocks
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventUpdated,
			EmailID:     &id,
			EmailSlug:   &slug,
			EmailTitle:  &title,
			Metadata:    metadata,
			Changes:     changes,
		}); err != nil {
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}
		if len(changes) > 0 || htmlChanged {
			if err := markStaleAreaApprovalsAfterContentChange(
				r.Context(),
				tx,
				id,
				user,
				"approval_stale_after_restore",
			); err != nil {
				http.Error(w, "failed to update area approvals", http.StatusInternalServerError)
				return
			}
		}
		if approvalBecameStale {
			if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
				ActorUserID: user.ID,
				ActorEmail:  user.Email,
				Action:      emailEventReviewStatusUpdated,
				EmailID:     &id,
				EmailSlug:   &slug,
				EmailTitle:  &title,
				Metadata: map[string]any{
					"reason": "approval_stale_after_restore",
				},
				Changes: map[string]any{
					"review_status": map[string]any{
						"before": currentReviewStatus,
						"after":  nextReviewStatus,
					},
				},
			}); err != nil {
				http.Error(w, "failed to record email event", http.StatusInternalServerError)
				return
			}
		}

		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to restore email version", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func loadEmailVersionDetail(r *http.Request, dbpool *pgxpool.Pool, emailID string, versionID string) (EmailVersionDetail, error) {
	var version EmailVersionDetail
	var editableFieldsJSON []byte
	err := dbpool.QueryRow(r.Context(), `
		SELECT
			email_versions.id,
			email_versions.email_id,
			email_versions.version_number,
			email_versions.created_at,
			email_versions.created_by_user_id,
			email_versions.created_by_email,
			email_versions.source,
			email_versions.restored_from_version_id,
			restored.version_number,
			email_versions.changed_field_count,
			email_versions.changed_metadata_count,
			email_versions.html_changed,
			email_versions.title,
			email_versions.subject,
			email_versions.preheader,
			email_versions.original_html,
			email_versions.template_html,
			email_versions.editable_fields
		FROM email_versions
		JOIN emails ON emails.id = email_versions.email_id
			AND emails.archived_at IS NULL
		LEFT JOIN email_versions restored ON restored.id = email_versions.restored_from_version_id
		WHERE email_versions.email_id = $1
			AND email_versions.id = $2;
	`, emailID, versionID).Scan(
		&version.ID,
		&version.EmailID,
		&version.VersionNumber,
		&version.CreatedAt,
		&version.CreatedByUserID,
		&version.CreatedByEmail,
		&version.Source,
		&version.RestoredFromVersionID,
		&version.RestoredFromVersionNumber,
		&version.ChangedFieldCount,
		&version.ChangedMetadataCount,
		&version.HTMLChanged,
		&version.Title,
		&version.Subject,
		&version.Preheader,
		&version.OriginalHTML,
		&version.TemplateHTML,
		&editableFieldsJSON,
	)
	if err != nil {
		return EmailVersionDetail{}, err
	}

	var editableFields emailedit.EditableFields
	if err := json.Unmarshal(editableFieldsJSON, &editableFields); err != nil {
		return EmailVersionDetail{}, err
	}
	renderedHTML, err := emailedit.RenderEditableHTMLWithMetadata(version.TemplateHTML, editableFields, emailedit.RenderMetadata{
		Preheader: stringFromPointer(version.Preheader),
	})
	if err != nil {
		return EmailVersionDetail{}, err
	}
	reviewHTML, err := emailreview.AddReviewBlocks(renderedHTML)
	if err != nil {
		return EmailVersionDetail{}, err
	}

	version.EditableFields = json.RawMessage(editableFieldsJSON)
	version.ReviewHTML = reviewHTML
	return version, nil
}

func insertEmailVersion(ctx context.Context, db emailEventExecutor, snapshot emailVersionSnapshot) error {
	_, err := db.Exec(ctx, `
		INSERT INTO email_versions (
			email_id,
			version_number,
			created_by_user_id,
			created_by_email,
			source,
			restored_from_version_id,
			title,
			subject,
			preheader,
			original_html,
			template_html,
			editable_fields,
			changed_field_count,
			changed_metadata_count,
			html_changed
		)
		SELECT
			$1,
			coalesce(max(version_number), 0) + 1,
			nullif($2, '')::uuid,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10,
			$11::jsonb,
			$12,
			$13,
			$14
		FROM email_versions
		WHERE email_id = $1;
	`,
		snapshot.EmailID,
		snapshot.CreatedByUserID,
		snapshot.CreatedByEmail,
		snapshot.Source,
		snapshot.RestoredFromVersionID,
		snapshot.Title,
		snapshot.Subject,
		snapshot.Preheader,
		snapshot.OriginalHTML,
		snapshot.TemplateHTML,
		snapshot.EditableFieldsJSON,
		snapshot.ChangedFieldCount,
		snapshot.ChangedMetadataCount,
		snapshot.HTMLChanged,
	)

	return err
}

func countEditableFieldChanges(changes map[string]any) int {
	fieldChanges, ok := changes["editable_fields"].(map[string]any)
	if !ok {
		return 0
	}

	return len(fieldChanges)
}

func countMetadataChanges(changes map[string]any) int {
	count := 0
	for _, key := range []string{"title", "subject", "preheader"} {
		if _, ok := changes[key]; ok {
			count++
		}
	}

	return count
}
