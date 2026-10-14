package main

import (
	"fmt"
	"sort"
	"strings"
)

// Email is the subset of an email row the planner needs. It matches both the
// DB loader and the JSON returned by GET /api/emails/{id}.
type Email struct {
	ID              string                `json:"id"`
	Sequence        string                `json:"sequence"`
	Stage           string                `json:"stage"`
	SortOrder       int                   `json:"sort_order"`
	Title           string                `json:"title"`
	Language        string                `json:"language"`
	Variant         string                `json:"variant"`
	AdaptationKey   string                `json:"adaptation_key"`
	AdaptationLabel string                `json:"adaptation_label"`
	ReviewStatus    string                `json:"review_status"`
	OwnerEmail      *string               `json:"owner_email"`
	EditableFields  map[string]FieldValue `json:"editable_fields"`
}

type FieldValue struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

const (
	masterLanguage = "en"
	defaultAdapt   = "default"
	// An adaptation that changes more than this share of the non-signature
	// text fields (median across its emails) becomes a branch, not an override.
	overrideMaxShare = 1.0 / 3
)

var supportedLanguages = []string{"en", "de", "es", "pl", "br"}

type AdaptationKind string

const (
	KindIdentical AdaptationKind = "identical" // same text as the base everywhere
	KindSignature AdaptationKind = "signature" // differs only in signature/sender fields
	KindOverride  AdaptationKind = "override"  // a few changed fields on top of the base
	KindBranch    AdaptationKind = "branch"    // differs too much, or has no base
	KindDuplicate AdaptationKind = "duplicate" // same text as another adaptation
)

type Slot struct {
	Key       string
	Stage     string
	SortOrder int
	Title     string
	Emails    []Email
}

type adaptationSample struct {
	Email Email
	Base  *Email
	Slot  string
}

type AdaptationReport struct {
	Key         string
	Labels      []string
	Emails      int
	Languages   []string
	Kind        AdaptationKind
	DuplicateOf string
	NoBase      int
	// Exceptions counts emails that change content fields although the
	// adaptation as a whole is a signature variant or a duplicate.
	Exceptions    int
	MedianShare   float64
	MaxShare      float64
	ChangedFields map[string]int
}

type TranslationIssue struct {
	Missing []string
	Extra   []string
	Cases   []string // "slot (lang)"
}

type Plan struct {
	TotalEmails        int
	Slots              []Slot
	SlotsWithoutMaster []string
	ArchivedOld        int
	Experiments        []Email
	UnsupportedLang    []Email
	Masters            int
	Translations       int
	TranslationsLinked int
	TranslationIssues  []TranslationIssue
	MissingTranslation map[string][]string
	Adaptations        []AdaptationReport
	DuplicateLabels    map[string][]string
	StatusCounts       map[string]int
	WithoutOwner       int
	ContentBefore      int
	ContentAfter       int
}

func isMainVariant(variant string) bool {
	return variant == "new" || variant == "v1"
}

func isSupported(language string) bool {
	for _, l := range supportedLanguages {
		if l == language {
			return true
		}
	}
	return false
}

func isSignatureField(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "signature") || strings.Contains(key, "sender")
}

func textFields(email Email) map[string]string {
	out := map[string]string{}
	for key, field := range email.EditableFields {
		if field.Type == "text" {
			out[key] = strings.Join(strings.Fields(fmt.Sprint(field.Value)), " ")
		}
	}
	return out
}

func textSignature(email Email) string {
	fields := textFields(email)
	var b strings.Builder
	for _, key := range sortedKeys(fields) {
		b.WriteString(key + "=" + fields[key] + "\x00")
	}
	return b.String()
}

// changedFields returns changed keys and the non-signature union size.
func changedFields(a, b map[string]string) (changed []string, contentKeys int) {
	keys := map[string]bool{}
	for key := range a {
		keys[key] = true
	}
	for key := range b {
		keys[key] = true
	}
	for key := range keys {
		if !isSignatureField(key) {
			contentKeys++
		}
		if a[key] != b[key] {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	return changed, contentKeys
}

func BuildPlan(emails []Email) Plan {
	plan := Plan{
		TotalEmails:        len(emails),
		MissingTranslation: map[string][]string{},
		DuplicateLabels:    map[string][]string{},
		StatusCounts:       map[string]int{},
	}

	slotByKey := map[string]*Slot{}
	var slotKeys []string
	for _, email := range emails {
		plan.StatusCounts[email.ReviewStatus]++
		if email.OwnerEmail == nil || strings.TrimSpace(*email.OwnerEmail) == "" {
			plan.WithoutOwner++
		}
		if email.Variant != "old" {
			plan.ContentBefore++
		}
		key := fmt.Sprintf("%s/%s/%d", email.Sequence, email.Stage, email.SortOrder)
		if slotByKey[key] == nil {
			slotByKey[key] = &Slot{Key: key, Stage: email.Stage, SortOrder: email.SortOrder}
			slotKeys = append(slotKeys, key)
		}
		slotByKey[key].Emails = append(slotByKey[key].Emails, email)
	}
	sort.SliceStable(slotKeys, func(i, j int) bool {
		return slotByKey[slotKeys[i]].SortOrder < slotByKey[slotKeys[j]].SortOrder
	})

	samples := map[string][]adaptationSample{}
	labels := map[string]map[string]bool{}
	keysByLabel := map[string]map[string]bool{}
	issues := map[string]*TranslationIssue{}
	var issueOrder []string

	for _, key := range slotKeys {
		slot := slotByKey[key]
		var master *Email
		for i := range slot.Emails {
			e := &slot.Emails[i]
			if e.Language == masterLanguage && e.AdaptationKey == defaultAdapt && isMainVariant(e.Variant) {
				master = e
			}
		}
		slot.Title = slot.Emails[0].Title
		if master != nil {
			slot.Title = master.Title
			plan.Masters++
		} else if hasMainContent(slot.Emails) {
			plan.SlotsWithoutMaster = append(plan.SlotsWithoutMaster, slotLabel(*slot))
		}

		present := map[string]bool{}
		for _, e := range slot.Emails {
			if e.AdaptationKey != defaultAdapt {
				addTo(labels, e.AdaptationKey, e.AdaptationLabel)
				addTo(keysByLabel, e.AdaptationLabel, e.AdaptationKey)
			}

			switch {
			case e.Variant == "old":
				plan.ArchivedOld++
				continue
			case !isMainVariant(e.Variant):
				plan.Experiments = append(plan.Experiments, e)
				continue
			case !isSupported(e.Language):
				plan.UnsupportedLang = append(plan.UnsupportedLang, e)
				continue
			}

			if e.AdaptationKey != defaultAdapt {
				samples[e.AdaptationKey] = append(samples[e.AdaptationKey], adaptationSample{
					Email: e,
					Base:  findBase(slot.Emails, e),
					Slot:  slotLabel(*slot),
				})
				continue
			}

			present[e.Language] = true
			if e.Language == masterLanguage || master == nil {
				continue
			}
			plan.Translations++
			missing, extra := keyDiff(textFields(*master), textFields(e))
			if len(missing) == 0 && len(extra) == 0 {
				plan.TranslationsLinked++
				continue
			}
			sig := strings.Join(missing, ",") + "|" + strings.Join(extra, ",")
			if issues[sig] == nil {
				issues[sig] = &TranslationIssue{Missing: missing, Extra: extra}
				issueOrder = append(issueOrder, sig)
			}
			issues[sig].Cases = append(issues[sig].Cases, fmt.Sprintf("%s (%s)", slotLabel(*slot), e.Language))
		}

		if master != nil {
			for _, lang := range supportedLanguages {
				if lang != masterLanguage && !present[lang] {
					plan.MissingTranslation[lang] = append(plan.MissingTranslation[lang], slotLabel(*slot))
				}
			}
		}
		plan.Slots = append(plan.Slots, *slot)
	}

	for _, sig := range issueOrder {
		plan.TranslationIssues = append(plan.TranslationIssues, *issues[sig])
	}
	sort.SliceStable(plan.TranslationIssues, func(i, j int) bool {
		return len(plan.TranslationIssues[i].Cases) > len(plan.TranslationIssues[j].Cases)
	})
	for label, keys := range keysByLabel {
		if len(keys) > 1 {
			plan.DuplicateLabels[label] = sortedKeys(keys)
		}
	}

	plan.Adaptations = classifyAdaptations(samples, labels)
	plan.ContentAfter = plan.Masters + plan.Translations + len(plan.Experiments)
	for _, report := range plan.Adaptations {
		// Overrides and branches keep content; the rest collapse into variables
		// or into another adaptation.
		switch report.Kind {
		case KindOverride, KindBranch:
			plan.ContentAfter += report.Emails
		case KindSignature, KindDuplicate:
			plan.ContentAfter += report.Exceptions
		}
	}
	return plan
}

func classifyAdaptations(samples map[string][]adaptationSample, labels map[string]map[string]bool) []AdaptationReport {
	keys := sortedKeys(samples)
	// Larger adaptations first, so duplicates point at the most complete one.
	sort.SliceStable(keys, func(i, j int) bool { return len(samples[keys[i]]) > len(samples[keys[j]]) })

	texts := map[string]map[string]string{} // adaptation -> "slot|lang" -> text
	for _, key := range keys {
		texts[key] = map[string]string{}
		for _, s := range samples[key] {
			texts[key][s.Slot+"|"+s.Email.Language] = textSignature(s.Email)
		}
	}

	var reports []AdaptationReport
	var canonical []string
	for _, key := range keys {
		report := AdaptationReport{
			Key:           key,
			Labels:        sortedKeys(labels[key]),
			Emails:        len(samples[key]),
			ChangedFields: map[string]int{},
		}
		langs := map[string]bool{}
		var shares []float64
		anyChange := false
		for _, s := range samples[key] {
			langs[s.Email.Language] = true
			if s.Base == nil {
				report.NoBase++
				continue
			}
			changed, contentKeys := changedFields(textFields(s.Email), textFields(*s.Base))
			contentChanged := 0
			for _, field := range changed {
				report.ChangedFields[field]++
				anyChange = true
				if !isSignatureField(field) {
					contentChanged++
				}
			}
			if contentChanged > 0 {
				report.Exceptions++
			}
			share := 0.0
			if contentKeys > 0 {
				share = float64(contentChanged) / float64(contentKeys)
			}
			shares = append(shares, share)
		}
		report.Languages = sortedKeys(langs)
		report.MedianShare, report.MaxShare = median(shares), maxOf(shares)

		switch {
		case report.NoBase == report.Emails:
			report.Kind = KindBranch
		case !anyChange:
			report.Kind = KindIdentical
		case report.MedianShare == 0:
			// Typically only the signature differs; the rare emails that also
			// change content become per-email overrides (Exceptions).
			report.Kind = KindSignature
		case report.MedianShare > overrideMaxShare:
			report.Kind = KindBranch
		default:
			report.Kind = KindOverride
		}

		for _, other := range canonical {
			if diff, ok := textDifferences(texts[other], texts[key]); ok && diff <= nearDuplicateMax(len(texts[key])) {
				report.DuplicateOf = other
				report.Kind = KindDuplicate
				report.Exceptions = diff
				break
			}
		}
		if report.Kind != KindDuplicate {
			canonical = append(canonical, key)
		}
		reports = append(reports, report)
	}
	return reports
}

// textDifferences counts locations of b whose text differs from a. ok is
// false when a does not cover every location of b.
func textDifferences(a, b map[string]string) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	diff := 0
	for loc, text := range b {
		other, ok := a[loc]
		if !ok {
			return 0, false
		}
		if other != text {
			diff++
		}
	}
	return diff, true
}

// nearDuplicateMax allows a couple of differing emails before an adaptation
// stops counting as a copy of another one.
func nearDuplicateMax(locations int) int {
	if locations >= 10 {
		return 2
	}
	return 0
}

func hasMainContent(emails []Email) bool {
	for _, e := range emails {
		if isMainVariant(e.Variant) {
			return true
		}
	}
	return false
}

func findBase(emails []Email, e Email) *Email {
	for i := range emails {
		b := emails[i]
		if b.AdaptationKey == defaultAdapt && b.Language == e.Language && b.Variant == e.Variant {
			return &emails[i]
		}
	}
	return nil
}

func keyDiff(master, own map[string]string) (missing, extra []string) {
	for key := range master {
		if _, ok := own[key]; !ok {
			missing = append(missing, key)
		}
	}
	for key := range own {
		if _, ok := master[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

func slotLabel(slot Slot) string {
	return fmt.Sprintf("%s #%d %s", slot.Stage, slot.SortOrder, slot.Emails[0].Title)
}

func addTo(m map[string]map[string]bool, key, value string) {
	if m[key] == nil {
		m[key] = map[string]bool{}
	}
	m[key][value] = true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	return sorted[len(sorted)/2]
}

func maxOf(values []float64) float64 {
	result := 0.0
	for _, v := range values {
		if v > result {
			result = v
		}
	}
	return result
}
