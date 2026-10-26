package main

import (
	"bytes"
	"strings"
	"testing"
)

func mail(id, lang, variant, adapt string, sort int, fields map[string]string) Email {
	ef := map[string]FieldValue{}
	for k, v := range fields {
		ef[k] = FieldValue{Type: "text", Value: v}
	}
	return Email{ID: id, Sequence: "b", Stage: "s", SortOrder: sort, Title: "T", Language: lang,
		Variant: variant, AdaptationKey: adapt, AdaptationLabel: adapt, ReviewStatus: "in_review", EditableFields: ef}
}

func TestBuildPlanClassifiesAdaptations(t *testing.T) {
	base := map[string]string{"a": "1", "b": "2", "c": "3", "d": "4", "signature": "Best, Team"}
	sig := map[string]string{"a": "1", "b": "2", "c": "3", "d": "4", "signature_greeting": "Best,", "signature_sender": "Team"}
	override := map[string]string{"a": "1", "b": "2", "c": "3", "d": "CHANGED", "signature": "Best, Team"}
	branch := map[string]string{"a": "x", "b": "y", "c": "z", "d": "4", "signature": "Best, Team"}

	plan := BuildPlan([]Email{
		mail("1", "en", "new", "default", 1, base),
		mail("2", "de", "new", "default", 1, base),
		mail("3", "en", "new", "alice", 1, sig),
		mail("4", "en", "new", "bob", 1, sig),
		mail("5", "en", "new", "carol", 1, override),
		mail("6", "en", "new", "other", 1, branch),
		mail("7", "es", "new", "es-new", 2, base),
		mail("8", "en", "old", "default", 1, base),
		mail("9", "en", "v2", "default", 1, base),
	})

	kinds := map[string]AdaptationReport{}
	for _, a := range plan.Adaptations {
		kinds[a.Key] = a
	}
	if kinds["alice"].Kind != KindSignature {
		t.Errorf("alice: %v", kinds["alice"].Kind)
	}
	if kinds["bob"].Kind != KindDuplicate || kinds["bob"].DuplicateOf != "alice" {
		t.Errorf("bob: %v of %q", kinds["bob"].Kind, kinds["bob"].DuplicateOf)
	}
	if kinds["carol"].Kind != KindOverride {
		t.Errorf("carol: %v", kinds["carol"].Kind)
	}
	if kinds["other"].Kind != KindBranch || kinds["es-new"].Kind != KindBranch {
		t.Errorf("branches: %v %v", kinds["other"].Kind, kinds["es-new"].Kind)
	}
	if plan.Masters != 1 || plan.TranslationsLinked != 1 || plan.ArchivedOld != 1 || len(plan.Experiments) != 1 {
		t.Errorf("unexpected counts: %+v", plan)
	}
	if len(plan.SlotsWithoutMaster) != 1 {
		t.Errorf("slot 2 has no EN master: %v", plan.SlotsWithoutMaster)
	}
	// master + translation + experiment + override + branch + branch
	if plan.ContentBefore != 8 || plan.ContentAfter != 6 {
		t.Errorf("content %d → %d", plan.ContentBefore, plan.ContentAfter)
	}

	var out bytes.Buffer
	WriteReport(&out, plan)
	if !strings.Contains(out.String(), "duplicate of `alice`") {
		t.Errorf("report missing duplicate verdict:\n%s", out.String())
	}
}

func TestTranslationKeyMismatchIsReported(t *testing.T) {
	plan := BuildPlan([]Email{
		mail("1", "en", "new", "default", 1, map[string]string{"a": "1", "signature": "x"}),
		mail("2", "pl", "new", "default", 1, map[string]string{"a": "1", "signature_sender": "x"}),
	})
	if plan.TranslationsLinked != 0 || len(plan.TranslationIssues) != 1 {
		t.Fatalf("expected one issue: %+v", plan.TranslationIssues)
	}
	issue := plan.TranslationIssues[0]
	if issue.Missing[0] != "signature" || issue.Extra[0] != "signature_sender" {
		t.Errorf("unexpected diff: %+v", issue)
	}
}

func TestNearDuplicateAndSignatureExceptions(t *testing.T) {
	base := map[string]string{"a": "1", "b": "2", "c": "3", "signature": "Best, Team"}
	sig := map[string]string{"a": "1", "b": "2", "c": "3", "signature_sender": "Team"}
	var emails []Email
	for i := 1; i <= 10; i++ {
		emails = append(emails, mail("b", "en", "new", "default", i, base))
		emails = append(emails, mail("x", "en", "new", "alice", i, sig))
		variant := sig
		if i == 3 {
			variant = map[string]string{"a": "1", "b": "LIST", "c": "3", "signature_sender": "Team"}
		}
		emails = append(emails, mail("y", "en", "new", "carol", i, variant))
	}
	byKey := map[string]AdaptationReport{}
	for _, a := range BuildPlan(emails).Adaptations {
		byKey[a.Key] = a
	}
	if byKey["alice"].Kind != KindSignature || byKey["alice"].Exceptions != 0 {
		t.Errorf("alice: %+v", byKey["alice"])
	}
	if byKey["carol"].Kind != KindDuplicate || byKey["carol"].DuplicateOf != "alice" || byKey["carol"].Exceptions != 1 {
		t.Errorf("carol should be a near-duplicate of alice with one override: %+v", byKey["carol"])
	}
}
