package ai

import (
	"strings"
	"testing"
)

func TestNormalizeGlossary(t *testing.T) {
	terms, err := normalizeGlossary([]GlossaryTerm{
		{Term: " Partner Portal ", Translations: map[string]string{"de": "  ", "fr": "Portail"}},
		{Term: "Kickstart Bonus", Translations: map[string]string{"de": " Kickstart-Bonus "}, Note: " strict "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if terms[0].Term != "Partner Portal" || len(terms[0].Translations) != 0 {
		t.Fatalf("expected empty and unknown languages dropped, got %+v", terms[0])
	}
	if terms[1].Translations["de"] != "Kickstart-Bonus" || terms[1].Note != "strict" {
		t.Fatalf("expected trimmed values, got %+v", terms[1])
	}

	if _, err := normalizeGlossary([]GlossaryTerm{{Term: "Silver"}, {Term: "silver"}}); err == nil {
		t.Fatal("expected a duplicate term in another case to be rejected")
	}
	if _, err := normalizeGlossary([]GlossaryTerm{{Term: " "}}); err == nil {
		t.Fatal("expected an empty term to be rejected")
	}
}

func TestGlossaryForFieldsMatchesWholeWords(t *testing.T) {
	terms := []GlossaryTerm{{Term: "Gold"}, {Term: "partner portal"}, {Term: "Kickstart Bonus"}, {Term: "Bitrix24"}}
	fields := []TranslationField{
		{EnBefore: "A golden chance", EnAfter: "A Golden chance in the Partner Portal."},
		{EnBefore: "Claim the Kickstart Bonus", EnAfter: "Claim it"},
	}
	var got []string
	for _, term := range glossaryForFields(terms, fields) {
		got = append(got, term.Term)
	}
	if strings.Join(got, ",") != "partner portal,Kickstart Bonus" {
		t.Fatalf("unexpected terms %v", got)
	}
}

func TestGlossaryInstructions(t *testing.T) {
	if glossaryInstructions("de", nil) != "" {
		t.Fatal("expected no glossary block without terms")
	}
	text := glossaryInstructions("de", []GlossaryTerm{
		{Term: "Partner Portal", Translations: map[string]string{"es": "Portal de socios"}},
		{Term: "Kickstart Bonus", Translations: map[string]string{"de": "Kickstart-Bonus"}, Note: "never Startbonus"},
	})
	for _, want := range []string{`- "Partner Portal": keep in English`, `- "Kickstart Bonus": "Kickstart-Bonus" (never Startbonus)`, "German"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %s", want, text)
		}
	}
	if strings.Contains(text, "Portal de socios") {
		t.Fatalf("expected only the German wording, got %s", text)
	}
}
