package emailblocks

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

func convertSeed(t *testing.T, seed string, edit func(string) string) Conversion {
	t.Helper()
	content, err := os.ReadFile("../../../../db/seeds/emails/" + seed + "/en.html")
	if os.IsNotExist(err) {
		t.Skip("seed emails are not in this checkout")
	}
	if err != nil {
		t.Fatal(err)
	}
	template := string(content)
	if edit != nil {
		template = edit(template)
	}
	fields, err := emailedit.ExtractEditableFields(template)
	if err != nil {
		t.Fatal(err)
	}
	return Convert(Defaults(), template, fields, emailedit.RenderMetadata{Subject: "Subject", Preheader: "Preheader"})
}

func TestConvertSeedsWithTheLibraryMarkup(t *testing.T) {
	for _, seed := range librarySeeds {
		t.Run(seed, func(t *testing.T) {
			conversion := convertSeed(t, seed, nil)
			if conversion.Match != MatchSame {
				t.Fatalf("expected the same render, got %s: %s", conversion.Match, conversion.Reason)
			}
			if got := strings.Join(conversion.Layout.Keys(), ","); got != "hero_banner,intro_body,primary_cta,signature,footer" {
				t.Errorf("unexpected blocks: %s", got)
			}
			if len(conversion.Recovered) > 0 || len(conversion.LostReviewBlocks) > 0 {
				t.Errorf("nothing to recover or lose: %+v / %+v", conversion.Recovered, conversion.LostReviewBlocks)
			}
		})
	}
}

func TestConvertRecoversAFixedButtonWidth(t *testing.T) {
	conversion := convertSeed(t, librarySeeds[0], func(template string) string {
		template = strings.Replace(template, ` data-edit-style-width-px="primary_cta_width_px"`, "", 1)
		template = strings.Replace(template, "{{ primary_cta_width_px }}", "196", 1)
		return strings.Replace(template, "width: 220px;", "width: 196px;", 1)
	})
	if conversion.Match != MatchSame {
		t.Fatalf("expected the same render, got %s: %s", conversion.Match, conversion.Reason)
	}
	if strings.Join(conversion.Recovered, ",") != "primary_cta_width_px" {
		t.Fatalf("the width must come from the old template: %+v", conversion.Recovered)
	}
	if width := conversion.Fields["primary_cta_width_px"]; width.Value != 196 || width.Type != emailedit.FieldTypeNumber {
		t.Errorf("unexpected width field: %+v", width)
	}
}

func TestConvertTellsMarkupFromContent(t *testing.T) {
	cases := map[string]struct {
		edit   func(string) string
		match  Match
		reason string
	}{
		"different padding": {
			edit:   func(s string) string { return strings.Replace(s, "padding: 0 0 0 0;", "padding: 0 0 26px 0;", 1) },
			match:  MatchContent,
			reason: "markup: ",
		},
		"fixed text outside the fields": {
			edit: func(s string) string {
				return strings.Replace(s, `<tr>`, `<tr><td>Fixed text that no field holds</td></tr><tr>`, 2)
			},
			match:  MatchNone,
			reason: "text: ",
		},
		"editor attribute the blocks do not have": {
			edit: func(s string) string {
				return strings.Replace(s, `data-review-block="intro_body"`, `data-review-block="intro_body" data-edit-attr-title="x"`, 1)
			},
			match: MatchSame,
		},
		"unknown field": {
			edit: func(s string) string {
				return strings.Replace(s, `data-edit-text="intro_body"`, `data-edit-text="promo_note"`, 1)
			},
			match:  MatchNone,
			reason: "fields no block has: promo_note",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			conversion := convertSeed(t, librarySeeds[0], c.edit)
			if conversion.Match != c.match || !strings.HasPrefix(conversion.Reason, c.reason) {
				t.Fatalf("expected %s (%q…), got %s: %s", c.match, c.reason, conversion.Match, conversion.Reason)
			}
		})
	}
}

func TestConvertListsReviewBlocksTheBlocksDoNotHave(t *testing.T) {
	conversion := convertSeed(t, librarySeeds[0], func(s string) string {
		return strings.Replace(s, `data-review-block="intro_body"`, `data-review-block="greeting"`, 1)
	})
	if strings.Join(conversion.LostReviewBlocks, ",") != "greeting" {
		t.Fatalf("expected greeting to be lost, got %+v (%s: %s)", conversion.LostReviewBlocks, conversion.Match, conversion.Reason)
	}
}

func TestConvertNeedsFields(t *testing.T) {
	conversion := Convert(Defaults(), "<html><body><p>As is</p></body></html>", emailedit.EditableFields{}, emailedit.RenderMetadata{})
	if conversion.Match != MatchNone || conversion.Reason != "no editable fields" {
		t.Fatalf("unexpected: %+v", conversion)
	}
}

func TestConvertTakesTheOneFieldSignature(t *testing.T) {
	signature := regexp.MustCompile(`(?s)<tr>\s*<td[^>]*data-review-block="signature".*?</tr>`)
	conversion := convertSeed(t, librarySeeds[0], func(s string) string {
		return signature.ReplaceAllString(s, mustDefaultHTML("signature_text"))
	})
	if conversion.Match != MatchSame {
		t.Fatalf("expected the same render, got %s: %s", conversion.Match, conversion.Reason)
	}
	if got := strings.Join(conversion.Layout.Keys(), ","); got != "hero_banner,intro_body,primary_cta,signature_text,footer" {
		t.Errorf("unexpected blocks: %s", got)
	}
}
