package emailedit

import (
	"strings"
	"testing"
)

const markdownTemplate = `<table><tr><td style="font-size: 16px; color: #063683" data-edit-text="body">Old</td></tr></table>`

func renderBody(t *testing.T, format, value string) string {
	t.Helper()
	rendered, err := RenderEditableHTML(markdownTemplate, EditableFields{
		"body": {Type: FieldTypeText, Value: value, Format: format},
	})
	if err != nil {
		t.Fatalf("render %q: %v", value, err)
	}
	return rendered
}

func TestMarkdownWithoutMarkupRendersLikePlainText(t *testing.T) {
	for _, value := range []string{
		"Dear Partner,\n\nYou've closed your second deal — you're on track.\nKeep going!",
		"Line one   \nLine two\n\n\n\nAfter blank lines",
		"\n\nLeading blank lines and trailing ones\n\n",
		"Hello {{Contact > Name}}, your manager is {=Document:ASSIGNED_BY_PRINTABLE}.",
		"Prices & terms: 5 < 6 > 4, \"quoted\" and 'single'",
		"An unmatched * star and a lone _ underscore, 50% off (today).",
		"Copyright © {{=date('Y')}} Bitrix24",
	} {
		if plain, markdown := renderBody(t, "", value), renderBody(t, FormatMarkdown, value); plain != markdown {
			t.Errorf("%q\nplain:    %s\nmarkdown: %s", value, plain, markdown)
		}
	}
}

func TestMarkdownFormatting(t *testing.T) {
	rendered := renderBody(t, FormatMarkdown, strings.Join([]string{
		"# Next steps",
		"",
		"Do **this** and *that*, then [open the portal](https://partners.bitrix24.com/private/).",
		"",
		"- first",
		"- second",
		"",
		"3. third",
		"4. fourth",
	}, "\n"))

	for _, want := range []string{
		`<h2 style="margin: 0 0 12px 0; font-family: Helvetica, Arial, sans-serif; font-size: 22px; line-height: 28px; font-weight: 700; color: #063683;">Next steps</h2>`,
		`Do <strong>this</strong> and <em>that</em>, then <a href="https://partners.bitrix24.com/private/" target="_blank" style="color: #409eef; text-decoration: underline;">open the portal</a>.`,
		`<ul style="margin: 16px 0 16px 0; padding: 0 0 0 24px;"><li style="margin: 0 0 8px 0;">first</li><li style="margin: 0;">second</li></ul>`,
		`<ol style="margin: 16px 0 0 0; padding: 0 0 0 24px;" start="3"><li style="margin: 0 0 8px 0;">third</li><li style="margin: 0;">fourth</li></ol>`,
	} {
		assertContains(t, rendered, want)
	}
}

func TestMarkdownLinkToABitrixExpression(t *testing.T) {
	rendered := renderBody(t, FormatMarkdown, "[Book a call]({{Responsible person > Booking link}}) with {{Responsible person > Name}}")
	assertContains(t, rendered, `<a href="{{Responsible person > Booking link}}" target="_blank"`)
	assertContains(t, rendered, `>Book a call</a> with {{Responsible person > Name}}`)
}

func TestMarkdownKeepsHTMLAsText(t *testing.T) {
	rendered := renderBody(t, FormatMarkdown, "<b>bold?</b> <script>alert(1)</script>\n\n    indented, not code\n\n> not a quote")
	if strings.Contains(rendered, "<b>") || strings.Contains(rendered, "<script>") || strings.Contains(rendered, "<pre") ||
		strings.Contains(rendered, "<blockquote") {
		t.Errorf("markup got through: %s", rendered)
	}
	assertContains(t, rendered, "&lt;b&gt;bold?&lt;/b&gt;")
}

func TestMarkdownRejectsUnsafeLinks(t *testing.T) {
	for _, value := range []string{"[x](javascript:alert(1))", "[x](ftp://example.com)"} {
		_, err := RenderEditableHTML(markdownTemplate, EditableFields{"body": {Type: FieldTypeText, Value: value, Format: FormatMarkdown}})
		if err == nil {
			t.Errorf("%q rendered", value)
		}
		if err := ValidateFormat("body", EditableField{Type: FieldTypeText, Value: value, Format: FormatMarkdown}); err == nil {
			t.Errorf("%q passed validation", value)
		}
	}
	if err := ValidateFormat("body", EditableField{Type: FieldTypeText, Value: "[mail](mailto:a@example.com)", Format: FormatMarkdown}); err != nil {
		t.Errorf("mailto: %v", err)
	}
}

func TestMarkdownNeedsABlockElement(t *testing.T) {
	_, err := RenderEditableHTML(`<a href="https://example.com" data-edit-text="cta">Go</a>`, EditableFields{
		"cta": {Type: FieldTypeText, Value: "**Go**", Format: FormatMarkdown},
	})
	if err == nil {
		t.Error("a Markdown field inside a link rendered")
	}
}

func TestValidateFormat(t *testing.T) {
	if err := ValidateFormat("body", EditableField{Type: FieldTypeText, Value: "x", Format: "html"}); err == nil {
		t.Error("unknown format passed")
	}
	if err := ValidateFormat("url", EditableField{Type: FieldTypeURL, Value: "https://example.com", Format: FormatMarkdown}); err == nil {
		t.Error("a formatted URL field passed")
	}
	if err := ValidateFormat("body", EditableField{Type: FieldTypeText, Value: "plain"}); err != nil {
		t.Error(err)
	}
}
