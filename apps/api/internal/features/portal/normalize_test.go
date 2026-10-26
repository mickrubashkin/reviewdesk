package portal

import (
	"strings"
	"testing"
	"time"
)

func TestRepairTimelineHTMLRestoresHiddenCTA(t *testing.T) {
	timeline := `<td><!--[if mso]> Agendar <![endif]--> <!--[if !mso]> <a href="https://x">Agendar reunião</a> &lt;!--<![endif]--></td>`
	text := NormalizedText(timeline)
	if !strings.Contains(text, "Agendar reunião") {
		t.Fatalf("CTA text lost: %q", text)
	}
	if strings.Count(text, "Agendar") != 1 {
		t.Fatalf("Outlook branch should stay hidden: %q", text)
	}
}

func TestNormalizedTextIgnoresYearAndFiller(t *testing.T) {
	sent := `<div>&zwnj;&nbsp;&zwnj;&nbsp;</div><p>Hello</p><p>© 2026 Acme.</p>`
	template := `<div>` + "‌ " + `</div><p>Hello</p><p>© {{=date('Y')}} Acme.</p>`
	if a, b := NormalizedText(sent), NormalizedText(template); a != b {
		t.Fatalf("expected equal texts:\n%q\n%q", a, b)
	}
}

func TestSimilarityAndLanguage(t *testing.T) {
	a := "Hi there, please book a call with your partner manager to continue the onboarding today"
	b := "Hi there, please book a call with your partner manager to continue the onboarding now"
	if s := Similarity(a, b); s < 0.8 || s >= 1 {
		t.Fatalf("unexpected similarity %v", s)
	}
	if Similarity(a, "Olá! Você conseguiu avançar com os ajustes?") > 0.1 {
		t.Fatal("unrelated texts should not match")
	}
	cases := map[string]string{
		a: "en",
		"Olá! Você conseguiu avançar com aqueles ajustes? Me coloco à disposição para uma chamada com você e sua equipe, não hesite.":                                                                                                                                                   "br",
		"Przeprowadzimy Cię przez proces instalacji. Nie zainstalowałeś jeszcze aplikacji? Jeśli nie wiesz, jak zacząć. © YEAR Acme. All rights reserved. This is an automatically generated notification for you, the partner, because you signed up for this program and the emails.": "pl",
		"Hola, gracias por su interés en el programa. Hemos intentado contactar con usted por los mensajes, para que pueda continuar con la solicitud.":                                                                                                                                 "es",
	}
	for text, want := range cases {
		if got := DetectLanguage(text); got != want {
			t.Errorf("%q: got %q want %q", text[:20], got, want)
		}
	}
}

func TestStageAt(t *testing.T) {
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	changes := []stageChange{
		{stageID: "C10:PREPARATION", at: base.Add(48 * time.Hour)},
		{stageID: "C10:NEW", at: base},
	}
	if got := stageAt(changes, base.Add(24*time.Hour)); got != "C10:NEW" {
		t.Errorf("got %q", got)
	}
	if got := stageAt(changes, base.Add(48*time.Hour+10*time.Second)); got != "C10:PREPARATION" {
		t.Errorf("robot sending right after the move: got %q", got)
	}
	if got := stageAt(changes, base.Add(-time.Hour)); got != "" {
		t.Errorf("before any history: got %q", got)
	}
}
