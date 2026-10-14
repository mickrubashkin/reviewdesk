package emails

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MarkdownDoc is one email rendered as Markdown for embedding in a larger
// document, with the fields a caller needs to order and group it.
type MarkdownDoc struct {
	ID              string
	Stage           string
	SortOrder       int
	Title           string
	Language        string
	Variant         string
	AdaptationLabel string
	ReviewStatus    string
	Markdown        string
}

// ExportEmailMarkdownDocs renders the given emails of one board. Sections nil
// means all sections; headingOffset nests the email headings.
func ExportEmailMarkdownDocs(
	ctx context.Context,
	dbpool *pgxpool.Pool,
	boardKey string,
	ids []string,
	sections []string,
	openCommentsOnly bool,
	headingOffset int,
) ([]MarkdownDoc, error) {
	options, err := newExportOptions(exportEmailsRequest{Sections: sections, OpenCommentsOnly: openCommentsOnly})
	if err != nil {
		return nil, err
	}
	options.headingOffset = headingOffset

	emails, err := loadEmailsForExport(ctx, dbpool, ids, options.sections[exportSectionComments])
	if err != nil {
		return nil, err
	}

	docs := make([]MarkdownDoc, 0, len(emails))
	for _, email := range emails {
		if email.Sequence != boardKey {
			continue
		}
		docs = append(docs, MarkdownDoc{
			ID:              email.ID,
			Stage:           email.Stage,
			SortOrder:       email.SortOrder,
			Title:           email.Title,
			Language:        email.Language,
			Variant:         email.Variant,
			AdaptationLabel: email.AdaptationLabel,
			ReviewStatus:    email.ReviewStatus,
			Markdown:        buildEmailMarkdown(email, options),
		})
	}

	return docs, nil
}
