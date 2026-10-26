package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// Email is what the audit needs from one email.
type Email struct {
	ID           string
	Board        string
	Stage        string
	SortOrder    int
	Title        string
	Language     string
	Variant      string
	Adaptation   string
	Subject      string
	Preheader    string
	TemplateHTML string
	Fields       emailedit.EditableFields
	PortalSends  int
	LiveMarked   bool
}

func (e Email) Live() bool { return e.PortalSends > 0 || e.LiveMarked }

func (e Email) Place() string {
	place := fmt.Sprintf("%s · %s #%d · %s", e.Board, e.Stage, e.SortOrder, strings.ToUpper(e.Language))
	if e.Variant != "" && e.Variant != "new" {
		place += " · " + e.Variant
	}
	if e.Adaptation != "" && e.Adaptation != "default" {
		place += " · " + e.Adaptation
	}
	return place
}

// minSharedEmails is how many emails must have a field key before it is
// considered for a shared text.
const minSharedEmails = 3

type Audit struct {
	Total, WithFields, WithoutFields        int
	Live, LiveWithFields, LiveWithoutFields int
	Layouts                                 []Layout
	WithoutFieldsEmails                     []Email
	WithoutFieldsBlocks                     map[string]int // review block kind → emails that have it
	Shared                                  []SharedKey
	CTA                                     CTAStats
	Lengths                                 []LengthStats
	Languages                               []string
}

// Layout is a group of emails with editable fields and the same blocks in the
// same order.
type Layout struct {
	Blocks    []string
	Emails    []Email
	Live      int
	Skeletons int // distinct markup skeletons (tags and attributes, no text)
	Languages []string
}

// SharedKey shows how many distinct values a field key has per language.
type SharedKey struct {
	Key        string
	Type       string
	Emails     int
	ByLanguage map[string]ValueSpread
	// AcrossLanguages is set for URLs and images, which usually do not
	// depend on the language.
	AcrossLanguages ValueSpread
}

type ValueSpread struct {
	Emails   int
	Distinct int
	Top      string
	TopCount int
}

type CTAStats struct {
	// Buckets count emails by CTA count: "0", "1", "2", "3+".
	WithFields    map[string]int
	WithoutFields map[string]int
	TwoPlus       []Email // emails with fields and more than one CTA
}

type LengthStats struct {
	Name      string
	Language  string
	Count     int
	P50, P90  int
	P95, Max  int
	Longest   string
	LongestAt string
}

var numberedBlock = regexp.MustCompile(`-\d+$`)

func BuildAudit(emails []Email) Audit {
	audit := Audit{
		Total:               len(emails),
		WithoutFieldsBlocks: map[string]int{},
		CTA:                 CTAStats{WithFields: map[string]int{}, WithoutFields: map[string]int{}},
	}
	languages := map[string]bool{}
	layouts := map[string]*Layout{}
	layoutSkeletons := map[string]map[string]bool{}
	layoutLanguages := map[string]map[string]bool{}

	for _, e := range emails {
		languages[e.Language] = true
		if e.Live() {
			audit.Live++
		}
		blocks, skeleton := templateStructure(e.TemplateHTML)

		if len(e.Fields) == 0 {
			audit.WithoutFields++
			if e.Live() {
				audit.LiveWithoutFields++
			}
			audit.WithoutFieldsEmails = append(audit.WithoutFieldsEmails, e)
			kinds := map[string]bool{}
			ctas := 0
			for _, block := range blocks {
				kind := numberedBlock.ReplaceAllString(block, "")
				kinds[kind] = true
				if kind == "cta" {
					ctas++
				}
			}
			for kind := range kinds {
				audit.WithoutFieldsBlocks[kind]++
			}
			audit.CTA.WithoutFields[ctaBucket(ctas)]++
			continue
		}

		audit.WithFields++
		if e.Live() {
			audit.LiveWithFields++
		}
		key := strings.Join(blocks, " → ")
		layout, ok := layouts[key]
		if !ok {
			layout = &Layout{Blocks: blocks}
			layouts[key] = layout
			layoutSkeletons[key] = map[string]bool{}
			layoutLanguages[key] = map[string]bool{}
		}
		layout.Emails = append(layout.Emails, e)
		if e.Live() {
			layout.Live++
		}
		layoutSkeletons[key][skeleton] = true
		layoutLanguages[key][e.Language] = true

		ctas := countCTAFields(e.Fields)
		audit.CTA.WithFields[ctaBucket(ctas)]++
		if ctas > 1 {
			audit.CTA.TwoPlus = append(audit.CTA.TwoPlus, e)
		}
	}

	for key, layout := range layouts {
		layout.Skeletons = len(layoutSkeletons[key])
		layout.Languages = sortedKeys(layoutLanguages[key])
		audit.Layouts = append(audit.Layouts, *layout)
	}
	sort.SliceStable(audit.Layouts, func(i, j int) bool {
		if len(audit.Layouts[i].Emails) != len(audit.Layouts[j].Emails) {
			return len(audit.Layouts[i].Emails) > len(audit.Layouts[j].Emails)
		}
		return strings.Join(audit.Layouts[i].Blocks, ",") < strings.Join(audit.Layouts[j].Blocks, ",")
	})

	audit.Languages = orderLanguages(sortedKeys(languages))
	audit.Shared = buildShared(emails)
	audit.Lengths = buildLengths(emails, audit.Languages)
	return audit
}

// templateStructure returns the review blocks of a template in document order
// and a hash of its markup without text and without field values.
func templateStructure(templateHTML string) ([]string, string) {
	blocks := []string{}
	if strings.TrimSpace(templateHTML) == "" {
		return blocks, ""
	}
	doc, err := html.Parse(strings.NewReader(templateHTML))
	if err != nil {
		return blocks, ""
	}
	hash := sha1.New()
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			hash.Write([]byte("<" + node.Data))
			attrs := make([]string, 0, len(node.Attr))
			for _, attr := range node.Attr {
				switch {
				case attr.Key == "data-review-block":
					blocks = append(blocks, attr.Val)
					attrs = append(attrs, attr.Key+"="+attr.Val)
				case strings.HasPrefix(attr.Key, "data-edit-"):
					attrs = append(attrs, attr.Key+"="+attr.Val)
				case attr.Key == "href" || attr.Key == "src" || attr.Key == "alt" || attr.Key == "title":
					// Values are content, not structure.
					attrs = append(attrs, attr.Key)
				default:
					attrs = append(attrs, attr.Key+"="+strings.Join(strings.Fields(attr.Val), " "))
				}
			}
			sort.Strings(attrs)
			hash.Write([]byte(" " + strings.Join(attrs, " ") + ">"))
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.Type == html.ElementNode {
			hash.Write([]byte("</" + node.Data + ">"))
		}
	}
	walk(doc)
	return blocks, hex.EncodeToString(hash.Sum(nil))[:12]
}

func countCTAFields(fields emailedit.EditableFields) int {
	count := 0
	for key, field := range fields {
		if field.Type == emailedit.FieldTypeURL && strings.Contains(key, "cta") {
			count++
		}
	}
	return count
}

func ctaBucket(count int) string {
	if count >= 3 {
		return "3+"
	}
	return fmt.Sprint(count)
}

func buildShared(emails []Email) []SharedKey {
	type values struct {
		fieldType  string
		emails     int
		byLanguage map[string]map[string]int
		all        map[string]int
	}
	keys := map[string]*values{}
	for _, e := range emails {
		for key, field := range e.Fields {
			if field.Type == emailedit.FieldTypeNumber {
				continue
			}
			v, ok := keys[key]
			if !ok {
				v = &values{fieldType: field.Type, byLanguage: map[string]map[string]int{}, all: map[string]int{}}
				keys[key] = v
			}
			value := strings.Join(strings.Fields(fmt.Sprint(field.Value)), " ")
			v.emails++
			if v.byLanguage[e.Language] == nil {
				v.byLanguage[e.Language] = map[string]int{}
			}
			v.byLanguage[e.Language][value]++
			v.all[value]++
		}
	}

	shared := []SharedKey{}
	for key, v := range keys {
		if v.emails < minSharedEmails {
			continue
		}
		item := SharedKey{Key: key, Type: v.fieldType, Emails: v.emails, ByLanguage: map[string]ValueSpread{}}
		for language, counts := range v.byLanguage {
			item.ByLanguage[language] = spread(counts)
		}
		if v.fieldType != emailedit.FieldTypeText {
			item.AcrossLanguages = spread(v.all)
		}
		shared = append(shared, item)
	}
	sort.SliceStable(shared, func(i, j int) bool {
		if shared[i].Emails != shared[j].Emails {
			return shared[i].Emails > shared[j].Emails
		}
		return shared[i].Key < shared[j].Key
	})
	return shared
}

func spread(counts map[string]int) ValueSpread {
	result := ValueSpread{Distinct: len(counts)}
	for value, count := range counts {
		result.Emails += count
		if count > result.TopCount || (count == result.TopCount && value < result.Top) {
			result.Top, result.TopCount = value, count
		}
	}
	return result
}

func buildLengths(emails []Email, languages []string) []LengthStats {
	type sample struct {
		length int
		text   string
		place  string
	}
	measures := []struct {
		name  string
		value func(Email) (string, bool)
	}{
		{"Subject", func(e Email) (string, bool) { return e.Subject, strings.TrimSpace(e.Subject) != "" }},
		{"Preheader", func(e Email) (string, bool) { return e.Preheader, strings.TrimSpace(e.Preheader) != "" }},
		{"CTA text", func(e Email) (string, bool) {
			field, ok := e.Fields["primary_cta_text"]
			text := fmt.Sprint(field.Value)
			return text, ok && strings.TrimSpace(text) != ""
		}},
	}

	stats := []LengthStats{}
	for _, measure := range measures {
		byLanguage := map[string][]sample{}
		for _, e := range emails {
			text, ok := measure.value(e)
			if !ok {
				continue
			}
			text = strings.Join(strings.Fields(text), " ")
			s := sample{utf8.RuneCountInString(text), text, e.Place()}
			byLanguage[e.Language] = append(byLanguage[e.Language], s)
			byLanguage["all"] = append(byLanguage["all"], s)
		}
		for _, language := range append(append([]string{}, languages...), "all") {
			samples := byLanguage[language]
			if len(samples) == 0 {
				continue
			}
			sort.SliceStable(samples, func(i, j int) bool { return samples[i].length < samples[j].length })
			longest := samples[len(samples)-1]
			stats = append(stats, LengthStats{
				Name: measure.name, Language: language, Count: len(samples),
				P50: percentile(samples, 50, func(s sample) int { return s.length }),
				P90: percentile(samples, 90, func(s sample) int { return s.length }),
				P95: percentile(samples, 95, func(s sample) int { return s.length }),
				Max: longest.length, Longest: longest.text, LongestAt: longest.place,
			})
		}
	}
	return stats
}

// percentile uses the nearest-rank method on sorted values.
func percentile[T any](sorted []T, p int, value func(T) int) int {
	rank := (p*len(sorted) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	return value(sorted[rank-1])
}

var languageOrder = map[string]int{"en": 0, "de": 1, "es": 2, "pl": 3, "br": 4}

func orderLanguages(languages []string) []string {
	sort.SliceStable(languages, func(i, j int) bool {
		a, aok := languageOrder[languages[i]]
		b, bok := languageOrder[languages[j]]
		switch {
		case aok && bok:
			return a < b
		case aok != bok:
			return aok
		default:
			return languages[i] < languages[j]
		}
	})
	return languages
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
