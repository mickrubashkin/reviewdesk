package emailblocks

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// Seeds with the markup the library was cut from.
var librarySeeds = []string{
	"07_first-sale/01-next-step",
	"07_first-sale/02-follow-up-1",
	"07_first-sale/03-follow-up-2",
	"07_first-sale/04-last-call",
	"08_second-sale/01-next-step",
	"08_second-sale/03-follow-up-2",
	"08_second-sale/04-last-call",
	"09_silver-prequalified/02-follow-up-1",
	"09_silver-prequalified/03-last-call",
}

var (
	titlePattern     = regexp.MustCompile(`(?s)<title>(.*?)</title>`)
	preheaderPattern = regexp.MustCompile(`(?s)<span data-email-preheader>(.*?)</span>`)
	spacePattern     = regexp.MustCompile(`\s+`)
	betweenTags      = regexp.MustCompile(`>\s+<`)
)

func normalizeHTML(s string) string {
	s = strings.ReplaceAll(s, ` data-email-subject=""`, "")
	s = spacePattern.ReplaceAllString(s, " ")
	return strings.TrimSpace(betweenTags.ReplaceAllString(s, "><"))
}

func TestAssembledDefaultsRenderLikeTheTemplate(t *testing.T) {
	library := map[string]string{}
	for _, block := range Defaults() {
		library[block.Key] = block.HTML
	}
	template, err := Assemble(library[FrameKey],
		library["hero_banner"], library["intro_body"], library["primary_cta"], library["signature"], library["footer"])
	if err != nil {
		t.Fatal(err)
	}

	for _, seed := range librarySeeds {
		t.Run(seed, func(t *testing.T) {
			content, err := os.ReadFile("../../../../db/seeds/emails/" + seed + "/en.html")
			if os.IsNotExist(err) {
				t.Skip("seed emails are not in this checkout")
			}
			if err != nil {
				t.Fatal(err)
			}
			original := string(content)
			fields, err := emailedit.ExtractEditableFields(original)
			if err != nil {
				t.Fatal(err)
			}
			metadata := emailedit.RenderMetadata{
				Subject:   strings.TrimSpace(titlePattern.FindStringSubmatch(original)[1]),
				Preheader: strings.TrimSpace(preheaderPattern.FindStringSubmatch(original)[1]),
			}

			want, err := emailedit.RenderEditableHTMLWithMetadata(original, fields, emailedit.RenderMetadata{Preheader: metadata.Preheader})
			if err != nil {
				t.Fatal(err)
			}
			got, err := emailedit.RenderEditableHTMLWithMetadata(template, fields, metadata)
			if err != nil {
				t.Fatal(err)
			}
			if normalizeHTML(got) != normalizeHTML(want) {
				t.Errorf("the email built from blocks differs from the template\n got: %s\nwant: %s",
					firstDifference(normalizeHTML(got), normalizeHTML(want)),
					firstDifference(normalizeHTML(want), normalizeHTML(got)))
			}
		})
	}
}

func firstDifference(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	start := max(i-80, 0)
	end := min(i+120, len(a))
	return "…" + a[start:end] + "…"
}

func TestDefaultsDescribeEveryField(t *testing.T) {
	frame := mustDefaultHTML(FrameKey)
	if err := Validate(FrameKey, frame, ""); err != nil {
		t.Errorf("frame: %v", err)
	}
	for _, block := range Defaults() {
		if block.Key == FrameKey {
			continue
		}
		if err := Validate(block.Key, block.HTML, frame); err != nil {
			t.Errorf("%s: %v", block.Key, err)
		}
		if len(block.Fields) == 0 {
			t.Errorf("%s has no fields", block.Key)
		}
		for _, field := range block.Fields {
			if field.Label == field.Key {
				t.Errorf("%s: field %s is in the HTML but not described in Defaults", block.Key, field.Key)
			}
		}
	}
}

func TestFieldsKeepDescriptionsAndFollowTheHTML(t *testing.T) {
	frame := mustDefaultHTML(FrameKey)
	block := `<tr><td data-review-block="note" data-edit-text="note_text">Hi</td></tr>
<tr><td><a data-edit-attr-href="note_url" data-edit-text="note_link" href="https://a.b">x</a></td></tr>`
	previous := []Field{
		{Key: "note_link", Label: "Link text", MaxLength: 20},
		{Key: "gone", Label: "Removed from the HTML"},
	}

	fields, err := Fields(frame, block, previous)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, field := range fields {
		keys = append(keys, field.Key+":"+field.Type)
	}
	if got := strings.Join(keys, ","); got != "note_text:text,note_link:text,note_url:url" {
		t.Errorf("fields in document order with types from the HTML, got %s", got)
	}
	if fields[1].Label != "Link text" || fields[1].MaxLength != 20 {
		t.Errorf("a known field keeps its description: %+v", fields[1])
	}
	if fields[0].Label != "note_text" {
		t.Errorf("a new field is labelled with its key: %+v", fields[0])
	}
}

func TestValidateRejectsBrokenBlocks(t *testing.T) {
	frame := mustDefaultHTML(FrameKey)
	cases := map[string]struct{ key, html string }{
		"empty":            {"note", "  "},
		"not a row":        {"note", `<p data-review-block="note">Hi</p>`},
		"no review block":  {"note", `<tr><td data-edit-text="note">Hi</td></tr>`},
		"marker in block":  {"note", `<tr><td data-review-block="note">` + BlocksMarker + `</td></tr>`},
		"conflicting keys": {"note", `<tr><td data-review-block="note"><b data-edit-text="a">1</b><i data-edit-text="a">2</i></td></tr>`},
		"frame no marker":  {FrameKey, `<html><body></body></html>`},
		"frame two marks":  {FrameKey, `<html><body>` + BlocksMarker + BlocksMarker + `</body></html>`},
		"frame with field": {FrameKey, `<html><body><p data-edit-text="x">1</p>` + BlocksMarker + `</body></html>`},
	}
	for name, c := range cases {
		if err := Validate(c.key, c.html, frame); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
