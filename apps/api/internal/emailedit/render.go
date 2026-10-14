package emailedit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
)

var templateMarkerPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_:-]+)\s*\}\}`)

// bitrixExpressionPattern matches Bitrix24 template expressions, which must
// reach the CRM robot byte-for-byte: {{=date('Y')}}, fields inserted by the
// robot editor such as {{Responsible person > Booking link}}, and
// {=Document:ASSIGNED_BY_PRINTABLE}. It also matches our own {{field_key}}
// markers, so callers that care tell them apart with templateMarkerPattern.
var bitrixExpressionPattern = regexp.MustCompile(`\{\{[^{}]+\}\}|\{=[A-Za-z_]+:[^{}]+\}`)

// FindBitrixExpressions lists the distinct Bitrix24 expressions in s, leaving
// out our own template markers.
func FindBitrixExpressions(s string) []string {
	seen := map[string]bool{}
	found := []string{}
	for _, match := range bitrixExpressionPattern.FindAllString(s, -1) {
		if seen[match] || templateMarkerPattern.FindString(match) == match {
			continue
		}
		seen[match] = true
		found = append(found, match)
	}
	return found
}

// IsBitrixExpression reports whether value, trimmed, is a single Bitrix24
// expression and nothing else.
func IsBitrixExpression(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && bitrixExpressionPattern.FindString(value) == value
}

// invisibleCharEntities keeps characters that email HTML relies on (e.g. the
// preheader filler) as entities: the Bitrix24 editor drops them when pasted raw.
var invisibleCharEntities = strings.NewReplacer(
	"\u200c", "&zwnj;",
	"\u200b", "&#8203;",
	"\u00a0", "&nbsp;",
	"\u00ad", "&shy;",
)

type RenderMetadata struct {
	Preheader string
}

func RenderEditableHTML(templateHTML string, fields EditableFields) (string, error) {
	return RenderEditableHTMLWithMetadata(templateHTML, fields, RenderMetadata{})
}

func RenderEditableHTMLWithMetadata(templateHTML string, fields EditableFields, metadata RenderMetadata) (string, error) {
	root, err := xhtml.Parse(strings.NewReader(templateHTML))
	if err != nil {
		return "", err
	}

	if err := renderNodeFields(root, fields); err != nil {
		return "", err
	}
	renderMetadataTargets(root, metadata)

	var out bytes.Buffer
	if err := xhtml.Render(&out, root); err != nil {
		return "", err
	}

	rendered, err := replaceTemplateMarkers(out.String(), fields)
	if err != nil {
		return "", err
	}
	return emailSafeOutput(rendered), nil
}

func emailSafeOutput(rendered string) string {
	rendered = invisibleCharEntities.Replace(rendered)
	return bitrixExpressionPattern.ReplaceAllStringFunc(rendered, html.UnescapeString)
}

func renderMetadataTargets(node *xhtml.Node, metadata RenderMetadata) {
	if node.Type == xhtml.ElementNode {
		if _, ok := attrValue(node, "data-email-preheader"); ok {
			replaceChildrenWithText(node, metadata.Preheader)
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderMetadataTargets(child, metadata)
	}
}

func renderNodeFields(node *xhtml.Node, fields EditableFields) error {
	if node.Type == xhtml.ElementNode {
		if key, ok := attrValue(node, "data-edit-text"); ok {
			value, err := stringFieldValue(fields, key)
			if err != nil {
				return err
			}
			replaceChildrenWithText(node, value)
		}

		if key, ok := attrValue(node, "data-edit-attr-href"); ok {
			value, err := urlFieldValue(fields, key)
			if err != nil {
				return err
			}
			setAttr(node, "href", value)
		}

		if key, ok := attrValue(node, "data-edit-attr-src"); ok {
			value, err := urlFieldValue(fields, key)
			if err != nil {
				return err
			}
			setAttr(node, "src", value)
		}

		if key, ok := attrValue(node, "data-edit-attr-alt"); ok {
			value, err := stringFieldValue(fields, key)
			if err != nil {
				return err
			}
			setAttr(node, "alt", value)
		}

		if key, ok := attrValue(node, "data-edit-style-width-px"); ok {
			value, err := intFieldValue(fields, key)
			if err != nil {
				return err
			}
			setAttr(node, "style", setStyleWidthPX(attrValueOrEmpty(node, "style"), value))
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := renderNodeFields(child, fields); err != nil {
			return err
		}
	}

	return nil
}

func replaceChildrenWithText(node *xhtml.Node, value string) {
	node.FirstChild = nil
	node.LastChild = nil

	lines := strings.Split(normalizeEditableTextBreaks(value), "\n")
	for i, line := range lines {
		if i > 0 {
			node.AppendChild(&xhtml.Node{
				Type: xhtml.ElementNode,
				Data: "br",
			})
		}

		if line != "" {
			node.AppendChild(&xhtml.Node{
				Type: xhtml.TextNode,
				Data: line,
			})
		}
	}
}

func normalizeEditableTextBreaks(value string) string {
	lines := strings.Split(value, "\n")
	normalizedLines := make([]string, 0, len(lines))
	blankLinePending := false

	for _, line := range lines {
		line = strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(line) == "" {
			blankLinePending = len(normalizedLines) > 0
			continue
		}

		if blankLinePending {
			normalizedLines = append(normalizedLines, "")
		}
		normalizedLines = append(normalizedLines, line)
		blankLinePending = false
	}

	return strings.Join(normalizedLines, "\n")
}

func setAttr(node *xhtml.Node, key string, value string) {
	for i, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			node.Attr[i].Val = value
			return
		}
	}

	node.Attr = append(node.Attr, xhtml.Attribute{Key: key, Val: value})
}

func setStyleWidthPX(style string, width int) string {
	parts := strings.Split(style, ";")
	nextParts := make([]string, 0, len(parts)+1)
	replaced := false

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		name, _, ok := strings.Cut(trimmed, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "width") {
			nextParts = append(nextParts, fmt.Sprintf("width: %dpx", width))
			replaced = true
			continue
		}

		nextParts = append(nextParts, trimmed)
	}

	if !replaced {
		nextParts = append(nextParts, fmt.Sprintf("width: %dpx", width))
	}

	return strings.Join(nextParts, "; ")
}

func replaceTemplateMarkers(templateHTML string, fields EditableFields) (string, error) {
	var replacementErr error

	rendered := templateMarkerPattern.ReplaceAllStringFunc(templateHTML, func(marker string) string {
		if replacementErr != nil {
			return marker
		}

		matches := templateMarkerPattern.FindStringSubmatch(marker)
		if len(matches) != 2 {
			return marker
		}

		key := matches[1]
		field, ok := fields[key]
		if !ok {
			// Not one of ours: a Bitrix24 field such as {{Responsible}}.
			return marker
		}

		switch field.Type {
		case FieldTypeNumber:
			value, err := intFieldValue(fields, key)
			if err != nil {
				replacementErr = err
				return marker
			}
			return strconv.Itoa(value)
		case FieldTypeURL, FieldTypeImage:
			value, err := urlFieldValue(fields, key)
			if err != nil {
				replacementErr = err
				return marker
			}
			return html.EscapeString(value)
		default:
			value, err := stringFieldValue(fields, key)
			if err != nil {
				replacementErr = err
				return marker
			}
			return html.EscapeString(value)
		}
	})

	if replacementErr != nil {
		return "", replacementErr
	}

	return rendered, nil
}

func stringFieldValue(fields EditableFields, key string) (string, error) {
	field, ok := fields[key]
	if !ok {
		return "", fmt.Errorf("editable field %q is missing", key)
	}

	value, ok := field.Value.(string)
	if !ok {
		return "", fmt.Errorf("editable field %q must be a string", key)
	}

	return value, nil
}

func urlFieldValue(fields EditableFields, key string) (string, error) {
	value, err := stringFieldValue(fields, key)
	if err != nil {
		return "", err
	}

	// Bitrix24 fills the link in when it sends the email, e.g. the responsible
	// manager's booking link; a URL that carries expressions in its query is
	// checked with them swapped for a placeholder.
	if IsBitrixExpression(value) {
		return value, nil
	}
	parsed, err := url.Parse(bitrixExpressionPattern.ReplaceAllString(value, "x"))
	if err != nil {
		return "", fmt.Errorf("editable field %q has invalid URL: %w", key, err)
	}

	field := fields[key]
	allowedScheme := parsed.Scheme == "http" || parsed.Scheme == "https"
	if field.Type == FieldTypeURL && parsed.Scheme == "mailto" {
		allowedScheme = true
	}
	if !allowedScheme {
		return "", fmt.Errorf("editable field %q has disallowed URL scheme %q", key, parsed.Scheme)
	}

	return value, nil
}

func intFieldValue(fields EditableFields, key string) (int, error) {
	field, ok := fields[key]
	if !ok {
		return 0, fmt.Errorf("editable field %q is missing", key)
	}

	switch value := field.Value.(type) {
	case int:
		return value, nil
	case float64:
		return int(value), nil
	case json.Number:
		return strconv.Atoi(string(value))
	default:
		return 0, fmt.Errorf("editable field %q must be a number", key)
	}
}
