package emailedit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	xhtml "golang.org/x/net/html"
)

// FormatMarkdown, as the format of a text field, makes its value Markdown:
// **bold**, *italic*, [links](url), # headings and - lists. Text without any
// of it renders exactly like a plain text field: lines joined with <br>,
// paragraphs with <br><br>.
const FormatMarkdown = "markdown"

// markdownContainers are the elements a Markdown field can fill: headings
// and lists are blocks, they cannot go inside a link or a span.
var markdownContainers = map[string]bool{"td": true, "th": true, "div": true}

// CanFormat reports whether the field on an element with this tag can be
// Markdown.
func CanFormat(tag string) bool {
	return markdownContainers[strings.ToLower(tag)]
}

// markdownParser knows only what an email text needs. Code, quotes, raw
// HTML, rules and autolinks stay plain text, so ordinary punctuation does
// not turn into markup by surprise.
var markdownParser = parser.NewParser(
	parser.WithBlockParsers(
		util.Prioritized(parser.NewListParser(), 300),
		util.Prioritized(parser.NewListItemParser(), 400),
		util.Prioritized(parser.NewATXHeadingParser(), 600),
		util.Prioritized(parser.NewParagraphParser(), 1000),
	),
	parser.WithInlineParsers(
		util.Prioritized(parser.NewLinkParser(), 200),
		util.Prioritized(parser.NewEmphasisParser(), 500),
	),
)

// Bitrix24 expressions are swapped for placeholders while parsing: their
// spaces and ">" would break a link, and they must reach the robot as they
// are.
const (
	expressionStart = "\ue000"
	expressionEnd   = "\ue001"
)

var expressionPlaceholderPattern = regexp.MustCompile(expressionStart + `(\d+)` + expressionEnd)

const (
	markdownLinkStyle = "color: #409eef; text-decoration: underline;"
	markdownFontStyle = "font-family: Helvetica, Arial, sans-serif;"
)

// renderMarkdownInto replaces the children of node with value rendered from
// Markdown. Headings take the text color of the element around them, since
// Outlook colors headings its own way otherwise.
func renderMarkdownInto(node *xhtml.Node, key, value string) error {
	expressions := []string{}
	source := bitrixExpressionPattern.ReplaceAllStringFunc(normalizeEditableTextBreaks(value), func(match string) string {
		expressions = append(expressions, match)
		return expressionStart + strconv.Itoa(len(expressions)-1) + expressionEnd
	})
	restore := func(s string) string {
		return expressionPlaceholderPattern.ReplaceAllStringFunc(s, func(match string) string {
			index, _ := strconv.Atoi(expressionPlaceholderPattern.FindStringSubmatch(match)[1])
			if index < len(expressions) {
				return expressions[index]
			}
			return match
		})
	}

	renderer := markdownRenderer{
		source:  []byte(source),
		restore: restore,
		key:     key,
		color:   inheritedColor(node),
	}
	document := markdownParser.Parse(text.NewReader(renderer.source))

	node.FirstChild = nil
	node.LastChild = nil
	return renderer.blocks(node, document)
}

type markdownRenderer struct {
	source  []byte
	restore func(string) string
	key     string
	color   string
}

// blocks renders the block children of parent: paragraphs in a row are
// separated with <br><br> like plain text; headings and lists are elements
// with margins, none at the edges.
func (renderer markdownRenderer) blocks(target *xhtml.Node, parent ast.Node) error {
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		first, last := child.PreviousSibling() == nil, child.NextSibling() == nil
		switch block := child.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			if previous, ok := child.PreviousSibling().(*ast.Paragraph); ok && previous != nil {
				target.AppendChild(element("br", ""))
				target.AppendChild(element("br", ""))
			}
			if err := renderer.inlines(target, child); err != nil {
				return err
			}
		case *ast.Heading:
			size, height := "16px", "24px"
			switch block.Level {
			case 1:
				size, height = "22px", "28px"
			case 2:
				size, height = "18px", "24px"
			}
			style := fmt.Sprintf("margin: %s 0 %s 0; %s font-size: %s; line-height: %s; font-weight: 700;",
				edgeMargin(first, "24px"), edgeMargin(last, "12px"), markdownFontStyle, size, height)
			if renderer.color != "" {
				style += " color: " + renderer.color + ";"
			}
			heading := element("h"+strconv.Itoa(min(block.Level+1, 4)), style)
			if err := renderer.inlines(heading, child); err != nil {
				return err
			}
			target.AppendChild(heading)
		case *ast.List:
			tag := "ul"
			if block.IsOrdered() {
				tag = "ol"
			}
			list := element(tag, fmt.Sprintf("margin: %s 0 %s 0; padding: 0 0 0 24px;",
				edgeMargin(first, "16px"), edgeMargin(last, "16px")))
			if block.IsOrdered() && block.Start != 1 {
				list.Attr = append(list.Attr, xhtml.Attribute{Key: "start", Val: strconv.Itoa(block.Start)})
			}
			for item := child.FirstChild(); item != nil; item = item.NextSibling() {
				style := "margin: 0 0 8px 0;"
				if item.NextSibling() == nil {
					style = "margin: 0;"
				}
				li := element("li", style)
				if err := renderer.blocks(li, item); err != nil {
					return err
				}
				list.AppendChild(li)
			}
			target.AppendChild(list)
		default:
			if err := renderer.blocks(target, child); err != nil {
				return err
			}
		}
	}
	return nil
}

func (renderer markdownRenderer) inlines(target *xhtml.Node, parent ast.Node) error {
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		switch inline := child.(type) {
		case *ast.Text:
			value := string(util.UnescapePunctuations(inline.Segment.Value(renderer.source)))
			if inline.SoftLineBreak() || inline.HardLineBreak() {
				value = strings.TrimRight(value, " \t")
			}
			appendText(target, renderer.restore(value))
			if inline.SoftLineBreak() || inline.HardLineBreak() {
				target.AppendChild(element("br", ""))
			}
		case *ast.String:
			appendText(target, renderer.restore(string(inline.Value)))
		case *ast.Emphasis:
			tag := "em"
			if inline.Level >= 2 {
				tag = "strong"
			}
			wrapper := element(tag, "")
			if err := renderer.inlines(wrapper, child); err != nil {
				return err
			}
			target.AppendChild(wrapper)
		case *ast.Link:
			href := renderer.restore(string(util.UnescapePunctuations(inline.Destination)))
			if err := checkLinkURL(href, true); err != nil {
				return fmt.Errorf("editable field %q: link %q: %w", renderer.key, href, err)
			}
			link := element("a", markdownLinkStyle)
			link.Attr = append([]xhtml.Attribute{{Key: "href", Val: href}, {Key: "target", Val: "_blank"}}, link.Attr...)
			if err := renderer.inlines(link, child); err != nil {
				return err
			}
			target.AppendChild(link)
		default:
			if err := renderer.inlines(target, child); err != nil {
				return err
			}
		}
	}
	return nil
}

func edgeMargin(atEdge bool, margin string) string {
	if atEdge {
		return "0"
	}
	return margin
}

func element(tag, style string) *xhtml.Node {
	node := &xhtml.Node{Type: xhtml.ElementNode, Data: tag}
	if style != "" {
		node.Attr = []xhtml.Attribute{{Key: "style", Val: style}}
	}
	return node
}

// appendText adds text to node, merged with a text node right before it so
// the output matches a plain text field.
func appendText(node *xhtml.Node, value string) {
	if value == "" {
		return
	}
	if last := node.LastChild; last != nil && last.Type == xhtml.TextNode {
		last.Data += value
		return
	}
	node.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: value})
}

var colorPattern = regexp.MustCompile(`(?i)(?:^|;)\s*color\s*:\s*([^;]+)`)

func inheritedColor(node *xhtml.Node) string {
	for current := node; current != nil; current = current.Parent {
		if current.Type != xhtml.ElementNode {
			continue
		}
		if match := colorPattern.FindStringSubmatch(attrValueOrEmpty(current, "style")); match != nil {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}

// ValidateFormat checks the format of a field and, for Markdown, its links,
// so a save can say what is wrong before the email is rendered.
func ValidateFormat(key string, field EditableField) error {
	switch field.Format {
	case "":
		return nil
	case FormatMarkdown:
	default:
		return fmt.Errorf("field %s: unknown format %q", key, field.Format)
	}
	if field.Type != FieldTypeText {
		return fmt.Errorf("field %s: only a text field can be formatted", key)
	}
	value, ok := field.Value.(string)
	if !ok {
		return fmt.Errorf("field %s: the value must be text", key)
	}
	return renderMarkdownInto(&xhtml.Node{Type: xhtml.ElementNode, Data: "div"}, key, value)
}

// FormattableFields lists the text fields of a template that can be
// Markdown: those on a td, th or div.
func FormattableFields(templateHTML string) (map[string]bool, error) {
	root, err := xhtml.Parse(strings.NewReader(templateHTML))
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			if key, ok := attrValue(node, "data-edit-text"); ok && CanFormat(node.Data) {
				keys[strings.TrimSpace(key)] = true
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return keys, nil
}

// FormatEditedFields makes Markdown every formattable text field of next
// whose value differs from previous. Text without markup renders the same
// either way, so a field switches the first time someone edits it, in a
// save that writes a new version anyway, and templates stay as they are.
func FormatEditedFields(templateHTML string, previous, next EditableFields) error {
	formattable, err := FormattableFields(templateHTML)
	if err != nil {
		return err
	}
	for key, field := range next {
		if field.Type != FieldTypeText || field.Shared || field.Format != "" || !formattable[key] {
			continue
		}
		if old, ok := previous[key]; ok && old.Value == field.Value {
			continue
		}
		field.Format = FormatMarkdown
		next[key] = field
	}
	return nil
}
