package emailtext

import (
	"regexp"
	"strings"
)

type ContentParts struct {
	Subject    string     `json:"subject"`
	Preheader  string     `json:"preheader"`
	BannerText string     `json:"banner_text"`
	BodyText   string     `json:"body_text"`
	PrimaryCTA string     `json:"primary_cta"`
	Links      []string   `json:"links"`
	LinkGroups LinkGroups `json:"link_groups"`
}

type LinkGroups struct {
	Primary []string `json:"primary"`
	Support []string `json:"support"`
	Footer  []string `json:"footer"`
	Other   []string `json:"other"`
}

type linkInfo struct {
	Text       string
	Href       string
	Attributes string
}

var (
	htmlCommentPattern = regexp.MustCompile(`(?is)<!--.*?-->`)
	headPattern        = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head>`)
	stylePattern       = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	scriptPattern      = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	xmlPattern         = regexp.MustCompile(`(?is)<xml\b[^>]*>.*?</xml>`)
	imgPattern         = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	h1Pattern          = regexp.MustCompile(`(?is)<h1\b[^>]*>(.*?)</h1>`)
	linkPattern        = regexp.MustCompile(`(?is)<a\b[^>]*>(.*?)</a\s*>`)
	htmlTagPattern     = regexp.MustCompile(`(?is)<[^>]*>`)
	whitespacePattern  = regexp.MustCompile(`\s+`)
	namedEntities      = strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&apos;", "'",
		"&nbsp;", " ",
		"&zwnj;", " ",
		"&ndash;", "-",
		"&mdash;", "-",
	)
)

func HTMLToText(value string) string {
	value = htmlCommentPattern.ReplaceAllString(value, " ")
	value = headPattern.ReplaceAllString(value, " ")
	value = stylePattern.ReplaceAllString(value, " ")
	value = scriptPattern.ReplaceAllString(value, " ")
	value = xmlPattern.ReplaceAllString(value, " ")
	value = htmlTagPattern.ReplaceAllString(value, " ")
	value = namedEntities.Replace(value)
	value = strings.ReplaceAll(value, "\u200c", " ")
	value = whitespacePattern.ReplaceAllString(value, " ")

	return strings.TrimSpace(value)
}

func ExtractContentParts(originalHTML string, subject string, preheader string, bodyText string) ContentParts {
	links := extractLinkInfos(originalHTML)
	primaryCTA := extractPrimaryCTAFromLinks(links)

	return ContentParts{
		Subject:    subject,
		Preheader:  preheader,
		BannerText: extractBannerText(originalHTML),
		BodyText:   bodyText,
		PrimaryCTA: primaryCTA,
		Links:      linkTexts(links),
		LinkGroups: classifyLinks(links, primaryCTA),
	}
}

func extractBannerText(value string) string {
	for _, image := range imgPattern.FindAllString(value, -1) {
		alt := extractAttribute(image, "alt")
		if isMeaningfulImageAlt(alt) {
			return alt
		}
	}

	matches := h1Pattern.FindStringSubmatch(value)
	if len(matches) >= 2 {
		return HTMLToText(matches[1])
	}

	return ""
}

func isMeaningfulImageAlt(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	lowerValue := strings.ToLower(value)
	return !strings.Contains(lowerValue, "logo") &&
		!strings.Contains(lowerValue, "telegram") &&
		!strings.Contains(lowerValue, "linkedin") &&
		!strings.Contains(lowerValue, "youtube") &&
		!strings.Contains(lowerValue, "facebook")
}

func extractPrimaryCTA(value string) string {
	return extractPrimaryCTAFromLinks(extractLinkInfos(value))
}

func extractPrimaryCTAFromLinks(links []linkInfo) string {
	for _, link := range links {
		text := link.Text
		if text == "" || isFooterLink(link) {
			continue
		}

		lowerAttrs := strings.ToLower(link.Attributes)
		if strings.Contains(lowerAttrs, "button") ||
			strings.Contains(lowerAttrs, "background-color") ||
			strings.Contains(lowerAttrs, "border-radius") ||
			strings.Contains(lowerAttrs, "display: inline-block") {
			return text
		}
	}

	return ""
}

func extractLinks(value string) []string {
	return linkTexts(extractLinkInfos(value))
}

func extractLinkInfos(value string) []linkInfo {
	seen := map[string]bool{}
	links := []linkInfo{}

	for _, link := range linkPattern.FindAllStringSubmatch(value, -1) {
		if len(link) < 2 {
			continue
		}

		text := HTMLToText(link[1])
		if text == "" || seen[text] {
			continue
		}

		attrs := link[0]
		seen[text] = true
		links = append(links, linkInfo{
			Text:       text,
			Href:       extractAttribute(attrs, "href"),
			Attributes: attrs,
		})
	}

	return links
}

func linkTexts(links []linkInfo) []string {
	texts := make([]string, 0, len(links))
	for _, link := range links {
		if link.Text != "" {
			texts = append(texts, link.Text)
		}
	}

	return texts
}

func classifyLinks(links []linkInfo, primaryCTA string) LinkGroups {
	groups := LinkGroups{
		Primary: []string{},
		Support: []string{},
		Footer:  []string{},
		Other:   []string{},
	}

	for _, link := range links {
		switch {
		case isFooterLink(link):
			groups.Footer = appendUnique(groups.Footer, link.Text)
		case primaryCTA != "" && strings.EqualFold(link.Text, primaryCTA):
			groups.Primary = appendUnique(groups.Primary, link.Text)
		case isSupportLink(link):
			groups.Support = appendUnique(groups.Support, link.Text)
		default:
			groups.Other = appendUnique(groups.Other, link.Text)
		}
	}

	return groups
}

func isSupportLink(link linkInfo) bool {
	value := strings.ToLower(link.Text + " " + link.Href)
	return strings.Contains(value, "book a call") ||
		strings.Contains(value, "book a short call") ||
		strings.Contains(value, "schedule") ||
		strings.Contains(value, "calendly") ||
		strings.Contains(value, "contact") ||
		strings.Contains(value, "support") ||
		strings.Contains(value, "help")
}

func isFooterLink(link linkInfo) bool {
	value := strings.ToLower(link.Text + " " + link.Href)
	return strings.Contains(value, "privacy") ||
		strings.Contains(value, "unsubscribe") ||
		strings.Contains(value, "terms") ||
		strings.Contains(value, "telegram") ||
		strings.Contains(value, "linkedin") ||
		strings.Contains(value, "youtube") ||
		strings.Contains(value, "facebook")
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}

	return append(values, value)
}

func extractAttribute(tag string, attribute string) string {
	pattern := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(attribute) + `\s*=\s*("([^"]*)"|'([^']*)')`)
	matches := pattern.FindStringSubmatch(tag)
	if len(matches) >= 3 && matches[2] != "" {
		return HTMLToText(matches[2])
	}
	if len(matches) >= 4 {
		return HTMLToText(matches[3])
	}

	return ""
}
