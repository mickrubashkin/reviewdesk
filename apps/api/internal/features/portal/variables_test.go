package portal

import "testing"

func TestNormalizeVariables(t *testing.T) {
	variables, err := normalizeVariables([]Variable{
		{Label: " Booking link ", Expression: " {{Responsible person > Booking link}} ", Example: "https://calendly.com/example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if variables[0].Label != "Booking link" || variables[0].Expression != "{{Responsible person > Booking link}}" {
		t.Errorf("variable was not trimmed: %+v", variables[0])
	}

	for name, input := range map[string][]Variable{
		"no name":         {{Expression: "{{X}}"}},
		"not expression":  {{Label: "Link", Expression: "https://calendly.com"}},
		"two expressions": {{Label: "Link", Expression: "{{A}} {{B}}"}},
		"duplicate":       {{Label: "A", Expression: "{{X}}"}, {Label: "B", Expression: "{{X}}"}},
	} {
		if _, err := normalizeVariables(input); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
