package emails

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

func testExportEmail(slug string) exportEmail {
	subject := "Welcome"
	email := exportEmail{
		EmailListItem: EmailListItem{
			ID: "id-" + slug, Sequence: "onboarding", Title: "Welcome email", Subject: &subject,
			Stage: "day-1", Language: "en", Variant: "new", ReviewStatus: "draft",
		},
		Slug:           slug,
		BoardName:      "Onboarding",
		TemplateHTML:   `<html><body><p data-review-block="a">Hi</p></body></html>`,
		EditableFields: emailedit.EditableFields{},
		BodyText:       "Hello there",
		Comments:       []exportComment{{Severity: "blocking", Status: "open", Body: "Fix typo", SelectedText: "Hi"}},
	}
	return email
}

func zipEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("invalid zip: %v", err)
	}
	entries := map[string]string{}
	for _, file := range reader.File {
		rc, _ := file.Open()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(rc)
		rc.Close()
		entries[file.Name] = buf.String()
	}
	return entries
}

func TestBuildExportArchiveMarkdown(t *testing.T) {
	data, err := buildExportArchive([]exportEmail{testExportEmail("welcome"), testExportEmail("welcome")}, exportFormatMarkdown, allExportOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	entries := zipEntries(t, data)

	for _, name := range []string{"onboarding/day-1/welcome.md", "onboarding/day-1/welcome-2.md", "_all.md"} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("missing %s in %v", name, entries)
		}
	}
	md := entries["onboarding/day-1/welcome.md"]
	for _, want := range []string{"# Welcome email", "**Stage:** day-1", "Hello there", "[blocking, open] Fix typo"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestBuildExportArchiveHTML(t *testing.T) {
	data, err := buildExportArchive([]exportEmail{testExportEmail("welcome")}, exportFormatHTML, allExportOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	entries := zipEntries(t, data)
	html, ok := entries["onboarding/day-1/welcome.html"]
	if !ok || !strings.Contains(html, "Hi") {
		t.Fatalf("unexpected entries: %v", entries)
	}
}

func allExportOptions(t *testing.T) exportOptions {
	t.Helper()
	options, err := newExportOptions(exportEmailsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return options
}

func TestBuildEmailMarkdownSectionSelection(t *testing.T) {
	options, err := newExportOptions(exportEmailsRequest{Sections: []string{exportSectionCopy}})
	if err != nil {
		t.Fatal(err)
	}
	md := buildEmailMarkdown(testExportEmail("welcome"), options)

	if !strings.Contains(md, "Hello there") {
		t.Errorf("copy section missing:\n%s", md)
	}
	for _, unwanted := range []string{"## Metadata", "Review comments", "Primary CTA"} {
		if strings.Contains(md, unwanted) {
			t.Errorf("unexpected %q:\n%s", unwanted, md)
		}
	}
	if _, err := newExportOptions(exportEmailsRequest{Sections: []string{"bogus"}}); err == nil {
		t.Error("expected error for unknown section")
	}
}

func TestBuildEmailMarkdownOpenCommentsOnly(t *testing.T) {
	email := testExportEmail("welcome")
	email.Comments = append(email.Comments, exportComment{Severity: "issue", Status: "resolved", Body: "Old note"})
	options := allExportOptions(t)
	options.openCommentsOnly = true

	md := buildEmailMarkdown(email, options)
	if !strings.Contains(md, "Fix typo") || strings.Contains(md, "Old note") {
		t.Errorf("open-only filter wrong:\n%s", md)
	}
}
