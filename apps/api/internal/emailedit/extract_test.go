package emailedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractEditableFields(t *testing.T) {
	templateHTML := `
		<div>
			<span data-email-preheader>Access your account.</span>
			<img src="https://example.com/banner.png" alt="Hero banner" data-edit-attr-src="hero_banner_src" data-edit-attr-alt="hero_banner_alt">
			<a href="https://example.com/private/" style="display:block; width: 196px;" data-edit-text="primary_cta_text" data-edit-attr-href="primary_cta_url" data-edit-style-width-px="primary_cta_width_px">Upload<br>documents</a>
		</div>
	`

	fields, err := ExtractEditableFields(templateHTML)
	if err != nil {
		t.Fatalf("ExtractEditableFields returned error: %v", err)
	}

	assertFieldMissing(t, fields, "preheader_text")
	assertFieldValue(t, fields, "hero_banner_src", FieldTypeImage, "https://example.com/banner.png")
	assertFieldValue(t, fields, "hero_banner_alt", FieldTypeText, "Hero banner")
	assertFieldValue(t, fields, "primary_cta_text", FieldTypeText, "Upload\ndocuments")
	assertFieldValue(t, fields, "primary_cta_url", FieldTypeURL, "https://example.com/private/")
	assertFieldValue(t, fields, "primary_cta_width_px", FieldTypeNumber, 196)
	assertFieldOrder(t, fields, "hero_banner_src", 1)
	assertFieldOrder(t, fields, "hero_banner_alt", 2)
	assertFieldOrder(t, fields, "primary_cta_text", 3)
	assertFieldOrder(t, fields, "primary_cta_url", 4)
	assertFieldOrder(t, fields, "primary_cta_width_px", 5)
}

func TestExtractEditableFieldsRejectsConflictingDuplicateKeys(t *testing.T) {
	templateHTML := `
		<div>
			<span data-edit-text="headline">First</span>
			<span data-edit-text="headline">Second</span>
		</div>
	`

	_, err := ExtractEditableFields(templateHTML)
	if err == nil {
		t.Fatal("ExtractEditableFields expected error for conflicting duplicate keys")
	}
}

func TestExtractEditableFieldsIgnoresSourceFormattingWhitespace(t *testing.T) {
	templateHTML := `
		<div>
			<div data-edit-text="intro_body">
				Dear Partner,<br /><br />
				You've closed your first deal — great start.
				Now the goal is to do it again.
			</div>
			<a data-edit-text="primary_cta_text">
				Take more
				leads
			</a>
		</div>
	`

	fields, err := ExtractEditableFields(templateHTML)
	if err != nil {
		t.Fatalf("ExtractEditableFields returned error: %v", err)
	}

	assertFieldValue(t, fields, "intro_body", FieldTypeText, "Dear Partner,\n\nYou've closed your first deal — great start. Now the goal is to do it again.")
	assertFieldValue(t, fields, "primary_cta_text", FieldTypeText, "Take more leads")
}

func TestExtractEditableFieldsCollapsesRepeatedBlankLines(t *testing.T) {
	templateHTML := `<div data-edit-text="body">First<br><br><br>Second</div>`

	fields, err := ExtractEditableFields(templateHTML)
	if err != nil {
		t.Fatalf("ExtractEditableFields returned error: %v", err)
	}

	assertFieldValue(t, fields, "body", FieldTypeText, "First\n\nSecond")
}

const bulletproofTemplatePath = "testdata/bulletproof-cta.html"

func readTestTemplate(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile(bulletproofTemplatePath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", bulletproofTemplatePath, err)
	}
	return string(content)
}

func TestExtractEditableFieldsPreservesTemplateBlankLines(t *testing.T) {
	fields, err := ExtractEditableFields(readTestTemplate(t))
	if err != nil {
		t.Fatalf("ExtractEditableFields returned error: %v", err)
	}

	assertFieldValue(
		t,
		fields,
		"intro_body",
		FieldTypeText,
		"Hi there,\n\nYour workspace is almost ready.\n\nConnect your calendar and invite one teammate to see your first report today.",
	)
}

func TestBulletproofCTATemplateExtractsAndRenders(t *testing.T) {
	templateHTML := readTestTemplate(t)

	fields, err := ExtractEditableFields(templateHTML)
	if err != nil {
		t.Fatalf("ExtractEditableFields returned error: %v", err)
	}

	assertFieldValue(t, fields, "primary_cta_text", FieldTypeText, "Finish setup")
	assertFieldValue(t, fields, "primary_cta_url", FieldTypeURL, "https://app.example.com/setup")
	assertFieldValue(t, fields, "primary_cta_width_px", FieldTypeNumber, 220)
	if fields["primary_cta_text"].Order >= fields["primary_cta_url"].Order {
		t.Fatalf("expected CTA text to be ordered before CTA URL")
	}

	fields["primary_cta_text"] = EditableField{Type: FieldTypeText, Value: "Open your workspace"}
	fields["primary_cta_url"] = EditableField{Type: FieldTypeURL, Value: "https://app.example.com/workspace"}
	fields["primary_cta_width_px"] = EditableField{Type: FieldTypeNumber, Value: 240}

	rendered, err := RenderEditableHTMLWithMetadata(templateHTML, fields, RenderMetadata{
		Preheader: "Your workspace is waiting.",
	})
	if err != nil {
		t.Fatalf("RenderEditableHTML returned error: %v", err)
	}

	// The regular link is edited through the DOM; the Outlook button lives in a
	// conditional comment and is filled through {{markers}}.
	assertContains(t, rendered, `href="https://app.example.com/workspace"`)
	assertContains(t, rendered, `width: 240px`)
	assertContains(t, rendered, `<center style="color: #ffffff; font-family: Arial, sans-serif; font-size: 16px">Open your workspace</center>`)
	assertContains(t, rendered, `width: 240px" arcsize`)
	assertContains(t, rendered, `{{=date('Y')}}`)
	assertContains(t, rendered, `Your workspace is waiting.`)
}

func TestAllTemplatesExtractAndRender(t *testing.T) {
	paths := append(readRepoGlob(t, "db/seeds/demo-emails/*/*/en.html"), "")
	if len(paths) < 2 {
		t.Fatal("expected demo email templates")
	}

	for _, path := range paths {
		name := strings.TrimPrefix(path, "db/seeds/demo-emails/")
		if path == "" {
			name = bulletproofTemplatePath
		}
		t.Run(name, func(t *testing.T) {
			templateHTML := readTestTemplate(t)
			if path != "" {
				templateHTML = readRepoFile(t, path)
			}

			fields, err := ExtractEditableFields(templateHTML)
			if err != nil {
				t.Fatalf("ExtractEditableFields returned error: %v", err)
			}

			for _, key := range []string{"primary_cta_text", "primary_cta_url"} {
				if _, ok := fields[key]; !ok {
					t.Fatalf("expected editable field %q", key)
				}
			}

			if _, err := RenderEditableHTMLWithMetadata(templateHTML, fields, RenderMetadata{
				Preheader: "Template preheader",
			}); err != nil {
				t.Fatalf("RenderEditableHTML returned error: %v", err)
			}
		})
	}
}

func assertFieldValue(t *testing.T, fields EditableFields, key string, fieldType string, value any) {
	t.Helper()

	field, ok := fields[key]
	if !ok {
		t.Fatalf("field %q is missing", key)
	}

	if field.Type != fieldType {
		t.Fatalf("field %q type = %q, want %q", key, field.Type, fieldType)
	}

	if field.Value != value {
		t.Fatalf("field %q value = %#v, want %#v", key, field.Value, value)
	}
}

func assertFieldOrder(t *testing.T, fields EditableFields, key string, order int) {
	t.Helper()

	field, ok := fields[key]
	if !ok {
		t.Fatalf("field %q is missing", key)
	}
	if field.Order != order {
		t.Fatalf("field %q order = %d, want %d", key, field.Order, order)
	}
}

func assertFieldMissing(t *testing.T, fields EditableFields, key string) {
	t.Helper()

	if _, ok := fields[key]; ok {
		t.Fatalf("field %q should not be extracted", key)
	}
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}

	for dir := workingDir; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, path)
		content, err := os.ReadFile(candidate)
		if err == nil {
			return string(content)
		}
		if !os.IsNotExist(err) {
			t.Fatalf("failed to read %s: %v", candidate, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	t.Fatalf("failed to find repo file %s from %s", path, workingDir)
	return ""
}

func readRepoGlob(t *testing.T, pattern string) []string {
	t.Helper()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}

	for dir := workingDir; ; dir = filepath.Dir(dir) {
		seedDir := filepath.Join(dir, "db", "seeds", "demo-emails")
		if info, err := os.Stat(seedDir); err == nil && info.IsDir() {
			matches, err := filepath.Glob(filepath.Join(dir, pattern))
			if err != nil {
				t.Fatalf("failed to glob %s: %v", pattern, err)
			}
			relativeMatches := make([]string, 0, len(matches))
			for _, match := range matches {
				relativePath, err := filepath.Rel(dir, match)
				if err != nil {
					t.Fatalf("failed to make relative path for %s: %v", match, err)
				}
				relativeMatches = append(relativeMatches, relativePath)
			}
			return relativeMatches
		} else if err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to inspect %s: %v", seedDir, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	t.Fatalf("failed to find repository root")
	return nil
}
