package emailedit

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

func ExtractEditableFields(templateHTML string) (EditableFields, error) {
	root, err := html.Parse(strings.NewReader(templateHTML))
	if err != nil {
		return nil, err
	}

	extractor := editableFieldsExtractor{
		fields: EditableFields{},
	}
	if err := extractor.extractNodeFields(root); err != nil {
		return nil, err
	}

	return extractor.fields, nil
}

type editableFieldsExtractor struct {
	fields EditableFields
	order  int
}

func (extractor *editableFieldsExtractor) extractNodeFields(node *html.Node) error {
	if node.Type == html.ElementNode {
		if key, ok := attrValue(node, "data-edit-text"); ok {
			if err := extractor.setField(key, EditableField{
				Type:  FieldTypeText,
				Value: textContentWithBreaks(node),
			}); err != nil {
				return err
			}
		}

		if key, ok := attrValue(node, "data-edit-attr-href"); ok {
			if err := extractor.setStringAttrField(key, FieldTypeURL, node, "href"); err != nil {
				return err
			}
		}

		if key, ok := attrValue(node, "data-edit-attr-src"); ok {
			if err := extractor.setStringAttrField(key, FieldTypeImage, node, "src"); err != nil {
				return err
			}
		}

		if key, ok := attrValue(node, "data-edit-attr-alt"); ok {
			if err := extractor.setStringAttrField(key, FieldTypeText, node, "alt"); err != nil {
				return err
			}
		}

		if key, ok := attrValue(node, "data-edit-style-width-px"); ok {
			width, err := styleWidthPX(attrValueOrEmpty(node, "style"))
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			if err := extractor.setField(key, EditableField{
				Type:  FieldTypeNumber,
				Value: width,
			}); err != nil {
				return err
			}
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := extractor.extractNodeFields(child); err != nil {
			return err
		}
	}

	return nil
}

func (extractor *editableFieldsExtractor) setStringAttrField(key string, fieldType string, node *html.Node, attr string) error {
	value, ok := attrValue(node, attr)
	if !ok {
		value = ""
	}

	return extractor.setField(key, EditableField{
		Type:  fieldType,
		Value: value,
	})
}

func (extractor *editableFieldsExtractor) setField(key string, field EditableField) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("editable field key is empty")
	}

	existing, ok := extractor.fields[key]
	if ok && (existing.Type != field.Type || fmt.Sprintf("%v", existing.Value) != fmt.Sprintf("%v", field.Value)) {
		return fmt.Errorf("editable field %q has conflicting values", key)
	}
	if ok {
		return nil
	}

	extractor.order++
	field.Order = extractor.order
	extractor.fields[key] = field
	return nil
}

func attrValue(node *html.Node, key string) (string, bool) {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val, true
		}
	}

	return "", false
}

func attrValueOrEmpty(node *html.Node, key string) string {
	value, _ := attrValue(node, key)
	return value
}

func textContentWithBreaks(node *html.Node) string {
	var builder strings.Builder
	writeTextContentWithBreaks(&builder, node)
	return normalizeEditableText(builder.String())
}

func writeTextContentWithBreaks(builder *strings.Builder, node *html.Node) {
	if node.Type == html.TextNode {
		appendTextSegment(builder, strings.Join(strings.Fields(node.Data), " "))
		return
	}

	if node.Type == html.ElementNode && strings.EqualFold(node.Data, "br") {
		builder.WriteString("\n")
		return
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		writeTextContentWithBreaks(builder, child)
	}
}

func appendTextSegment(builder *strings.Builder, segment string) {
	if segment == "" {
		return
	}

	current := builder.String()
	if current != "" && !strings.HasSuffix(current, "\n") && !strings.HasSuffix(current, " ") {
		builder.WriteString(" ")
	}
	builder.WriteString(segment)
}

func normalizeEditableText(value string) string {
	lines := strings.Split(value, "\n")
	normalizedLines := make([]string, 0, len(lines))
	blankLinePending := false

	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}

	for _, line := range lines {
		if line == "" {
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

func styleWidthPX(style string) (int, error) {
	for _, part := range strings.Split(style, ";") {
		name, value, ok := strings.Cut(part, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "width") {
			continue
		}

		widthValue := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "px"))
		width, err := strconv.Atoi(widthValue)
		if err != nil {
			return 0, fmt.Errorf("invalid width value %q", strings.TrimSpace(value))
		}

		return width, nil
	}

	return 0, fmt.Errorf("width style is missing")
}
