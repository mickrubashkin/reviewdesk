package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFindUsesCoversHrefsTextAndFields(t *testing.T) {
	email := Email{ID: "e1", TemplateHTML: `<html><body><table><tr><td>
		<p>Please book a short intro call with our team so we can start.</p>
		<p><a href="https://calendly.com/alice/30min?month=1">Book a Call</a></p>
		<p>Or open https://calendly.com/alice/intro directly. Team: <a href="https://calendly.com/d/abc-123">here</a></p>
		<!--[if mso]><v:roundrect href="https://calendly.com/alice/vml"></v:roundrect><![endif]-->
		<p><a href="https://calendly.com/alice/x">https://calendly.com/alice/x</a></p>
	</td></tr></table></body></html>`}
	if err := json.Unmarshal([]byte(`{"cta":{"value":"https://calendly.com/alice/30min"},"n":{"value":3}}`), &email.Fields); err != nil {
		t.Fatal(err)
	}

	uses, err := FindUses([]Email{email})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, use := range uses {
		got = append(got, use.Source+" "+use.Link+" "+plan(use.Link))
	}
	want := []string{
		"template https://calendly.com/alice/30min?month=1 replace",
		"template text https://calendly.com/alice/intro replace",
		"template https://calendly.com/d/abc-123 keep (shared event)",
		"Outlook button https://calendly.com/alice/vml replace",
		"template https://calendly.com/alice/x replace",
		"link text https://calendly.com/alice/x replace",
		"field cta https://calendly.com/alice/30min replace",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(uses[0].Context, "intro call") {
		t.Fatalf("expected the button to carry the sentence around it, got %q", uses[0].Context)
	}
}

func TestReplaceLinksKeepsSharedEvents(t *testing.T) {
	expression := "{{Responsible person: Calendly onboarding link}}"
	got := replaceLinks("https://calendly.com/alice/30min?month=2026-10 or https://calendly.com/d/abc-123", expression)
	want := expression + " or https://calendly.com/d/abc-123"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
