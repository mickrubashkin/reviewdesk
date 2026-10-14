package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

var kindMeaning = map[AdaptationKind]string{
	KindIdentical: "same text as the base → drop, no content",
	KindSignature: "mostly only signature/sender differs → sender variable; differing emails become overrides",
	KindOverride:  "a few fields differ → override on top of the master",
	KindBranch:    "differs a lot or has no base → separate branch",
	KindDuplicate: "same text as another adaptation (a couple of differing emails allowed) → merge",
}

func WriteReport(w io.Writer, plan Plan) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format, args...) }

	p("# Slot migration dry-run\n\n")
	p("Read-only analysis. Nothing in the database was changed.\n\n")

	p("## Summary\n\n")
	p("| | Count |\n|---|---|\n")
	p("| Emails analysed | %d |\n", plan.TotalEmails)
	p("| Slots (stage + position) | %d |\n", len(plan.Slots))
	p("| EN masters | %d |\n", plan.Masters)
	p("| Translations (default adaptation) | %d |\n", plan.Translations)
	p("| … linked to the master automatically | %d |\n", plan.TranslationsLinked)
	p("| … need manual check (field keys differ) | %d |\n", plan.Translations-plan.TranslationsLinked)
	p("| `old` emails → read-only history | %d |\n", plan.ArchivedOld)
	p("| Experiments (variant other than new/v1) | %d |\n", len(plan.Experiments))
	p("| Content documents to maintain: now → after | **%d → %d** |\n\n", plan.ContentBefore, plan.ContentAfter)

	p("## Adaptations\n\n")
	p("| Adaptation | Label(s) | Emails | Languages | Verdict | Changed share (median / max) | Most changed fields |\n")
	p("|---|---|---|---|---|---|---|\n")
	for _, a := range plan.Adaptations {
		verdict := string(a.Kind)
		if a.Kind == KindDuplicate {
			verdict = "duplicate of `" + a.DuplicateOf + "`"
		}
		if (a.Kind == KindDuplicate || a.Kind == KindSignature) && a.Exceptions > 0 {
			verdict += fmt.Sprintf(" + %d email override(s)", a.Exceptions)
		}
		if a.NoBase > 0 && a.NoBase < a.Emails {
			verdict += fmt.Sprintf(" (%d without base)", a.NoBase)
		}
		p("| `%s` | %s | %d | %s | %s | %.0f%% / %.0f%% | %s |\n",
			a.Key, strings.Join(a.Labels, ", "), a.Emails, strings.Join(a.Languages, ", "),
			verdict, a.MedianShare*100, a.MaxShare*100, topFields(a.ChangedFields, 4))
	}
	p("\nVerdicts:\n\n")
	for _, kind := range []AdaptationKind{KindDuplicate, KindSignature, KindOverride, KindBranch, KindIdentical} {
		p("- **%s**: %s\n", kind, kindMeaning[kind])
	}
	p("\nShares ignore signature/sender fields. Branch threshold: median above %.0f%%.\n\n", overrideMaxShare*100)

	if len(plan.DuplicateLabels) > 0 {
		p("### Different adaptations with the same label\n\n")
		for _, label := range sortedKeys(plan.DuplicateLabels) {
			p("- \"%s\": `%s`\n", label, strings.Join(plan.DuplicateLabels[label], "`, `"))
		}
		p("\n")
	}

	if len(plan.SlotsWithoutMaster) > 0 {
		p("## Slots without an EN master\n\n")
		p("These slots only exist as adaptations or other languages, so there is nothing to translate from.\n\n")
		for _, s := range plan.SlotsWithoutMaster {
			p("- %s\n", s)
		}
		p("\n")
	}

	p("## Translations that need a manual check\n\n")
	if len(plan.TranslationIssues) == 0 {
		p("None — every translation has the same fields as its master.\n\n")
	}
	for _, issue := range plan.TranslationIssues {
		p("- %d case(s). Missing: %s. Extra: %s.\n  Examples: %s\n",
			len(issue.Cases), listOrDash(issue.Missing), listOrDash(issue.Extra), strings.Join(first(issue.Cases, 3), "; "))
	}
	p("\n")

	p("## Missing translations\n\n")
	for _, lang := range supportedLanguages {
		if slots := plan.MissingTranslation[lang]; len(slots) > 0 {
			p("- **%s** (%d): %s\n", strings.ToUpper(lang), len(slots), strings.Join(slots, "; "))
		}
	}
	p("\n")

	if len(plan.Experiments) > 0 || len(plan.UnsupportedLang) > 0 {
		p("## Needs a decision\n\n")
		for _, e := range plan.Experiments {
			p("- Experiment: %s #%d \"%s\" (%s, variant `%s`)\n", e.Stage, e.SortOrder, e.Title, e.Language, e.Variant)
		}
		for _, e := range plan.UnsupportedLang {
			p("- Language outside EN/DE/ES/PL/BR: %s #%d \"%s\" (`%s`)\n", e.Stage, e.SortOrder, e.Title, e.Language)
		}
		p("\n")
	}

	p("## Workflow usage today\n\n")
	statuses := sortedKeys(plan.StatusCounts)
	sort.SliceStable(statuses, func(i, j int) bool { return plan.StatusCounts[statuses[i]] > plan.StatusCounts[statuses[j]] })
	for _, s := range statuses {
		p("- `%s`: %d\n", s, plan.StatusCounts[s])
	}
	p("- Without owner: %d of %d\n", plan.WithoutOwner, plan.TotalEmails)
}

func topFields(counts map[string]int, n int) string {
	keys := sortedKeys(counts)
	sort.SliceStable(keys, func(i, j int) bool { return counts[keys[i]] > counts[keys[j]] })
	keys = first(keys, n)
	for i, k := range keys {
		keys[i] = fmt.Sprintf("`%s` ×%d", k, counts[k])
	}
	if len(keys) == 0 {
		return "—"
	}
	return strings.Join(keys, ", ")
}

func listOrDash(values []string) string {
	if len(values) == 0 {
		return "—"
	}
	return "`" + strings.Join(values, "`, `") + "`"
}

func first[T any](values []T, n int) []T {
	if len(values) > n {
		return values[:n]
	}
	return values
}
