package emailedit

import (
	"strings"
	"testing"
)

func TestRenderEditableHTML(t *testing.T) {
	templateHTML := `
		<div>
			<!--[if mso]>
				<v:roundrect href="{{ primary_cta_url }}" style="height:43px;width:{{ primary_cta_width_px }}px;">
					<center>{{ primary_cta_text }}</center>
				</v:roundrect>
			<![endif]-->
			<a href="https://example.com/old" style="display:block; width: 196px;" data-edit-text="primary_cta_text" data-edit-attr-href="primary_cta_url" data-edit-style-width-px="primary_cta_width_px">Old CTA</a>
		</div>
	`
	fields := EditableFields{
		"primary_cta_text": {
			Type:  FieldTypeText,
			Value: "Start now",
		},
		"primary_cta_url": {
			Type:  FieldTypeURL,
			Value: "https://example.com/start",
		},
		"primary_cta_width_px": {
			Type:  FieldTypeNumber,
			Value: 220,
		},
	}

	rendered, err := RenderEditableHTML(templateHTML, fields)
	if err != nil {
		t.Fatalf("RenderEditableHTML returned error: %v", err)
	}

	assertContains(t, rendered, `href="https://example.com/start"`)
	assertContains(t, rendered, `width: 220px`)
	assertContains(t, rendered, `Start now`)
	if strings.Contains(rendered, "{{ primary_cta") {
		t.Fatalf("rendered HTML still contains CTA markers:\n%s", rendered)
	}
}

func TestRenderEditableHTMLPreservesBlankEditorLines(t *testing.T) {
	templateHTML := `<p data-edit-text="body">Old body</p>`
	fields := EditableFields{
		"body": {
			Type:  FieldTypeText,
			Value: "First paragraph\n\nSecond paragraph",
		},
	}

	rendered, err := RenderEditableHTML(templateHTML, fields)
	if err != nil {
		t.Fatalf("RenderEditableHTML returned error: %v", err)
	}

	assertContains(t, rendered, `First paragraph<br/><br/>Second paragraph`)
}

func TestRenderEditableHTMLCollapsesRepeatedBlankEditorLines(t *testing.T) {
	templateHTML := `<p data-edit-text="body">Old body</p>`
	fields := EditableFields{
		"body": {
			Type:  FieldTypeText,
			Value: "First paragraph\n\n\nSecond paragraph",
		},
	}

	rendered, err := RenderEditableHTML(templateHTML, fields)
	if err != nil {
		t.Fatalf("RenderEditableHTML returned error: %v", err)
	}

	assertContains(t, rendered, `First paragraph<br/><br/>Second paragraph`)
	if strings.Contains(rendered, `<br/><br/><br/>`) {
		t.Fatalf("rendered HTML should not include repeated blank lines:\n%s", rendered)
	}
}

func TestRenderEditableHTMLRejectsUnsafeURLScheme(t *testing.T) {
	templateHTML := `<a href="https://example.com" data-edit-attr-href="cta_url">CTA</a>`
	fields := EditableFields{
		"cta_url": {
			Type:  FieldTypeURL,
			Value: "javascript:alert(1)",
		},
	}

	_, err := RenderEditableHTML(templateHTML, fields)
	if err == nil {
		t.Fatal("RenderEditableHTML expected error for unsafe URL scheme")
	}
}

func TestRenderEditableHTMLAllowsMailtoURLScheme(t *testing.T) {
	templateHTML := `<a href="mailto:old@example.com" data-edit-attr-href="contact_url">Contact us</a>`
	fields := EditableFields{
		"contact_url": {
			Type:  FieldTypeURL,
			Value: "mailto:partners@example.com",
		},
	}

	rendered, err := RenderEditableHTML(templateHTML, fields)
	if err != nil {
		t.Fatalf("RenderEditableHTML returned error: %v", err)
	}

	assertContains(t, rendered, `href="mailto:partners@example.com"`)
}

func TestRenderEditableHTMLRejectsMailtoImageURL(t *testing.T) {
	templateHTML := `<img src="https://example.com/image.png" data-edit-attr-src="image_url">`
	fields := EditableFields{
		"image_url": {
			Type:  FieldTypeImage,
			Value: "mailto:partners@example.com",
		},
	}

	_, err := RenderEditableHTML(templateHTML, fields)
	if err == nil {
		t.Fatal("RenderEditableHTML expected error for mailto image URL")
	}
}

func TestRenderEditableHTMLWithMetadataUpdatesPreheaderTarget(t *testing.T) {
	templateHTML := `
		<div>
			<span data-email-preheader>Old preheader</span>
			&nbsp;&zwnj;&nbsp;
		</div>
	`

	rendered, err := RenderEditableHTMLWithMetadata(templateHTML, EditableFields{}, RenderMetadata{
		Preheader: "New preheader",
	})
	if err != nil {
		t.Fatalf("RenderEditableHTMLWithMetadata returned error: %v", err)
	}

	assertContains(t, rendered, `data-email-preheader="">New preheader</span>`)
	if strings.Contains(rendered, "Old preheader") {
		t.Fatalf("rendered HTML still contains old preheader:\n%s", rendered)
	}
}

func assertContains(t *testing.T, value string, substring string) {
	t.Helper()

	if !strings.Contains(value, substring) {
		t.Fatalf("expected rendered HTML to contain %q:\n%s", substring, value)
	}
}

func TestRenderKeepsBitrixExpressionsAndInvisibleEntities(t *testing.T) {
	template := `<!doctype html><html><body>` +
		`<div style="display:none">&zwnj;&nbsp;&zwnj;&nbsp;</div>` +
		`<td data-edit-text="footer_copyright">© {{=date('Y')}} Example.</td></body></html>`
	fields, err := ExtractEditableFields(template)
	if err != nil {
		t.Fatal(err)
	}

	out, err := RenderEditableHTML(template, fields)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"{{=date('Y')}}", "&zwnj;&nbsp;&zwnj;&nbsp;"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered HTML lost %q:\n%s", want, out)
		}
	}
	if strings.ContainsAny(out, "\u200c\u00a0") {
		t.Errorf("rendered HTML contains raw invisible characters:\n%q", out)
	}
}

func TestRenderKeepsBitrixFieldsInLinksAndText(t *testing.T) {
	template := `<!doctype html><html><body>` +
		`<p data-edit-text="signature_sender">{{Ответственный > Имя}}</p>` +
		`<a data-edit-attr-href="primary_cta_url" data-edit-text="primary_cta_text" href="https://example.com">Book a call</a>` +
		`<a data-edit-attr-href="secondary_cta_url" href="https://example.com">More</a>` +
		`</body></html>`
	fields, err := ExtractEditableFields(template)
	if err != nil {
		t.Fatal(err)
	}
	fields["primary_cta_url"] = EditableField{Type: FieldTypeURL, Value: "{{Responsible person > Booking link}}"}
	fields["secondary_cta_url"] = EditableField{Type: FieldTypeURL, Value: "https://example.com/?manager={=Document:ASSIGNED_BY_ID}"}

	out, err := RenderEditableHTML(template, fields)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`>{{Ответственный > Имя}}</p>`,
		`href="{{Responsible person > Booking link}}"`,
		`href="https://example.com/?manager={=Document:ASSIGNED_BY_ID}"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered HTML lost %q:\n%s", want, out)
		}
	}
}

func TestRenderLeavesUnknownMarkersForBitrix(t *testing.T) {
	template := `<!doctype html><html><body><p data-edit-text="intro_body">Hi</p></body></html>`
	fields, err := ExtractEditableFields(template)
	if err != nil {
		t.Fatal(err)
	}
	fields["intro_body"] = EditableField{Type: FieldTypeText, Value: "Hi {{Responsible}}"}

	out, err := RenderEditableHTML(template, fields)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Hi {{Responsible}}") {
		t.Errorf("rendered HTML lost the Bitrix field:\n%s", out)
	}
}

func TestRenderStillRejectsBrokenLinksAroundExpressions(t *testing.T) {
	template := `<!doctype html><html><body><a data-edit-attr-href="primary_cta_url" href="https://example.com">Go</a></body></html>`
	fields, err := ExtractEditableFields(template)
	if err != nil {
		t.Fatal(err)
	}
	fields["primary_cta_url"] = EditableField{Type: FieldTypeURL, Value: "javascript:{{Responsible}}"}

	if _, err := RenderEditableHTML(template, fields); err == nil {
		t.Fatal("expected a javascript: link to be rejected")
	}
}

func TestFindBitrixExpressionsSkipsTemplateMarkers(t *testing.T) {
	got := FindBitrixExpressions(`{{primary_cta_url}} {{=date('Y')}} {{Responsible > Name}} {=Document:TITLE} {{=date('Y')}}`)
	want := []string{"{{=date('Y')}}", "{{Responsible > Name}}", "{=Document:TITLE}"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("FindBitrixExpressions = %q, want %q", got, want)
	}
}
