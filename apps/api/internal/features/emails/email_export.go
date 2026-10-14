package emails

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

const (
	exportFormatHTML     = "html"
	exportFormatMarkdown = "md"
	maxExportEmails      = 500
)

// Markdown sections a caller can opt into. A nil list means all sections.
const (
	exportSectionMetadata = "metadata"
	exportSectionCopy     = "copy"
	exportSectionLinks    = "links"
	exportSectionNotes    = "notes"
	exportSectionComments = "comments"
)

var exportSections = []string{
	exportSectionMetadata,
	exportSectionCopy,
	exportSectionLinks,
	exportSectionNotes,
	exportSectionComments,
}

type exportEmailsRequest struct {
	IDs      []string `json:"ids"`
	Format   string   `json:"format"`
	Sections []string `json:"sections"`
	// OpenCommentsOnly drops resolved comments from the Markdown export.
	OpenCommentsOnly bool `json:"open_comments_only"`
}

type exportOptions struct {
	sections         map[string]bool
	openCommentsOnly bool
	// headingOffset nests the email under other headings when it is embedded
	// in a larger document.
	headingOffset int
}

func newExportOptions(request exportEmailsRequest) (exportOptions, error) {
	options := exportOptions{sections: map[string]bool{}, openCommentsOnly: request.OpenCommentsOnly}
	if request.Sections == nil {
		for _, section := range exportSections {
			options.sections[section] = true
		}
		return options, nil
	}
	for _, section := range request.Sections {
		valid := false
		for _, known := range exportSections {
			valid = valid || known == section
		}
		if !valid {
			return options, fmt.Errorf("unknown section %q", section)
		}
		options.sections[section] = true
	}
	return options, nil
}

type exportEmail struct {
	EmailListItem
	Slug           string
	BoardName      string
	TemplateHTML   string
	EditableFields emailedit.EditableFields
	BodyText       string
	ContentParts   emailtext.ContentParts
	Comments       []exportComment
}

type exportComment struct {
	Severity     string
	Status       string
	AuthorEmail  string
	SelectedText string
	Body         string
}

func exportEmailsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
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

		var request exportEmailsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if request.Format != exportFormatHTML && request.Format != exportFormatMarkdown {
			http.Error(w, "format must be html or md", http.StatusBadRequest)
			return
		}
		if len(request.IDs) == 0 {
			http.Error(w, "ids is required", http.StatusBadRequest)
			return
		}
		if len(request.IDs) > maxExportEmails {
			http.Error(w, fmt.Sprintf("at most %d emails can be exported at once", maxExportEmails), http.StatusBadRequest)
			return
		}

		options, err := newExportOptions(request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		emails, err := loadEmailsForExport(r.Context(), dbpool, request.IDs, request.Format == exportFormatMarkdown && options.sections[exportSectionComments])
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load emails for export: %v\n", err)
			http.Error(w, "failed to load emails", http.StatusInternalServerError)
			return
		}
		if len(emails) == 0 {
			http.Error(w, "no emails found", http.StatusNotFound)
			return
		}

		archive, err := buildExportArchive(emails, request.Format, options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to build export archive: %v\n", err)
			http.Error(w, "failed to build archive", http.StatusInternalServerError)
			return
		}

		fileName := fmt.Sprintf("reviewdesk-emails-%s-%s.zip", request.Format, time.Now().UTC().Format("20060102-150405"))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
		_, _ = w.Write(archive)
	}
}

func loadEmailsForExport(ctx context.Context, dbpool *pgxpool.Pool, ids []string, withComments bool) ([]exportEmail, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT
			emails.id,
			emails.slug,
			emails.sequence,
			COALESCE(boards.name, emails.sequence),
			emails.title,
			emails.subject,
			emails.preheader,
			emails.send_timing,
			emails.send_condition,
			emails.stage,
			emails.sort_order,
			emails.language,
			emails.variant,
			emails.adaptation_key,
			emails.adaptation_label,
			emails.review_status,
			emails.owner_email,
			emails.reviewer_email,
			emails.due_date::text,
			emails.implementation_notes,
			emails.template_html,
			emails.editable_fields,
			COALESCE(emails.body_text, ''),
			emails.content_parts
		FROM emails
		LEFT JOIN boards ON boards.key = emails.sequence
		WHERE emails.id = ANY($1::uuid[])
			AND emails.archived_at IS NULL
		ORDER BY emails.sequence, emails.sort_order, emails.created_at;
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	emails := []exportEmail{}
	for rows.Next() {
		var email exportEmail
		var editableFieldsJSON, contentPartsJSON []byte

		if err := rows.Scan(
			&email.ID,
			&email.Slug,
			&email.Sequence,
			&email.BoardName,
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
			&email.TemplateHTML,
			&editableFieldsJSON,
			&email.BodyText,
			&contentPartsJSON,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(editableFieldsJSON, &email.EditableFields); err != nil {
			return nil, fmt.Errorf("email %s editable fields: %w", email.ID, err)
		}
		if len(contentPartsJSON) > 0 {
			_ = json.Unmarshal(contentPartsJSON, &email.ContentParts)
		}
		emails = append(emails, email)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if withComments {
		if err := loadExportComments(ctx, dbpool, emails); err != nil {
			return nil, err
		}
	}

	return emails, nil
}

func loadExportComments(ctx context.Context, dbpool *pgxpool.Pool, emails []exportEmail) error {
	indexByID := make(map[string]int, len(emails))
	ids := make([]string, 0, len(emails))
	for i, email := range emails {
		indexByID[email.ID] = i
		ids = append(ids, email.ID)
	}

	rows, err := dbpool.Query(ctx, `
		SELECT
			comments.email_id,
			comments.severity,
			comments.status,
			COALESCE(users.email, ''),
			comments.selected_text,
			comments.body
		FROM comments
		LEFT JOIN users ON users.id = comments.user_id
		WHERE comments.email_id = ANY($1::uuid[])
		ORDER BY comments.created_at;
	`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var emailID string
		var comment exportComment
		if err := rows.Scan(&emailID, &comment.Severity, &comment.Status, &comment.AuthorEmail, &comment.SelectedText, &comment.Body); err != nil {
			return err
		}
		if i, ok := indexByID[emailID]; ok {
			emails[i].Comments = append(emails[i].Comments, comment)
		}
	}

	return rows.Err()
}

func buildExportArchive(emails []exportEmail, format string, options exportOptions) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	usedNames := map[string]int{}

	var combined strings.Builder
	for _, email := range emails {
		var content, extension string
		if format == exportFormatHTML {
			html, err := emailedit.RenderEditableHTMLWithMetadata(email.TemplateHTML, email.EditableFields, emailedit.RenderMetadata{
				Preheader: stringFromPointer(email.Preheader),
			})
			if err != nil {
				return nil, fmt.Errorf("render email %s: %w", email.ID, err)
			}
			content, extension = html, "html"
		} else {
			content, extension = buildEmailMarkdown(email, options), "md"
			if combined.Len() > 0 {
				combined.WriteString("\n\n---\n\n")
			}
			combined.WriteString(content)
		}

		name := uniqueExportPath(usedNames, email, extension)
		if err := writeZipFile(archive, name, content); err != nil {
			return nil, err
		}
	}

	if format == exportFormatMarkdown {
		if err := writeZipFile(archive, "_all.md", combined.String()); err != nil {
			return nil, err
		}
	}

	if err := archive.Close(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func writeZipFile(archive *zip.Writer, name string, content string) error {
	file, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = file.Write([]byte(content))
	return err
}

var exportPathUnsafePattern = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func exportPathSegment(value string, fallback string) string {
	segment := strings.Trim(exportPathUnsafePattern.ReplaceAllString(strings.TrimSpace(value), "-"), "-.")
	if segment == "" {
		return fallback
	}
	return segment
}

func uniqueExportPath(used map[string]int, email exportEmail, extension string) string {
	base := fmt.Sprintf("%s/%s/%s",
		exportPathSegment(email.Sequence, "board"),
		exportPathSegment(email.Stage, "no-stage"),
		exportPathSegment(email.Slug, "email"),
	)
	used[base]++
	if used[base] > 1 {
		base = fmt.Sprintf("%s-%d", base, used[base])
	}
	return base + "." + extension
}

func buildEmailMarkdown(email exportEmail, options exportOptions) string {
	var b strings.Builder
	h := func(level int) string { return strings.Repeat("#", level+options.headingOffset) }

	fmt.Fprintf(&b, "%s %s\n\n", h(1), oneLine(email.Title))

	if options.sections[exportSectionMetadata] {
		b.WriteString(h(2) + " Metadata\n\n")
		writeMarkdownField(&b, "Board", email.BoardName)
		writeMarkdownField(&b, "Sequence key", email.Sequence)
		writeMarkdownField(&b, "Stage", email.Stage)
		writeMarkdownField(&b, "Order in stage", fmt.Sprintf("%d", email.SortOrder))
		writeMarkdownField(&b, "Send timing", stringFromPointer(email.SendTiming))
		writeMarkdownField(&b, "Send condition", stringFromPointer(email.SendCondition))
		writeMarkdownField(&b, "Language", email.Language)
		writeMarkdownField(&b, "Adaptation", email.AdaptationLabel)
		writeMarkdownField(&b, "Version", email.Variant)
		writeMarkdownField(&b, "Review status", email.ReviewStatus)
		writeMarkdownField(&b, "Owner", stringFromPointer(email.OwnerEmail))
		writeMarkdownField(&b, "Reviewer", stringFromPointer(email.ReviewerEmail))
		writeMarkdownField(&b, "Due date", stringFromPointer(email.DueDate))
		writeMarkdownField(&b, "Slug", email.Slug)
		writeMarkdownField(&b, "ID", email.ID)
		b.WriteString("\n")
	}

	parts := email.ContentParts
	if options.sections[exportSectionCopy] {
		b.WriteString(h(2) + " Subject\n\n" + valueOrDash(stringFromPointer(email.Subject)) + "\n\n")
		b.WriteString(h(2) + " Preheader\n\n" + valueOrDash(stringFromPointer(email.Preheader)) + "\n\n")
		if parts.BannerText != "" {
			b.WriteString(h(2) + " Banner text\n\n" + parts.BannerText + "\n\n")
		}
		b.WriteString(h(2) + " Body text\n\n" + valueOrDash(email.BodyText) + "\n\n")
	}

	if options.sections[exportSectionLinks] {
		b.WriteString(h(2) + " Primary CTA\n\n" + valueOrDash(parts.PrimaryCTA) + "\n")
		writeMarkdownList(&b, h(2), "Primary links", parts.LinkGroups.Primary)
		writeMarkdownList(&b, h(2), "Support links", parts.LinkGroups.Support)
		writeMarkdownList(&b, h(2), "Footer links", parts.LinkGroups.Footer)
		writeMarkdownList(&b, h(2), "Other links", parts.LinkGroups.Other)
		b.WriteString("\n")
	}

	if notes := stringFromPointer(email.ImplementationNotes); options.sections[exportSectionNotes] && strings.TrimSpace(notes) != "" {
		b.WriteString(h(2) + " Implementation notes\n\n" + notes + "\n\n")
	}

	if options.sections[exportSectionComments] {
		var comments []exportComment
		for _, comment := range email.Comments {
			if !options.openCommentsOnly || comment.Status == "open" {
				comments = append(comments, comment)
			}
		}
		if len(comments) > 0 {
			b.WriteString(h(2) + " Review comments\n\n")
			for _, comment := range comments {
				fmt.Fprintf(&b, "- [%s, %s] %s", comment.Severity, comment.Status, oneLine(comment.Body))
				if comment.AuthorEmail != "" {
					fmt.Fprintf(&b, " (%s)", comment.AuthorEmail)
				}
				if selected := oneLine(comment.SelectedText); selected != "" {
					fmt.Fprintf(&b, "\n  > %s", selected)
				}
				b.WriteString("\n")
			}
		}
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func writeMarkdownField(b *strings.Builder, label string, value string) {
	fmt.Fprintf(b, "- **%s:** %s\n", label, valueOrDash(oneLine(value)))
}

func writeMarkdownList(b *strings.Builder, heading string, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s %s\n\n", heading, title)
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", oneLine(value))
	}
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}
