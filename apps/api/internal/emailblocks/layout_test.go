package emailblocks

import (
	"strings"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

func TestInstanceHTMLRenamesFieldsOfARepeat(t *testing.T) {
	block := Block{
		Key: "cta",
		HTML: `<tr><td data-review-block="cta"><!--[if mso]><v:roundrect href="{{ cta_url }}">{{ cta_text }} {{Responsible}}</v:roundrect><![endif]-->` +
			`<a href="https://example.com" data-edit-attr-href='cta_url' data-edit-text="cta_text" data-other="cta_text">Go</a></td></tr>`,
		Fields: []Field{{Key: "cta_url"}, {Key: "cta_text"}},
	}
	got := InstanceHTML(block, "2")
	for _, want := range []string{
		`data-review-block="cta_2"`,
		`href="{{ cta_url_2 }}"`,
		`{{ cta_text_2 }}`,
		`{{Responsible}}`,
		`data-edit-attr-href='cta_url_2'`,
		`data-edit-text="cta_text_2"`,
		`data-other="cta_text"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	if InstanceHTML(block, "") != block.HTML {
		t.Error("the first copy must keep the block's HTML")
	}
}

func TestLayoutValidate(t *testing.T) {
	library := Defaults()
	for _, test := range []struct {
		name   string
		layout Layout
		want   string
	}{
		{"empty", Layout{}, "at least one block"},
		{"frame", Layout{{Block: FrameKey}}, "frame"},
		{"unknown", Layout{{Block: "nope"}}, `no block "nope"`},
		{"repeat without suffix", Layout{{Block: "intro_body"}, {Block: "intro_body"}}, "twice"},
		{"bad suffix", Layout{{Block: "intro_body", Suffix: "Two!"}}, "suffix"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.layout.Validate(library)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected an error with %q, got %v", test.want, err)
			}
		})
	}
	if err := (Layout{{Block: "intro_body"}, {Block: "intro_body", Suffix: "2"}}).Validate(library); err != nil {
		t.Fatalf("a repeat with a suffix is fine: %v", err)
	}
}

func TestLayoutBuildAndFields(t *testing.T) {
	library := Defaults()
	layout := Layout{{Block: "intro_body"}, {Block: "intro_body", Suffix: "2"}, {Block: "primary_cta"}, {Block: "footer"}}
	template, err := layout.Build(library)
	if err != nil {
		t.Fatal(err)
	}
	previous := emailedit.EditableFields{
		"intro_body":      {Type: emailedit.FieldTypeText, Value: "Kept"},
		"primary_cta_url": {Type: emailedit.FieldTypeText, Value: "wrong type, replaced"},
		"gone":            {Type: emailedit.FieldTypeText, Value: "dropped"},
	}
	fields, err := layout.Fields(library, template, previous)
	if err != nil {
		t.Fatal(err)
	}
	if fields["intro_body"].Value != "Kept" {
		t.Errorf("a kept field must keep its value: %+v", fields["intro_body"])
	}
	if _, ok := fields["intro_body_2"]; !ok {
		t.Error("the repeat must have its own field")
	}
	if fields["primary_cta_url"].Type != emailedit.FieldTypeURL {
		t.Errorf("a field of another type takes the block's: %+v", fields["primary_cta_url"])
	}
	if _, ok := fields["gone"]; ok {
		t.Error("a field of no block must go")
	}
	if !fields["footer_legal_text"].Shared || fields["intro_body"].Shared {
		t.Error("new fields take the block's shared flag")
	}
	if fields["intro_body"].Order >= fields["intro_body_2"].Order {
		t.Error("fields follow the layout's order")
	}
	if _, err := emailedit.RenderEditableHTML(template, fields); err != nil {
		t.Fatalf("the email must render: %v", err)
	}
}

func TestDiff(t *testing.T) {
	before := Layout{{Block: "hero_banner"}, {Block: "intro_body"}, {Block: "primary_cta"}}
	after := Layout{{Block: "primary_cta"}, {Block: "intro_body"}, {Block: "intro_body", Suffix: "2"}}
	changes := Diff(before, after)
	if len(changes) != 2 ||
		changes[0] != (BlockChange{Action: "removed", Block: "hero_banner", ID: "hero_banner"}) ||
		changes[1] != (BlockChange{Action: "added", Block: "intro_body", ID: "intro_body_2"}) {
		t.Fatalf("unexpected changes: %+v", changes)
	}
	if len(Diff(nil, before)) != 3 {
		t.Fatal("switching to blocks adds every block")
	}
}
