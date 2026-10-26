package portal

import "testing"

func TestImportTitle(t *testing.T) {
	cases := map[string]string{
		"Bitrix24 Partner Program | Kickstart Bonus Activated":     "Kickstart Bonus Activated",
		"Bitrix24: You haven’t started yet":                        "You haven’t started yet",
		"Programa de Parceiros Bitrix24 | Bem-vindo a bordo!":      "Bem-vindo a bordo!",
		"Next steps for your Bitrix24 Partnership":                 "Next steps for your Bitrix24 Partnership",
		"Sua parceria Bitrix24 está quase pronta! Vamos conversar": "Sua parceria Bitrix24 está quase pronta! Vamos conversar",
	}
	for subject, want := range cases {
		if got := importTitle(subject); got != want {
			t.Errorf("%q → %q, want %q", subject, got, want)
		}
	}
}

func TestStageKeyAndTopStage(t *testing.T) {
	if got := stageKey("The first sale with a Grant"); got != "the-first-sale-with-a-grant" {
		t.Errorf("stageKey: %q", got)
	}
	if got := topStage(map[string]int{"C10:5": 3, "C10:LOSE": 12}); got != "C10:LOSE" {
		t.Errorf("topStage: %q", got)
	}
	if got := topStage(map[string]int{}); got != "" {
		t.Errorf("empty topStage: %q", got)
	}
}

func TestIsManualOrTestSubject(t *testing.T) {
	for subject, want := range map[string]bool{
		"RE: Bitrix24: Your partner application received": true,
		"Re: Request to Pause Remaining Subscription":     true,
		"Fwd: Contract":        true,
		"teste":                true,
		"Test form acme":       true,
		"Тест отправки письма": true,
		"Bitrix24 Partner Program | Kickstart Bonus": false,
		"Testimonials from partners":                 false,
	} {
		if got := IsManualOrTestSubject(subject); got != want {
			t.Errorf("%q: got %v want %v", subject, got, want)
		}
	}
}
