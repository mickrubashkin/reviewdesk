package emails

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	featureauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/features/auth"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailreview"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/boards"
	xhtml "golang.org/x/net/html"
)

func RegisterEmailRoutes(r chi.Router, dbpool *pgxpool.Pool) {
	r.Get("/api/emails", listEmailsHandler(dbpool))
	r.Post("/api/emails", createEmailHandler(dbpool))
	r.Post("/api/emails/inspect-html", inspectEmailHTMLHandler(dbpool))
	r.Post("/api/emails/export", exportEmailsHandler(dbpool))
	r.Post("/api/slots", createSlotHandler(dbpool))
	r.Get("/api/emails/{id}", getEmailHandler(dbpool))
	r.Get("/api/emails/{id}/activity", listEmailActivityHandler(dbpool))
	r.Get("/api/emails/{id}/area-approvals", listEmailAreaApprovalsHandler(dbpool))
	r.Get("/api/emails/{id}/rendered", getRenderedEmailHandler(dbpool))
	r.Get("/api/emails/{id}/versions", listEmailVersionsHandler(dbpool))
	r.Get("/api/emails/{id}/versions/{versionId}", getEmailVersionHandler(dbpool))
	r.Post("/api/emails/{id}/versions/{versionId}/restore", restoreEmailVersionHandler(dbpool))
	r.Patch("/api/emails/{id}/editable-fields", updateEmailEditableFieldsHandler(dbpool))
	r.Patch("/api/emails/{id}/planning-fields", updateEmailPlanningFieldsHandler(dbpool))
	r.Patch("/api/emails/{id}/review-status", updateEmailReviewStatusHandler(dbpool))
	r.Patch("/api/emails/{id}/area-approvals/{area}", updateEmailAreaApprovalHandler(dbpool))
	r.Post("/api/emails/{id}/duplicate-as", duplicateEmailAsHandler(dbpool))
	r.Post("/api/emails/{id}/duplicate", duplicateEmailHandler(dbpool))
	r.Post("/api/emails/{id}/adaptations", createEmailAdaptationHandler(dbpool))
	r.Patch("/api/emails/{id}/archive", archiveEmailHandler(dbpool))
	r.Patch("/api/emails/{id}/live", markEmailLiveHandler(dbpool))
	r.Get("/api/emails/{id}/translation", translationHandler(dbpool))
	r.Post("/api/emails/{id}/translation/confirm", confirmTranslationHandler(dbpool))
	r.Patch("/api/emails/{id}/translation-link", translationLinkHandler(dbpool))
	r.Patch("/api/emails/{id}/place", placeEmailHandler(dbpool))
	r.Get("/api/admin/email-events", listEmailEventsHandler(dbpool))
}

func listEmailsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boardKey := strings.TrimSpace(r.URL.Query().Get("board"))
		args := []any{}
		boardFilter := ""
		if boardKey != "" {
			args = append(args, boardKey)
			boardFilter = "AND sequence = $1"
		}

		rows, err := dbpool.Query(r.Context(), `
			SELECT
				id,
				sequence,
				title,
				subject,
				preheader,
				send_timing,
				send_condition,
				stage,
				sort_order,
				language,
				variant,
				adaptation_key,
				adaptation_label,
				review_status,
				owner_email,
				reviewer_email,
				due_date::text,
				implementation_notes,
				(
					SELECT count(*)::int
					FROM comments
					WHERE comments.email_id = emails.id
						AND comments.status = 'open'
				) AS open_comment_count,
				(
					SELECT count(*)::int
					FROM comments
					WHERE comments.email_id = emails.id
						AND comments.status = 'open'
						AND comments.severity = 'blocking'
				) AS open_blocking_comment_count,
				COALESCE(portal.send_count, 0),
				portal.last_sent_at,
				live_marked_at,
				translation_of,
				COALESCE(translation.stale, false)
			FROM emails
			LEFT JOIN LATERAL (
				-- Sent emails the portal sync tied to this one: confirmed by a
				-- person, or matched automatically with no decision yet.
				SELECT sum(g.send_count)::int AS send_count, max(g.last_sent_at) AS last_sent_at
				FROM portal_email_groups g
				WHERE (g.decision IN ('confirmed', 'created') AND g.decision_email_id = emails.id)
					OR (g.decision IS NULL AND g.match_status = 'matched' AND g.match_email_id = emails.id)
			) portal ON true
			LEFT JOIN LATERAL (
				-- Outdated when the master's current text differs from the
				-- master version the translation was made from.
				SELECT email_text_hash(fv.subject, fv.preheader, fv.editable_fields)
					<> email_text_hash(lv.subject, lv.preheader, lv.editable_fields) AS stale
				FROM email_versions fv
				JOIN LATERAL (
					SELECT subject, preheader, editable_fields FROM email_versions
					WHERE email_id = emails.translation_of
					ORDER BY version_number DESC LIMIT 1
				) lv ON true
				WHERE emails.translation_of IS NOT NULL
					AND fv.email_id = emails.translation_of
					AND fv.version_number = emails.translated_from_version
			) translation ON true
			WHERE archived_at IS NULL
			`+boardFilter+`
			ORDER BY sort_order, created_at;
		`, args...)
		if err != nil {
			http.Error(w, "failed to load emails", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		emails := []EmailListItem{}

		for rows.Next() {
			var email EmailListItem

			err := rows.Scan(
				&email.ID,
				&email.Sequence,
				&email.Title,
				&email.Subject,
				&email.Preheader,
				&email.SendTiming,
				&email.SendCondition,
				&email.Stage,
				&email.SortOrder,
				&email.Language,
				&email.Variant,
				&email.AdaptationKey,
				&email.AdaptationLabel,
				&email.ReviewStatus,
				&email.OwnerEmail,
				&email.ReviewerEmail,
				&email.DueDate,
				&email.ImplementationNotes,
				&email.OpenCommentCount,
				&email.OpenBlockingCommentCount,
				&email.PortalSendCount,
				&email.PortalLastSentAt,
				&email.LiveMarkedAt,
				&email.TranslationOf,
				&email.TranslationStale,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to scan email row: %v\n", err)
				http.Error(w, "failed to read emails", http.StatusInternalServerError)
				return
			}

			emails = append(emails, email)
		}

		if err := rows.Err(); err != nil {
			http.Error(w, "failed to read emails", http.StatusInternalServerError)
			return
		}

		_ = json.NewEncoder(w).Encode(emails)
	}
}

func getEmailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var email EmailDetail
		var editableFields []byte

		err := dbpool.QueryRow(r.Context(), `
			SELECT
				id,
				slug,
				sequence,
				title,
				subject,
				preheader,
				send_timing,
				send_condition,
				stage,
				sort_order,
				language,
				variant,
				adaptation_key,
				adaptation_label,
				review_status,
				owner_email,
				reviewer_email,
				due_date::text,
				implementation_notes,
				(
					SELECT count(*)::int
					FROM comments
					WHERE comments.email_id = emails.id
						AND comments.status = 'open'
				) AS open_comment_count,
				(
					SELECT count(*)::int
					FROM comments
					WHERE comments.email_id = emails.id
						AND comments.status = 'open'
						AND comments.severity = 'blocking'
				) AS open_blocking_comment_count,
				live_marked_at,
				live_marked_by,
				live_note,
				original_html,
				review_html,
				template_html,
				template_hash,
				template_version,
				editable_fields
			FROM emails
			WHERE id = $1;
		`, id).Scan(
			&email.ID,
			&email.Slug,
			&email.Sequence,
			&email.Title,
			&email.Subject,
			&email.Preheader,
			&email.SendTiming,
			&email.SendCondition,
			&email.Stage,
			&email.SortOrder,
			&email.Language,
			&email.Variant,
			&email.AdaptationKey,
			&email.AdaptationLabel,
			&email.ReviewStatus,
			&email.OwnerEmail,
			&email.ReviewerEmail,
			&email.DueDate,
			&email.ImplementationNotes,
			&email.OpenCommentCount,
			&email.OpenBlockingCommentCount,
			&email.LiveMarkedAt,
			&email.LiveMarkedBy,
			&email.LiveNote,
			&email.OriginalHTML,
			&email.ReviewHTML,
			&email.TemplateHTML,
			&email.TemplateHash,
			&email.TemplateVersion,
			&editableFields,
		)

		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get email %s: %v\n", id, err)
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		email.EditableFields = json.RawMessage(editableFields)

		_ = json.NewEncoder(w).Encode(email)
	}
}

type createEmailRequest struct {
	Sequence        *string `json:"sequence"`
	Title           string  `json:"title"`
	Subject         *string `json:"subject"`
	Preheader       *string `json:"preheader"`
	SendTiming      *string `json:"send_timing"`
	SendCondition   *string `json:"send_condition"`
	Stage           string  `json:"stage"`
	SortOrder       int     `json:"sort_order"`
	Language        string  `json:"language"`
	Variant         string  `json:"variant"`
	AdaptationLabel *string `json:"adaptation_label"`
	OriginalHTML    string  `json:"original_html"`
}

type inspectEmailHTMLRequest struct {
	OriginalHTML string `json:"original_html"`
}

type emailHTMLInspection struct {
	ReviewBlockCount         int                       `json:"review_block_count"`
	OriginalReviewBlockCount int                       `json:"original_review_block_count"`
	EditableFieldCount       int                       `json:"editable_field_count"`
	EditableFields           []editableFieldInspection `json:"editable_fields"`
	Warnings                 []string                  `json:"warnings"`
	ReviewHTML               string                    `json:"review_html"`
	Detection                importDetection           `json:"detection"`
}

type editableFieldInspection struct {
	Key          string `json:"key"`
	Type         string `json:"type"`
	ValuePreview string `json:"value_preview"`
}

func createEmailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.IsAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var request createEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to create email", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		created, err := CreateEmailTx(r, dbpool, tx, user, NewEmailParams{
			Sequence:        stringFromPointer(request.Sequence),
			Title:           request.Title,
			Subject:         request.Subject,
			Preheader:       request.Preheader,
			SendTiming:      request.SendTiming,
			SendCondition:   request.SendCondition,
			Stage:           request.Stage,
			SortOrder:       request.SortOrder,
			Language:        request.Language,
			Variant:         request.Variant,
			AdaptationLabel: request.AdaptationLabel,
			OriginalHTML:    request.OriginalHTML,
		})
		if err != nil {
			writeCreateEmailError(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to create email", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)
	}
}

// NewEmailParams describes an email to create; see CreateEmailTx.
type NewEmailParams struct {
	Sequence        string
	Title           string
	Subject         *string
	Preheader       *string
	SendTiming      *string
	SendCondition   *string
	Stage           string
	SortOrder       int
	Language        string
	Variant         string
	AdaptationLabel *string
	OriginalHTML    string
}

// createEmailInputError is a validation problem the caller should see as 400.
type createEmailInputError string

func (e createEmailInputError) Error() string { return string(e) }

var errCreateEmailConflict = errors.New("email already exists")

func writeCreateEmailError(w http.ResponseWriter, err error) {
	var inputErr createEmailInputError
	switch {
	case errors.As(err, &inputErr):
		http.Error(w, inputErr.Error(), http.StatusBadRequest)
	case errors.Is(err, errCreateEmailConflict):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		fmt.Fprintf(os.Stderr, "failed to create email: %v\n", err)
		http.Error(w, "failed to create email", http.StatusInternalServerError)
	}
}

// CreateEmailTx validates and inserts one email with its creation event and
// initial version. The caller owns the transaction.
func CreateEmailTx(r *http.Request, dbpool *pgxpool.Pool, tx pgx.Tx, user AuthUser, params NewEmailParams) (EmailDetail, error) {
	sequence := strings.TrimSpace(params.Sequence)
	if sequence == "" {
		sequence = "onboarding"
	}
	title := strings.TrimSpace(params.Title)
	stage := strings.TrimSpace(params.Stage)
	language := strings.ToLower(strings.TrimSpace(params.Language))
	if language == "" {
		language = "en"
	}
	variant := strings.ToLower(strings.TrimSpace(params.Variant))
	if variant == "" {
		variant = "v1"
	}
	adaptationKey, adaptationLabel, err := normalizeEmailAdaptation(params.AdaptationLabel)
	if err != nil {
		return EmailDetail{}, createEmailInputError("adaptation label must contain letters, numbers, spaces, hyphens, or underscores")
	}
	originalHTML := strings.TrimSpace(params.OriginalHTML)

	if title == "" || stage == "" || originalHTML == "" {
		return EmailDetail{}, createEmailInputError("title, stage, and original_html are required")
	}
	exists, err := boards.Exists(r.Context(), dbpool, sequence)
	if err != nil {
		return EmailDetail{}, fmt.Errorf("validate board: %w", err)
	}
	if !exists {
		return EmailDetail{}, createEmailInputError("board not found")
	}

	subject := trimmedOptionalString(params.Subject)
	preheader := trimmedOptionalString(params.Preheader)
	sendTiming := trimmedOptionalString(params.SendTiming)
	sendCondition := trimmedOptionalString(params.SendCondition)
	reviewHTML, err := emailreview.AddReviewBlocks(originalHTML)
	if err != nil {
		return EmailDetail{}, createEmailInputError("invalid email HTML")
	}
	editableFields, err := emailedit.ExtractEditableFields(originalHTML)
	if err != nil {
		return EmailDetail{}, createEmailInputError("invalid editable fields in HTML")
	}
	editableFieldsJSON, err := editableFields.JSON()
	if err != nil {
		return EmailDetail{}, createEmailInputError("invalid editable fields in HTML")
	}
	contentParts := emailtext.ExtractContentParts(
		originalHTML,
		stringFromPointer(subject),
		stringFromPointer(preheader),
		"",
	)
	contentPartsJSON, err := json.Marshal(contentParts)
	if err != nil {
		return EmailDetail{}, err
	}
	slug := emailSlugWithAdaptation(
		newEmailSlug(sequence, stage, title, language, variant),
		adaptationKey,
	)

	created, err := insertDuplicatedEmail(r, tx, duplicateEmailInsert{
		Slug:            slug,
		Sequence:        sequence,
		Title:           title,
		Subject:         subject,
		Preheader:       preheader,
		SendTiming:      sendTiming,
		SendCondition:   sendCondition,
		Stage:           stage,
		SortOrder:       params.SortOrder,
		Language:        language,
		Variant:         variant,
		AdaptationKey:   adaptationKey,
		AdaptationLabel: adaptationLabel,
		ContentParts:    contentPartsJSON,
		OriginalHTML:    originalHTML,
		ReviewHTML:      &reviewHTML,
		TemplateHTML:    originalHTML,
		EditableFields:  editableFieldsJSON,
	})
	if err != nil {
		if isEmailCreationConflict(err) {
			return EmailDetail{}, errCreateEmailConflict
		}
		return EmailDetail{}, err
	}
	if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
		ActorUserID: user.ID,
		ActorEmail:  user.Email,
		Action:      emailEventCreated,
		EmailID:     &created.ID,
		EmailSlug:   &created.Slug,
		EmailTitle:  &created.Title,
		Metadata: map[string]any{
			"slug":       created.Slug,
			"sequence":   created.Sequence,
			"stage":      created.Stage,
			"language":   created.Language,
			"variant":    created.Variant,
			"adaptation": created.AdaptationKey,
			"sort_order": created.SortOrder,
		},
	}); err != nil {
		return EmailDetail{}, fmt.Errorf("record email event: %w", err)
	}
	if err := insertEmailVersion(r.Context(), tx, emailVersionSnapshot{
		EmailID:            created.ID,
		CreatedByUserID:    user.ID,
		CreatedByEmail:     user.Email,
		Source:             "initial",
		Title:              created.Title,
		Subject:            created.Subject,
		Preheader:          created.Preheader,
		OriginalHTML:       created.OriginalHTML,
		TemplateHTML:       created.TemplateHTML,
		EditableFieldsJSON: editableFieldsJSON,
	}); err != nil {
		return EmailDetail{}, fmt.Errorf("record email version: %w", err)
	}

	return created, nil
}

func inspectEmailHTMLHandler(_ *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.IsAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var request inspectEmailHTMLRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		inspection, err := inspectEmailHTML(strings.TrimSpace(request.OriginalHTML))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(inspection)
	}
}

type updateEmailEditableFieldsRequest struct {
	Title          *string                  `json:"title"`
	Subject        *string                  `json:"subject"`
	Preheader      *string                  `json:"preheader"`
	EditableFields emailedit.EditableFields `json:"editable_fields"`
	OriginalHTML   *string                  `json:"original_html"`
}

type updateEmailReviewStatusRequest struct {
	ReviewStatus string `json:"review_status"`
}

type updateEmailReviewStatusResponse struct {
	ReviewStatus string `json:"review_status"`
}

type updateEmailPlanningFieldsRequest struct {
	OwnerEmail          *string `json:"owner_email"`
	ReviewerEmail       *string `json:"reviewer_email"`
	DueDate             *string `json:"due_date"`
	ImplementationNotes *string `json:"implementation_notes"`
	SendTiming          *string `json:"send_timing"`
	SendCondition       *string `json:"send_condition"`
	AdaptationLabel     *string `json:"adaptation_label"`
}

type updateEmailPlanningFieldsResponse struct {
	OwnerEmail          *string `json:"owner_email"`
	ReviewerEmail       *string `json:"reviewer_email"`
	DueDate             *string `json:"due_date"`
	ImplementationNotes *string `json:"implementation_notes"`
	SendTiming          *string `json:"send_timing"`
	SendCondition       *string `json:"send_condition"`
	AdaptationLabel     string  `json:"adaptation_label"`
}

type renderedEmailResponse struct {
	HTML string `json:"html"`
}

func getRenderedEmailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var templateHTML string
		var preheader *string
		var editableFieldsJSON []byte
		err := dbpool.QueryRow(r.Context(), `
			SELECT template_html, preheader, editable_fields
			FROM emails
			WHERE id = $1
				AND archived_at IS NULL;
		`, id).Scan(&templateHTML, &preheader, &editableFieldsJSON)
		if err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		var editableFields emailedit.EditableFields
		if err := json.Unmarshal(editableFieldsJSON, &editableFields); err != nil {
			http.Error(w, "invalid editable fields", http.StatusInternalServerError)
			return
		}

		html, err := emailedit.RenderEditableHTMLWithMetadata(templateHTML, editableFields, emailedit.RenderMetadata{
			Preheader: stringFromPointer(preheader),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to render email %s: %v\n", id, err)
			http.Error(w, "failed to render email", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(renderedEmailResponse{HTML: html})
	}
}

func updateEmailEditableFieldsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
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

		var request updateEmailEditableFieldsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if request.EditableFields == nil {
			http.Error(w, "editable_fields is required", http.StatusBadRequest)
			return
		}
		if request.OriginalHTML != nil && !auth.IsSuperAdmin(user) {
			http.Error(w, "original_html requires super admin", http.StatusForbidden)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to update editable fields", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		var originalHTML string
		var templateHTML string
		var currentTitle string
		var currentSubject *string
		var currentPreheader *string
		var currentEditableFieldsJSON []byte
		var slug string
		var currentReviewStatus string
		err = tx.QueryRow(r.Context(), `
			SELECT slug, original_html, template_html, title, subject, preheader, editable_fields, review_status
			FROM emails
			WHERE id = $1
				AND archived_at IS NULL
			FOR UPDATE;
		`, id).Scan(&slug, &originalHTML, &templateHTML, &currentTitle, &currentSubject, &currentPreheader, &currentEditableFieldsJSON, &currentReviewStatus)
		if err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		title := currentTitle
		if request.Title != nil {
			title = strings.TrimSpace(*request.Title)
		}
		if title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}

		subject := currentSubject
		if request.Subject != nil {
			subject = request.Subject
		}
		preheader := currentPreheader
		if request.Preheader != nil {
			preheader = request.Preheader
		}

		nextOriginalHTML := originalHTML
		nextTemplateHTML := templateHTML
		nextEditableFields := request.EditableFields
		originalHTMLChanged := false
		if request.OriginalHTML != nil {
			nextOriginalHTML = strings.TrimSpace(*request.OriginalHTML)
			if nextOriginalHTML == "" {
				http.Error(w, "original_html is required", http.StatusBadRequest)
				return
			}
			if _, err := emailreview.AddReviewBlocks(nextOriginalHTML); err != nil {
				http.Error(w, "invalid email HTML", http.StatusBadRequest)
				return
			}
			extractedFields, err := emailedit.ExtractEditableFields(nextOriginalHTML)
			if err != nil {
				http.Error(w, "invalid editable fields in original_html", http.StatusBadRequest)
				return
			}
			nextEditableFields = mergeEditableFields(extractedFields, request.EditableFields)
			nextTemplateHTML = nextOriginalHTML
			originalHTMLChanged = nextOriginalHTML != originalHTML
		}

		renderedHTML, err := emailedit.RenderEditableHTMLWithMetadata(nextTemplateHTML, nextEditableFields, emailedit.RenderMetadata{
			Preheader: stringFromPointer(preheader),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid editable fields for email %s: %v\n", id, err)
			http.Error(w, "invalid editable fields", http.StatusBadRequest)
			return
		}

		reviewHTML, err := emailreview.AddReviewBlocks(renderedHTML)
		if err != nil {
			http.Error(w, "invalid email HTML", http.StatusBadRequest)
			return
		}

		fieldsJSON, err := nextEditableFields.JSON()
		if err != nil {
			http.Error(w, "invalid editable fields", http.StatusBadRequest)
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
			http.Error(w, "failed to update editable fields", http.StatusInternalServerError)
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
			nextEditableFields,
		)
		if err != nil {
			http.Error(w, "failed to update editable fields", http.StatusInternalServerError)
			return
		}
		if originalHTMLChanged {
			changes["original_html"] = map[string]any{
				"before": originalHTML,
				"after":  nextOriginalHTML,
			}
		}
		changedReviewBlocks, err := changedReviewBlocksForEmailUpdate(templateHTML, nextTemplateHTML, changes)
		if err != nil {
			http.Error(w, "failed to update editable fields", http.StatusInternalServerError)
			return
		}
		nextReviewStatus := currentReviewStatus
		approvalBecameStale := len(changes) > 0 && isApprovedEmailReviewStatus(currentReviewStatus)
		if approvalBecameStale {
			nextReviewStatus = "changes_requested"
		}
		commentAnchorTexts, err := changedCommentAnchorTexts(
			nextTemplateHTML,
			renderedHTML,
			currentTitle,
			currentSubject,
			currentPreheader,
			currentEditableFieldsJSON,
			title,
			subject,
			preheader,
			nextEditableFields,
		)
		if err != nil {
			http.Error(w, "failed to update editable fields", http.StatusInternalServerError)
			return
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
		`, id, title, subject, preheader, fieldsJSON, contentPartsJSON, reviewHTML, renderedBodyText, nextOriginalHTML, nextTemplateHTML, nextReviewStatus)
		if err != nil {
			http.Error(w, "failed to update editable fields", http.StatusInternalServerError)
			return
		}
		if result.RowsAffected() == 0 {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}
		if len(changes) > 0 || originalHTMLChanged {
			if err := insertEmailVersion(r.Context(), tx, emailVersionSnapshot{
				EmailID:              id,
				CreatedByUserID:      user.ID,
				CreatedByEmail:       user.Email,
				Source:               "manual",
				Title:                title,
				Subject:              subject,
				Preheader:            preheader,
				OriginalHTML:         nextOriginalHTML,
				TemplateHTML:         nextTemplateHTML,
				EditableFieldsJSON:   fieldsJSON,
				ChangedFieldCount:    countEditableFieldChanges(changes),
				ChangedMetadataCount: countMetadataChanges(changes),
				HTMLChanged:          originalHTMLChanged,
			}); err != nil {
				http.Error(w, "failed to record email version", http.StatusInternalServerError)
				return
			}
		}
		if err := resetCommentAnchorsForReviewBlocks(r.Context(), tx, id, commentAnchorTexts); err != nil {
			http.Error(w, "failed to update editable fields", http.StatusInternalServerError)
			return
		}
		updateEventMetadata := map[string]any{}
		if len(changedReviewBlocks) > 0 {
			updateEventMetadata["changed_review_blocks"] = changedReviewBlocks
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventUpdated,
			EmailID:     &id,
			EmailSlug:   &slug,
			EmailTitle:  &title,
			Metadata:    updateEventMetadata,
			Changes:     changes,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to insert email update event for %s: %v\n", id, err)
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}
		if len(changes) > 0 || originalHTMLChanged {
			if err := markStaleAreaApprovalsAfterContentChange(
				r.Context(),
				tx,
				id,
				user,
				"approval_stale_after_edit",
			); err != nil {
				fmt.Fprintf(os.Stderr, "failed to mark stale area approvals for %s: %v\n", id, err)
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
					"reason": "approval_stale_after_edit",
				},
				Changes: map[string]any{
					"review_status": map[string]any{
						"before": currentReviewStatus,
						"after":  nextReviewStatus,
					},
				},
			}); err != nil {
				fmt.Fprintf(os.Stderr, "failed to insert stale approval event for %s: %v\n", id, err)
				http.Error(w, "failed to record email event", http.StatusInternalServerError)
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update editable fields", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func updateEmailPlanningFieldsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
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

		var request updateEmailPlanningFieldsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		ownerEmail := trimmedOptionalString(request.OwnerEmail)
		reviewerEmail := trimmedOptionalString(request.ReviewerEmail)
		dueDate, err := normalizedOptionalDate(request.DueDate)
		if err != nil {
			http.Error(w, "due_date must use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		implementationNotes := trimmedOptionalString(request.ImplementationNotes)
		sendTiming := trimmedOptionalString(request.SendTiming)
		sendCondition := trimmedOptionalString(request.SendCondition)
		_, adaptationLabel, err := normalizeEmailAdaptation(request.AdaptationLabel)
		if err != nil {
			http.Error(w, "adaptation_label must contain letters, numbers, spaces, hyphens, or underscores", http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to update planning fields", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		var slug string
		var title string
		var currentOwnerEmail *string
		var currentReviewerEmail *string
		var currentDueDate *string
		var currentImplementationNotes *string
		var currentSendTiming *string
		var currentSendCondition *string
		var currentAdaptationLabel string
		err = tx.QueryRow(r.Context(), `
			SELECT slug, title, owner_email, reviewer_email, due_date::text, implementation_notes, send_timing, send_condition, adaptation_label
			FROM emails
			WHERE id = $1
				AND archived_at IS NULL
			FOR UPDATE;
		`, id).Scan(&slug, &title, &currentOwnerEmail, &currentReviewerEmail, &currentDueDate, &currentImplementationNotes, &currentSendTiming, &currentSendCondition, &currentAdaptationLabel)
		if err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}
		if request.SendTiming == nil {
			sendTiming = currentSendTiming
		}
		if request.SendCondition == nil {
			sendCondition = currentSendCondition
		}
		if request.AdaptationLabel == nil {
			adaptationLabel = currentAdaptationLabel
		}

		changes := map[string]any{}
		addStringChange(changes, "owner_email", currentOwnerEmail, ownerEmail)
		addStringChange(changes, "reviewer_email", currentReviewerEmail, reviewerEmail)
		addStringChange(changes, "due_date", currentDueDate, dueDate)
		addStringChange(changes, "implementation_notes", currentImplementationNotes, implementationNotes)
		addStringChange(changes, "send_timing", currentSendTiming, sendTiming)
		addStringChange(changes, "send_condition", currentSendCondition, sendCondition)
		addStringChange(changes, "adaptation_label", &currentAdaptationLabel, &adaptationLabel)

		if len(changes) > 0 {
			result, err := tx.Exec(r.Context(), `
				UPDATE emails
				SET owner_email = $2,
					reviewer_email = $3,
					due_date = $4,
					implementation_notes = $5,
					send_timing = $6,
					adaptation_label = $7,
					send_condition = $8,
					updated_at = now()
				WHERE id = $1
					AND archived_at IS NULL;
			`, id, ownerEmail, reviewerEmail, dueDate, implementationNotes, sendTiming, adaptationLabel, sendCondition)
			if err != nil {
				http.Error(w, "failed to update planning fields", http.StatusInternalServerError)
				return
			}
			if result.RowsAffected() == 0 {
				http.Error(w, "email not found", http.StatusNotFound)
				return
			}

			if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
				ActorUserID: user.ID,
				ActorEmail:  user.Email,
				Action:      emailEventPlanningUpdated,
				EmailID:     &id,
				EmailSlug:   &slug,
				EmailTitle:  &title,
				Changes:     changes,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "failed to insert email planning event for %s: %v\n", id, err)
				http.Error(w, "failed to record email event", http.StatusInternalServerError)
				return
			}
		}

		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update planning fields", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updateEmailPlanningFieldsResponse{
			OwnerEmail:          ownerEmail,
			ReviewerEmail:       reviewerEmail,
			DueDate:             dueDate,
			ImplementationNotes: implementationNotes,
			SendTiming:          sendTiming,
			SendCondition:       sendCondition,
			AdaptationLabel:     adaptationLabel,
		})
	}
}

func updateEmailReviewStatusHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
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

		var request updateEmailReviewStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		nextStatus := strings.TrimSpace(request.ReviewStatus)
		if !isValidEmailReviewStatus(nextStatus) {
			http.Error(w, "invalid review_status", http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to update review status", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		var slug string
		var title string
		var currentStatus string
		var currentSubject *string
		var currentPreheader *string
		var currentTemplateHash *string
		var currentTemplateHTML string
		var currentEditableFieldsText string
		err = tx.QueryRow(r.Context(), `
			SELECT slug, title, review_status, subject, preheader, template_hash, template_html, editable_fields::text
			FROM emails
			WHERE id = $1
				AND archived_at IS NULL
			FOR UPDATE;
		`, id).Scan(
			&slug,
			&title,
			&currentStatus,
			&currentSubject,
			&currentPreheader,
			&currentTemplateHash,
			&currentTemplateHTML,
			&currentEditableFieldsText,
		)
		if err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		if currentStatus != nextStatus {
			if isApprovedEmailReviewStatus(nextStatus) {
				blockers, err := emailApprovalGateBlockers(r.Context(), tx, id)
				if err != nil {
					http.Error(w, "failed to update review status", http.StatusInternalServerError)
					return
				}
				if len(blockers) > 0 {
					http.Error(w, strings.Join(blockers, "; "), http.StatusConflict)
					return
				}
			}

			eventMetadata := map[string]any{}
			if isApprovedEmailReviewStatus(nextStatus) {
				for key, value := range approvalContentSnapshot(
					title,
					currentSubject,
					currentPreheader,
					currentTemplateHash,
					currentTemplateHTML,
					currentEditableFieldsText,
				) {
					eventMetadata[key] = value
				}
			}
			if currentStatus == "changes_requested" && isApprovedEmailReviewStatus(nextStatus) {
				eventMetadata["reason"] = "reapproved"
				reapprovesStaleEdit, err := latestReviewStatusEventMarkedApprovalStale(r.Context(), tx, id)
				if err != nil {
					http.Error(w, "failed to update review status", http.StatusInternalServerError)
					return
				}
				if reapprovesStaleEdit {
					eventMetadata["reason"] = "reapproved_after_stale_edit"
				}
			}

			result, err := tx.Exec(r.Context(), `
				UPDATE emails
				SET review_status = $2,
					updated_at = now()
				WHERE id = $1
					AND archived_at IS NULL;
			`, id, nextStatus)
			if err != nil {
				http.Error(w, "failed to update review status", http.StatusInternalServerError)
				return
			}
			if result.RowsAffected() == 0 {
				http.Error(w, "email not found", http.StatusNotFound)
				return
			}

			if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
				ActorUserID: user.ID,
				ActorEmail:  user.Email,
				Action:      emailEventReviewStatusUpdated,
				EmailID:     &id,
				EmailSlug:   &slug,
				EmailTitle:  &title,
				Metadata:    eventMetadata,
				Changes: map[string]any{
					"review_status": map[string]any{
						"before": currentStatus,
						"after":  nextStatus,
					},
				},
			}); err != nil {
				fmt.Fprintf(os.Stderr, "failed to insert email review status event for %s: %v\n", id, err)
				http.Error(w, "failed to record email event", http.StatusInternalServerError)
				return
			}
		}

		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update review status", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updateEmailReviewStatusResponse{ReviewStatus: nextStatus})
	}
}

func emailApprovalGateBlockers(ctx context.Context, db emailEventExecutor, emailID string) ([]string, error) {
	blockers := []string{}

	var openBlockingCommentCount int
	err := db.QueryRow(ctx, `
		SELECT count(*)::int
		FROM comments
		WHERE email_id = $1
			AND status = 'open'
			AND severity = 'blocking';
	`, emailID).Scan(&openBlockingCommentCount)
	if err != nil {
		return nil, err
	}
	if openBlockingCommentCount > 0 {
		blockers = append(blockers, "resolve blocking comments before approving")
	}

	rows, err := db.Query(ctx, `
		SELECT board_approval_areas.name, coalesce(email_area_approvals.status, 'pending') AS status
		FROM emails
		JOIN boards ON boards.key = coalesce(nullif(trim(emails.sequence), ''), 'onboarding')
		JOIN board_approval_areas ON board_approval_areas.board_id = boards.id
			AND board_approval_areas.archived_at IS NULL
			AND board_approval_areas.required = true
		LEFT JOIN email_area_approvals ON email_area_approvals.email_id = emails.id
			AND email_area_approvals.board_approval_area_id = board_approval_areas.id
		WHERE emails.id = $1
			AND emails.archived_at IS NULL
			AND coalesce(email_area_approvals.status, 'pending') <> 'approved'
		ORDER BY board_approval_areas.sort_order, board_approval_areas.created_at;
	`, emailID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pendingRequiredAreas := []string{}
	for rows.Next() {
		var name string
		var status string
		if err := rows.Scan(&name, &status); err != nil {
			return nil, err
		}
		pendingRequiredAreas = append(pendingRequiredAreas, fmt.Sprintf("%s (%s)", name, status))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pendingRequiredAreas) > 0 {
		blockers = append(blockers, "complete required approvals before approving: "+strings.Join(pendingRequiredAreas, ", "))
	}

	return blockers, nil
}

func latestReviewStatusEventMarkedApprovalStale(ctx context.Context, db emailEventExecutor, emailID string) (bool, error) {
	var reason string
	err := db.QueryRow(ctx, `
		SELECT coalesce(metadata->>'reason', '')
		FROM email_events
		WHERE email_id = $1
			AND action = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1;
	`, emailID, emailEventReviewStatusUpdated).Scan(&reason)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return reason == "approval_stale_after_edit", nil
}

func approvalContentSnapshot(
	title string,
	subject *string,
	preheader *string,
	templateHash *string,
	templateHTML string,
	editableFieldsText string,
) map[string]any {
	templateFingerprint := stringFromPointer(templateHash)
	if templateFingerprint == "" {
		templateFingerprint = contentHash(templateHTML)
	}
	editableFieldsHash := contentHash(editableFieldsText)
	contentHashValue := contentHash(strings.Join([]string{
		"approval-snapshot-v1",
		title,
		stringFromPointer(subject),
		stringFromPointer(preheader),
		templateFingerprint,
		editableFieldsHash,
	}, "\x00"))

	return map[string]any{
		"approval_snapshot_version":     1,
		"approved_content_hash":         contentHashValue,
		"approved_editable_fields_hash": editableFieldsHash,
		"approved_template_hash":        templateFingerprint,
	}
}

func contentHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func isValidEmailReviewStatus(status string) bool {
	switch status {
	case "draft", "in_review", "changes_requested", "approved", "production_approved":
		return true
	default:
		return false
	}
}

func isApprovedEmailReviewStatus(status string) bool {
	return status == "approved" || status == "production_approved"
}

type duplicateEmailRequest struct {
	Language  string  `json:"language"`
	Variant   *string `json:"variant"`
	Title     *string `json:"title"`
	Subject   *string `json:"subject"`
	Preheader *string `json:"preheader"`
}

type createEmailAdaptationRequest struct {
	Label string `json:"label"`
}

type duplicateEmailAsRequest struct {
	Sequence        *string `json:"sequence"`
	Stage           *string `json:"stage"`
	SortOrder       *int    `json:"sort_order"`
	Language        *string `json:"language"`
	Variant         *string `json:"variant"`
	AdaptationLabel *string `json:"adaptation_label"`
	Title           *string `json:"title"`
	Subject         *string `json:"subject"`
	Preheader       *string `json:"preheader"`
}

func duplicateEmailAsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
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

		var request duplicateEmailAsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		source, err := loadEmailForDuplicate(r, dbpool, id)
		if err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		sequence := source.Sequence
		if request.Sequence != nil {
			sequence = strings.TrimSpace(*request.Sequence)
		}
		if sequence == "" {
			http.Error(w, "board is required", http.StatusBadRequest)
			return
		}
		exists, err := boards.Exists(r.Context(), dbpool, sequence)
		if err != nil {
			http.Error(w, "failed to validate board", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "board not found", http.StatusBadRequest)
			return
		}

		stage := source.Stage
		if request.Stage != nil {
			stage = strings.TrimSpace(*request.Stage)
		}
		if stage == "" {
			http.Error(w, "stage is required", http.StatusBadRequest)
			return
		}

		sortOrder := source.SortOrder
		if request.SortOrder != nil {
			sortOrder = *request.SortOrder
		}

		language := source.Language
		if request.Language != nil {
			language = strings.ToLower(strings.TrimSpace(*request.Language))
		}
		if language == "" {
			http.Error(w, "language is required", http.StatusBadRequest)
			return
		}

		variant := source.Variant
		if request.Variant != nil {
			variant = strings.ToLower(strings.TrimSpace(*request.Variant))
		}
		if variant == "" {
			http.Error(w, "variant is required", http.StatusBadRequest)
			return
		}

		adaptationKey := source.AdaptationKey
		adaptationLabel := source.AdaptationLabel
		if request.AdaptationLabel != nil {
			adaptationKey, adaptationLabel, err = normalizeEmailAdaptation(request.AdaptationLabel)
			if err != nil {
				http.Error(w, "invalid adaptation_label", http.StatusBadRequest)
				return
			}
		}

		title := source.Title
		if request.Title != nil {
			title = strings.TrimSpace(*request.Title)
		}
		if title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}

		subject := source.Subject
		if request.Subject != nil {
			subject = request.Subject
		}

		preheader := source.Preheader
		if request.Preheader != nil {
			preheader = request.Preheader
		}

		if language == source.Language &&
			variant == source.Variant &&
			adaptationKey == source.AdaptationKey &&
			sequence == source.Sequence &&
			stage == source.Stage &&
			sortOrder == source.SortOrder &&
			title == source.Title &&
			stringFromPointer(subject) == stringFromPointer(source.Subject) &&
			stringFromPointer(preheader) == stringFromPointer(source.Preheader) {
			http.Error(w, "target must change board, stage, language, version, adaptation, event, or copy", http.StatusBadRequest)
			return
		}

		baseSlug := duplicateEmailSlug(source.Slug, source.Language, source.Variant, language, variant)
		if sequence != source.Sequence || stage != source.Stage || title != source.Title {
			baseSlug = newEmailSlug(sequence, stage, title, language, variant)
		}
		slug := emailSlugWithAdaptation(baseSlug, adaptationKey)
		contentParts, err := json.Marshal(emailtext.ExtractContentParts(
			source.OriginalHTML,
			stringFromPointer(subject),
			stringFromPointer(preheader),
			stringFromPointer(source.BodyText),
		))
		if err != nil {
			http.Error(w, "failed to build duplicated email", http.StatusInternalServerError)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to duplicate email", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		created, err := insertDuplicatedEmail(r, tx, duplicateEmailInsert{
			Slug:            slug,
			Sequence:        sequence,
			Title:           title,
			Subject:         subject,
			Preheader:       preheader,
			SendTiming:      source.SendTiming,
			SendCondition:   source.SendCondition,
			Stage:           stage,
			SortOrder:       sortOrder,
			Language:        language,
			Variant:         variant,
			AdaptationKey:   adaptationKey,
			AdaptationLabel: adaptationLabel,
			BodyText:        source.BodyText,
			ContentParts:    contentParts,
			OriginalHTML:    source.OriginalHTML,
			ReviewHTML:      source.ReviewHTML,
			TemplateHTML:    source.TemplateHTML,
			TemplateHash:    source.TemplateHash,
			TemplateVersion: source.TemplateVersion,
			EditableFields:  source.EditableFields,
		})
		if err != nil {
			if isEmailCreationConflict(err) {
				http.Error(w, "email duplicate already exists", http.StatusConflict)
				return
			}
			fmt.Fprintf(os.Stderr, "failed to duplicate email as %s: %v\n", id, err)
			http.Error(w, "failed to duplicate email", http.StatusInternalServerError)
			return
		}
		// A copy of an EN email into another language of the same slot is
		// its translation.
		if source.Language == "en" && language != "en" &&
			sequence == source.Sequence && stage == source.Stage && sortOrder == source.SortOrder {
			if err := linkTranslation(r.Context(), tx, created.ID, id); err != nil {
				http.Error(w, "failed to link translation", http.StatusInternalServerError)
				return
			}
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventDuplicated,
			EmailID:     &created.ID,
			EmailSlug:   &created.Slug,
			EmailTitle:  &created.Title,
			Metadata: map[string]any{
				"source_email_id":  id,
				"source_slug":      source.Slug,
				"created_email_id": created.ID,
				"sequence":         created.Sequence,
				"stage":            created.Stage,
				"sort_order":       created.SortOrder,
				"language":         created.Language,
				"variant":          created.Variant,
				"adaptation_key":   created.AdaptationKey,
				"adaptation_label": created.AdaptationLabel,
				"mode":             "duplicate_as",
			},
		}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to insert email duplicate-as event for %s: %v\n", created.ID, err)
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to duplicate email", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)
	}
}

func duplicateEmailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
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

		var request duplicateEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		language := strings.ToLower(strings.TrimSpace(request.Language))
		if language == "" {
			http.Error(w, "language is required", http.StatusBadRequest)
			return
		}

		source, err := loadEmailForDuplicate(r, dbpool, id)
		if err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		variant := source.Variant
		if request.Variant != nil {
			variant = strings.ToLower(strings.TrimSpace(*request.Variant))
		}
		if variant == "" {
			http.Error(w, "variant is required", http.StatusBadRequest)
			return
		}

		title := source.Title
		if request.Title != nil {
			title = strings.TrimSpace(*request.Title)
		}
		if title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}

		subject := source.Subject
		if request.Subject != nil {
			subject = request.Subject
		}

		preheader := source.Preheader
		if request.Preheader != nil {
			preheader = request.Preheader
		}

		slug := emailSlugWithAdaptation(
			duplicateEmailSlug(source.Slug, source.Language, source.Variant, language, variant),
			source.AdaptationKey,
		)
		contentParts, err := json.Marshal(emailtext.ExtractContentParts(
			source.OriginalHTML,
			stringFromPointer(subject),
			stringFromPointer(preheader),
			stringFromPointer(source.BodyText),
		))
		if err != nil {
			http.Error(w, "failed to build duplicated email", http.StatusInternalServerError)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to duplicate email", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		created, err := insertDuplicatedEmail(r, tx, duplicateEmailInsert{
			Slug:            slug,
			Sequence:        source.Sequence,
			Title:           title,
			Subject:         subject,
			Preheader:       preheader,
			SendTiming:      source.SendTiming,
			SendCondition:   source.SendCondition,
			Stage:           source.Stage,
			SortOrder:       source.SortOrder,
			Language:        language,
			Variant:         variant,
			AdaptationKey:   source.AdaptationKey,
			AdaptationLabel: source.AdaptationLabel,
			BodyText:        source.BodyText,
			ContentParts:    contentParts,
			OriginalHTML:    source.OriginalHTML,
			ReviewHTML:      source.ReviewHTML,
			TemplateHTML:    source.TemplateHTML,
			TemplateHash:    source.TemplateHash,
			TemplateVersion: source.TemplateVersion,
			EditableFields:  source.EditableFields,
		})
		if err != nil {
			if isEmailCreationConflict(err) {
				http.Error(w, "email duplicate already exists", http.StatusConflict)
				return
			}
			fmt.Fprintf(os.Stderr, "failed to duplicate email %s: %v\n", id, err)
			http.Error(w, "failed to duplicate email", http.StatusInternalServerError)
			return
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventDuplicated,
			EmailID:     &created.ID,
			EmailSlug:   &created.Slug,
			EmailTitle:  &created.Title,
			Metadata: map[string]any{
				"source_email_id":  id,
				"source_slug":      source.Slug,
				"created_email_id": created.ID,
				"language":         created.Language,
				"variant":          created.Variant,
				"adaptation_key":   created.AdaptationKey,
			},
		}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to insert email duplicate event for %s: %v\n", created.ID, err)
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to duplicate email", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)
	}
}

func createEmailAdaptationHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
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

		var request createEmailAdaptationRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		label := strings.TrimSpace(request.Label)
		key := normalizeAdaptationKey(label)
		if label == "" || key == "" {
			http.Error(w, "label is required", http.StatusBadRequest)
			return
		}

		source, err := loadEmailForDuplicate(r, dbpool, id)
		if err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		contentParts, err := json.Marshal(emailtext.ExtractContentParts(
			source.OriginalHTML,
			stringFromPointer(source.Subject),
			stringFromPointer(source.Preheader),
			stringFromPointer(source.BodyText),
		))
		if err != nil {
			http.Error(w, "failed to build adaptation", http.StatusInternalServerError)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to create adaptation", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		created, err := insertDuplicatedEmail(r, tx, duplicateEmailInsert{
			Slug:            adaptationEmailSlug(source.Slug, source.AdaptationKey, key),
			Sequence:        source.Sequence,
			Title:           source.Title,
			Subject:         source.Subject,
			Preheader:       source.Preheader,
			SendTiming:      source.SendTiming,
			SendCondition:   source.SendCondition,
			Stage:           source.Stage,
			SortOrder:       source.SortOrder,
			Language:        source.Language,
			Variant:         source.Variant,
			AdaptationKey:   key,
			AdaptationLabel: label,
			BodyText:        source.BodyText,
			ContentParts:    contentParts,
			OriginalHTML:    source.OriginalHTML,
			ReviewHTML:      source.ReviewHTML,
			TemplateHTML:    source.TemplateHTML,
			TemplateHash:    source.TemplateHash,
			TemplateVersion: source.TemplateVersion,
			EditableFields:  source.EditableFields,
		})
		if err != nil {
			if isEmailCreationConflict(err) {
				http.Error(w, "email adaptation already exists", http.StatusConflict)
				return
			}
			fmt.Fprintf(os.Stderr, "failed to create adaptation from email %s: %v\n", id, err)
			http.Error(w, "failed to create adaptation", http.StatusInternalServerError)
			return
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventAdaptationCreated,
			EmailID:     &created.ID,
			EmailSlug:   &created.Slug,
			EmailTitle:  &created.Title,
			Metadata: map[string]any{
				"source_email_id":  id,
				"source_slug":      source.Slug,
				"created_email_id": created.ID,
				"adaptation_key":   created.AdaptationKey,
				"adaptation_label": created.AdaptationLabel,
			},
		}); err != nil {
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to create adaptation", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)
	}
}

func archiveEmailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
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

		var snapshot emailArchiveSnapshot
		err := dbpool.QueryRow(r.Context(), `
			SELECT id, slug, title, language, variant, stage
			FROM emails
			WHERE id = $1
				AND archived_at IS NULL;
		`, id).Scan(
			&snapshot.ID,
			&snapshot.Slug,
			&snapshot.Title,
			&snapshot.Language,
			&snapshot.Variant,
			&snapshot.Stage,
		)
		if err != nil {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to archive email", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		archivedSlug := archivedEmailSlug(snapshot.Slug, snapshot.ID)
		result, err := tx.Exec(r.Context(), `
			UPDATE emails
			SET archived_at = now(),
				archived_by = $2,
				slug = $3,
				updated_at = now()
			WHERE id = $1
				AND archived_at IS NULL;
		`, id, user.ID, archivedSlug)
		if err != nil {
			http.Error(w, "failed to archive email", http.StatusInternalServerError)
			return
		}
		if result.RowsAffected() == 0 {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventArchived,
			EmailID:     &snapshot.ID,
			EmailSlug:   &snapshot.Slug,
			EmailTitle:  &snapshot.Title,
			Metadata: map[string]any{
				"slug":     snapshot.Slug,
				"title":    snapshot.Title,
				"language": snapshot.Language,
				"variant":  snapshot.Variant,
				"stage":    snapshot.Stage,
			},
		}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to insert email archive event for %s: %v\n", id, err)
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to archive email", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func listEmailEventsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.IsSuperAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		filters := emailEventFilters{
			ActorEmail: strings.TrimSpace(r.URL.Query().Get("actor_email")),
			Action:     strings.TrimSpace(r.URL.Query().Get("action")),
			Email:      strings.TrimSpace(r.URL.Query().Get("email")),
			Limit:      featureauth.ParseLimit(r.URL.Query().Get("limit")),
		}

		events, err := listEmailEvents(r.Context(), dbpool, filters)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list email events: %v\n", err)
			http.Error(w, "failed to load email events", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(events)
	}
}

func inspectEmailHTML(originalHTML string) (emailHTMLInspection, error) {
	if originalHTML == "" {
		return emailHTMLInspection{}, errors.New("original_html is required")
	}

	originalReviewBlockCount, err := countReviewBlocks(originalHTML)
	if err != nil {
		return emailHTMLInspection{}, errors.New("invalid email HTML")
	}

	reviewHTML, err := emailreview.AddReviewBlocks(originalHTML)
	if err != nil {
		return emailHTMLInspection{}, errors.New("invalid email HTML")
	}

	reviewBlockCount, err := countReviewBlocks(reviewHTML)
	if err != nil {
		return emailHTMLInspection{}, errors.New("invalid generated review HTML")
	}

	editableFields, err := emailedit.ExtractEditableFields(originalHTML)
	if err != nil {
		return emailHTMLInspection{}, fmt.Errorf("invalid editable fields: %w", err)
	}

	warnings := []string{}
	if originalReviewBlockCount == 0 {
		warnings = append(warnings, "No data-review-block markers were found; review blocks will be generated automatically.")
	}
	if reviewBlockCount == 0 {
		warnings = append(warnings, "No reviewable content was found. Block comments may be unavailable.")
	}
	if len(editableFields) == 0 {
		warnings = append(warnings, "No editable fields were found. Admin editing will be limited after creation.")
	}
	detection, err := detectImportedHTML(originalHTML)
	if err != nil {
		return emailHTMLInspection{}, errors.New("invalid email HTML")
	}
	warnings = append(warnings, importWarnings(detection)...)

	return emailHTMLInspection{
		ReviewBlockCount:         reviewBlockCount,
		OriginalReviewBlockCount: originalReviewBlockCount,
		EditableFieldCount:       len(editableFields),
		EditableFields:           inspectEditableFields(editableFields),
		Warnings:                 warnings,
		ReviewHTML:               reviewHTML,
		Detection:                detection,
	}, nil
}

func countReviewBlocks(htmlValue string) (int, error) {
	root, err := xhtml.Parse(strings.NewReader(htmlValue))
	if err != nil {
		return 0, err
	}

	return countReviewBlockNodes(root), nil
}

func countReviewBlockNodes(node *xhtml.Node) int {
	count := 0
	if node.Type == xhtml.ElementNode && htmlAttrValue(node, "data-review-block") != "" {
		count++
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		count += countReviewBlockNodes(child)
	}

	return count
}

func inspectEditableFields(fields emailedit.EditableFields) []editableFieldInspection {
	inspected := make([]editableFieldInspection, 0, len(fields))
	for key, field := range fields {
		inspected = append(inspected, editableFieldInspection{
			Key:          key,
			Type:         field.Type,
			ValuePreview: truncateInspectionValue(fmt.Sprintf("%v", field.Value)),
		})
	}
	sort.Slice(inspected, func(i int, j int) bool {
		firstOrder := fields[inspected[i].Key].Order
		secondOrder := fields[inspected[j].Key].Order
		if firstOrder != secondOrder {
			return firstOrder < secondOrder
		}

		return inspected[i].Key < inspected[j].Key
	})

	return inspected
}

func truncateInspectionValue(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= 80 {
		return value
	}

	return string(runes[:77]) + "..."
}

type emailDuplicateSource struct {
	Slug            string
	Sequence        string
	Title           string
	Subject         *string
	Preheader       *string
	SendTiming      *string
	SendCondition   *string
	Stage           string
	SortOrder       int
	Language        string
	Variant         string
	AdaptationKey   string
	AdaptationLabel string
	BodyText        *string
	OriginalHTML    string
	ReviewHTML      *string
	TemplateHTML    string
	TemplateHash    *string
	TemplateVersion *string
	EditableFields  []byte
}

type emailArchiveSnapshot struct {
	ID       string
	Slug     string
	Title    string
	Language string
	Variant  string
	Stage    string
}

func loadEmailForDuplicate(r *http.Request, dbpool *pgxpool.Pool, id string) (emailDuplicateSource, error) {
	var source emailDuplicateSource
	err := dbpool.QueryRow(r.Context(), `
		SELECT
			slug,
			sequence,
			title,
			subject,
			preheader,
			send_timing,
			send_condition,
			stage,
			sort_order,
			language,
			variant,
			adaptation_key,
			adaptation_label,
			body_text,
			original_html,
			review_html,
			template_html,
			template_hash,
			template_version,
			editable_fields
		FROM emails
		WHERE id = $1
			AND archived_at IS NULL;
	`, id).Scan(
		&source.Slug,
		&source.Sequence,
		&source.Title,
		&source.Subject,
		&source.Preheader,
		&source.SendTiming,
		&source.SendCondition,
		&source.Stage,
		&source.SortOrder,
		&source.Language,
		&source.Variant,
		&source.AdaptationKey,
		&source.AdaptationLabel,
		&source.BodyText,
		&source.OriginalHTML,
		&source.ReviewHTML,
		&source.TemplateHTML,
		&source.TemplateHash,
		&source.TemplateVersion,
		&source.EditableFields,
	)

	return source, err
}

type duplicateEmailInsert struct {
	Slug            string
	Sequence        string
	Title           string
	Subject         *string
	Preheader       *string
	SendTiming      *string
	SendCondition   *string
	Stage           string
	SortOrder       int
	Language        string
	Variant         string
	AdaptationKey   string
	AdaptationLabel string
	BodyText        *string
	ContentParts    []byte
	OriginalHTML    string
	ReviewHTML      *string
	TemplateHTML    string
	TemplateHash    *string
	TemplateVersion *string
	EditableFields  []byte
}

func insertDuplicatedEmail(r *http.Request, db emailEventExecutor, email duplicateEmailInsert) (EmailDetail, error) {
	var created EmailDetail
	var editableFields []byte

	err := db.QueryRow(r.Context(), `
		INSERT INTO emails (
			slug,
			sequence,
			title,
			subject,
			preheader,
			send_timing,
			stage,
			sort_order,
			language,
			variant,
			adaptation_key,
			adaptation_label,
			body_text,
			content_parts,
			original_html,
			review_html,
			template_html,
			template_hash,
			template_version,
			editable_fields,
			send_condition
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15, $16, $17, $18, $19, $20::jsonb, $21)
		RETURNING
			id,
			slug,
			sequence,
			title,
			subject,
			preheader,
			send_timing,
			send_condition,
			stage,
			sort_order,
			language,
			variant,
			adaptation_key,
			adaptation_label,
			review_status,
			owner_email,
			reviewer_email,
			due_date::text,
			implementation_notes,
			0 AS open_comment_count,
			0 AS open_blocking_comment_count,
			original_html,
			review_html,
			template_html,
			template_hash,
			template_version,
			editable_fields;
	`, email.Slug, email.Sequence, email.Title, email.Subject, email.Preheader, email.SendTiming, email.Stage, email.SortOrder, email.Language, email.Variant, email.AdaptationKey, email.AdaptationLabel, email.BodyText, email.ContentParts, email.OriginalHTML, email.ReviewHTML, email.TemplateHTML, email.TemplateHash, email.TemplateVersion, email.EditableFields, email.SendCondition).Scan(
		&created.ID,
		&created.Slug,
		&created.Sequence,
		&created.Title,
		&created.Subject,
		&created.Preheader,
		&created.SendTiming,
		&created.SendCondition,
		&created.Stage,
		&created.SortOrder,
		&created.Language,
		&created.Variant,
		&created.AdaptationKey,
		&created.AdaptationLabel,
		&created.ReviewStatus,
		&created.OwnerEmail,
		&created.ReviewerEmail,
		&created.DueDate,
		&created.ImplementationNotes,
		&created.OpenCommentCount,
		&created.OpenBlockingCommentCount,
		&created.OriginalHTML,
		&created.ReviewHTML,
		&created.TemplateHTML,
		&created.TemplateHash,
		&created.TemplateVersion,
		&editableFields,
	)
	if err != nil {
		return EmailDetail{}, err
	}

	created.EditableFields = json.RawMessage(editableFields)
	return created, nil
}

func buildEmailUpdateChanges(
	currentTitle string,
	currentSubject *string,
	currentPreheader *string,
	currentEditableFieldsJSON []byte,
	nextTitle string,
	nextSubject *string,
	nextPreheader *string,
	nextEditableFields emailedit.EditableFields,
) (map[string]any, error) {
	changes := map[string]any{}
	addStringChange(changes, "title", &currentTitle, &nextTitle)
	addStringChange(changes, "subject", currentSubject, nextSubject)
	addStringChange(changes, "preheader", currentPreheader, nextPreheader)

	currentFields := emailedit.EditableFields{}
	if len(currentEditableFieldsJSON) > 0 {
		if err := json.Unmarshal(currentEditableFieldsJSON, &currentFields); err != nil {
			return nil, err
		}
	}

	fieldChanges := map[string]any{}
	seen := map[string]bool{}
	for key, currentField := range currentFields {
		nextField, ok := nextEditableFields[key]
		seen[key] = true
		if !ok {
			fieldChanges[key] = map[string]any{
				"before": currentField,
				"after":  nil,
			}
			continue
		}
		if !reflect.DeepEqual(currentField, nextField) {
			fieldChanges[key] = map[string]any{
				"before": currentField,
				"after":  nextField,
			}
		}
	}
	for key, nextField := range nextEditableFields {
		if seen[key] {
			continue
		}
		fieldChanges[key] = map[string]any{
			"before": nil,
			"after":  nextField,
		}
	}
	if len(fieldChanges) > 0 {
		changes["editable_fields"] = fieldChanges
	}

	return changes, nil
}

func addStringChange(changes map[string]any, key string, before *string, after *string) {
	if stringFromPointer(before) == stringFromPointer(after) {
		return
	}
	changes[key] = map[string]any{
		"before": before,
		"after":  after,
	}
}

func normalizedOptionalDate(value *string) (*string, error) {
	trimmed := strings.TrimSpace(stringFromPointer(value))
	if trimmed == "" {
		return nil, nil
	}
	if _, err := time.Parse("2006-01-02", trimmed); err != nil {
		return nil, err
	}
	return &trimmed, nil
}

func mergeEditableFields(
	extractedFields emailedit.EditableFields,
	submittedFields emailedit.EditableFields,
) emailedit.EditableFields {
	mergedFields := emailedit.EditableFields{}
	for key, extractedField := range extractedFields {
		if submittedField, ok := submittedFields[key]; ok && submittedField.Type == extractedField.Type {
			if shouldUseExtractedTextField(extractedField, submittedField) {
				mergedFields[key] = extractedField
				continue
			}
			mergedFields[key] = submittedField
			continue
		}
		mergedFields[key] = extractedField
	}

	return mergedFields
}

func shouldUseExtractedTextField(extractedField emailedit.EditableField, submittedField emailedit.EditableField) bool {
	if extractedField.Type != emailedit.FieldTypeText || submittedField.Type != emailedit.FieldTypeText {
		return false
	}

	extractedValue, extractedOK := extractedField.Value.(string)
	submittedValue, submittedOK := submittedField.Value.(string)
	if !extractedOK || !submittedOK || extractedValue == submittedValue {
		return false
	}

	return canonicalEditableText(extractedValue) == canonicalEditableText(submittedValue)
}

func canonicalEditableText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func changedCommentAnchorTexts(
	templateHTML string,
	renderedHTML string,
	currentTitle string,
	currentSubject *string,
	currentPreheader *string,
	currentEditableFieldsJSON []byte,
	nextTitle string,
	nextSubject *string,
	nextPreheader *string,
	nextEditableFields emailedit.EditableFields,
) (map[string]string, error) {
	currentFields := emailedit.EditableFields{}
	if len(currentEditableFieldsJSON) > 0 {
		if err := json.Unmarshal(currentEditableFieldsJSON, &currentFields); err != nil {
			return nil, err
		}
	}

	textFieldBlocks, err := editableTextFieldReviewBlocks(templateHTML)
	if err != nil {
		return nil, err
	}

	changedBlocks := map[string]bool{}
	for key, reviewBlock := range textFieldBlocks {
		currentField, currentOK := currentFields[key]
		nextField, nextOK := nextEditableFields[key]
		if !nextOK || nextField.Type != emailedit.FieldTypeText {
			continue
		}
		if !currentOK || currentField.Type != emailedit.FieldTypeText || !reflect.DeepEqual(currentField.Value, nextField.Value) {
			changedBlocks[reviewBlock] = true
		}
	}

	anchorTexts := map[string]string{}
	if len(changedBlocks) > 0 {
		renderedBlockTexts, err := reviewBlockTexts(renderedHTML, changedBlocks)
		if err != nil {
			return nil, err
		}
		for reviewBlock := range changedBlocks {
			if text, ok := renderedBlockTexts[reviewBlock]; ok {
				anchorTexts[reviewBlock] = text
			}
		}
	}

	if stringFromPointer(currentSubject) != stringFromPointer(nextSubject) {
		anchorTexts["subject"] = stringFromPointer(nextSubject)
	} else if currentSubject == nil && nextSubject == nil && currentTitle != nextTitle {
		anchorTexts["subject"] = nextTitle
	}
	if stringFromPointer(currentPreheader) != stringFromPointer(nextPreheader) {
		anchorTexts["preheader"] = stringFromPointer(nextPreheader)
	}

	return anchorTexts, nil
}

func changedReviewBlocksForEmailUpdate(
	currentTemplateHTML string,
	nextTemplateHTML string,
	changes map[string]any,
) ([]string, error) {
	changedBlocks := map[string]bool{}
	if _, ok := changes["subject"]; ok {
		changedBlocks["subject"] = true
	}
	if _, ok := changes["preheader"]; ok {
		changedBlocks["preheader"] = true
	}

	editableFieldChanges, ok := changes["editable_fields"].(map[string]any)
	if ok && len(editableFieldChanges) > 0 {
		currentFieldBlocks, err := editableFieldReviewBlocks(currentTemplateHTML)
		if err != nil {
			return nil, err
		}
		nextFieldBlocks, err := editableFieldReviewBlocks(nextTemplateHTML)
		if err != nil {
			return nil, err
		}
		for field := range editableFieldChanges {
			if reviewBlock := nextFieldBlocks[field]; reviewBlock != "" {
				changedBlocks[reviewBlock] = true
				continue
			}
			if reviewBlock := currentFieldBlocks[field]; reviewBlock != "" {
				changedBlocks[reviewBlock] = true
			}
		}
	}

	blocks := make([]string, 0, len(changedBlocks))
	for reviewBlock := range changedBlocks {
		blocks = append(blocks, reviewBlock)
	}
	sort.Strings(blocks)
	return blocks, nil
}

func resetCommentAnchorsForReviewBlocks(
	ctx context.Context,
	db emailEventExecutor,
	emailID string,
	anchorTexts map[string]string,
) error {
	for reviewBlock, text := range anchorTexts {
		_, err := db.Exec(ctx, `
			UPDATE comments
			SET selected_text = $3,
				start_offset = 0,
				end_offset = $4
			WHERE email_id = $1
				AND review_block = $2;
		`, emailID, reviewBlock, text, jsStringLength(text))
		if err != nil {
			return err
		}
	}

	return nil
}

func editableTextFieldReviewBlocks(templateHTML string) (map[string]string, error) {
	root, err := xhtml.Parse(strings.NewReader(templateHTML))
	if err != nil {
		return nil, err
	}

	blocks := map[string]string{}
	collectEditableTextFieldReviewBlocks(root, "", blocks)
	return blocks, nil
}

func editableFieldReviewBlocks(templateHTML string) (map[string]string, error) {
	root, err := xhtml.Parse(strings.NewReader(templateHTML))
	if err != nil {
		return nil, err
	}

	blocks := map[string]string{}
	collectEditableFieldReviewBlocks(root, "", blocks)
	return blocks, nil
}

func collectEditableTextFieldReviewBlocks(node *xhtml.Node, currentReviewBlock string, blocks map[string]string) {
	if node.Type == xhtml.ElementNode {
		if reviewBlock := htmlAttrValue(node, "data-review-block"); reviewBlock != "" {
			currentReviewBlock = reviewBlock
		}
		if key := htmlAttrValue(node, "data-edit-text"); key != "" && currentReviewBlock != "" {
			blocks[key] = currentReviewBlock
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectEditableTextFieldReviewBlocks(child, currentReviewBlock, blocks)
	}
}

func collectEditableFieldReviewBlocks(node *xhtml.Node, currentReviewBlock string, blocks map[string]string) {
	if node.Type == xhtml.ElementNode {
		if reviewBlock := htmlAttrValue(node, "data-review-block"); reviewBlock != "" {
			currentReviewBlock = reviewBlock
		}
		for _, attr := range node.Attr {
			if strings.HasPrefix(strings.ToLower(attr.Key), "data-edit-") && strings.TrimSpace(attr.Val) != "" && currentReviewBlock != "" {
				blocks[strings.TrimSpace(attr.Val)] = currentReviewBlock
			}
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectEditableFieldReviewBlocks(child, currentReviewBlock, blocks)
	}
}

func reviewBlockTexts(renderedHTML string, targetBlocks map[string]bool) (map[string]string, error) {
	root, err := xhtml.Parse(strings.NewReader(renderedHTML))
	if err != nil {
		return nil, err
	}

	texts := map[string]string{}
	collectReviewBlockTexts(root, targetBlocks, texts)
	return texts, nil
}

func collectReviewBlockTexts(node *xhtml.Node, targetBlocks map[string]bool, texts map[string]string) {
	if node.Type == xhtml.ElementNode {
		reviewBlock := htmlAttrValue(node, "data-review-block")
		if targetBlocks[reviewBlock] {
			texts[reviewBlock] = strings.TrimSpace(htmlTextContent(node))
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectReviewBlockTexts(child, targetBlocks, texts)
	}
}

func htmlAttrValue(node *xhtml.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}

	return ""
}

func htmlTextContent(node *xhtml.Node) string {
	if node.Type == xhtml.TextNode {
		return node.Data
	}

	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		builder.WriteString(htmlTextContent(child))
	}
	return builder.String()
}

func jsStringLength(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func duplicateEmailSlug(sourceSlug string, sourceLanguage string, sourceVariant string, targetLanguage string, targetVariant string) string {
	base := strings.Split(sourceSlug, "--")[0]
	sourceVariant = normalizeAdaptationKey(sourceVariant)
	if sourceVariant != "" && sourceVariant != "new" {
		base = strings.TrimSuffix(base, "-"+sourceVariant)
	}
	sourceLanguage = strings.ToLower(strings.TrimSpace(sourceLanguage))
	if sourceLanguage != "" {
		base = strings.TrimSuffix(base, "-"+sourceLanguage)
	}

	slug := base + "-" + targetLanguage
	if targetVariant != "new" {
		slug += "-" + normalizeAdaptationKey(targetVariant)
	}

	return slug
}

func adaptationEmailSlug(sourceSlug string, sourceAdaptationKey string, targetAdaptationKey string) string {
	base := strings.TrimSuffix(sourceSlug, "--"+sourceAdaptationKey)
	return base + "--" + targetAdaptationKey
}

func emailSlugWithAdaptation(baseSlug string, adaptationKey string) string {
	if adaptationKey == "" || adaptationKey == "default" {
		return baseSlug
	}

	return baseSlug + "--" + adaptationKey
}

func normalizeEmailAdaptation(label *string) (string, string, error) {
	trimmed := strings.TrimSpace(stringFromPointer(label))
	if trimmed == "" || strings.EqualFold(trimmed, "default") {
		return "default", "Default", nil
	}

	key := normalizeAdaptationKey(trimmed)
	if key == "" {
		return "", "", errors.New("invalid adaptation label")
	}

	return key, trimmed, nil
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

func newEmailSlug(sequence string, stage string, title string, language string, variant string) string {
	parts := []string{
		normalizeAdaptationKey(sequence),
		normalizeAdaptationKey(stage),
		normalizeAdaptationKey(title),
		strings.ToLower(strings.TrimSpace(language)),
	}
	if variant != "new" {
		parts = append(parts, normalizeAdaptationKey(variant))
	}

	filteredParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}
	if len(filteredParts) == 0 {
		return "email"
	}

	return strings.Join(filteredParts, "-")
}

func archivedEmailSlug(slug string, id string) string {
	return fmt.Sprintf("%s--archived-%s", slug, id)
}

func trimmedOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func isEmailCreationConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		(pgErr.ConstraintName == "emails_slug_unique" || pgErr.ConstraintName == "emails_active_adaptation_unique")
}

func stringFromPointer(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
