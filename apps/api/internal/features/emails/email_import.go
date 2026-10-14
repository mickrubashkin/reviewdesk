package emails

import (
	"sort"
	"strings"

	xhtml "golang.org/x/net/html"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// importDetection is what can be read from HTML pasted from a CRM robot, so
// the form does not have to ask for it.
type importDetection struct {
	Language         string `json:"detected_language"`
	Subject          string `json:"detected_subject"`
	Preheader        string `json:"detected_preheader"`
	HasPreheaderSlot bool   `json:"has_preheader_slot"`
	// NestedMarkupFields are text fields that contain links or formatting the
	// template does not support; that markup is lost when the field renders.
	NestedMarkupFields []nestedMarkupField `json:"nested_markup_fields"`
	BitrixExpressions  []string            `json:"bitrix_expressions"`
}

type nestedMarkupField struct {
	Key  string   `json:"key"`
	Tags []string `json:"tags"`
}

func detectImportedHTML(originalHTML string) (importDetection, error) {
	root, err := xhtml.Parse(strings.NewReader(originalHTML))
	if err != nil {
		return importDetection{}, err
	}

	detection := importDetection{
		NestedMarkupFields: []nestedMarkupField{},
		BitrixExpressions:  emailedit.FindBitrixExpressions(originalHTML),
	}

	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			switch {
			case node.Data == "html":
				detection.Language = normalizeImportLanguage(htmlAttrValue(node, "lang"))
			case node.Data == "title" && detection.Subject == "":
				detection.Subject = strings.Join(strings.Fields(htmlTextContent(node)), " ")
			}
			if hasAttr(node, "data-email-preheader") {
				detection.HasPreheaderSlot = true
				detection.Preheader = strings.Join(strings.Fields(htmlTextContent(node)), " ")
			}
			if key := htmlAttrValue(node, "data-edit-text"); key != "" {
				if tags := nestedElementTags(node); len(tags) > 0 {
					detection.NestedMarkupFields = append(detection.NestedMarkupFields, nestedMarkupField{Key: key, Tags: tags})
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)

	return detection, nil
}

func importWarnings(detection importDetection) []string {
	warnings := []string{}
	for _, field := range detection.NestedMarkupFields {
		warnings = append(warnings, "Field \""+field.Key+"\" contains <"+strings.Join(field.Tags, ">, <")+"> that the template does not support. It will be lost when the email is rendered or edited.")
	}
	if !detection.HasPreheaderSlot {
		warnings = append(warnings, "No preheader slot (data-email-preheader) was found, so the preheader cannot be set from the service.")
	}
	return warnings
}

// normalizeImportLanguage maps an HTML lang attribute to the service language
// codes, where Brazilian Portuguese is "br".
func normalizeImportLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return ""
	}
	if lang == "pt" || strings.HasPrefix(lang, "pt-") {
		return "br"
	}
	primary, _, _ := strings.Cut(lang, "-")
	return primary
}

func nestedElementTags(node *xhtml.Node) []string {
	tags := map[string]bool{}
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == xhtml.ElementNode && child.Data != "br" {
				tags[child.Data] = true
			}
			walk(child)
		}
	}
	walk(node)

	result := make([]string, 0, len(tags))
	for tag := range tags {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

func hasAttr(node *xhtml.Node, key string) bool {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return true
		}
	}
	return false
}
