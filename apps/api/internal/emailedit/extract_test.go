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

func TestExtractEditableFieldsPreservesSeedBlankLines(t *testing.T) {
	templateHTML := readRepoFile(t, "db/seeds/emails/01_registered/01_application-received/en.html")

	fields, err := ExtractEditableFields(templateHTML)
	if err != nil {
		t.Fatalf("ExtractEditableFields returned error: %v", err)
	}

	assertFieldValue(
		t,
		fields,
		"intro_body",
		FieldTypeText,
		"Hi there,\n\nYour application to join the Bitrix24 Partner Program has been received.\n\nYour partner account is ready. Use the credentials below to log in.",
	)
	assertFieldValue(
		t,
		fields,
		"next_steps_body",
		FieldTypeText,
		"To continue, log in to your partner account and upload the required documents.\nThis helps us verify your application faster and approve your account without delays.",
	)
}

func TestAnnotatedSeedTemplatesExtractAndRender(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		ctaText   string
		ctaURL    string
		preheader string
	}{
		{
			name:      "application received",
			path:      "db/seeds/emails/01_registered/01_application-received/en.html",
			ctaText:   "Upload documents",
			ctaURL:    "https://partners.bitrix24.com/private/verify",
			preheader: "Access your account using the login details below.",
		},
		{
			name:      "qualified next step",
			path:      "db/seeds/emails/02_qualified/01-next-step/en.html",
			ctaText:   "Book a Call",
			ctaURL:    "https://calendly.com/alice/30min",
			preheader: "Next step: complete your partner verification",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			templateHTML := readRepoFile(t, test.path)

			fields, err := ExtractEditableFields(templateHTML)
			if err != nil {
				t.Fatalf("ExtractEditableFields returned error: %v", err)
			}

			assertFieldValue(t, fields, "primary_cta_text", FieldTypeText, test.ctaText)
			assertFieldValue(t, fields, "primary_cta_url", FieldTypeURL, test.ctaURL)
			assertFieldValue(t, fields, "primary_cta_width_px", FieldTypeNumber, 196)
			if fields["primary_cta_text"].Order >= fields["primary_cta_url"].Order {
				t.Fatalf("expected CTA text to be ordered before CTA URL")
			}

			rendered, err := RenderEditableHTMLWithMetadata(templateHTML, fields, RenderMetadata{
				Preheader: test.preheader,
			})
			if err != nil {
				t.Fatalf("RenderEditableHTML returned error: %v", err)
			}

			assertContains(t, rendered, `href="`+test.ctaURL+`"`)
			assertContains(t, rendered, `width: 196px`)
			assertContains(t, rendered, test.ctaText)
		})
	}
}

func TestAllEnglishSeedTemplatesExtractAndRender(t *testing.T) {
	paths := readRepoGlob(t, "db/seeds/emails/**/**/en.html")
	if len(paths) == 0 {
		t.Fatal("expected English seed templates")
	}

	for _, path := range paths {
		t.Run(strings.TrimPrefix(path, "db/seeds/emails/"), func(t *testing.T) {
			templateHTML := readRepoFile(t, path)

			fields, err := ExtractEditableFields(templateHTML)
			if err != nil {
				t.Fatalf("ExtractEditableFields returned error: %v", err)
			}

			for _, key := range []string{
				"hero_banner_src",
				"hero_banner_alt",
				"intro_body",
				"primary_cta_text",
				"primary_cta_url",
				"primary_cta_width_px",
				"signature_greeting",
				"signature_sender",
				"footer_logo_src",
				"footer_logo_alt",
				"footer_copyright",
				"footer_legal_text",
				"privacy_policy_link_text",
				"privacy_policy_link_url",
			} {
				if _, ok := fields[key]; !ok {
					t.Fatalf("expected editable field %q", key)
				}
			}

			if _, err := RenderEditableHTMLWithMetadata(templateHTML, fields, RenderMetadata{
				Preheader: "Seed preheader",
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

	if strings.HasPrefix(path, "db/seeds/emails/") {
		t.Skipf("seed email %s is not in this checkout", path)
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
		seedDir := filepath.Join(dir, "db", "seeds", "emails")
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

	t.Skip("seed emails are not in this checkout")
	return nil
}
