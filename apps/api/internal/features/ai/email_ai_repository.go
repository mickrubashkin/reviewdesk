package ai

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getEmailForAI(ctx context.Context, dbpool *pgxpool.Pool, id string) (EmailDetail, error) {
	var email EmailDetail

	err := dbpool.QueryRow(ctx, `
		SELECT
			id,
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
			body_text,
			content_parts::text,
			updated_at,
			original_html
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
		&email.Stage,
		&email.SortOrder,
		&email.Language,
		&email.Variant,
		&email.BodyText,
		&email.ContentParts,
		&email.UpdatedAt,
		&email.OriginalHTML,
	)

	return email, err
}
