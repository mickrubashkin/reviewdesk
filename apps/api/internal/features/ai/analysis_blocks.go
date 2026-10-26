package ai

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// The subject and preheader are review blocks of the preview header, not of
// the HTML; a recommendation about them points there.
var headerBlocks = []string{"subject", "preheader"}

// footerBlockPattern matches the blocks of the footer, which the review
// leaves out like the footer links.
var footerBlockPattern = regexp.MustCompile(`^(footer|privacy_policy)`)

const (
	maxAnalysisBlocks    = 30
	maxAnalysisBlockText = 300
)

// analysisBlock is a block of the email the review can point a
// recommendation at: its data-review-block and its text.
type analysisBlock struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// analysisBlocks lists the review blocks of the email body in order, with
// their text; blocks without text (images) and the footer are left out.
func analysisBlocks(email EmailDetail) []analysisBlock {
	source := email.OriginalHTML
	if email.ReviewHTML != nil && strings.TrimSpace(*email.ReviewHTML) != "" {
		source = *email.ReviewHTML
	}
	root, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return nil
	}
	blocks := []analysisBlock{}
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if len(blocks) >= maxAnalysisBlocks {
			return
		}
		if node.Type == html.ElementNode {
			if id := reviewBlockOf(node); id != "" && !seen[id] && !footerBlockPattern.MatchString(id) {
				seen[id] = true
				if text := limitText(strings.Join(strings.Fields(textOf(node)), " "), maxAnalysisBlockText); text != "" {
					blocks = append(blocks, analysisBlock{ID: id, Text: text})
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return blocks
}

// analysisBlockIDs are the values a recommendation's block may take: a
// block of the email, the subject or preheader, or "" for the whole email.
func analysisBlockIDs(blocks []analysisBlock) []string {
	ids := append([]string{""}, headerBlocks...)
	for _, block := range blocks {
		ids = append(ids, block.ID)
	}
	return ids
}

func reviewBlockOf(node *html.Node) string {
	for _, attr := range node.Attr {
		if attr.Key == "data-review-block" {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func textOf(node *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		switch {
		case node.Type == html.TextNode:
			text.WriteString(" " + node.Data)
		case node.Type == html.ElementNode && (node.Data == "style" || node.Data == "script"):
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return text.String()
}
