package emails

import (
	"strings"
	"testing"
)

const robotHTML = `<!doctype html><html lang="pt-BR"><head><title>Bitrix24: Follow-up da sua candidatura</title></head><body>
<div style="display:none"><span data-email-preheader="">Veja os próximos passos</span></div>
<table><tr><td data-edit-text="intro_body">Olá!<br>Fique à vontade para <strong><a href="https://calendly.com/x">agendar</a></strong>.</td></tr>
<tr><td><strong data-edit-text="signature_sender">Gustavo</strong></td></tr>
<tr><td data-edit-text="footer_copyright">© {{=date('Y')}} Example.</td></tr></table></body></html>`

func TestDetectImportedHTML(t *testing.T) {
	detection, err := detectImportedHTML(robotHTML)
	if err != nil {
		t.Fatal(err)
	}
	if detection.Language != "br" {
		t.Errorf("language: %q", detection.Language)
	}
	if detection.Subject != "Bitrix24: Follow-up da sua candidatura" {
		t.Errorf("subject: %q", detection.Subject)
	}
	if !detection.HasPreheaderSlot || detection.Preheader != "Veja os próximos passos" {
		t.Errorf("preheader: %v %q", detection.HasPreheaderSlot, detection.Preheader)
	}
	if len(detection.NestedMarkupFields) != 1 || detection.NestedMarkupFields[0].Key != "intro_body" ||
		strings.Join(detection.NestedMarkupFields[0].Tags, ",") != "a,strong" {
		t.Errorf("nested markup: %+v", detection.NestedMarkupFields)
	}
	if len(detection.BitrixExpressions) != 1 || detection.BitrixExpressions[0] != "{{=date('Y')}}" {
		t.Errorf("expressions: %v", detection.BitrixExpressions)
	}
}

func TestNormalizeImportLanguage(t *testing.T) {
	for in, want := range map[string]string{"en": "en", "es-ES": "es", "pt-BR": "br", "pt": "br", "DE": "de", "": ""} {
		if got := normalizeImportLanguage(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestImportWarningsFlagMissingPreheaderSlot(t *testing.T) {
	warnings := importWarnings(importDetection{})
	if len(warnings) != 1 || !strings.Contains(warnings[0], "preheader") {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestValidateSlotRequest(t *testing.T) {
	ok := createSlotRequest{Sequence: "onboarding", Stage: "lost", Title: "No reply", Emails: []createSlotEmailRequest{
		{Language: "EN", OriginalHTML: "<p>x</p>"}, {Language: "es", OriginalHTML: "<p>y</p>"},
	}}
	if err := validateSlotRequest(&ok); err != nil || ok.Emails[0].Language != "en" {
		t.Fatalf("valid request rejected: %v", err)
	}

	cases := map[string]createSlotRequest{
		"no EN":     {Sequence: "b", Stage: "s", Title: "t", Emails: []createSlotEmailRequest{{Language: "es", OriginalHTML: "x"}}},
		"duplicate": {Sequence: "b", Stage: "s", Title: "t", Emails: []createSlotEmailRequest{{Language: "en", OriginalHTML: "x"}, {Language: "EN", OriginalHTML: "y"}}},
		"empty":     {Sequence: "b", Stage: "s", Title: "t", Emails: []createSlotEmailRequest{{Language: "en", OriginalHTML: " "}}},
		"no title":  {Sequence: "b", Stage: "s", Emails: []createSlotEmailRequest{{Language: "en", OriginalHTML: "x"}}},
	}
	for name, request := range cases {
		if err := validateSlotRequest(&request); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
