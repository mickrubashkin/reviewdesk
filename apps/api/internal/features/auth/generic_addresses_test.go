package auth

import "testing"

func TestIsGenericAddress(t *testing.T) {
	for _, email := range []string{
		"sales@bitrix24.com",
		"Sales@bitrix24.com",
		"sales-de@bitrix24.de",
		"sales.br@bitrix24.com.br",
		"partners@bitrix24.com",
		"noreply@acme.example",
		"no-reply@acme.example",
		"info@bitrix24.es",
		"info2@bitrix24.pl",
	} {
		if !isGenericAddress(email) {
			t.Errorf("expected %s to be generic", email)
		}
	}
	for _, email := range []string{
		"salesky@acme.example",
		"infante@acme.example",
		"a.sales@bitrix24.com",
		"partnership.lead@acme.example",
		"not-an-email",
	} {
		if isGenericAddress(email) {
			t.Errorf("expected %s to be personal", email)
		}
	}
}

func TestGenericAddressesCannotSignIn(t *testing.T) {
	t.Setenv("AUTH_ALLOWED_DOMAINS", "acme.example")
	if isAllowedAuthEmail("sales@acme.example") {
		t.Fatal("expected sales@ to be refused")
	}
	if !isAllowedAuthEmail("d.person@acme.example") {
		t.Fatal("expected a personal address to stay allowed")
	}
}
