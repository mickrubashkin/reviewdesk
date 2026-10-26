package ai

import (
	"strings"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

func TestBuildTranslationFieldsSkipsKeysTheTranslationLacks(t *testing.T) {
	changes := []emails.TranslationChange{
		{Key: "subject", Before: "Hi", After: "Hello"},
		{Key: "body_intro", Before: "Book a call", After: "Book a 15-min call"},
		{Key: "signature", Before: "Alice", After: "Alice K."},
	}
	current := map[string]string{"subject": "Hallo", "body_intro": "Termin buchen", "signature_sender": "Alice"}

	fields, skipped := buildTranslationFields(changes, current)
	if len(fields) != 2 || fields[1].Key != "body_intro" || fields[1].Current != "Termin buchen" || fields[1].EnAfter != "Book a 15-min call" {
		t.Fatalf("unexpected fields %+v", fields)
	}
	if strings.Join(skipped, ",") != "signature" {
		t.Fatalf("expected signature to be skipped, got %v", skipped)
	}
}

func TestCheckSuggestionGuardsBitrixExpressions(t *testing.T) {
	field := TranslationField{EnAfter: "Book at {{Responsible person: Calendly onboarding link}} or call {{Responsible person > Phone}}."}

	field.Suggestion = "Buchen Sie unter {{Responsible person > Phone}} oder {{Responsible person: Calendly onboarding link}}."
	if problem := checkSuggestion(field); problem != "" {
		t.Fatalf("expected reordered but intact expressions to pass, got %q", problem)
	}
	field.Suggestion = "Buchen Sie unter {{Verantwortliche Person: Calendly-Link}}."
	if checkSuggestion(field) == "" {
		t.Fatal("expected a translated expression to be flagged")
	}
	field.Suggestion = " "
	if checkSuggestion(field) == "" {
		t.Fatal("expected an empty suggestion to be flagged")
	}
}
