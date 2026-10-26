package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// sharedKeys are the fields that become shared texts: logo, copyright, legal
// text, privacy link, login block and the signature. An email can still keep
// its own text in any of them.
var sharedKeys = []string{
	"footer_logo_src",
	"footer_logo_alt",
	"footer_copyright",
	"footer_legal_text",
	"privacy_policy_link_text",
	"privacy_policy_link_url",
	"signature",
	"login_here_text",
	"login_label",
	"password_label",
	"password_security_note_text",
	"password_security_note_url",
	"partner_login_link_text",
	"partner_login_link_url",
}

// minEmails is how many emails of a language must have a field before it
// gets a shared text.
const minEmails = 2

// Email is what the plan needs from one active email.
type Email struct {
	ID       string
	Place    string
	Language string
	Fields   emailedit.EditableFields
	Live     bool
}

// Existing is a shared text already in the database.
type Existing struct {
	Type  string
	Value string
}

// Item is one shared text: a key in one language.
type Item struct {
	Key      string
	Language string
	Type     string
	Value    string
	// Exists: the shared text is already in the database; its value is used.
	Exists bool
	// Link are emails whose field equals the value and is not shared yet;
	// Linked already point at it.
	Link   []Email
	Linked int
	// Own are emails that keep a different text; they stay as they are.
	Own []Email
}

// Skipped is a key and language left without a shared text.
type Skipped struct {
	Key, Language, Reason string
}

type Plan struct {
	Items   []Item
	Skipped []Skipped
}

// BuildPlan picks a shared text for every key and language: the existing one,
// or the value most emails of that language already have when it is the
// majority. Only emails with exactly that value get linked, so no email
// changes its HTML.
func BuildPlan(emails []Email, existing map[string]Existing) Plan {
	plan := Plan{}
	type place struct{ key, language string }
	byPlace := map[place][]Email{}
	for _, email := range emails {
		for _, key := range sharedKeys {
			if _, ok := email.Fields[key]; ok {
				p := place{key, email.Language}
				byPlace[p] = append(byPlace[p], email)
			}
		}
	}
	places := make([]place, 0, len(byPlace))
	for p := range byPlace {
		places = append(places, p)
	}
	sort.Slice(places, func(i, j int) bool {
		if places[i].key != places[j].key {
			return keyOrder(places[i].key) < keyOrder(places[j].key)
		}
		return languageLess(places[i].language, places[j].language)
	})

	for _, p := range places {
		group := byPlace[p]
		fieldType := group[0].Fields[p.key].Type
		counts := map[string]int{}
		mixedTypes := false
		for _, email := range group {
			field := email.Fields[p.key]
			if field.Type != fieldType {
				mixedTypes = true
			}
			counts[fmt.Sprint(field.Value)]++
		}

		item := Item{Key: p.key, Language: p.language, Type: fieldType}
		if current, ok := existing[p.key+"\x1f"+p.language]; ok {
			item.Exists, item.Type, item.Value = true, current.Type, current.Value
		} else {
			switch {
			case mixedTypes:
				plan.Skipped = append(plan.Skipped, Skipped{p.key, p.language, "the field has different types"})
				continue
			case fieldType == emailedit.FieldTypeNumber:
				plan.Skipped = append(plan.Skipped, Skipped{p.key, p.language, "numbers are not shared"})
				continue
			case len(group) < minEmails:
				plan.Skipped = append(plan.Skipped, Skipped{p.key, p.language, fmt.Sprintf("only %d email", len(group))})
				continue
			}
			value, count := majority(counts)
			if count*2 <= len(group) {
				plan.Skipped = append(plan.Skipped, Skipped{p.key, p.language,
					fmt.Sprintf("no majority: %d values in %d emails", len(counts), len(group))})
				continue
			}
			item.Value = value
		}

		for _, email := range group {
			field := email.Fields[p.key]
			switch {
			case field.Type != item.Type || fmt.Sprint(field.Value) != item.Value:
				item.Own = append(item.Own, email)
			case field.Shared:
				item.Linked++
			default:
				item.Link = append(item.Link, email)
			}
		}
		plan.Items = append(plan.Items, item)
	}
	return plan
}

// majority returns the most common value and its count; ties go to the
// alphabetically first value so the plan is stable.
func majority(counts map[string]int) (string, int) {
	best, bestCount := "", -1
	for value, count := range counts {
		if count > bestCount || (count == bestCount && value < best) {
			best, bestCount = value, count
		}
	}
	return best, bestCount
}

func keyOrder(key string) int {
	for i, k := range sharedKeys {
		if k == key {
			return i
		}
	}
	return len(sharedKeys)
}

func languageLess(a, b string) bool {
	if a == "en" || b == "en" {
		return a == "en" && b != "en"
	}
	return a < b
}

func WritePlan(w io.Writer, plan Plan, applied bool) {
	link, live, own := 0, 0, 0
	for _, item := range plan.Items {
		link += len(item.Link)
		own += len(item.Own)
		for _, email := range item.Link {
			if email.Live {
				live++
			}
		}
	}
	fmt.Fprintln(w, "# Shared texts")
	fmt.Fprintln(w)
	if applied {
		fmt.Fprintln(w, "Applied: shared texts created, fields linked. No email HTML changed.")
	} else {
		fmt.Fprintln(w, "Dry run, nothing written. Linking changes no email HTML: only fields that already have the shared value get linked.")
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- Shared texts: %d (%d new)\n", len(plan.Items), countNew(plan.Items))
	fmt.Fprintf(w, "- Fields to link: %d (in live emails: %d)\n", link, live)
	fmt.Fprintf(w, "- Fields that keep their own text: %d\n", own)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "| Key | Lang | Value | Link | Already linked | Own text |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, item := range plan.Items {
		value := shorten(item.Value, 70)
		if !item.Exists {
			value += " (new)"
		}
		fmt.Fprintf(w, "| `%s` | %s | %s | %d | %d | %d |\n", item.Key, strings.ToUpper(item.Language),
			escapeCell(value), len(item.Link), item.Linked, len(item.Own))
	}

	if len(plan.Skipped) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Not shared")
		fmt.Fprintln(w)
		for _, skipped := range plan.Skipped {
			fmt.Fprintf(w, "- `%s` %s: %s\n", skipped.Key, strings.ToUpper(skipped.Language), skipped.Reason)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Emails that keep their own text")
	fmt.Fprintln(w)
	listed := false
	for _, item := range plan.Items {
		for _, email := range item.Own {
			listed = true
			fmt.Fprintf(w, "- `%s` %s · %s: «%s»\n", item.Key, strings.ToUpper(item.Language), email.Place,
				shorten(fmt.Sprint(email.Fields[item.Key].Value), 80))
		}
	}
	if !listed {
		fmt.Fprintln(w, "None.")
	}
}

func countNew(items []Item) int {
	count := 0
	for _, item := range items {
		if !item.Exists {
			count++
		}
	}
	return count
}

func shorten(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func escapeCell(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}
