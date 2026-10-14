package portal

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	xhtml "golang.org/x/net/html"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// Reconstruction is an editable email rebuilt from a sent copy: the copy in
// the deal timeline has lost <head> and all data-* markup, but keeps the body
// structure of the template it was made from.
type Reconstruction struct {
	HTML          string   `json:"html"`
	FilledFields  []string `json:"filled_fields"`
	MissingFields []string `json:"missing_fields"`
	// FromTemplate is true when the result is the template rendered with the
	// sent values (exact markup, Outlook button included); false when the
	// structures differ and the marked-up sent copy is used instead.
	FromTemplate bool     `json:"from_template"`
	Warnings     []string `json:"warnings"`
}

var transplantedAttrs = []string{
	"data-review-block",
	"data-edit-text",
	"data-edit-attr-href",
	"data-edit-attr-src",
	"data-edit-attr-alt",
	"data-edit-style-width-px",
	"data-email-preheader",
}

// Reconstruct aligns the sent copy with templateHTML element by element,
// copies the editable-field markup onto the sent copy, reads the field values
// from it and renders the template with them.
func Reconstruct(sentHTML, templateHTML, subject, language string) (Reconstruction, error) {
	result := Reconstruction{FilledFields: []string{}, MissingFields: []string{}, Warnings: []string{}}

	template, err := xhtml.Parse(strings.NewReader(templateHTML))
	if err != nil {
		return result, fmt.Errorf("parse template: %w", err)
	}
	sent, err := xhtml.Parse(strings.NewReader(RepairTimelineHTML(sentHTML)))
	if err != nil {
		return result, fmt.Errorf("parse sent email: %w", err)
	}

	templateNodes, sentNodes := bodyElements(template), bodyElements(sent)
	for _, pair := range alignElements(templateNodes, sentNodes) {
		for _, key := range transplantedAttrs {
			if value, ok := attr(pair.template, key); ok {
				setAttribute(pair.sent, key, value)
			}
		}
	}

	var marked bytes.Buffer
	if err := xhtml.Render(&marked, sent); err != nil {
		return result, err
	}
	sentFields, err := emailedit.ExtractEditableFields(marked.String())
	if err != nil {
		return result, fmt.Errorf("read values from the sent email: %w", err)
	}
	templateFields, err := emailedit.ExtractEditableFields(templateHTML)
	if err != nil {
		return result, fmt.Errorf("read template fields: %w", err)
	}

	merged := emailedit.EditableFields{}
	for key, field := range templateFields {
		if value, ok := sentFields[key]; ok && value.Type == field.Type {
			value.Order = field.Order
			if keepsTemplateExpression(fmt.Sprint(field.Value), fmt.Sprint(value.Value)) {
				value.Value = field.Value
			}
			if strings.HasSuffix(key, "_alt") && fmt.Sprint(value.Value) == "" && fmt.Sprint(field.Value) != "" {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%q is empty in the sent copy (Bitrix24 may drop alt texts); fill it in after import.", key))
			}
			merged[key] = value
			result.FilledFields = append(result.FilledFields, key)
		} else {
			merged[key] = field
			result.MissingFields = append(result.MissingFields, key)
		}
	}
	sort.Strings(result.FilledFields)
	sort.Strings(result.MissingFields)

	var rendered string
	if len(result.MissingFields) == 0 {
		rendered, err = emailedit.RenderEditableHTML(templateHTML, merged)
		if err != nil {
			return result, err
		}
		result.FromTemplate = true
	} else {
		// Different structure (e.g. no CTA block): keep the sent structure so
		// nothing from the template's own copy leaks in.
		rendered, err = emailedit.RenderEditableHTML(marked.String(), sentFields)
		if err != nil {
			return result, err
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"The sent email does not have every block of the template (%s missing), so its own structure was kept. The Outlook version of the button may need a check.",
			strings.Join(result.MissingFields, ", ")))
	}

	result.HTML = withHead(rendered, templateHTML, subject, language)
	return result, nil
}

// keepsTemplateExpression reports whether sent is template with its Bitrix24
// expressions evaluated, e.g. "© {{=date('Y')}}" sent as "© 2026". The
// template keeps the expression so the year stays current.
func keepsTemplateExpression(template, sent string) bool {
	if !strings.Contains(template, "{{=") {
		return false
	}
	parts := bitrixExpressionSplit.Split(template, -1)
	for i, part := range parts {
		parts[i] = regexp.QuoteMeta(part)
	}
	pattern, err := regexp.Compile("^" + strings.Join(parts, ".+?") + "$")
	return err == nil && pattern.MatchString(sent)
}

var bitrixExpressionSplit = regexp.MustCompile(`\{\{=[^{}]*\}\}`)

// SkeletonSimilarity compares the element structure of two emails in [0, 1];
// used to pick the template a sent email was made from.
func SkeletonSimilarity(sentHTML, templateHTML string) float64 {
	sent, err1 := xhtml.Parse(strings.NewReader(RepairTimelineHTML(sentHTML)))
	template, err2 := xhtml.Parse(strings.NewReader(templateHTML))
	if err1 != nil || err2 != nil {
		return 0
	}
	a, b := bodyElements(template), bodyElements(sent)
	if len(a)+len(b) == 0 {
		return 0
	}
	return 2 * float64(len(alignElements(a, b))) / float64(len(a)+len(b))
}

func bodyElements(root *xhtml.Node) []*xhtml.Node {
	var nodes []*xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch n.Data {
			case "head", "style", "script", "title":
				return
			case "html", "body", "br":
			default:
				nodes = append(nodes, n)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return nodes
}

type alignedPair struct{ template, sent *xhtml.Node }

// alignElements is a longest-common-subsequence match on tag names, so
// inserted or missing elements (an extra <strong>, a dropped block) only
// affect their own spot.
func alignElements(a, b []*xhtml.Node) []alignedPair {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i].Data == b[j].Data {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var pairs []alignedPair
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i].Data == b[j].Data:
			pairs = append(pairs, alignedPair{a[i], b[j]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			i++
		default:
			j++
		}
	}
	return pairs
}

func attr(node *xhtml.Node, key string) (string, bool) {
	for _, a := range node.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func setAttribute(node *xhtml.Node, key, value string) {
	for i := range node.Attr {
		if node.Attr[i].Key == key {
			node.Attr[i].Val = value
			return
		}
	}
	node.Attr = append(node.Attr, xhtml.Attribute{Key: key, Val: value})
}

var (
	htmlLangPattern  = regexp.MustCompile(`(?i)(<html\b[^>]*\blang=")[^"]*(")`)
	titlePattern     = regexp.MustCompile(`(?is)<title>.*?</title>`)
	headBlockPattern = regexp.MustCompile(`(?is)<head\b.*?</head>`)
)

var htmlLangByLanguage = map[string]string{"br": "pt-BR"}

// withHead puts the template's <head> (meta tags, styles) on the result, with
// the sent subject as <title> and the email's language in <html lang>.
func withHead(rendered, templateHTML, subject, language string) string {
	if head := headBlockPattern.FindString(templateHTML); head != "" {
		if headBlockPattern.MatchString(rendered) {
			rendered = headBlockPattern.ReplaceAllLiteralString(rendered, head)
		}
	}
	if subject != "" {
		rendered = titlePattern.ReplaceAllLiteralString(rendered, "<title>"+html.EscapeString(subject)+"</title>")
	}
	if language != "" {
		lang := language
		if mapped, ok := htmlLangByLanguage[language]; ok {
			lang = mapped
		}
		rendered = htmlLangPattern.ReplaceAllString(rendered, "${1}"+lang+"${2}")
	}
	return rendered
}
