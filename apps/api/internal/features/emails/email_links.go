package emails

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailreview"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
)

// LinkActor is who a bulk link change is recorded under.
type LinkActor struct {
	UserID string
	Email  string
}

// ReplaceEmailLinks rewrites links in an email with replace, which gets the
// whole template HTML (hrefs, link text, Outlook VML buttons inside
// conditional comments) and then each URL field value, and returns its input
// unchanged when there is nothing to swap. The original HTML, text fields and
// review status stay as they are; the email gets a new version and an update
// event, so Bitrix24 shows it as changed since upload. It reports whether
// anything changed (false means nothing was written).
func ReplaceEmailLinks(ctx context.Context, tx pgx.Tx, actor LinkActor, emailID string, replace func(string) string) (bool, error) {
	var slug, originalHTML, templateHTML, title string
	var subject, preheader *string
	var fieldsJSON []byte
	var builtFromBlocks bool
	err := tx.QueryRow(ctx, `
		SELECT slug, original_html, template_html, title, subject, preheader, editable_fields, layout IS NOT NULL
		FROM emails
		WHERE id = $1 AND archived_at IS NULL
		FOR UPDATE;
	`, emailID).Scan(&slug, &originalHTML, &templateHTML, &title, &subject, &preheader, &fieldsJSON, &builtFromBlocks)
	if err != nil {
		return false, fmt.Errorf("load email %s: %w", emailID, err)
	}
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return false, fmt.Errorf("email %s fields: %w", emailID, err)
	}

	// The template of an email built from blocks comes from the library;
	// links written in a block change with the block.
	nextTemplateHTML := templateHTML
	if !builtFromBlocks {
		nextTemplateHTML = replace(templateHTML)
	}
	fieldChanges := map[string]any{}
	nextFields := emailedit.EditableFields{}
	for key, field := range fields {
		if value, ok := field.Value.(string); ok && field.Type == emailedit.FieldTypeURL {
			if next := replace(value); next != value {
				fieldChanges[key] = map[string]any{"before": value, "after": next}
				field.Value = next
			}
		}
		nextFields[key] = field
	}
	if nextTemplateHTML == templateHTML && len(fieldChanges) == 0 {
		return false, nil
	}

	renderedHTML, err := emailedit.RenderEditableHTMLWithMetadata(nextTemplateHTML, nextFields, emailedit.RenderMetadata{
		Preheader: stringFromPointer(preheader),
		Subject:   stringFromPointer(subject),
	})
	if err != nil {
		return false, fmt.Errorf("render email %s: %w", emailID, err)
	}
	reviewHTML, err := emailreview.AddReviewBlocks(renderedHTML)
	if err != nil {
		return false, fmt.Errorf("review blocks of email %s: %w", emailID, err)
	}
	nextFieldsJSON, err := nextFields.JSON()
	if err != nil {
		return false, err
	}
	bodyText := emailtext.HTMLToText(renderedHTML)
	contentPartsJSON, err := json.Marshal(emailtext.ExtractContentParts(
		renderedHTML, stringFromPointer(subject), stringFromPointer(preheader), bodyText,
	))
	if err != nil {
		return false, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE emails
		SET template_html = $2,
			editable_fields = $3::jsonb,
			content_parts = $4::jsonb,
			review_html = $5,
			body_text = $6,
			updated_at = now()
		WHERE id = $1;
	`, emailID, nextTemplateHTML, nextFieldsJSON, contentPartsJSON, reviewHTML, bodyText); err != nil {
		return false, fmt.Errorf("update email %s: %w", emailID, err)
	}
	if err := insertEmailVersion(ctx, tx, emailVersionSnapshot{
		EmailID:            emailID,
		CreatedByUserID:    actor.UserID,
		CreatedByEmail:     actor.Email,
		Source:             "manual",
		Title:              title,
		Subject:            subject,
		Preheader:          preheader,
		OriginalHTML:       originalHTML,
		TemplateHTML:       nextTemplateHTML,
		EditableFieldsJSON: nextFieldsJSON,
		ChangedFieldCount:  len(fieldChanges),
		HTMLChanged:        nextTemplateHTML != templateHTML,
	}); err != nil {
		return false, fmt.Errorf("version of email %s: %w", emailID, err)
	}
	changes := map[string]any{}
	if len(fieldChanges) > 0 {
		changes["editable_fields"] = fieldChanges
	}
	if err := InsertEmailEvent(ctx, tx, EmailEventParam{
		ActorUserID: actor.UserID,
		ActorEmail:  actor.Email,
		Action:      emailEventUpdated,
		EmailID:     &emailID,
		EmailSlug:   &slug,
		EmailTitle:  &title,
		Metadata:    map[string]any{"source": "bulk_link_replace", "template_changed": nextTemplateHTML != templateHTML},
		Changes:     changes,
	}); err != nil {
		return false, fmt.Errorf("event of email %s: %w", emailID, err)
	}
	return true, nil
}
