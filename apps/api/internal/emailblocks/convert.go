package emailblocks

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"golang.org/x/net/html"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// Match says how close an email built from blocks renders to the email in
// the old format it is converted from.
type Match string

const (
	// MatchSame: the same HTML up to whitespace and the <title>, which mail
	// clients do not show (the subject comes from the Bitrix24 robot).
	MatchSame Match = "same"
	// MatchContent: the same text, links and images; the markup differs
	// (inline styles, spacing). The email looks almost the same.
	MatchContent Match = "content"
	// MatchNone: the email cannot be converted automatically.
	MatchNone Match = "none"
)

// Conversion is what switching an email in the old format to blocks gives.
type Conversion struct {
	Match Match
	// Reason explains MatchNone; for MatchContent it shows the first place
	// where the markup differs.
	Reason string
	Layout Layout
	// Fields are the email's fields for the new template; save them with
	// the layout.
	Fields emailedit.EditableFields
	// Recovered are fields the old template had as fixed markup (a button
	// width) and the new one makes editable; their values come from the old
	// template.
	Recovered []string
	// LostReviewBlocks are review blocks of the old template the new one
	// does not have; comments on them lose their place.
	LostReviewBlocks []string
	// MovedReviewBlocks are lost review blocks whose text went to a block
	// of the new template (a list item to the list); their comments can
	// move there.
	MovedReviewBlocks map[string]string
}

// Convert works out the blocks of an email in the old format from its
// fields: every field must belong to a block of the library (after the keys
// of the old templates are renamed, see legacyFields), and the blocks go in
// the order their fields come in the template. The email built from
// them is rendered and compared with the old one.
func Convert(library []Block, template string, fields emailedit.EditableFields, metadata emailedit.RenderMetadata) Conversion {
	return ConvertWith(library, template, fields, metadata, ConvertOptions{})
}

// ConvertOptions change how the fields of an old template map to blocks.
type ConvertOptions struct {
	// OneFieldSignature puts a greeting and sender in two fields into the
	// one-field signature, so a translation gets the blocks of an EN email
	// written that way.
	OneFieldSignature bool
}

// ConvertWith is Convert with options.
func ConvertWith(library []Block, template string, fields emailedit.EditableFields, metadata emailedit.RenderMetadata, options ConvertOptions) Conversion {
	if len(fields) == 0 {
		return Conversion{Match: MatchNone, Reason: "no editable fields"}
	}
	renamed, moved, err := legacyFields(template, fields, options)
	if err != nil {
		return Conversion{Match: MatchNone, Reason: err.Error()}
	}
	layout, err := layoutFromFields(library, renamed)
	if err != nil {
		return Conversion{Match: MatchNone, Reason: err.Error()}
	}
	built, err := layout.Build(library)
	if err != nil {
		return Conversion{Match: MatchNone, Reason: err.Error()}
	}

	own := emailedit.EditableFields{}
	for key, field := range renamed {
		own[key] = field
	}
	recovered, err := recoverFixedValues(template, built, own)
	if err != nil {
		return Conversion{Match: MatchNone, Reason: err.Error()}
	}
	for key, field := range recovered {
		own[key] = field
	}
	newFields, err := layout.Fields(library, built, own)
	if err != nil {
		return Conversion{Match: MatchNone, Reason: err.Error()}
	}

	before, err := emailedit.RenderEditableHTMLWithMetadata(template, fields, metadata)
	if err != nil {
		return Conversion{Match: MatchNone, Reason: "the email does not render: " + err.Error()}
	}
	after, err := emailedit.RenderEditableHTMLWithMetadata(built, newFields, metadata)
	if err != nil {
		return Conversion{Match: MatchNone, Reason: "the blocks do not render: " + err.Error()}
	}

	lost := missing(reviewBlocks(template), reviewBlocks(built))
	conversion := Conversion{
		Layout:            layout,
		Fields:            newFields,
		Recovered:         sortedKeys(recovered),
		LostReviewBlocks:  lost,
		MovedReviewBlocks: movedReviewBlocks(template, built, lost, moved),
	}
	normalBefore, normalAfter := normalizeForCompare(before), normalizeForCompare(after)
	if normalBefore == normalAfter {
		conversion.Match = MatchSame
		return conversion
	}
	if difference := contentDifference(before, after); difference != "" {
		conversion.Match = MatchNone
		conversion.Reason = difference
		return conversion
	}
	conversion.Match = MatchContent
	conversion.Reason = "markup: " + FirstDifference(normalBefore, normalAfter) + " → " + FirstDifference(normalAfter, normalBefore)
	return conversion
}

// layoutFromFields puts the blocks that own the email's fields in the order
// the fields come; a field no block has stops the conversion.
func layoutFromFields(library []Block, fields emailedit.EditableFields) (Layout, error) {
	owner := map[string]string{}
	for _, block := range library {
		for _, field := range block.Fields {
			owner[field.Key] = block.Key
		}
	}
	unknown := []string{}
	first := map[string]int{}
	for key, field := range fields {
		block, ok := owner[key]
		if !ok {
			unknown = append(unknown, key)
			continue
		}
		if order, seen := first[block]; !seen || field.Order < order {
			first[block] = field.Order
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("fields no block has: %s", strings.Join(unknown, ", "))
	}
	layout := Layout{}
	for block := range first {
		layout = append(layout, LayoutItem{Block: block})
	}
	sort.Slice(layout, func(i, j int) bool { return first[layout[i].Block] < first[layout[j].Block] })
	return layout, nil
}

// recoverFixedValues reads the values of the fields the new template has and
// the email does not from the old template: the field is put on the element
// of the old template that carries a field it shares an element with in the
// new one (the button width goes on the button). A field with no such
// element keeps the block's value.
func recoverFixedValues(oldTemplate, newTemplate string, fields emailedit.EditableFields) (emailedit.EditableFields, error) {
	newRoot, err := html.Parse(strings.NewReader(newTemplate))
	if err != nil {
		return nil, err
	}
	type placement struct{ attr, key, anchor string }
	placements := []placement{}
	walkElements(newRoot, func(node *html.Node) {
		attrs := editAttrs(node)
		for _, missing := range attrs {
			if _, ok := fields[missing.Val]; ok {
				continue
			}
			for _, anchor := range attrs {
				if _, ok := fields[anchor.Val]; ok {
					placements = append(placements, placement{missing.Key, missing.Val, anchor.Val})
					break
				}
			}
		}
	})
	if len(placements) == 0 {
		return emailedit.EditableFields{}, nil
	}

	oldRoot, err := html.Parse(strings.NewReader(oldTemplate))
	if err != nil {
		return nil, err
	}
	placed := map[string]bool{}
	walkElements(oldRoot, func(node *html.Node) {
		for _, attr := range editAttrs(node) {
			for _, p := range placements {
				if p.anchor == attr.Val && !placed[p.key] {
					node.Attr = append(node.Attr, html.Attribute{Key: p.attr, Val: p.key})
					placed[p.key] = true
				}
			}
		}
	})
	var out strings.Builder
	if err := html.Render(&out, oldRoot); err != nil {
		return nil, err
	}
	extracted, err := emailedit.ExtractEditableFields(out.String())
	if err != nil {
		return nil, fmt.Errorf("reading %s from the old template: %w", strings.Join(sortedKeys(placed), ", "), err)
	}
	recovered := emailedit.EditableFields{}
	for key := range placed {
		recovered[key] = extracted[key]
	}
	return recovered, nil
}

func editAttrs(node *html.Node) []html.Attribute {
	attrs := []html.Attribute{}
	for _, attr := range node.Attr {
		if strings.HasPrefix(attr.Key, "data-edit-") {
			attrs = append(attrs, attr)
		}
	}
	return attrs
}

func walkElements(node *html.Node, visit func(*html.Node)) {
	if node.Type == html.ElementNode {
		visit(node)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkElements(child, visit)
	}
}

func reviewBlocks(template string) []string {
	root, err := html.Parse(strings.NewReader(template))
	if err != nil {
		return nil
	}
	blocks := []string{}
	walkElements(root, func(node *html.Node) {
		for _, attr := range node.Attr {
			if attr.Key == "data-review-block" && !slices.Contains(blocks, attr.Val) {
				blocks = append(blocks, attr.Val)
			}
		}
	})
	return blocks
}

// movedReviewBlocks finds, for each lost review block, the block of the new
// template that has the text of its fields.
func movedReviewBlocks(oldTemplate, newTemplate string, lost []string, movedFields map[string]string) map[string]string {
	moved := map[string]string{}
	root, err := html.Parse(strings.NewReader(oldTemplate))
	if err != nil {
		return moved
	}
	newBlocks := reviewBlocks(newTemplate)
	walkElements(root, func(node *html.Node) {
		block := attr(node, "data-review-block")
		if block == "" || !slices.Contains(lost, block) || moved[block] != "" {
			return
		}
		walkElements(node, func(inner *html.Node) {
			for _, field := range editAttrs(inner) {
				target, ok := movedFields[field.Val]
				if !ok {
					target = field.Val
				}
				if moved[block] == "" && slices.Contains(newBlocks, target) {
					moved[block] = target
				}
			}
		})
	})
	return moved
}

func missing(from, in []string) []string {
	out := []string{}
	for _, value := range from {
		if !slices.Contains(in, value) {
			out = append(out, value)
		}
	}
	return out
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var (
	titleElement = regexp.MustCompile(`(?is)<title[^>]*>.*?</title>`)
	spaces       = regexp.MustCompile(`\s+`)
	spaceBetween = regexp.MustCompile(`>\s+<`)
)

// normalizeForCompare drops what a reader of the email does not get: the
// <title>, the lang of the document (an email built from blocks gets its
// email's), whitespace between tags and inside styles, and the data-edit-*
// marks of the editor.
func normalizeForCompare(rendered string) string {
	rendered = titleElement.ReplaceAllString(rendered, "")
	if root, err := html.Parse(strings.NewReader(rendered)); err == nil {
		walkElements(root, func(node *html.Node) {
			attrs := node.Attr[:0]
			for _, a := range node.Attr {
				if strings.HasPrefix(a.Key, "data-edit-") || node.Data == "html" && a.Key == "lang" {
					continue
				}
				if a.Key == "style" {
					a.Val = normalizeStyle(a.Val)
				}
				attrs = append(attrs, a)
			}
			node.Attr = attrs
		})
		var out strings.Builder
		if err := html.Render(&out, root); err == nil {
			rendered = out.String()
		}
	}
	rendered = spaces.ReplaceAllString(rendered, " ")
	return strings.TrimSpace(spaceBetween.ReplaceAllString(rendered, "><"))
}

func normalizeStyle(style string) string {
	declarations := []string{}
	for _, declaration := range strings.Split(style, ";") {
		property, value, ok := strings.Cut(declaration, ":")
		if !ok {
			if declaration = strings.TrimSpace(declaration); declaration != "" {
				declarations = append(declarations, declaration)
			}
			continue
		}
		declarations = append(declarations, strings.TrimSpace(property)+": "+spaces.ReplaceAllString(strings.TrimSpace(value), " "))
	}
	return strings.Join(declarations, "; ")
}

// emailContent is what a reader gets from an email: its text and the links
// and images in it, in order. Comments (the Outlook button) are left out:
// they repeat the text and link of the button.
type emailContent struct {
	text   string
	links  []string
	images []string
}

func contentOf(rendered string) emailContent {
	content := emailContent{links: []string{}, images: []string{}}
	root, err := html.Parse(strings.NewReader(rendered))
	if err != nil {
		return content
	}
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		switch node.Type {
		case html.TextNode:
			// A bullet drawn as text in the old lists is a list marker in
			// the new ones.
			if strings.TrimSpace(node.Data) != "•" {
				text.WriteString(" " + node.Data)
			}
		case html.ElementNode:
			switch node.Data {
			case "head", "style", "script", "title":
				return
			case "a":
				content.links = append(content.links, attr(node, "href"))
			case "img":
				content.images = append(content.images, attr(node, "src")+" ("+attr(node, "alt")+")")
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	content.text = strings.TrimSpace(spaces.ReplaceAllString(strings.ReplaceAll(text.String(), " ", " "), " "))
	return content
}

func attr(node *html.Node, key string) string {
	for _, a := range node.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// contentDifference describes the first difference in text, links or
// images, or returns "" when there is none.
func contentDifference(before, after string) string {
	a, b := contentOf(before), contentOf(after)
	if a.text != b.text {
		return "text: " + FirstDifference(a.text, b.text) + " → " + FirstDifference(b.text, a.text)
	}
	if !slices.Equal(a.links, b.links) {
		return fmt.Sprintf("links: %s → %s", strings.Join(a.links, ", "), strings.Join(b.links, ", "))
	}
	if !slices.Equal(a.images, b.images) {
		return fmt.Sprintf("images: %s → %s", strings.Join(a.images, ", "), strings.Join(b.images, ", "))
	}
	return ""
}

// FirstDifference shows a around the first place it differs from b.
func FirstDifference(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	start, end := max(i-40, 0), min(i+60, len(a))
	for start > 0 && !utf8Start(a[start]) {
		start--
	}
	for end < len(a) && !utf8Start(a[end]) {
		end++
	}
	return "«…" + a[start:end] + "…»"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
