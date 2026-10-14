package portal

import (
	"strings"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

const reconstructTemplate = `<!doctype html><html lang="en"><head><title>Template subject</title><style>td{color:red}</style></head><body>
<div style="display:none"><span data-email-preheader=""></span>&zwnj;&nbsp;</div>
<table><tr><td><img src="https://x/banner-en.jpg" alt="Banner" data-review-block="hero_banner" data-edit-attr-src="hero_banner_src" data-edit-attr-alt="hero_banner_alt"></td></tr>
<tr><td data-review-block="intro_body" data-edit-text="intro_body">Hello,<br><br>Template text.</td></tr>
<tr><td><!--[if mso]><v:roundrect href="{{ primary_cta_url }}">{{ primary_cta_text }}</v:roundrect><![endif]--><!--[if !mso]><!-- --><a href="https://x/cta" data-review-block="primary_cta" data-edit-text="primary_cta_text" data-edit-attr-href="primary_cta_url">Click</a><!--<![endif]--></td></tr>
<tr><td data-review-block="signature"><strong data-edit-text="signature_sender">Team</strong></td></tr>
<tr><td data-edit-text="footer_copyright">© {{=date('Y')}} Example.</td></tr></table></body></html>`

// What the deal timeline keeps: no head, no data-* attributes, values of the
// real email, and the broken non-Outlook comment around the button.
const reconstructSent = `<div style="display:none">&zwnj;&nbsp;</div>
<table><tr><td><img src="https://x/banner-br.jpg" alt="Faixa"></td></tr>
<tr><td>Olá,<br><br>Texto <strong>real</strong>.</td></tr>
<tr><td><!--[if mso]> Agendar <![endif]--> <!--[if !mso]> <a href="https://calendly.com/m">Agendar reunião</a> &lt;!--<![endif]--></td></tr>
<tr><td><strong>Gustavo</strong></td></tr>
<tr><td>© 2026 Example.</td></tr></table>`

func TestReconstructRebuildsEditableEmail(t *testing.T) {
	result, err := Reconstruct(reconstructSent, reconstructTemplate, "Assunto", "br")
	if err != nil {
		t.Fatal(err)
	}
	if !result.FromTemplate || len(result.MissingFields) != 0 {
		t.Fatalf("expected a full template match: %+v", result)
	}

	fields, err := emailedit.ExtractEditableFields(result.HTML)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"hero_banner_src":  "https://x/banner-br.jpg",
		"hero_banner_alt":  "Faixa",
		"primary_cta_text": "Agendar reunião",
		"primary_cta_url":  "https://calendly.com/m",
		"signature_sender": "Gustavo",
		"footer_copyright": "© {{=date('Y')}} Example.",
	}
	// Manual formatting inside a text field is flattened to text.
	if intro := fields["intro_body"].Value.(string); !strings.HasPrefix(intro, "Olá,\n\nTexto real") {
		t.Errorf("intro_body = %q", intro)
	}
	for key, value := range want {
		if got := fields[key].Value; got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
	for _, expected := range []string{
		`<title>Assunto</title>`,
		`lang="pt-BR"`,
		`href="https://calendly.com/m">Agendar reunião</v:roundrect>`,
		"&zwnj;",
	} {
		if !strings.Contains(result.HTML, expected) {
			t.Errorf("result is missing %q", expected)
		}
	}
	if strings.Contains(result.HTML, "Template text") {
		t.Error("template copy leaked into the result")
	}
}

func TestReconstructKeepsSentStructureWhenBlocksDiffer(t *testing.T) {
	withoutCTA := strings.Replace(reconstructSent,
		`<tr><td><!--[if mso]> Agendar <![endif]--> <!--[if !mso]> <a href="https://calendly.com/m">Agendar reunião</a> &lt;!--<![endif]--></td></tr>`, "", 1)
	result, err := Reconstruct(withoutCTA, reconstructTemplate, "Assunto", "br")
	if err != nil {
		t.Fatal(err)
	}
	if result.FromTemplate || len(result.Warnings) == 0 {
		t.Fatalf("expected the sent structure with a warning: %+v", result)
	}
	if strings.Contains(result.HTML, ">Click<") {
		t.Error("the template button must not be added to an email that has none")
	}
	if SkeletonSimilarity(withoutCTA, reconstructTemplate) >= SkeletonSimilarity(reconstructSent, reconstructTemplate) {
		t.Error("a missing block should lower the structural similarity")
	}
}
