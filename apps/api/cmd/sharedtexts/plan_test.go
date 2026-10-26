package main

import (
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

func email(id, language string, fields map[string]string) Email {
	editable := emailedit.EditableFields{}
	for key, value := range fields {
		editable[key] = emailedit.EditableField{Type: "text", Value: value}
	}
	return Email{ID: id, Place: id, Language: language, Fields: editable}
}

func TestBuildPlanLinksMajorityAndKeepsOwnTexts(t *testing.T) {
	emails := []Email{
		email("en1", "en", map[string]string{"footer_copyright": "© Acme", "signature": "Team"}),
		email("en2", "en", map[string]string{"footer_copyright": "© Acme", "signature": "Jane"}),
		email("en3", "en", map[string]string{"footer_copyright": "© Old"}),
		email("de1", "de", map[string]string{"footer_copyright": "© Acme"}),
	}
	existing := map[string]Existing{"signature\x1fen": {Type: "text", Value: "Team"}}

	plan := BuildPlan(emails, existing)

	items := map[string]Item{}
	for _, item := range plan.Items {
		items[item.Key+" "+item.Language] = item
	}
	copyright := items["footer_copyright en"]
	if copyright.Value != "© Acme" || len(copyright.Link) != 2 || len(copyright.Own) != 1 || copyright.Exists {
		t.Errorf("copyright: %+v", copyright)
	}
	signature := items["signature en"]
	if !signature.Exists || len(signature.Link) != 1 || len(signature.Own) != 1 {
		t.Errorf("an existing shared text must be used as is: %+v", signature)
	}
	if _, ok := items["footer_copyright de"]; ok {
		t.Error("a language with one email must be skipped")
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Language != "de" {
		t.Errorf("skipped: %+v", plan.Skipped)
	}
}

func TestBuildPlanSkipsWithoutMajority(t *testing.T) {
	emails := []Email{
		email("a", "es", map[string]string{"footer_copyright": "A"}),
		email("b", "es", map[string]string{"footer_copyright": "B"}),
	}
	plan := BuildPlan(emails, nil)
	if len(plan.Items) != 0 || len(plan.Skipped) != 1 {
		t.Fatalf("expected the key to be skipped, got %+v", plan)
	}
}
