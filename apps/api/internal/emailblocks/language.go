package emailblocks

import (
	"html"
	"regexp"
	"strings"
)

// htmlLangs are the document languages of the board's language codes that
// are not language tags themselves.
var htmlLangs = map[string]string{
	"br": "pt-BR",
	"vn": "vi",
}

// HTMLLang is the lang of the document of an email in language.
func HTMLLang(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if lang, ok := htmlLangs[language]; ok {
		return lang
	}
	return language
}

var (
	htmlStartTag = regexp.MustCompile(`(?is)<html\b[^>]*>`)
	langAttr     = regexp.MustCompile(`(?is)(\slang\s*=\s*)("[^"]*"|'[^']*'|[^\s>]+)`)
)

// WithLanguage gives a template built from blocks the language of its email:
// the frame is written in English, so mail clients would offer to translate
// a German email.
func WithLanguage(template, language string) string {
	lang := HTMLLang(language)
	if lang == "" {
		return template
	}
	value := `"` + html.EscapeString(lang) + `"`
	return htmlStartTag.ReplaceAllStringFunc(template, func(tag string) string {
		if langAttr.MatchString(tag) {
			return langAttr.ReplaceAllString(tag, "${1}"+value)
		}
		return tag[:len("<html")] + " lang=" + value + tag[len("<html"):]
	})
}
