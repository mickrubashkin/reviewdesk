// Package emailblocks is the shared library emails are built from: a frame
// (the document around the content) and blocks, table rows with data-edit-*
// fields, that go into it in order. The starting set is cut from the most
// common template (onboarding · first-sale #701 · EN), so an email assembled
// from it renders like that template.
package emailblocks

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/html"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// FrameKey is the block the others go into.
const FrameKey = "frame"

// BlocksMarker marks where in the frame the blocks go.
const BlocksMarker = "<!-- blocks -->"

// Block is one piece of the library.
type Block struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	HTML        string  `json:"html"`
	Fields      []Field `json:"fields"`
}

// Field describes a data-edit-* field of a block. Key and Type come from the
// HTML; the rest is what the editor and the checks on save use.
type Field struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	Label     string `json:"label"`
	Required  bool   `json:"required,omitempty"`
	MaxLength int    `json:"max_length,omitempty"`
	// Shared: an email takes the shared text with the same key unless it
	// keeps its own.
	Shared bool `json:"shared,omitempty"`
	// Format is the format a new email gets the field in: "markdown" for a
	// field whose text written in the block is Markdown (a list).
	Format string `json:"format,omitempty"`
}

//go:embed defaults/*.html
var defaultHTML embed.FS

// Defaults is the starting library in display order, the frame first.
func Defaults() []Block {
	blocks := []Block{
		{Key: FrameKey, Name: "Frame",
			Description: "The document around the blocks: head, preheader, background, 600 px column. Blocks go where the frame has " + BlocksMarker + "."},
		{Key: "hero_banner", Name: "Hero banner",
			Description: "Full-width image at the top.",
			Fields: []Field{
				{Key: "hero_banner_src", Label: "Banner image", Required: true},
				{Key: "hero_banner_alt", Label: "Banner alt text"},
			}},
		{Key: "intro_body", Name: "Main text",
			Description: "Greeting and the main message; line breaks are kept.",
			Fields: []Field{
				{Key: "intro_body", Label: "Text", Required: true},
			}},
		{Key: "primary_cta", Name: "Button",
			Description: "The email's one button, with an Outlook version.",
			Fields: []Field{
				{Key: "primary_cta_text", Label: "Button text", Required: true, MaxLength: 30},
				{Key: "primary_cta_url", Label: "Button link", Required: true},
				{Key: "primary_cta_width_px", Label: "Button width, px"},
			}},
		{Key: "signature", Name: "Signature",
			Description: "Greeting and sender; shared by default, an email can keep its own.",
			Fields: []Field{
				{Key: "signature_greeting", Label: "Greeting", Shared: true},
				{Key: "signature_sender", Label: "Sender", Shared: true},
			}},
		{Key: "signature_text", Name: "Signature, one field",
			Description: "Greeting and sender as one text, the way most older emails have it; shared by default, an email can keep its own.",
			Fields: []Field{
				{Key: "signature", Label: "Signature", Shared: true},
			}},
		{Key: "footer", Name: "Footer",
			Description: "Logo, social links, copyright, legal text and privacy policy; all texts are shared.",
			Fields: []Field{
				{Key: "footer_logo_src", Label: "Logo", Shared: true},
				{Key: "footer_logo_alt", Label: "Logo alt text", Shared: true},
				{Key: "footer_copyright", Label: "Copyright", Shared: true},
				{Key: "footer_legal_text", Label: "Legal text", Shared: true},
				{Key: "privacy_policy_link_text", Label: "Privacy policy text", Shared: true},
				{Key: "privacy_policy_link_url", Label: "Privacy policy link", Shared: true},
			}},
		// Added on 21 Oct 2026 for the emails the first set could not build;
		// at the end, so the blocks already in the database keep their order.
		{Key: "body_text", Name: "Text",
			Description: "A paragraph under the text above or the login details, before the button. Markdown: links, bold, lists.",
			Fields: []Field{
				{Key: "body_text", Label: "Text", Required: true},
			}},
		{Key: "closing_text", Name: "Closing text",
			Description: "A paragraph after the button, before the signature. Markdown: links, bold, lists.",
			Fields: []Field{
				{Key: "closing_text", Label: "Text", Required: true},
			}},
		{Key: "list", Name: "List",
			Description: "A heading and a bulleted list; one point per line, starting with \"- \".",
			Fields: []Field{
				{Key: "list_heading", Label: "Heading"},
				{Key: "list_items", Label: "Points", Required: true, Format: emailedit.FormatMarkdown},
			}},
		{Key: "login", Name: "Login details",
			Description: "Login and password from the portal, a link to change the password and the login page; all texts are shared.",
			Fields: []Field{
				{Key: "login_label", Label: "Login label", Shared: true},
				{Key: "password_label", Label: "Password label", Shared: true},
				{Key: "password_security_note_text", Label: "Password note", Shared: true},
				{Key: "password_security_note_url", Label: "Password note link", Shared: true},
				{Key: "login_here_text", Label: "Login page label", Shared: true},
				{Key: "partner_login_link_text", Label: "Login page link text", Shared: true},
				{Key: "partner_login_link_url", Label: "Login page link", Shared: true},
			}},
	}

	frame := mustDefaultHTML(FrameKey)
	for i := range blocks {
		blocks[i].HTML = mustDefaultHTML(blocks[i].Key)
		if blocks[i].Key == FrameKey {
			blocks[i].Fields = []Field{}
			continue
		}
		fields, err := Fields(frame, blocks[i].HTML, blocks[i].Fields)
		if err != nil {
			panic(fmt.Sprintf("default block %s: %v", blocks[i].Key, err))
		}
		blocks[i].Fields = fields
	}
	return blocks
}

func mustDefaultHTML(key string) string {
	content, err := defaultHTML.ReadFile("defaults/" + key + ".html")
	if err != nil {
		panic(err)
	}
	return string(content)
}

// Assemble puts blocks into the frame in order and returns the template.
func Assemble(frameHTML string, blocksHTML ...string) (string, error) {
	if strings.Count(frameHTML, BlocksMarker) != 1 {
		return "", fmt.Errorf("the frame must have %s exactly once", BlocksMarker)
	}
	return strings.Replace(frameHTML, BlocksMarker, strings.Join(blocksHTML, "\n"), 1), nil
}

// Fields reads the fields of a block from its HTML in document order. A
// field that was there before keeps its label, limits and shared flag; a new
// one gets its key as the label.
func Fields(frameHTML, blockHTML string, previous []Field) ([]Field, error) {
	template, err := Assemble(frameHTML, blockHTML)
	if err != nil {
		return nil, err
	}
	extracted, err := emailedit.ExtractEditableFields(template)
	if err != nil {
		return nil, err
	}

	byKey := map[string]Field{}
	for _, field := range previous {
		byKey[field.Key] = field
	}
	fields := make([]Field, 0, len(extracted))
	for key, editable := range extracted {
		field, ok := byKey[key]
		if !ok {
			field = Field{Key: key, Label: key}
		}
		field.Type = editable.Type
		fields = append(fields, field)
	}
	sort.SliceStable(fields, func(i, j int) bool {
		return extracted[fields[i].Key].Order < extracted[fields[j].Key].Order
	})
	return fields, nil
}

// Validate checks HTML for the block with key before it is saved; frameHTML
// is the frame the block goes into.
func Validate(key, blockHTML, frameHTML string) error {
	if strings.TrimSpace(blockHTML) == "" {
		return fmt.Errorf("the HTML is empty")
	}
	if key == FrameKey {
		if _, err := Assemble(blockHTML); err != nil {
			return err
		}
		fields, err := emailedit.ExtractEditableFields(blockHTML)
		if err != nil {
			return err
		}
		if len(fields) > 0 {
			return fmt.Errorf("the frame must not have editable fields, they belong in blocks")
		}
		return nil
	}

	if strings.Contains(blockHTML, BlocksMarker) {
		return fmt.Errorf("only the frame can have %s", BlocksMarker)
	}
	// Blocks are rows of the frame's content table: anything else is moved
	// out of the table by the HTML parser and lands in the wrong place.
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(blockHTML)), "<tr") {
		return fmt.Errorf("a block must be a table row: start it with <tr>")
	}
	template, err := Assemble(frameHTML, blockHTML)
	if err != nil {
		return err
	}
	if _, err := emailedit.ExtractEditableFields(template); err != nil {
		return err
	}
	if !hasReviewBlock(template) {
		return fmt.Errorf("a block needs data-review-block on an element, comments attach to it")
	}
	return nil
}

func hasReviewBlock(template string) bool {
	root, err := html.Parse(strings.NewReader(template))
	if err != nil {
		return false
	}
	var walk func(*html.Node) bool
	walk = func(node *html.Node) bool {
		if node.Type == html.ElementNode {
			for _, attr := range node.Attr {
				if attr.Key == "data-review-block" {
					return true
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if walk(child) {
				return true
			}
		}
		return false
	}
	return walk(root)
}
