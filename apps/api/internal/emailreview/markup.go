package emailreview

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

func AddReviewBlocks(originalHTML string) (string, error) {
	if strings.Contains(originalHTML, "data-review-block") {
		return originalHTML, nil
	}

	root, err := html.Parse(strings.NewReader(originalHTML))
	if err != nil {
		return "", err
	}

	counters := map[string]int{}
	_ = markReviewBlocks(root, counters)

	var out bytes.Buffer
	err = html.Render(&out, root)
	if err != nil {
		return "", err
	}

	return out.String(), nil
}

func markReviewBlocks(node *html.Node, counters map[string]int) bool {
	hasMarkedChild := false

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if markReviewBlocks(child, counters) {
			hasMarkedChild = true
		}
	}

	if hasMarkedChild && !canContainInlineReviewBlocks(node) {
		return true
	}

	if !isCandidateNode(node) || hasReviewBlock(node) {
		return hasMarkedChild || hasReviewBlock(node)
	}

	text := normalizedText(reviewableText(node))
	if !isMeaningfulReviewText(node, text) {
		return false
	}

	kind := reviewBlockKind(node, text)
	counters[kind]++

	node.Attr = append(node.Attr, html.Attribute{
		Key: "data-review-block",
		Val: fmt.Sprintf("%s-%03d", kind, counters[kind]),
	})

	return true
}

func canContainInlineReviewBlocks(node *html.Node) bool {
	return node.Type == html.ElementNode && strings.EqualFold(node.Data, "td")
}

func isCandidateNode(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}

	switch strings.ToLower(node.Data) {
	case "h1", "h2", "h3", "p", "li", "a", "td", "img":
		return true
	default:
		return false
	}
}

func reviewBlockKind(node *html.Node, text string) string {
	tag := strings.ToLower(node.Data)

	switch tag {
	case "img":
		return "banner"
	case "h1", "h2", "h3":
		return "headline"
	case "a":
		return "cta"
	case "li":
		return "list-item"
	default:
		return "body"
	}

}

func isMeaningfulReviewText(node *html.Node, text string) bool {
	if strings.EqualFold(node.Data, "img") {
		return isMeaningfulImageAlt(text)
	}

	if strings.EqualFold(node.Data, "a") {
		return isMeaningfulLinkText(text)
	}

	return isMeaningfulText(text)
}

func isMeaningfulImageAlt(text string) bool {
	if len([]rune(text)) < 8 {
		return false
	}

	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "logo") ||
		strings.Contains(lowerText, "telegram") ||
		strings.Contains(lowerText, "linkedin") ||
		strings.Contains(lowerText, "youtube") ||
		strings.Contains(lowerText, "facebook") ||
		strings.Contains(lowerText, "spacer") ||
		strings.Contains(lowerText, "tracking") ||
		strings.Contains(lowerText, "pixel") {
		return false
	}

	return true
}

func isMeaningfulLinkText(text string) bool {
	if len([]rune(text)) < 4 {
		return false
	}

	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "unsubscribe") ||
		strings.Contains(lowerText, "privacy") ||
		strings.Contains(lowerText, "terms") ||
		strings.Contains(lowerText, "telegram") ||
		strings.Contains(lowerText, "linkedin") ||
		strings.Contains(lowerText, "youtube") ||
		strings.Contains(lowerText, "facebook") {
		return false
	}

	return true
}

func isMeaningfulText(text string) bool {
	if len([]rune(text)) < 20 {
		return false
	}

	return true
}

func hasReviewBlock(node *html.Node) bool {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, "data-review-block") {
			return true
		}
	}

	return false
}

func reviewableText(node *html.Node) string {
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, "img") {
		return attrValue(node, "alt")
	}

	return textContent(node)
}

func attrValue(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}

	return ""
}

func textContent(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}

	var parts []string
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		value := textContent(child)
		if value != "" {
			parts = append(parts, value)
		}
	}

	return strings.Join(parts, " ")
}

func normalizedText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
