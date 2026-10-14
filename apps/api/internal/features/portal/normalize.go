package portal

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
)

// The copy Bitrix24 keeps in the deal timeline breaks the "bulletproof button"
// pattern: `<!--[if !mso]><!-- --> <a>…</a> <!--<![endif]-->` becomes
// `<!--[if !mso]> <a>…</a> &lt;!--<![endif]-->`, hiding the CTA inside a
// comment. Delivered emails are fine; only the stored copy is affected.
var timelineNonMSOBlock = regexp.MustCompile(`(?s)<!--\[if !mso\]>(.*?)(?:&lt;!--)?<!\[endif\]-->`)

// RepairTimelineHTML restores the non-Outlook branch so the CTA is visible
// again and can be compared and imported.
func RepairTimelineHTML(body string) string {
	return timelineNonMSOBlock.ReplaceAllString(body, "<!--[if !mso]><!-- -->$1<!--<![endif]-->")
}

var (
	copyrightYear    = regexp.MustCompile(`©\s*(?:\d{4}|\{\{=date\('Y'\)\}\})`)
	bitrixDateExpr   = regexp.MustCompile(`\{\{=date\('Y'\)\}\}`)
	invisibleSpacing = strings.NewReplacer("‌", " ", " ", " ", "​", " ", "&zwnj;", " ", "&nbsp;", " ")
	wordPattern      = regexp.MustCompile(`[\p{L}\p{N}]+`)
)

// NormalizedText is the visible text used to compare and group emails. It
// ignores markup, invisible filler and the copyright year (a Bitrix
// expression in templates, a plain year in sent copies).
func NormalizedText(body string) string {
	body = RepairTimelineHTML(body)
	body = copyrightYear.ReplaceAllString(body, "© YEAR")
	body = bitrixDateExpr.ReplaceAllString(body, "YEAR")
	body = invisibleSpacing.Replace(body)
	return strings.Join(strings.Fields(html.UnescapeString(emailtext.HTMLToText(body))), " ")
}

func Fingerprint(normalizedText string) string {
	sum := sha256.Sum256([]byte(normalizedText))
	return hex.EncodeToString(sum[:16])
}

// Similarity is the Jaccard index of lower-cased word bigrams, in [0, 1].
func Similarity(a, b string) float64 {
	return jaccard(bigrams(a), bigrams(b))
}

func jaccard(setA, setB map[string]bool) float64 {
	if len(setA) == 0 && len(setB) == 0 {
		return 1
	}
	intersection := 0
	for gram := range setA {
		if setB[gram] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	return float64(intersection) / float64(union)
}

func bigrams(text string) map[string]bool {
	words := wordPattern.FindAllString(strings.ToLower(text), -1)
	set := map[string]bool{}
	for i := 0; i+1 < len(words); i++ {
		set[words[i]+" "+words[i+1]] = true
	}
	if len(words) == 1 {
		set[words[0]] = true
	}
	return set
}

// Sent copies have no <html lang>, so the language is guessed from common
// words. Codes follow the service: Brazilian Portuguese is "br".
var stopwords = map[string][]string{
	"en": {"the", "and", "your", "you", "to", "with", "for", "this", "our", "partner"},
	"de": {"und", "der", "die", "das", "ihre", "sie", "mit", "für", "ist", "nicht"},
	"es": {"el", "los", "las", "su", "para", "con", "que", "una", "por", "del"},
	"pl": {"i", "się", "na", "do", "w", "z", "jest", "nie", "twoje", "aby", "cię", "jak", "przez", "jeszcze", "od", "że"},
	"br": {"você", "seu", "sua", "para", "com", "não", "uma", "os", "das", "do"},
}

// detectionWords limits detection to the start of the email, and the text
// after "©" is dropped: every language shares the same English legal footer,
// which would otherwise win for short emails.
const detectionWords = 120

func DetectLanguage(text string) string {
	if body, _, found := strings.Cut(text, "©"); found {
		text = body
	}
	counts := map[string]int{}
	words := wordPattern.FindAllString(strings.ToLower(text), -1)
	if len(words) > detectionWords {
		words = words[:detectionWords]
	}
	for _, word := range words {
		counts[word]++
	}
	best, bestScore := "", 0
	languages := make([]string, 0, len(stopwords))
	for language := range stopwords {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	for _, language := range languages {
		score := 0
		for _, word := range stopwords[language] {
			score += counts[word]
		}
		if score > bestScore {
			best, bestScore = language, score
		}
	}
	if bestScore < 3 {
		return ""
	}
	return best
}
