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
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
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
	r.Post("/api/emails/bulk", bulkEmailHandler(dbpool))
	r.Post("/api/slots", createSlotHandler(dbpool))
	r.Patch("/api/slots", updateSlotHandler(dbpool))
	r.Post("/api/slots/move", moveSlotHandler(dbpool))
	r.Get("/api/emails/{id}", getEmailHandler(dbpool))
	r.Get("/api/emails/{id}/activity", listEmailActivityHandler(dbpool))
	r.Get("/api/emails/{id}/area-approvals", listEmailAreaApprovalsHandler(dbpool))
	r.Get("/api/emails/{id}/rendered", getRenderedEmailHandler(dbpool))
	r.Put("/api/emails/{id}/layout", updateEmailLayoutHandler(dbpool))
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
	r.Patch("/api/emails/{id}/restore", restoreEmailHandler(dbpool))
	r.Patch("/api/emails/{id}/live", markEmailLiveHandler(dbpool))
	r.Patch("/api/emails/{id}/uploaded", markEmailUploadedHandler(dbpool))
	r.Patch("/api/emails/{id}/take", takeEmailHandler(dbpool))
	r.Get("/api/emails/{id}/translation", translationHandler(dbpool))
	r.Post("/api/emails/{id}/translation/confirm", confirmTranslationHandler(dbpool))
	r.Patch("/api/emails/{id}/translation-link", translationLinkHandler(dbpool))
	r.Get("/api/emails/{id}/based-on", basedOnHandler(dbpool))
	r.Post("/api/emails/{id}/based-on/confirm", confirmBasedOnHandler(dbpool))
	r.Patch("/api/emails/{id}/based-on", basedOnLinkHandler(dbpool))
	r.Get("/api/reference-differences", referenceDifferencesHandler(dbpool))
	r.Patch("/api/emails/{id}/place", placeEmailHandler(dbpool))
	r.Get("/api/admin/email-events", listEmailEventsHandler(dbpool))
	r.Get("/api/shared-texts", listSharedTextsHandler(dbpool))
	r.Put("/api/shared-texts/{key}/{language}", updateSharedTextHandler(dbpool))
	r.Post("/api/shared-texts/{key}/{language}", createBoardSharedTextHandler(dbpool))
	r.Delete("/api/shared-texts/{key}/{language}", deleteBoardSharedTextHandler(dbpool))
	r.Get("/api/blocks", listEmailBlocksHandler(dbpool))
	r.Get("/api/block-catalog", listBlockCatalogHandler(dbpool))
	r.Get("/api/default-layout", defaultLayoutHandler(dbpool))
	r.Put("/api/emails/{id}/region", updateEmailRegionHandler(dbpool))
	r.Post("/api/layout-preview", layoutPreviewHandler(dbpool))
	r.Put("/api/blocks/{key}", updateEmailBlockHandler(dbpool))
	r.Post("/api/blocks/{key}/preview", previewEmailBlockHandler(dbpool))
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
		if user, ok := auth.FromRequest(r); ok {
			visible, scoped, err := auth.ScopedBoards(r.Context(), dbpool, user)
			if err != nil {
				http.Error(w, "failed to load emails", http.StatusInternalServerError)
				return
			}
			if scoped {
				args = append(args, visible)
				boardFilter += fmt.Sprintf(" AND sequence = ANY($%d)", len(args))
			}
		}
		// ?archived=only lists the board's history instead of its active
		// emails, newest archive first.
		archivedFilter := "archived_at IS NULL"
		order := "sort_order, created_at"
		if r.URL.Query().Get("archived") == "only" {
			archivedFilter = "archived_at IS NOT NULL"
			order = "archived_at DESC, sort_order"
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
				region_key,
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
				COALESCE(translation.stale, false),
				based_on,
				COALESCE(reference.stale, false),
				(SELECT max(version_number) FROM email_versions WHERE email_versions.email_id = emails.id) AS latest_version,
				uploads.uploaded_version,
				uploads.uploaded_at,
				uploads.robots,
				archived_at,
				(SELECT email FROM users WHERE users.id = emails.archived_by) AS archived_by_email
			FROM emails
			LEFT JOIN email_upload_summary uploads ON uploads.email_id = emails.id
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
				-- Fields shared in the translation have their own translation.
				SELECT email_text_hash(fv.subject, fv.preheader, fv.editable_fields, emails.editable_fields)
					<> email_text_hash(lv.subject, lv.preheader, lv.editable_fields, emails.editable_fields) AS stale
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
			LEFT JOIN LATERAL (
				-- Same for a region's email and the reference email it adapts.
				SELECT email_text_hash(fv.subject, fv.preheader, fv.editable_fields, emails.editable_fields)
					<> email_text_hash(lv.subject, lv.preheader, lv.editable_fields, emails.editable_fields) AS stale
				FROM email_versions fv
				JOIN LATERAL (
					SELECT subject, preheader, editable_fields FROM email_versions
					WHERE email_id = emails.based_on
					ORDER BY version_number DESC LIMIT 1
				) lv ON true
				WHERE emails.based_on IS NOT NULL
					AND fv.email_id = emails.based_on
					AND fv.version_number = emails.based_on_version
			) reference ON true
			WHERE `+archivedFilter+`
			`+boardFilter+`
			ORDER BY `+order+`;
		`, args...)
		if err != nil {
			http.Error(w, "failed to load emails", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		emails := []EmailListItem{}

		for rows.Next() {
			var email EmailListItem
			var uploadRobots []byte

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
				&email.RegionKey,
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
				&email.BasedOn,
				&email.BasedOnStale,
				&email.LatestVersion,
				&email.UploadedVersion,
				&email.UploadedAt,
				&uploadRobots,
				&email.ArchivedAt,
				&email.ArchivedByEmail,
			)
			if err == nil {
				err = json.Unmarshal(uploadRobots, &email.UploadRobots)
			}
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
		var layout []byte
		var uploadRobots []byte

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
				region_key,
				translation_of,
				based_on,
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
				(SELECT max(version_number) FROM email_versions WHERE email_versions.email_id = emails.id) AS latest_version,
				uploads.uploaded_version,
				uploads.uploaded_at,
				uploads.robots,
				archived_at,
				(SELECT email FROM users WHERE users.id = emails.archived_by) AS archived_by_email,
				original_html,
				review_html,
				template_html,
				template_hash,
				template_version,
				editable_fields,
				layout
			FROM emails
			LEFT JOIN email_upload_summary uploads ON uploads.email_id = emails.id
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
			&email.RegionKey,
			&email.TranslationOf,
			&email.BasedOn,
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
			&email.LatestVersion,
			&email.UploadedVersion,
			&email.UploadedAt,
			&uploadRobots,
			&email.ArchivedAt,
			&email.ArchivedByEmail,
			&email.OriginalHTML,
			&email.ReviewHTML,
			&email.TemplateHTML,
			&email.TemplateHash,
			&email.TemplateVersion,
			&editableFields,
			&layout,
		)

		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get email %s: %v\n", id, err)
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}

		email.EditableFields = json.RawMessage(editableFields)
		if err := json.Unmarshal(uploadRobots, &email.UploadRobots); err != nil {
			http.Error(w, "invalid upload marks", http.StatusInternalServerError)
			return
		}
		if email.Layout, err = parseLayout(layout); err != nil {
			http.Error(w, "invalid email layout", http.StatusInternalServerError)
			return
		}
		if email.Checks, err = checkStoredEmail(r.Context(), dbpool, layout, editableFields, email.Subject, email.Preheader); err != nil {
			fmt.Fprintf(os.Stderr, "failed to check email %s: %v\n", id, err)
			http.Error(w, "failed to check the email", http.StatusInternalServerError)
			return
		}

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
	// RegionKey puts the email in a region of the board; its adaptation
	// then follows the region and adaptation_label is ignored.
	RegionKey    *string `json:"region_key"`
	OriginalHTML string  `json:"original_html"`
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
		var request createEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !requireManageBoard(w, r, dbpool, user, strings.TrimSpace(stringFromPointer(request.Sequence))) {
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
			RegionKey:       strings.TrimSpace(stringFromPointer(request.RegionKey)),
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
	// RegionKey is a region of the board to put the email in; "" for none.
	RegionKey    string
	OriginalHTML string
	// Layout builds the email from the block library instead of
	// OriginalHTML: shared fields take the texts of its language and board,
	// the others the text written in the block.
	Layout emailblocks.Layout
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
	if params.Layout != nil && originalHTML != "" {
		return EmailDetail{}, createEmailInputError("give either blocks or HTML, not both")
	}

	if title == "" || stage == "" || (originalHTML == "" && params.Layout == nil) {
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
	var built *emailFromLayout
	if params.Layout != nil {
		if built, err = buildNewEmailFromLayout(r.Context(), tx, params.Layout, language, sequence, subject, preheader); err != nil {
			return EmailDetail{}, err
		}
		originalHTML = built.template
	}
	renderedHTML := originalHTML
	if built != nil {
		renderedHTML = built.rendered
	}
	reviewHTML, err := emailreview.AddReviewBlocks(renderedHTML)
	if err != nil {
		return EmailDetail{}, createEmailInputError("invalid email HTML")
	}
	editableFields, err := emailedit.ExtractEditableFields(originalHTML)
	if err != nil {
		return EmailDetail{}, createEmailInputError("invalid editable fields in HTML")
	}
	var layoutJSONValue []byte
	if built != nil {
		editableFields = built.fields
		if layoutJSONValue, err = layoutJSON(params.Layout); err != nil {
			return EmailDetail{}, err
		}
	}
	editableFieldsJSON, err := editableFields.JSON()
	if err != nil {
		return EmailDetail{}, createEmailInputError("invalid editable fields in HTML")
	}
	contentParts := emailtext.ExtractContentParts(
		renderedHTML,
		stringFromPointer(subject),
		stringFromPointer(preheader),
		"",
	)
	contentPartsJSON, err := json.Marshal(contentParts)
	if err != nil {
		return EmailDetail{}, err
	}
	if params.RegionKey != "" {
		var found bool
		adaptationKey, adaptationLabel, found, err = boards.RegionAdaptation(r.Context(), tx, sequence, params.RegionKey)
		if err != nil {
			return EmailDetail{}, fmt.Errorf("load region: %w", err)
		}
		if !found {
			return EmailDetail{}, createEmailInputError("the board has no region " + params.RegionKey)
		}
	}
	slug := emailSlugWithAdaptation(
		newEmailSlug(sequence, stage, title, language, variant),
		adaptationKey,
	)

	created, err := insertDuplicatedEmail(r.Context(), tx, duplicateEmailInsert{
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
		Layout:          layoutJSONValue,
	})
	if err != nil {
		if isEmailCreationConflict(err) {
			return EmailDetail{}, errCreateEmailConflict
		}
		return EmailDetail{}, err
	}
	if params.RegionKey != "" {
		if _, err := tx.Exec(r.Context(), `UPDATE emails SET region_key = $2 WHERE id = $1;`, created.ID, params.RegionKey); err != nil {
			return EmailDetail{}, fmt.Errorf("set region: %w", err)
		}
		if err := linkToSlotReference(r.Context(), tx, created.ID); err != nil {
			return EmailDetail{}, fmt.Errorf("link reference: %w", err)
		}
		created.RegionKey = &params.RegionKey
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
		EditableFieldsJSON: created.EditableFields,
	}); err != nil {
		return EmailDetail{}, fmt.Errorf("record email version: %w", err)
	}

	return created, nil
}

func inspectEmailHTMLHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if manages, err := auth.ManagesAnyBoard(r.Context(), dbpool, user); err != nil || !manages {
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
	// Version is the latest saved version, read together with the fields
	// so a copy of the HTML can be matched to the version it came from.
	Version *int `json:"version"`
	// Subject and ReviewStatus are read in the same query, so the copy
	// dialog shows what was actually copied, not a stale list entry.
	Subject      string `json:"subject"`
	ReviewStatus string `json:"review_status"`
}

func getRenderedEmailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var templateHTML string
		var preheader *string
		var editableFieldsJSON []byte
		var version *int
		var subject, reviewStatus string
		err := dbpool.QueryRow(r.Context(), `
			SELECT template_html, preheader, editable_fields,
				(SELECT max(version_number) FROM email_versions WHERE email_id = emails.id),
				COALESCE(subject, ''), review_status
			FROM emails
			WHERE id = $1
				AND archived_at IS NULL;
		`, id).Scan(&templateHTML, &preheader, &editableFieldsJSON, &version, &subject, &reviewStatus)
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
			Subject:   subject,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to render email %s: %v\n", id, err)
			http.Error(w, "failed to render email", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(renderedEmailResponse{HTML: html, Version: version, Subject: subject, ReviewStatus: reviewStatus})
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
		if canEdit, err := CanEditEmail(r.Context(), dbpool, user, id); err != nil || !canEdit {
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

		_, err = saveEmailContent(r.Context(), tx, user, id, emailContentUpdate{
			Title:          request.Title,
			Subject:        request.Subject,
			Preheader:      request.Preheader,
			EditableFields: request.EditableFields,
			OriginalHTML:   request.OriginalHTML,
		}, emailContentSaveOptions{Source: "manual", Check: true})
		var contentErr emailContentError
		if errors.As(err, &contentErr) {
			http.Error(w, contentErr.message, contentErr.status)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to update editable fields for %s: %v\n", id, err)
			http.Error(w, "failed to update editable fields", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update editable fields", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// emailContentUpdate is a change to an email's text; nil keeps the current
// value.
type emailContentUpdate struct {
	Title          *string
	Subject        *string
	Preheader      *string
	EditableFields emailedit.EditableFields
	OriginalHTML   *string
	// Layout switches the email to these blocks. An email built from blocks
	// is assembled from the library on every save, so a save with nil
	// picks up changed blocks.
	Layout emailblocks.Layout
}

type emailContentSaveOptions struct {
	// Source is the version source: "manual", "shared_text" or "block".
	Source string
	// KeepStatus leaves a ready email ready. A shared text is checked on its
	// own screen, so its edit does not send every email back to draft.
	KeepStatus bool
	// Metadata goes into the update event.
	Metadata map[string]any
	// SameText is a save that changes the markup only (a switch to
	// blocks): the blocks are not marked as changed, so comments on them
	// stay current.
	SameText bool
	// Check refuses a save that brings in a new error of form (no button,
	// an empty required field). It is for edits people make on the email
	// itself; saves that follow another change (a translation taking the EN
	// blocks, a shared text, a block) go through.
	Check bool
}

// emailContentError is a save refused because of the input, with the HTTP
// status to answer with.
type emailContentError struct {
	status  int
	message string
}

func (e emailContentError) Error() string { return e.message }

// saveEmailContent writes new text into an active email inside tx: renders
// the HTML, records a version and an event when something changed, resets
// comment anchors on changed blocks and marks approvals stale. Shared fields
// take their value from the shared text in the email's language and board.
// An email built from blocks gets its template from the library; fields it
// does not get in update keep their value. It reports whether anything
// changed.
func saveEmailContent(ctx context.Context, tx pgx.Tx, user AuthUser, id string, update emailContentUpdate, opts emailContentSaveOptions) (bool, error) {
	var originalHTML string
	var templateHTML string
	var currentTitle string
	var currentSubject *string
	var currentPreheader *string
	var currentEditableFieldsJSON []byte
	var slug string
	var language string
	var board string
	var currentReviewStatus string
	var currentLayoutJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT slug, lower(language), coalesce(sequence, ''), original_html, template_html, title, subject, preheader, editable_fields, review_status, layout
		FROM emails
		WHERE id = $1
			AND archived_at IS NULL
		FOR UPDATE;
	`, id).Scan(&slug, &language, &board, &originalHTML, &templateHTML, &currentTitle, &currentSubject, &currentPreheader, &currentEditableFieldsJSON, &currentReviewStatus, &currentLayoutJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, emailContentError{http.StatusNotFound, "email not found"}
	}
	if err != nil {
		return false, err
	}
	currentLayout, err := parseLayout(currentLayoutJSON)
	if err != nil {
		return false, err
	}
	nextLayout := currentLayout
	if update.Layout != nil {
		nextLayout = update.Layout
	}
	if nextLayout != nil && update.OriginalHTML != nil {
		return false, emailContentError{http.StatusBadRequest, "the email is built from blocks: change its blocks, not its HTML"}
	}

	title := currentTitle
	if update.Title != nil {
		title = strings.TrimSpace(*update.Title)
	}
	if title == "" {
		return false, emailContentError{http.StatusBadRequest, "title is required"}
	}

	subject := currentSubject
	if update.Subject != nil {
		subject = update.Subject
	}
	preheader := currentPreheader
	if update.Preheader != nil {
		preheader = update.Preheader
	}

	nextOriginalHTML := originalHTML
	nextTemplateHTML := templateHTML
	nextEditableFields := update.EditableFields
	originalHTMLChanged := false
	if update.OriginalHTML != nil {
		nextOriginalHTML = strings.TrimSpace(*update.OriginalHTML)
		if nextOriginalHTML == "" {
			return false, emailContentError{http.StatusBadRequest, "original_html is required"}
		}
		if _, err := emailreview.AddReviewBlocks(nextOriginalHTML); err != nil {
			return false, emailContentError{http.StatusBadRequest, "invalid email HTML"}
		}
		extractedFields, err := emailedit.ExtractEditableFields(nextOriginalHTML)
		if err != nil {
			return false, emailContentError{http.StatusBadRequest, "invalid editable fields in original_html"}
		}
		nextEditableFields = mergeEditableFields(extractedFields, update.EditableFields)
		nextTemplateHTML = nextOriginalHTML
		originalHTMLChanged = nextOriginalHTML != originalHTML
	}
	if nextLayout != nil {
		var currentFields emailedit.EditableFields
		if err := json.Unmarshal(currentEditableFieldsJSON, &currentFields); err != nil {
			return false, err
		}
		for key, field := range update.EditableFields {
			currentFields[key] = field
		}
		nextTemplateHTML, nextEditableFields, err = buildFromLayout(ctx, tx, nextLayout, language, currentFields)
		if err != nil {
			return false, err
		}
		// The assembled template is the email's HTML; there is no other.
		nextOriginalHTML = nextTemplateHTML
		originalHTMLChanged = nextTemplateHTML != templateHTML || nextOriginalHTML != originalHTML
	}
	layoutChanged := !sameLayout(currentLayout, nextLayout)
	nextLayoutJSON, err := layoutJSON(nextLayout)
	if err != nil {
		return false, err
	}
	nextEditableFields, err = resolveSharedFields(ctx, tx, language, board, nextEditableFields)
	if err != nil {
		return false, err
	}

	if update.OriginalHTML == nil {
		previousFields := emailedit.EditableFields{}
		if len(currentEditableFieldsJSON) > 0 {
			if err := json.Unmarshal(currentEditableFieldsJSON, &previousFields); err != nil {
				return false, err
			}
		}
		if err := emailedit.FormatEditedFields(nextTemplateHTML, previousFields, nextEditableFields); err != nil {
			return false, emailContentError{http.StatusBadRequest, "invalid email HTML"}
		}
	}
	for key, field := range nextEditableFields {
		if err := emailedit.ValidateFormat(key, field); err != nil {
			return false, emailContentError{http.StatusBadRequest, err.Error()}
		}
	}
	if opts.Check {
		before, err := checkStoredEmail(ctx, tx, currentLayoutJSON, currentEditableFieldsJSON, currentSubject, currentPreheader)
		if err != nil {
			return false, err
		}
		after, err := checkEmail(ctx, tx, nil, nextLayout, nextEditableFields, subject, preheader)
		if err != nil {
			return false, err
		}
		if added := emailblocks.NewErrors(before, after); len(added) > 0 {
			return false, emailContentError{http.StatusBadRequest, issueMessages(added)}
		}
	}

	renderedHTML, err := emailedit.RenderEditableHTMLWithMetadata(nextTemplateHTML, nextEditableFields, emailedit.RenderMetadata{
		Preheader: stringFromPointer(preheader),
		Subject:   stringFromPointer(subject),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid editable fields for email %s: %v\n", id, err)
		return false, emailContentError{http.StatusBadRequest, "invalid editable fields"}
	}

	reviewHTML, err := emailreview.AddReviewBlocks(renderedHTML)
	if err != nil {
		return false, emailContentError{http.StatusBadRequest, "invalid email HTML"}
	}

	fieldsJSON, err := nextEditableFields.JSON()
	if err != nil {
		return false, emailContentError{http.StatusBadRequest, "invalid editable fields"}
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
		return false, err
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
		return false, err
	}
	if originalHTMLChanged && nextLayout == nil {
		changes["original_html"] = map[string]any{
			"before": originalHTML,
			"after":  nextOriginalHTML,
		}
	}
	if layoutChanged {
		changes["layout"] = layoutChange(currentLayout, nextLayout)
	}
	changedReviewBlocks, err := changedReviewBlocksForEmailUpdate(templateHTML, nextTemplateHTML, changes)
	if err != nil {
		return false, err
	}
	nextReviewStatus := currentReviewStatus
	approvalBecameStale := len(changes) > 0 && isApprovedEmailReviewStatus(currentReviewStatus) && !opts.KeepStatus
	if approvalBecameStale {
		nextReviewStatus = "draft"
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
		return false, err
	}

	if _, err := tx.Exec(ctx, `
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
			layout = $12::jsonb,
			updated_at = now()
		WHERE id = $1
			AND archived_at IS NULL;
	`, id, title, subject, preheader, fieldsJSON, contentPartsJSON, reviewHTML, renderedBodyText, nextOriginalHTML, nextTemplateHTML, nextReviewStatus, nextLayoutJSON); err != nil {
		return false, err
	}
	changed := len(changes) > 0 || originalHTMLChanged
	if changed {
		if err := insertEmailVersion(ctx, tx, emailVersionSnapshot{
			EmailID:              id,
			CreatedByUserID:      user.ID,
			CreatedByEmail:       user.Email,
			Source:               opts.Source,
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
			return false, fmt.Errorf("record email version: %w", err)
		}
	}
	if err := resetCommentAnchorsForReviewBlocks(ctx, tx, id, commentAnchorTexts); err != nil {
		return false, err
	}
	updateEventMetadata := map[string]any{}
	for key, value := range opts.Metadata {
		updateEventMetadata[key] = value
	}
	if len(changedReviewBlocks) > 0 && !opts.SameText {
		updateEventMetadata["changed_review_blocks"] = changedReviewBlocks
	}
	if err := InsertEmailEvent(ctx, tx, EmailEventParam{
		ActorUserID: user.ID,
		ActorEmail:  user.Email,
		Action:      emailEventUpdated,
		EmailID:     &id,
		EmailSlug:   &slug,
		EmailTitle:  &title,
		Metadata:    updateEventMetadata,
		Changes:     changes,
	}); err != nil {
		return false, fmt.Errorf("record email update event: %w", err)
	}
	if changed {
		if err := markStaleAreaApprovalsAfterContentChange(
			ctx,
			tx,
			id,
			user,
			"approval_stale_after_edit",
		); err != nil {
			return false, fmt.Errorf("mark stale area approvals: %w", err)
		}
	}
	if approvalBecameStale {
		if err := InsertEmailEvent(ctx, tx, EmailEventParam{
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
			return false, fmt.Errorf("record stale approval event: %w", err)
		}
	}
	return changed, nil
}

func updateEmailPlanningFieldsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !requireManageEmail(w, r, dbpool, user, id) {
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
		if !requireManageEmail(w, r, dbpool, user, id) {
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

		var blocked emailStatusBlockedError
		err = setEmailReviewStatus(r.Context(), tx, user, id, nextStatus)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			http.Error(w, "email not found", http.StatusNotFound)
			return
		case errors.As(err, &blocked):
			http.Error(w, blocked.Error(), http.StatusConflict)
			return
		case err != nil:
			fmt.Fprintf(os.Stderr, "failed to update review status for %s: %v\n", id, err)
			http.Error(w, "failed to update review status", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update review status", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updateEmailReviewStatusResponse{ReviewStatus: nextStatus})
	}
}

// emailStatusBlockedError lists why an email cannot be marked ready yet.
type emailStatusBlockedError []string

func (e emailStatusBlockedError) Error() string { return strings.Join(e, "; ") }

// setEmailReviewStatus changes the status of one email inside tx and records
// it in the history. Marking ready goes through the approval gates and the
// checks of form, warnings included; a missing or archived email returns
// pgx.ErrNoRows.
func setEmailReviewStatus(ctx context.Context, tx pgx.Tx, user AuthUser, id string, nextStatus string) error {
	var slug string
	var title string
	var currentStatus string
	var currentSubject *string
	var currentPreheader *string
	var currentTemplateHash *string
	var currentTemplateHTML string
	var currentEditableFieldsText string
	var currentLayoutJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT slug, title, review_status, subject, preheader, template_hash, template_html, editable_fields::text, layout
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
		&currentLayoutJSON,
	)
	if err != nil {
		return err
	}
	if currentStatus == nextStatus {
		return nil
	}

	eventMetadata := map[string]any{}
	if isApprovedEmailReviewStatus(nextStatus) {
		blockers, err := emailApprovalGateBlockers(ctx, tx, id)
		if err != nil {
			return err
		}
		issues, err := checkStoredEmail(ctx, tx, currentLayoutJSON, []byte(currentEditableFieldsText), currentSubject, currentPreheader)
		if err != nil {
			return err
		}
		for _, issue := range issues {
			blockers = append(blockers, issue.Message)
		}
		if len(blockers) > 0 {
			return emailStatusBlockedError(blockers)
		}
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
	if currentStatus == "draft" && isApprovedEmailReviewStatus(nextStatus) {
		reapprovesStaleEdit, err := latestReviewStatusEventMarkedApprovalStale(ctx, tx, id)
		if err != nil {
			return err
		}
		if reapprovesStaleEdit {
			eventMetadata["reason"] = "reapproved_after_stale_edit"
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE emails
		SET review_status = $2,
			updated_at = now()
		WHERE id = $1;
	`, id, nextStatus); err != nil {
		return err
	}

	return InsertEmailEvent(ctx, tx, EmailEventParam{
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
	})
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
		blockers = append(blockers, "resolve blocking comments before marking the email ready")
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
	case "draft", "ready":
		return true
	default:
		return false
	}
}

func isApprovedEmailReviewStatus(status string) bool {
	return status == "ready"
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
	// RegionKey puts the copy in a region of the target board; its
	// adaptation then follows the region and adaptation_label is ignored.
	RegionKey *string `json:"region_key"`
	Title     *string `json:"title"`
	Subject   *string `json:"subject"`
	Preheader *string `json:"preheader"`
}

func duplicateEmailAsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !requireManageEmail(w, r, dbpool, user, id) {
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
		if !requireManageBoard(w, r, dbpool, user, sequence) {
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
		regionKey := ""
		if request.RegionKey != nil && strings.TrimSpace(*request.RegionKey) != "" {
			regionKey = strings.TrimSpace(*request.RegionKey)
			var found bool
			adaptationKey, adaptationLabel, found, err = boards.RegionAdaptation(r.Context(), dbpool, sequence, regionKey)
			if err != nil {
				http.Error(w, "failed to load the region", http.StatusInternalServerError)
				return
			}
			if !found {
				http.Error(w, "the board has no region "+regionKey, http.StatusBadRequest)
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

		created, err := insertDuplicatedEmail(r.Context(), tx, duplicateEmailInsert{
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
			Layout:          source.Layout,
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
		if regionKey != "" {
			if _, err := tx.Exec(r.Context(), `UPDATE emails SET region_key = $2 WHERE id = $1;`, created.ID, regionKey); err != nil {
				http.Error(w, "failed to set the region", http.StatusInternalServerError)
				return
			}
			if err := linkToSlotReference(r.Context(), tx, created.ID); err != nil {
				http.Error(w, "failed to link the reference", http.StatusInternalServerError)
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
				"region_key":       regionKey,
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
		if !requireManageEmail(w, r, dbpool, user, id) {
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

		created, err := insertDuplicatedEmail(r.Context(), tx, duplicateEmailInsert{
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
			Layout:          source.Layout,
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
		if !requireManageEmail(w, r, dbpool, user, id) {
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

		created, err := insertDuplicatedEmail(r.Context(), tx, duplicateEmailInsert{
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
			Layout:          source.Layout,
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
		if !requireManageEmail(w, r, dbpool, user, id) {
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to archive email", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		err = archiveEmail(r.Context(), tx, user, id)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to archive email %s: %v\n", id, err)
			http.Error(w, "failed to archive email", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to archive email", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// archiveEmail archives one email inside tx and records it in the history;
// a missing or already archived email returns pgx.ErrNoRows.
func archiveEmail(ctx context.Context, tx pgx.Tx, user AuthUser, id string) error {
	var snapshot emailArchiveSnapshot
	err := tx.QueryRow(ctx, `
		SELECT id, slug, title, language, variant, stage
		FROM emails
		WHERE id = $1
			AND archived_at IS NULL
		FOR UPDATE;
	`, id).Scan(
		&snapshot.ID,
		&snapshot.Slug,
		&snapshot.Title,
		&snapshot.Language,
		&snapshot.Variant,
		&snapshot.Stage,
	)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE emails
		SET archived_at = now(),
			archived_by = $2,
			slug = $3,
			updated_at = now()
		WHERE id = $1;
	`, id, user.ID, archivedEmailSlug(snapshot.Slug, snapshot.ID)); err != nil {
		return err
	}
	return InsertEmailEvent(ctx, tx, EmailEventParam{
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
	})
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
	Layout          []byte
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
	return loadEmailForDuplicateTx(r.Context(), dbpool, id)
}

func loadEmailForDuplicateTx(ctx context.Context, db rowQuerier, id string) (emailDuplicateSource, error) {
	var source emailDuplicateSource
	err := db.QueryRow(ctx, `
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
			editable_fields,
			layout
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
		&source.Layout,
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
	// Layout is copied as is: the copy is built from the same blocks.
	Layout []byte
}

func insertDuplicatedEmail(ctx context.Context, db emailEventExecutor, email duplicateEmailInsert) (EmailDetail, error) {
	var created EmailDetail
	var editableFields []byte

	// Fields equal to a shared text of the email's language and board point
	// at it; copied marks that do not fit are dropped.
	linkedFields, err := linkSharedFieldsJSON(ctx, db, email.Language, email.Sequence, email.EditableFields, true)
	if err != nil {
		return EmailDetail{}, err
	}
	email.EditableFields = linkedFields

	err = db.QueryRow(ctx, `
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
			send_condition,
			layout
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15, $16, $17, $18, $19, $20::jsonb, $21, $22::jsonb)
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
	`, email.Slug, email.Sequence, email.Title, email.Subject, email.Preheader, email.SendTiming, email.Stage, email.SortOrder, email.Language, email.Variant, email.AdaptationKey, email.AdaptationLabel, email.BodyText, email.ContentParts, email.OriginalHTML, email.ReviewHTML, email.TemplateHTML, email.TemplateHash, email.TemplateVersion, email.EditableFields, email.SendCondition, email.Layout).Scan(
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
		// Order follows the template: a block added above moves it without
		// changing the field.
		currentCompared, nextCompared := currentField, nextField
		currentCompared.Order, nextCompared.Order = 0, 0
		if !reflect.DeepEqual(currentCompared, nextCompared) {
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
