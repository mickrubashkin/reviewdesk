package emailblocks

import "testing"

func TestWithLanguage(t *testing.T) {
	cases := map[string]struct{ template, language, want string }{
		"replaces the frame's":   {`<!doctype html><html lang="en" xmlns="x"><head>`, "de", `<!doctype html><html lang="de" xmlns="x"><head>`},
		"a code that is no tag":  {`<html lang='en'>`, "br", `<html lang="pt-BR">`},
		"adds one when missing":  {`<HTML xmlns="x">`, "pl", `<HTML lang="pl" xmlns="x">`},
		"no language, no change": {`<html lang="en">`, "", `<html lang="en">`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := WithLanguage(c.template, c.language); got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}
