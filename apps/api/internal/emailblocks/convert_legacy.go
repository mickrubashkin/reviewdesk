package emailblocks

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

var (
	// A paragraph after the main text, alone or cut around a link in the
	// text: body_outro, body_outro_001 + secondary_cta + body_outro_002.
	outroKeyPattern = regexp.MustCompile(`^(?:body_outro(?:_\d+)?|secondary_cta(?:_text|_url|_link)?)$`)
	outroTextKey    = regexp.MustCompile(`^body_outro(?:_\d+)?$`)
	listItemKey     = regexp.MustCompile(`^body[-_]list[-_]item_(\d+)$`)
	// A link at the start of a list item: list-item-link_001_text and _url.
	listItemLinkKey = regexp.MustCompile(`^list[-_]item[-_]link_\d+(?:_text|_url)?$`)
	boldStyle       = regexp.MustCompile(`(?i)font-weight\s*:\s*(?:700|bold)`)
	blankLines      = regexp.MustCompile(`\n{3,}`)
)

// legacyRenames are keys of the old templates the library names differently.
var legacyRenames = map[string]string{
	"body_list_heading": "list_heading",
	"boy_list_heading":  "list_heading",
	"signatrue":         "signature",
	"next_steps_body":   "body_text",
}

// legacyFields gives the fields of an email in the old format the keys of
// the library: typos are fixed, numbered list items (with the links in them)
// become one Markdown list and a paragraph with a link inside becomes one
// Markdown text, before the button the Text block, after it the Closing
// text. The email reads the same; only its markup changes. moved maps every
// old key that is gone to the key that took its text.
func legacyFields(template string, fields emailedit.EditableFields, options ConvertOptions) (out emailedit.EditableFields, moved map[string]string, err error) {
	// The stored order goes stale when a template is edited (two fields
	// with one number); the blocks follow the template.
	inTemplate, err := emailedit.ExtractEditableFields(template)
	if err != nil {
		return nil, nil, err
	}
	out = emailedit.EditableFields{}
	for key, field := range fields {
		if extracted, ok := inTemplate[key]; ok {
			field.Order = extracted.Order
		}
		out[key] = field
	}
	moved = map[string]string{}
	put := func(key string, field emailedit.EditableField) error {
		if _, taken := out[key]; taken {
			return fmt.Errorf("the email has two fields for %s", key)
		}
		out[key] = field
		return nil
	}

	for from, to := range legacyRenames {
		field, ok := out[from]
		if !ok {
			continue
		}
		delete(out, from)
		moved[from] = to
		if err := put(to, field); err != nil {
			return nil, nil, err
		}
	}

	if options.OneFieldSignature {
		if err := mergeSignature(out, moved, put); err != nil {
			return nil, nil, err
		}
	}

	rendered, err := emailedit.RenderEditableHTML(template, fields)
	if err != nil {
		return nil, nil, err
	}
	root, err := html.Parse(strings.NewReader(rendered))
	if err != nil {
		return nil, nil, err
	}
	if err := mergeListItems(root, out, moved, put); err != nil {
		return nil, nil, err
	}
	if err := mergeOutro(root, out, moved, put); err != nil {
		return nil, nil, err
	}
	return out, moved, nil
}

// mergeListItems puts the numbered list items into one Markdown list, one
// point per item. An item that is a cell of its own keeps its text; one with
// a link or words around its field is read from its rendered cell.
func mergeListItems(root *html.Node, out emailedit.EditableFields, moved map[string]string, put func(string, emailedit.EditableField) error) error {
	type item struct {
		number int
		key    string
		field  emailedit.EditableField
	}
	items := []item{}
	for key, field := range out {
		match := listItemKey.FindStringSubmatch(key)
		if match == nil {
			continue
		}
		number, _ := strconv.Atoi(match[1])
		items = append(items, item{number, key, field})
	}
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool { return items[i].number < items[j].number })
	lines := make([]string, len(items))
	for i, item := range items {
		text, err := fieldCellMarkdown(root, item.key, item.field)
		if err != nil {
			return err
		}
		lines[i] = "- " + strings.Join(strings.Fields(text), " ")
	}
	for key := range out {
		if listItemKey.MatchString(key) || listItemLinkKey.MatchString(key) {
			delete(out, key)
			moved[key] = "list_items"
		}
	}
	return put("list_items", emailedit.EditableField{
		Type: emailedit.FieldTypeText, Value: strings.Join(lines, "\n"), Order: items[0].field.Order,
		Format: emailedit.FormatMarkdown,
	})
}

// mergeOutro turns the paragraph after the main text into the Text or the
// Closing text. A paragraph that is a field of its own keeps its text; one
// cut into fields around a link is read from the rendered email as Markdown,
// up to the signature that may share its cell.
func mergeOutro(root *html.Node, out emailedit.EditableFields, moved map[string]string, put func(string, emailedit.EditableField) error) error {
	first, firstKey := 0, ""
	for key, field := range out {
		if !outroKeyPattern.MatchString(key) {
			continue
		}
		if firstKey == "" || field.Order < first {
			first, firstKey = field.Order, key
		}
	}
	if firstKey == "" {
		return nil
	}
	target := "body_text"
	if button, ok := out["primary_cta_text"]; ok && button.Order < first {
		target = "closing_text"
	}

	var element *html.Node
	walkElements(root, func(node *html.Node) {
		if element == nil && outroTextKey.MatchString(attr(node, "data-edit-text")) {
			element = node
		}
	})
	if element == nil {
		return fmt.Errorf("the text after the main text is not in the email")
	}

	field := emailedit.EditableField{Type: emailedit.FieldTypeText, Order: first}
	if element.Data == "td" {
		field.Value = out[attr(element, "data-edit-text")].Value
	} else {
		cell := closestCell(element)
		if cell == nil {
			return fmt.Errorf("the text after the main text is not in a table cell")
		}
		var text strings.Builder
		writeMarkdown(&text, cell)
		field.Value = tidyMarkdown(text.String())
		field.Format = emailedit.FormatMarkdown
	}
	for key := range out {
		if outroKeyPattern.MatchString(key) {
			delete(out, key)
			moved[key] = target
		}
	}
	return put(target, field)
}

// mergeSignature puts the greeting and sender into the one-field
// signature, on two lines.
func mergeSignature(out emailedit.EditableFields, moved map[string]string, put func(string, emailedit.EditableField) error) error {
	greeting, hasGreeting := out["signature_greeting"]
	sender, hasSender := out["signature_sender"]
	if !hasGreeting && !hasSender {
		return nil
	}
	lines := []string{}
	order := sender.Order
	for _, field := range []emailedit.EditableField{greeting, sender} {
		if value, _ := field.Value.(string); strings.TrimSpace(value) != "" {
			lines = append(lines, strings.TrimSpace(value))
		}
	}
	if hasGreeting {
		order = greeting.Order
	}
	delete(out, "signature_greeting")
	delete(out, "signature_sender")
	moved["signature_greeting"], moved["signature_sender"] = "signature", "signature"
	return put("signature", emailedit.EditableField{Type: emailedit.FieldTypeText, Value: strings.Join(lines, "\n"), Order: order})
}

// fieldCellMarkdown is the text of the cell a field is in: the field's own
// value when the cell is the field, else the cell read as Markdown.
func fieldCellMarkdown(root *html.Node, key string, field emailedit.EditableField) (string, error) {
	var element *html.Node
	walkElements(root, func(node *html.Node) {
		if element == nil && attr(node, "data-edit-text") == key {
			element = node
		}
	})
	if element == nil {
		return "", fmt.Errorf("field %s is not in the email", key)
	}
	if element.Data == "td" {
		value, _ := field.Value.(string)
		return value, nil
	}
	cell := closestCell(element)
	if cell == nil {
		return "", fmt.Errorf("field %s is not in a table cell", key)
	}
	var text strings.Builder
	writeMarkdown(&text, cell)
	return tidyMarkdown(text.String()), nil
}

func closestCell(node *html.Node) *html.Node {
	for cell := node.Parent; cell != nil; cell = cell.Parent {
		if cell.Type == html.ElementNode && cell.Data == "td" {
			return cell
		}
	}
	return nil
}

// writeMarkdown writes the text of node with its links as Markdown and stops
// at the signature (one field or greeting and sender): it is a block of its
// own.
func writeMarkdown(text *strings.Builder, node *html.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			// Line breaks of the source are spaces; <br> makes a new line.
			text.WriteString(spaces.ReplaceAllString(child.Data, " "))
		case html.ElementNode:
			switch key := attr(child, "data-edit-text"); {
			case strings.HasPrefix(key, "signature") || key == "signatrue":
				return false
			case child.Data == "br":
				text.WriteString("\n")
			case child.Data == "a":
				var label strings.Builder
				writeMarkdown(&label, child)
				link := "[" + strings.TrimSpace(label.String()) + "](" + attr(child, "href") + ")"
				if boldStyle.MatchString(attr(child, "style")) {
					link = "**" + link + "**"
				}
				text.WriteString(link)
			default:
				if !writeMarkdown(text, child) {
					return false
				}
			}
		}
	}
	return true
}

func tidyMarkdown(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}
