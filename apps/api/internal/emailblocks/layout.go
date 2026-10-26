package emailblocks

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// LayoutItem is one block of an email. A block used more than once gets a
// suffix on every repeat, and its field keys and review block carry it
// (intro_body_2), so each copy keeps its own values and comments.
type LayoutItem struct {
	Block  string `json:"block"`
	Suffix string `json:"suffix,omitempty"`
}

// Layout is the blocks an email is built from, in order, without the frame.
type Layout []LayoutItem

// DefaultLayout is the most common structure of the emails (audit, 17 Oct
// 2026: 62% of them): a new email starts with it when its board has no
// emails built from blocks yet.
func DefaultLayout() Layout {
	return Layout{{Block: "hero_banner"}, {Block: "intro_body"}, {Block: ButtonKey}, {Block: "signature"}, {Block: "footer"}}
}

// ID names the copy of the block inside an email: the block key, plus the
// suffix for a repeat.
func (item LayoutItem) ID() string {
	if item.Suffix == "" {
		return item.Block
	}
	return item.Block + "_" + item.Suffix
}

var suffixPattern = regexp.MustCompile(`^[a-z0-9]{1,8}$`)

// Validate checks the layout against the library: known blocks, no frame,
// every copy named once.
func (layout Layout) Validate(library []Block) error {
	if len(layout) == 0 {
		return fmt.Errorf("an email needs at least one block")
	}
	known := map[string]bool{}
	for _, block := range library {
		known[block.Key] = true
	}
	seen := map[string]bool{}
	for _, item := range layout {
		switch {
		case item.Block == FrameKey:
			return fmt.Errorf("the frame is not a block of an email, it is always there")
		case !known[item.Block]:
			return fmt.Errorf("there is no block %q in the library", item.Block)
		case item.Suffix != "" && !suffixPattern.MatchString(item.Suffix):
			return fmt.Errorf("block %s: the suffix must be up to 8 lowercase letters or digits", item.Block)
		case seen[item.ID()]:
			return fmt.Errorf("block %s is in the email twice: give the repeat a suffix", item.ID())
		}
		seen[item.ID()] = true
	}
	return nil
}

// Keys lists the copies in order, for history and diffs.
func (layout Layout) Keys() []string {
	keys := make([]string, len(layout))
	for i, item := range layout {
		keys[i] = item.ID()
	}
	return keys
}

// Build assembles the template of an email from the library.
func (layout Layout) Build(library []Block) (string, error) {
	if err := layout.Validate(library); err != nil {
		return "", err
	}
	byKey := map[string]Block{}
	for _, block := range library {
		byKey[block.Key] = block
	}
	frame, ok := byKey[FrameKey]
	if !ok {
		return "", fmt.Errorf("the library has no frame")
	}
	parts := make([]string, len(layout))
	for i, item := range layout {
		parts[i] = InstanceHTML(byKey[item.Block], item.Suffix)
	}
	return Assemble(frame.HTML, parts...)
}

// InstanceFields are the block's fields as they are named in an email with
// this suffix.
func InstanceFields(block Block, suffix string) []Field {
	fields := make([]Field, len(block.Fields))
	for i, field := range block.Fields {
		field.Key = suffixed(field.Key, suffix)
		fields[i] = field
	}
	return fields
}

var (
	// data-edit-text="key", data-review-block='key' and the like.
	keyAttrPattern = regexp.MustCompile(`(data-(?:edit-[a-z-]+|review-block)\s*=\s*)("([^"]*)"|'([^']*)')`)
	// {{ key }} markers inside Outlook conditional comments.
	markerPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)
)

// InstanceHTML is the block's HTML with its field keys and review blocks
// renamed for the suffix. Bitrix24 expressions such as {{Responsible}} are
// not field keys and stay as they are.
func InstanceHTML(block Block, suffix string) string {
	if suffix == "" {
		return block.HTML
	}
	keys := map[string]bool{}
	for _, field := range block.Fields {
		keys[field.Key] = true
	}
	html := keyAttrPattern.ReplaceAllStringFunc(block.HTML, func(match string) string {
		parts := keyAttrPattern.FindStringSubmatch(match)
		value, quote := parts[3], `"`
		if strings.HasPrefix(parts[2], "'") {
			value, quote = parts[4], `'`
		}
		if !keys[value] && !strings.HasPrefix(parts[1], "data-review-block") {
			return match
		}
		return parts[1] + quote + suffixed(value, suffix) + quote
	})
	return markerPattern.ReplaceAllStringFunc(html, func(match string) string {
		key := markerPattern.FindStringSubmatch(match)[1]
		if !keys[key] {
			return match
		}
		return strings.Replace(match, key, suffixed(key, suffix), 1)
	})
}

func suffixed(key, suffix string) string {
	if suffix == "" {
		return key
	}
	return key + "_" + suffix
}

// BlockChange is a block added to or removed from a layout.
type BlockChange struct {
	Action string `json:"action"` // "added" or "removed"
	Block  string `json:"block"`
	ID     string `json:"id"`
	// Name is the block's name in the library, for people.
	Name string `json:"name,omitempty"`
}

// Diff lists the copies added and removed from before to after; a move is
// neither.
func Diff(before, after Layout) []BlockChange {
	changes := []BlockChange{}
	inBefore, inAfter := map[string]bool{}, map[string]bool{}
	for _, item := range before {
		inBefore[item.ID()] = true
	}
	for _, item := range after {
		inAfter[item.ID()] = true
	}
	for _, item := range before {
		if !inAfter[item.ID()] {
			changes = append(changes, BlockChange{Action: "removed", Block: item.Block, ID: item.ID()})
		}
	}
	for _, item := range after {
		if !inBefore[item.ID()] {
			changes = append(changes, BlockChange{Action: "added", Block: item.Block, ID: item.ID()})
		}
	}
	return changes
}

// Uses reports whether the layout has the block; every layout uses the
// frame.
func (layout Layout) Uses(key string) bool {
	if key == FrameKey {
		return true
	}
	for _, item := range layout {
		if item.Block == key {
			return true
		}
	}
	return false
}

// Fields gives an email built from the layout its fields, template being
// what Build returned: a key it already had keeps its value and shared mark,
// a new one takes the value written in the block and the block's shared
// flag and format.
func (layout Layout) Fields(library []Block, template string, previous emailedit.EditableFields) (emailedit.EditableFields, error) {
	extracted, err := emailedit.ExtractEditableFields(template)
	if err != nil {
		return nil, err
	}
	byKey := map[string]Block{}
	for _, block := range library {
		byKey[block.Key] = block
	}
	described := map[string]Field{}
	for _, item := range layout {
		for _, field := range InstanceFields(byKey[item.Block], item.Suffix) {
			described[field.Key] = field
		}
	}
	fields := emailedit.EditableFields{}
	for key, field := range extracted {
		if kept, ok := previous[key]; ok && kept.Type == field.Type {
			kept.Order = field.Order
			fields[key] = kept
			continue
		}
		field.Shared = described[key].Shared
		if field.Type == emailedit.FieldTypeText {
			field.Format = described[key].Format
		}
		fields[key] = field
	}
	return fields, nil
}
