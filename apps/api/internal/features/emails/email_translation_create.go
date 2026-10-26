package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailreview"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
)

var (
	ErrNotAMaster          = errors.New("only an EN email can be the source of a translation")
	ErrTranslationExists   = errors.New("this slot already has an email in this language")
	ErrTranslationLanguage = errors.New("pick a language other than EN")
)

// MasterText returns an EN master's subject, preheader and text fields by
// the keys translations use; pgx.ErrNoRows when the email is gone and
// ErrNotAMaster when it is not EN.
func MasterText(ctx context.Context, db rowQuerier, masterID string) (map[string]string, error) {
	var language string
	var subject, preheader *string
	var fieldsJSON []byte
	if err := db.QueryRow(ctx, `
		SELECT lower(language), subject, preheader, editable_fields FROM emails WHERE id = $1 AND archived_at IS NULL;
	`, masterID).Scan(&language, &subject, &preheader, &fieldsJSON); err != nil {
		return nil, err
	}
	if language != "en" {
		return nil, ErrNotAMaster
	}
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return nil, err
	}
	text := map[string]string{"subject": stringFromPointer(subject), "preheader": stringFromPointer(preheader)}
	for key, field := range fields {
		// Shared fields get the text of the target language, not a translation.
		if field.Type == emailedit.FieldTypeText && !field.Shared {
			text[key] = fmt.Sprint(field.Value)
		}
	}
	return text, nil
}

// CanTranslateInto reports whether user may create the translation of the
// master email into language: admins always, managers of the master's board,
// translators for their languages.
func CanTranslateInto(ctx context.Context, db rowQuerier, user auth.AuthUser, masterID, language string) (bool, error) {
	if manages, err := auth.CanManageEmail(ctx, db, user, masterID); err != nil || manages {
		return manages, err
	}
	var languages []string
	if err := db.QueryRow(ctx, `SELECT languages FROM users WHERE id = $1;`, user.ID).Scan(&languages); err != nil {
		return false, err
	}
	return slices.Contains(languages, language), nil
}

// CreateTranslationTx creates the translation of an EN master into language
// in the master's slot, with the given text (subject, preheader and text
// fields by key; missing keys keep the English). It starts as a draft linked
// to the master's current version. The caller owns the transaction.
func CreateTranslationTx(r *http.Request, tx pgx.Tx, user AuthUser, masterID, language string, text map[string]string, metadata map[string]any) (EmailDetail, error) {
	ctx := r.Context()
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" || language == "en" {
		return EmailDetail{}, ErrTranslationLanguage
	}
	source, err := loadEmailForDuplicateTx(ctx, tx, masterID)
	if err != nil {
		return EmailDetail{}, err
	}
	if source.Language != "en" {
		return EmailDetail{}, ErrNotAMaster
	}
	var taken bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM emails
			WHERE sequence = $1 AND stage = $2 AND sort_order = $3 AND lower(language) = $4
				AND adaptation_key = $5 AND archived_at IS NULL
		);
	`, source.Sequence, source.Stage, source.SortOrder, language, source.AdaptationKey).Scan(&taken); err != nil {
		return EmailDetail{}, err
	}
	if taken {
		return EmailDetail{}, ErrTranslationExists
	}

	var fields emailedit.EditableFields
	if err := json.Unmarshal(source.EditableFields, &fields); err != nil {
		return EmailDetail{}, err
	}
	for key, field := range fields {
		if value, ok := text[key]; ok && field.Type == emailedit.FieldTypeText {
			field.Value = value
			fields[key] = field
		}
	}
	// Shared fields (footer, signature...) take the text of the target
	// language instead of a translation.
	fields, err = resolveSharedFields(ctx, tx, language, source.Sequence, fields)
	if err != nil {
		return EmailDetail{}, err
	}
	subject, preheader := source.Subject, source.Preheader
	if value, ok := text["subject"]; ok {
		subject = trimmedOptionalString(&value)
	}
	if value, ok := text["preheader"]; ok {
		preheader = trimmedOptionalString(&value)
	}

	renderedHTML, err := emailedit.RenderEditableHTMLWithMetadata(source.TemplateHTML, fields, emailedit.RenderMetadata{
		Preheader: stringFromPointer(preheader),
		Subject:   stringFromPointer(subject),
	})
	if err != nil {
		return EmailDetail{}, fmt.Errorf("render translation: %w", err)
	}
	reviewHTML, err := emailreview.AddReviewBlocks(renderedHTML)
	if err != nil {
		return EmailDetail{}, fmt.Errorf("render translation: %w", err)
	}
	fieldsJSON, err := fields.JSON()
	if err != nil {
		return EmailDetail{}, err
	}
	bodyText := emailtext.HTMLToText(renderedHTML)
	contentParts, err := json.Marshal(emailtext.ExtractContentParts(renderedHTML, stringFromPointer(subject), stringFromPointer(preheader), bodyText))
	if err != nil {
		return EmailDetail{}, err
	}

	created, err := insertDuplicatedEmail(r.Context(), tx, duplicateEmailInsert{
		Slug:            emailSlugWithAdaptation(duplicateEmailSlug(source.Slug, source.Language, source.Variant, language, source.Variant), source.AdaptationKey),
		Sequence:        source.Sequence,
		Title:           source.Title,
		Subject:         subject,
		Preheader:       preheader,
		SendTiming:      source.SendTiming,
		SendCondition:   source.SendCondition,
		Stage:           source.Stage,
		SortOrder:       source.SortOrder,
		Language:        language,
		Variant:         source.Variant,
		AdaptationKey:   source.AdaptationKey,
		AdaptationLabel: source.AdaptationLabel,
		BodyText:        &bodyText,
		ContentParts:    contentParts,
		OriginalHTML:    source.OriginalHTML,
		ReviewHTML:      &reviewHTML,
		TemplateHTML:    source.TemplateHTML,
		TemplateHash:    source.TemplateHash,
		TemplateVersion: source.TemplateVersion,
		Layout:          source.Layout,
		EditableFields:  fieldsJSON,
	})
	if err != nil {
		if isEmailCreationConflict(err) {
			return EmailDetail{}, ErrTranslationExists
		}
		return EmailDetail{}, err
	}
	if err := linkTranslation(ctx, tx, created.ID, masterID); err != nil {
		return EmailDetail{}, err
	}

	eventMetadata := map[string]any{
		"mode":            "translation_from_en",
		"source_email_id": masterID,
		"source_slug":     source.Slug,
		"language":        language,
		"stage":           created.Stage,
		"sort_order":      created.SortOrder,
	}
	for key, value := range metadata {
		eventMetadata[key] = value
	}
	if err := InsertEmailEvent(ctx, tx, EmailEventParam{
		ActorUserID: user.ID,
		ActorEmail:  user.Email,
		Action:      emailEventCreated,
		EmailID:     &created.ID,
		EmailSlug:   &created.Slug,
		EmailTitle:  &created.Title,
		Metadata:    eventMetadata,
	}); err != nil {
		return EmailDetail{}, err
	}
	if err := insertEmailVersion(ctx, tx, emailVersionSnapshot{
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
		return EmailDetail{}, err
	}
	return created, nil
}
