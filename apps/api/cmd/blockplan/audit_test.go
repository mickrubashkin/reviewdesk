package main

import (
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

const templateWithBlocks = `<html><body>
<p data-review-block="intro_body"><span data-edit-text="intro_body">x</span></p>
<a data-review-block="primary_cta" data-edit-attr-href="primary_cta_url" href="#">Go</a>
<p data-review-block="footer_legal"><span data-edit-text="footer_legal_text">x</span></p>
</body></html>`

func fields(intro, legal string) emailedit.EditableFields {
	return emailedit.EditableFields{
		"intro_body":        {Type: emailedit.FieldTypeText, Value: intro},
		"primary_cta_url":   {Type: emailedit.FieldTypeURL, Value: "https://example.com"},
		"primary_cta_text":  {Type: emailedit.FieldTypeText, Value: "Book a call"},
		"footer_legal_text": {Type: emailedit.FieldTypeText, Value: legal},
	}
}

func TestBuildAudit(t *testing.T) {
	emails := []Email{
		{ID: "1", Language: "en", Subject: "Hello", TemplateHTML: templateWithBlocks, Fields: fields("One", "Legal  text"), PortalSends: 3},
		{ID: "2", Language: "en", Subject: "Hello there", TemplateHTML: templateWithBlocks, Fields: fields("Two", "Legal text")},
		{ID: "3", Language: "de", Subject: "Hallo", TemplateHTML: templateWithBlocks, Fields: fields("Eins", "Rechtliches")},
		{ID: "4", Language: "en", Subject: "Old", LiveMarked: true, Fields: emailedit.EditableFields{},
			TemplateHTML: `<p data-review-block="body-001">a</p><a data-review-block="cta-001">b</a><a data-review-block="cta-002">c</a>`},
	}
	audit := BuildAudit(emails)

	if audit.Total != 4 || audit.WithFields != 3 || audit.WithoutFields != 1 || audit.Live != 2 || audit.LiveWithoutFields != 1 {
		t.Fatalf("unexpected summary %+v", audit)
	}
	if len(audit.Layouts) != 1 || len(audit.Layouts[0].Emails) != 3 || audit.Layouts[0].Skeletons != 1 || audit.Layouts[0].Live != 1 {
		t.Fatalf("expected one layout of three emails with one markup, got %+v", audit.Layouts)
	}
	if got := audit.Layouts[0].Blocks; len(got) != 3 || got[1] != "primary_cta" {
		t.Fatalf("unexpected blocks %v", got)
	}
	if audit.WithoutFieldsBlocks["cta"] != 1 || audit.CTA.WithoutFields["2"] != 1 || audit.CTA.WithFields["1"] != 3 {
		t.Fatalf("unexpected cta stats %+v %+v", audit.CTA, audit.WithoutFieldsBlocks)
	}

	byKey := map[string]SharedKey{}
	for _, key := range audit.Shared {
		byKey[key.Key] = key
	}
	if legal := byKey["footer_legal_text"].ByLanguage["en"]; legal.Distinct != 1 || legal.Emails != 2 {
		t.Fatalf("expected one EN legal text once whitespace is normalized, got %+v", legal)
	}
	if intro := byKey["intro_body"].ByLanguage["en"]; intro.Distinct != 2 {
		t.Fatalf("expected two EN intros, got %+v", intro)
	}
	if url := byKey["primary_cta_url"].AcrossLanguages; url.Distinct != 1 || url.Emails != 3 {
		t.Fatalf("expected one CTA URL across languages, got %+v", url)
	}
	if audit.Languages[0] != "en" || audit.Languages[1] != "de" {
		t.Fatalf("expected EN first, got %v", audit.Languages)
	}
}

func TestPercentileNearestRank(t *testing.T) {
	values := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	identity := func(v int) int { return v }
	if percentile(values, 50, identity) != 5 || percentile(values, 95, identity) != 10 || percentile(values[:1], 90, identity) != 1 {
		t.Fatal("unexpected percentiles")
	}
}
