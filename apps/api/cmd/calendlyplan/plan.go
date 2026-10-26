package main

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

type Email struct {
	ID           string
	Board        string
	Stage        string
	SortOrder    int
	Title        string
	Language     string
	Variant      string
	TemplateHTML string
	Fields       map[string]struct {
		Value any `json:"value"`
	}
}

// Use is one Calendly link in one email: in the template (an href or plain
// text) or in an editable field value.
type Use struct {
	Email   Email
	Link    string
	Source  string // "template" or "field <key>"
	Text    string // link text, when the link is an <a href>
	Context string // the sentence around it
}

var calendlyPattern = regexp.MustCompile(`https?://(?:www\.)?calendly\.com/[^\s"'<>)]*`)

// keepLink reports links that are not a manager's personal page, e.g. shared
// team events under calendly.com/d/, and so stay as they are.
func keepLink(link string) bool {
	return strings.Contains(link, "calendly.com/d/")
}

func FindUses(emails []Email) ([]Use, error) {
	var uses []Use
	for _, email := range emails {
		doc, err := html.Parse(strings.NewReader(email.TemplateHTML))
		if err != nil {
			return nil, fmt.Errorf("email %s: %w", email.ID, err)
		}
		uses = append(uses, templateUses(email, doc)...)

		keys := make([]string, 0, len(email.Fields))
		for key := range email.Fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value, _ := email.Fields[key].Value.(string)
			for _, link := range calendlyPattern.FindAllString(value, -1) {
				uses = append(uses, Use{Email: email, Link: link, Source: "field " + key, Context: shorten(value)})
			}
		}
	}
	return uses, nil
}

func templateUses(email Email, node *html.Node) []Use {
	var uses []Use
	if node.Type == html.ElementNode && node.Data == "a" {
		for _, attr := range node.Attr {
			if attr.Key == "href" && calendlyPattern.MatchString(attr.Val) {
				uses = append(uses, Use{
					Email:   email,
					Link:    calendlyPattern.FindString(attr.Val),
					Source:  "template",
					Text:    shorten(textOf(node)),
					Context: shorten(textOf(blockOf(node))),
				})
			}
		}
	}
	if node.Type == html.TextNode && (node.Parent == nil || node.Parent.Data != "a") {
		for _, link := range calendlyPattern.FindAllString(node.Data, -1) {
			uses = append(uses, Use{Email: email, Link: link, Source: "template text", Context: shorten(textOf(blockOf(node)))})
		}
	}
	if node.Type == html.TextNode && node.Parent != nil && node.Parent.Data == "a" {
		for _, link := range calendlyPattern.FindAllString(node.Data, -1) {
			uses = append(uses, Use{Email: email, Link: link, Source: "link text", Context: shorten(textOf(blockOf(node)))})
		}
	}
	// Outlook buttons (VML) live in conditional comments the parser keeps as
	// plain text.
	if node.Type == html.CommentNode {
		for _, link := range calendlyPattern.FindAllString(node.Data, -1) {
			uses = append(uses, Use{Email: email, Link: link, Source: "Outlook button", Context: shorten(textOf(blockOf(node)))})
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		uses = append(uses, templateUses(email, child)...)
	}
	return uses
}

// blockOf returns the nearest paragraph-like ancestor that says more than
// the link itself, for context: a button's own cell often holds only its label.
func blockOf(node *html.Node) *html.Node {
	own := len(strings.Fields(textOf(node)))
	block := node
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		switch parent.Data {
		case "p", "td", "li", "div", "table":
			block = parent
			if len(strings.Fields(textOf(parent))) > own+3 {
				return parent
			}
		case "body":
			return block
		}
	}
	return block
}

func textOf(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			builder.WriteString(n.Data)
			builder.WriteString(" ")
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

func shorten(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if runes := []rune(value); len(runes) > 160 {
		return string(runes[:157]) + "…"
	}
	return value
}

func WriteReport(w io.Writer, uses []Use, expression string) {
	byLink := map[string][]Use{}
	emailsByLink := map[string]map[string]bool{}
	allEmails := map[string]bool{}
	for _, use := range uses {
		byLink[use.Link] = append(byLink[use.Link], use)
		if emailsByLink[use.Link] == nil {
			emailsByLink[use.Link] = map[string]bool{}
		}
		emailsByLink[use.Link][use.Email.ID] = true
		allEmails[use.Email.ID] = true
	}
	links := make([]string, 0, len(byLink))
	for link := range byLink {
		links = append(links, link)
	}
	sort.Slice(links, func(i, j int) bool {
		if len(emailsByLink[links[i]]) != len(emailsByLink[links[j]]) {
			return len(emailsByLink[links[i]]) > len(emailsByLink[links[j]])
		}
		return links[i] < links[j]
	})

	fmt.Fprintf(w, "# Calendly links → %s\n\n", expression)
	fmt.Fprintf(w, "%d uses of %d links in %d emails. Dry run: nothing was changed.\n\n", len(uses), len(links), len(allEmails))
	fmt.Fprintln(w, "| Link | Emails | Uses | Plan |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for _, link := range links {
		fmt.Fprintf(w, "| %s | %d | %d | %s |\n", link, len(emailsByLink[link]), len(byLink[link]), plan(link))
	}
	for _, link := range links {
		fmt.Fprintf(w, "\n## %s — %s\n\n", link, plan(link))
		for _, use := range byLink[link] {
			e := use.Email
			fmt.Fprintf(w, "- **%s / %s #%d %s %s** «%s» (%s)", e.Board, e.Stage, e.SortOrder, strings.ToUpper(e.Language), e.Variant, e.Title, use.Source)
			if use.Text != "" {
				fmt.Fprintf(w, ", link text «%s»", use.Text)
			}
			fmt.Fprintf(w, "\n  > %s\n", use.Context)
		}
	}
}

func plan(link string) string {
	if keepLink(link) {
		return "keep (shared event)"
	}
	return "replace"
}
