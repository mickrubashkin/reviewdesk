package emails

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// CopyBoardEmailsTx copies the active EN `new` emails of the source board's
// main branch into the target board: same stage, position, timing and
// condition, as drafts with a fresh history. Translations, `old` variants,
// branches, portal and upload marks, owners and comments stay behind. It
// returns the number of copied emails. The caller owns the transaction.
func CopyBoardEmailsTx(ctx context.Context, tx pgx.Tx, actor AuthUser, sourceBoardKey, targetBoardKey string) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			id,
			slug,
			title,
			subject,
			preheader,
			send_timing,
			send_condition,
			stage,
			sort_order,
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
			layout
		FROM emails
		WHERE sequence = $1
			AND archived_at IS NULL
			AND lower(language) = 'en'
			AND variant = 'new'
			AND adaptation_key = 'default'
		ORDER BY stage, sort_order;
	`, sourceBoardKey)
	if err != nil {
		return 0, err
	}
	type sourceEmail struct {
		id     string
		insert duplicateEmailInsert
	}
	var sources []sourceEmail
	for rows.Next() {
		var source sourceEmail
		email := &source.insert
		if err := rows.Scan(
			&source.id,
			&email.Slug,
			&email.Title,
			&email.Subject,
			&email.Preheader,
			&email.SendTiming,
			&email.SendCondition,
			&email.Stage,
			&email.SortOrder,
			&email.Variant,
			&email.AdaptationKey,
			&email.AdaptationLabel,
			&email.BodyText,
			&email.ContentParts,
			&email.OriginalHTML,
			&email.ReviewHTML,
			&email.TemplateHTML,
			&email.TemplateHash,
			&email.TemplateVersion,
			&email.EditableFields,
			&email.Layout,
		); err != nil {
			rows.Close()
			return 0, err
		}
		email.Sequence = targetBoardKey
		email.Language = "en"
		sources = append(sources, source)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, source := range sources {
		sourceSlug := source.insert.Slug
		slug, err := freeEmailSlug(ctx, tx, boardCopySlug(sourceSlug, sourceBoardKey, targetBoardKey))
		if err != nil {
			return 0, err
		}
		source.insert.Slug = slug

		created, err := insertDuplicatedEmail(ctx, tx, source.insert)
		if err != nil {
			return 0, fmt.Errorf("copy email %s: %w", sourceSlug, err)
		}
		if err := InsertEmailEvent(ctx, tx, EmailEventParam{
			ActorUserID: actor.ID,
			ActorEmail:  actor.Email,
			Action:      emailEventCreated,
			EmailID:     &created.ID,
			EmailSlug:   &created.Slug,
			EmailTitle:  &created.Title,
			Metadata: map[string]any{
				"mode":             "board_copy",
				"source_email_id":  source.id,
				"source_slug":      sourceSlug,
				"source_board_key": sourceBoardKey,
				"stage":            created.Stage,
				"sort_order":       created.SortOrder,
			},
		}); err != nil {
			return 0, err
		}
		if err := insertEmailVersion(ctx, tx, emailVersionSnapshot{
			EmailID:            created.ID,
			CreatedByUserID:    actor.ID,
			CreatedByEmail:     actor.Email,
			Source:             "initial",
			Title:              created.Title,
			Subject:            created.Subject,
			Preheader:          created.Preheader,
			OriginalHTML:       created.OriginalHTML,
			TemplateHTML:       created.TemplateHTML,
			EditableFieldsJSON: created.EditableFields,
		}); err != nil {
			return 0, err
		}
	}

	return len(sources), nil
}

// boardCopySlug swaps the source board prefix of a slug for the target
// board key: onboarding-approved-next-step-en → india-approved-next-step-en.
func boardCopySlug(slug, sourceBoardKey, targetBoardKey string) string {
	if rest, ok := strings.CutPrefix(slug, sourceBoardKey+"-"); ok {
		return targetBoardKey + "-" + rest
	}
	return targetBoardKey + "-" + slug
}

// freeEmailSlug returns slug, or slug with the first free numeric suffix
// when an email (archived ones included) already uses it.
func freeEmailSlug(ctx context.Context, db rowQuerier, slug string) (string, error) {
	candidate := slug
	for suffix := 2; ; suffix++ {
		var taken bool
		if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM emails WHERE slug = $1);`, candidate).Scan(&taken); err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", slug, suffix)
	}
}
