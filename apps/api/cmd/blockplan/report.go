package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

const maxLayoutsShown = 40

func WriteReport(w io.Writer, audit Audit) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format, args...) }

	p("# Template audit\n\n")
	p("Read-only analysis of active (not archived) emails. Nothing in the database was changed.\n\n")

	p("## Summary\n\n")
	p("| | Emails | Live |\n|---|---|---|\n")
	p("| All | %d | %d |\n", audit.Total, audit.Live)
	p("| With editable fields | %d | %d |\n", audit.WithFields, audit.LiveWithFields)
	p("| Without editable fields (as is) | %d | %d |\n", audit.WithoutFields, audit.LiveWithoutFields)
	p("| Layouts among emails with fields | %d | |\n\n", len(audit.Layouts))
	p("Live = sends matched on the portal or a manual live mark.\n\n")

	p("## Layouts (emails with fields)\n\n")
	p("Emails grouped by their blocks in order. *Markups* counts distinct HTML skeletons inside the group (same blocks, different markup): 1 means one block template covers the whole group.\n\n")
	p("| # | Emails | Live | Languages | Markups | Blocks | Examples |\n|---|---|---|---|---|---|---|\n")
	for i, layout := range first(audit.Layouts, maxLayoutsShown) {
		examples := []string{}
		for _, e := range first(layout.Emails, 3) {
			examples = append(examples, e.Place())
		}
		p("| %d | %d | %d | %s | %d | %s | %s |\n", i+1, len(layout.Emails), layout.Live,
			strings.ToUpper(strings.Join(layout.Languages, ", ")), layout.Skeletons,
			"`"+strings.Join(layout.Blocks, "` → `")+"`", escape(strings.Join(examples, "; ")))
	}
	if len(audit.Layouts) > maxLayoutsShown {
		p("\n…and %d more layouts.\n", len(audit.Layouts)-maxLayoutsShown)
	}
	p("\n")

	p("## Emails without fields\n\n")
	p("Their blocks come from automatic review markup (by tag), not from a template.\n\n")
	p("| Block kind | Emails |\n|---|---|\n")
	kinds := sortedKeys(audit.WithoutFieldsBlocks)
	sort.SliceStable(kinds, func(i, j int) bool {
		return audit.WithoutFieldsBlocks[kinds[i]] > audit.WithoutFieldsBlocks[kinds[j]]
	})
	for _, kind := range kinds {
		p("| `%s` | %d |\n", kind, audit.WithoutFieldsBlocks[kind])
	}
	p("\n### Live ones\n\n")
	live, notLive := 0, 0
	for _, e := range audit.WithoutFieldsEmails {
		if !e.Live() {
			notLive++
			continue
		}
		live++
		mark := fmt.Sprintf("%d sends", e.PortalSends)
		if e.PortalSends == 0 {
			mark = "marked live"
		}
		p("- %s — \"%s\" (%s)\n", escape(e.Place()), escape(e.Title), mark)
	}
	if live == 0 {
		p("None.\n")
	}
	p("\nNot live: %d.\n\n", notLive)

	p("## Shared text candidates\n\n")
	p("Field keys in at least %d emails. Each cell is *distinct values / emails* for that language; ✓ = one value, so the field can become a shared text without changing any email. URLs and images also show the spread across all languages.\n\n", minSharedEmails)
	header := "| Key | Type | Emails |"
	divider := "|---|---|---|"
	for _, language := range audit.Languages {
		header += " " + strings.ToUpper(language) + " |"
		divider += "---|"
	}
	p("%s All languages |\n%s---|\n", header, divider)
	for _, key := range audit.Shared {
		row := fmt.Sprintf("| `%s` | %s | %d |", key.Key, key.Type, key.Emails)
		for _, language := range audit.Languages {
			row += " " + spreadCell(key.ByLanguage[language]) + " |"
		}
		if key.AcrossLanguages.Emails > 0 {
			row += " " + spreadCell(key.AcrossLanguages) + " |"
		} else {
			row += " |"
		}
		p("%s\n", row)
	}
	p("\n### Most common EN value per key\n\n")
	for _, key := range audit.Shared {
		en, ok := key.ByLanguage["en"]
		if !ok {
			continue
		}
		p("- `%s` (%d of %d EN emails): %s\n", key.Key, en.TopCount, en.Emails, quote(en.Top, 140))
	}
	p("\n")

	p("## CTA buttons\n\n")
	p("With fields: URL fields whose key contains `cta`. Without fields: links the review markup marked as `cta`, so it counts every meaningful link, not only buttons.\n\n")
	p("| CTAs | With fields | Without fields |\n|---|---|---|\n")
	for _, bucket := range []string{"0", "1", "2", "3+"} {
		p("| %s | %d | %d |\n", bucket, audit.CTA.WithFields[bucket], audit.CTA.WithoutFields[bucket])
	}
	if len(audit.CTA.TwoPlus) > 0 {
		p("\nEmails with fields and more than one CTA:\n\n")
		for _, e := range audit.CTA.TwoPlus {
			p("- %s — \"%s\"\n", escape(e.Place()), escape(e.Title))
		}
	}
	p("\n")

	p("## Lengths (characters)\n\n")
	p("Nearest-rank percentiles. CTA text = `primary_cta_text` of emails with fields.\n\n")
	p("| Text | Language | Emails | p50 | p90 | p95 | Max | Longest |\n|---|---|---|---|---|---|---|---|\n")
	for _, stat := range audit.Lengths {
		language := strings.ToUpper(stat.Language)
		if stat.Language == "all" {
			language = "**all**"
		}
		p("| %s | %s | %d | %d | %d | %d | %d | %s (%s) |\n", stat.Name, language, stat.Count,
			stat.P50, stat.P90, stat.P95, stat.Max, quote(stat.Longest, 80), escape(stat.LongestAt))
	}
	p("\n")
}

func spreadCell(s ValueSpread) string {
	switch {
	case s.Emails == 0:
		return "—"
	case s.Distinct == 1:
		return fmt.Sprintf("✓ 1/%d", s.Emails)
	default:
		return fmt.Sprintf("%d/%d", s.Distinct, s.Emails)
	}
}

func quote(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		text = string(runes[:limit]) + "…"
	}
	return "«" + escape(text) + "»"
}

func escape(text string) string {
	return strings.ReplaceAll(text, "|", "\\|")
}

func first[T any](values []T, n int) []T {
	if len(values) > n {
		return values[:n]
	}
	return values
}
