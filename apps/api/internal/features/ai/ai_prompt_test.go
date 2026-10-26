package ai

import (
	"encoding/json"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
)

func TestBuildEmailAnalysisInputOmitsFooterLinksAndFooterPrimaryCTA(t *testing.T) {
	contentParts := EmailContentParts{
		PrimaryCTA: "Privacy policy",
		BodyText:   "Body copy. Acme. All rights reserved. Privacy policy",
		Links:      []string{"Privacy policy", "Read docs"},
		LinkGroups: emailtext.LinkGroups{
			Primary: []string{"Privacy policy"},
			Support: []string{"Contact support"},
			Footer:  []string{"Unsubscribe"},
			Other:   []string{"Read docs"},
		},
	}
	contentPartsBytes, err := json.Marshal(contentParts)
	if err != nil {
		t.Fatal(err)
	}
	contentPartsText := string(contentPartsBytes)

	input := buildEmailAnalysisInput("", "", "Russian", EmailDetail{
		Title:        "Test email",
		ContentParts: &contentPartsText,
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		t.Fatalf("expected valid JSON input, got %v", err)
	}

	emailInput := payload["email"].(map[string]any)
	if payload["response_language"] != "Russian" {
		t.Fatalf("expected response language in AI input, got %#v", payload["response_language"])
	}
	if emailInput["primary_cta"] != "" {
		t.Fatalf("expected footer primary CTA to be removed, got %q", emailInput["primary_cta"])
	}
	if emailInput["body_text"] != "Body copy." {
		t.Fatalf("expected footer text to be removed from body, got %q", emailInput["body_text"])
	}

	links := emailInput["links"].(map[string]any)
	if _, ok := links["footer"]; ok {
		t.Fatalf("expected footer field to be omitted from AI links input, got %#v", links["footer"])
	}

	assertStringSlice(t, links["primary"], []string{})
	assertStringSlice(t, links["support"], []string{"Contact support"})
	assertStringSlice(t, links["other"], []string{"Read docs"})
}

func assertStringSlice(t *testing.T, value any, expected []string) {
	t.Helper()

	values, ok := value.([]any)
	if !ok {
		t.Fatalf("expected JSON array, got %#v", value)
	}
	if len(values) != len(expected) {
		t.Fatalf("expected %#v, got %#v", expected, values)
	}

	for index, expectedValue := range expected {
		if values[index] != expectedValue {
			t.Fatalf("expected %#v, got %#v", expected, values)
		}
	}
}
