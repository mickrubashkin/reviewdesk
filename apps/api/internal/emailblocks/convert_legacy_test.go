package emailblocks

import (
	"strings"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// The emails the first set of blocks could not build: a paragraph with a
// link after the button, lists, the login details. They read the same from
// the new blocks; only the markup differs.
func TestConvertLegacyLayouts(t *testing.T) {
	cases := []struct {
		seed   string
		layout string
		fields map[string]string
	}{
		{
			seed:   "01_registered/01_application-received",
			layout: "hero_banner,intro_body,login,body_text,primary_cta,signature,footer",
			fields: map[string]string{"body_text": "To continue, log in to your partner account and upload the required documents.\nThis helps us verify your application faster and approve your account without delays."},
		},
		{
			seed:   "06_partner-app-installed/01-next-step",
			layout: "hero_banner,intro_body,primary_cta,closing_text,signature,footer",
			fields: map[string]string{"closing_text": "If you're not sure where to start, you can **[book a quick call](https://calendly.com/alice/30min)** — we'll walk you through your first lead."},
		},
		{
			seed:   "10_silver/01-next-step",
			layout: "hero_banner,intro_body,list,body_text,primary_cta,signature,footer",
			fields: map[string]string{
				"list_heading": "Here's what to focus on next:",
				"list_items":   "- keep working with leads consistently\n- increase your deal volume and size\n- build long-term client relationships",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.seed, func(t *testing.T) {
			conversion := convertSeed(t, c.seed, nil)
			if conversion.Match != MatchContent {
				t.Fatalf("expected the same content, got %s: %s", conversion.Match, conversion.Reason)
			}
			if got := strings.Join(conversion.Layout.Keys(), ","); got != c.layout {
				t.Fatalf("blocks: got %s, want %s", got, c.layout)
			}
			for key, want := range c.fields {
				if got := conversion.Fields[key].Value; got != want {
					t.Errorf("%s: got %q, want %q", key, got, want)
				}
			}
		})
	}

	// The other emails of these kinds, with their typos and key variants.
	for _, seed := range []string{
		"04_nfr-installed/01-next-step", "04_nfr-installed/02-follow-up-1",
		"06_partner-app-installed/02-follow-up-1", "06_partner-app-installed/03-follow-up-2", "06_partner-app-installed/04-last-call",
		"08_second-sale/02-follow-up-1", "09_silver-prequalified/01-next-step",
	} {
		t.Run(seed, func(t *testing.T) {
			if conversion := convertSeed(t, seed, nil); conversion.Match != MatchContent {
				t.Fatalf("expected the same content, got %s: %s", conversion.Match, conversion.Reason)
			}
		})
	}
}

func TestLegacyFields(t *testing.T) {
	template := `<table><tr><td data-edit-text="primary_cta_text">Go</td></tr><tr><td>` +
		`<span data-edit-text="body_outro_001">Not sure? You can</span> ` +
		`<a href="https://example.com/call" style="font-weight: 700" data-edit-text="secondary_cta">book a call</a>` +
		`<span data-edit-text="body_outro_002">.</span><br /><br /><span data-edit-text="signatrue">Bye</span>` +
		`</td></tr><tr><td data-edit-text="boy_list_heading">Steps</td></tr>` +
		`<tr><td data-edit-text="body-list-item_002">two</td></tr><tr><td data-edit-text="body-list-item_001">one</td></tr>` +
		`<tr><td>📺 <a href="https://example.com/v" style="font-weight: 700" data-edit-text="list-item-link_003_text" ` +
		`data-edit-attr-href="list-item-link_003_url">watch</a> <span data-edit-text="body-list-item_003">the video.</span></td></tr></table>`
	fields, err := emailedit.ExtractEditableFields(template)
	if err != nil {
		t.Fatal(err)
	}
	renamed, moved, err := legacyFields(template, fields, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"primary_cta_text": "Go",
		"closing_text":     "Not sure? You can **[book a call](https://example.com/call)**.",
		"signature":        "Bye",
		"list_heading":     "Steps",
		"list_items":       "- one\n- two\n- 📺 **[watch](https://example.com/v)** the video.",
	}
	if len(renamed) != len(want) {
		t.Fatalf("got %+v", renamed)
	}
	for key, value := range want {
		if renamed[key].Value != value {
			t.Errorf("%s: got %q, want %q", key, renamed[key].Value, value)
		}
	}
	if renamed["closing_text"].Format != emailedit.FormatMarkdown || renamed["list_items"].Format != emailedit.FormatMarkdown {
		t.Error("the merged text and the list are Markdown")
	}
	for from, to := range map[string]string{
		"body_outro_001": "closing_text", "secondary_cta": "closing_text", "signatrue": "signature",
		"boy_list_heading": "list_heading", "body-list-item_001": "list_items", "list-item-link_003_url": "list_items",
	} {
		if moved[from] != to {
			t.Errorf("%s moved to %q, want %q", from, moved[from], to)
		}
	}
}
