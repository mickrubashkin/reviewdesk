package emailedit

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// markdownFromFormattedText reads the text of a field element that holds
// bold, italics or links as Markdown, so the formatting survives the first
// render. ok is false when the element holds none of them: the field stays
// plain text.
func markdownFromFormattedText(node *html.Node) (value string, ok bool) {
	if !CanFormat(node.Data) || !holdsInlineFormatting(node) {
		return "", false
	}
	var builder strings.Builder
	writeMarkdownChildren(&builder, node)
	lines := strings.Split(normalizeEditableText(builder.String()), "\n")
	for i, line := range lines {
		lines[i] = escapeBlockMarker(line)
	}
	return strings.Join(lines, "\n"), true
}

func holdsInlineFormatting(node *html.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		if _, ok := markdownDelimiter(child); ok && strings.TrimSpace(textContentWithBreaks(child)) != "" {
			return true
		}
		if holdsInlineFormatting(child) {
			return true
		}
	}
	return false
}

func writeMarkdownChildren(builder *strings.Builder, node *html.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		writeMarkdown(builder, child)
	}
}

var whitespaceRun = regexp.MustCompile(`[ \t\r\n\f]+`)

func writeMarkdown(builder *strings.Builder, node *html.Node) {
	switch node.Type {
	case html.TextNode:
		builder.WriteString(escapeMarkdownText(whitespaceRun.ReplaceAllString(node.Data, " ")))
		return
	case html.ElementNode:
	default:
		return
	}

	tag := strings.ToLower(node.Data)
	if tag == "br" {
		builder.WriteString("\n")
		return
	}
	var inner strings.Builder
	writeMarkdownChildren(&inner, node)
	delimiter, ok := markdownDelimiter(node)
	if !ok {
		builder.WriteString(inner.String())
		return
	}
	if tag == "a" {
		href := strings.TrimSpace(attrValueOrEmpty(node, "href"))
		builder.WriteString(wrapLines(inner.String(), "[", "]("+markdownDestination(href)+")"))
		return
	}
	builder.WriteString(wrapLines(inner.String(), delimiter, delimiter))
}

// markdownDelimiter tells the Markdown an element turns into: ** for bold,
// * for italics, and "[" for a link with an address a Markdown link takes.
func markdownDelimiter(node *html.Node) (string, bool) {
	switch strings.ToLower(node.Data) {
	case "strong", "b":
		return "**", true
	case "em", "i":
		return "*", true
	case "a":
		href := strings.TrimSpace(attrValueOrEmpty(node, "href"))
		if href != "" && checkLinkURL(href, true) == nil {
			return "[", true
		}
	}
	return "", false
}

// wrapLines wraps each line of value in open and close, leaving the spaces
// around the text outside: "** bold**" would not be bold, and emphasis ends
// at a blank line.
func wrapLines(value, open, close string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		start := strings.Index(line, text)
		lines[i] = line[:start] + open + text + close + line[start+len(text):]
	}
	return strings.Join(lines, "\n")
}

// markdownDestination puts an address with spaces or brackets in <…>, where
// Markdown takes them as they are. Bitrix24 expressions are swapped out
// before parsing, so their spaces do not count.
func markdownDestination(href string) string {
	if strings.ContainsAny(bitrixExpressionPattern.ReplaceAllString(href, "x"), " ()") {
		return "<" + href + ">"
	}
	return href
}

// escapeMarkdownText keeps text that would read as markup as it is: \, *
// and brackets always, _ only at a word's edge, so "#USER_NAME#" stays
// readable. Bitrix24 expressions pass through unchanged.
func escapeMarkdownText(value string) string {
	var builder strings.Builder
	last := 0
	for _, match := range bitrixExpressionPattern.FindAllStringIndex(value, -1) {
		builder.WriteString(escapeMarkdownPunctuation(value[last:match[0]]))
		builder.WriteString(value[match[0]:match[1]])
		last = match[1]
	}
	builder.WriteString(escapeMarkdownPunctuation(value[last:]))
	return builder.String()
}

func escapeMarkdownPunctuation(value string) string {
	var builder strings.Builder
	for i, r := range value {
		switch r {
		case '\\', '*', '[', ']':
			builder.WriteByte('\\')
		case '_':
			before, _ := utf8.DecodeLastRuneInString(value[:i])
			after, _ := utf8.DecodeRuneInString(value[i+1:])
			if !isWordRune(before) || !isWordRune(after) {
				builder.WriteByte('\\')
			}
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func isWordRune(r rune) bool {
	return r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

var (
	headingOrBulletMarker = regexp.MustCompile(`^(#{1,6}|[-+])( |$)`)
	orderedListMarker     = regexp.MustCompile(`^(\d{1,9})([.)])( |$)`)
)

// escapeBlockMarker keeps a line that starts like a heading or a list item
// a line of text.
func escapeBlockMarker(line string) string {
	if headingOrBulletMarker.MatchString(line) {
		return `\` + line
	}
	if match := orderedListMarker.FindStringSubmatchIndex(line); match != nil {
		return line[:match[4]] + `\` + line[match[4]:]
	}
	return line
}
