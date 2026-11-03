package main

import (
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

// statusOrder is the order of the report: what gets applied, then what is
// held back, the reasons a person can act on first.
var statusOrder = []struct {
	status, title, hint string
}{
	{emails.CommentEditApply, "Applied", "The selected text was found once in the comment's field and is replaced by the comment."},
	{emails.CommentEditImage, "Text on an image", "The words are drawn on the banner image; only its alt text is a field. It needs a new image."},
	{emails.CommentEditShared, "Shared text", "The field takes its value from a shared text: change the shared text, or unlink the field in this email."},
	{emails.CommentEditConflict, "Overlapping edits", "Two comments change the same words of a field: pick one."},
	{emails.CommentEditAmbiguous, "Text occurs more than once", "The selected text is in the field more than once: edit by hand."},
	{emails.CommentEditElsewhere, "Text in another field", "The selected text is not in the comment's field but in another one."},
	{emails.CommentEditQuestion, "A request or a question", "The comment asks for a change rather than giving the new text."},
	{emails.CommentEditNotText, "A link or a fragment", "The comment is a link or a few characters, not replacement text."},
	{emails.CommentEditStale, "Text changed since", "The selected text is no longer in the email: the comment may be done, or out of date."},
}

func countByStatus(planned []emails.CommentEditEmail) map[string]int {
	counts := map[string]int{}
	for _, email := range planned {
		for _, edit := range email.Edits {
			counts[edit.Status]++
		}
	}
	return counts
}

func writeSummary(w io.Writer, planned []emails.CommentEditEmail, applied bool) {
	if applied {
		fmt.Fprintln(w, "Applied.")
	} else {
		fmt.Fprintln(w, "Dry run, nothing written.")
	}
	counts := countByStatus(planned)
	changed := 0
	for _, email := range planned {
		if email.Changed {
			changed++
		}
	}
	failed := 0
	for _, email := range planned {
		if email.Error != "" {
			failed++
		}
	}
	fmt.Fprintf(w, "Emails with open comments: %d; emails changed: %d; could not save: %d\n", len(planned), changed, failed)
	for _, s := range statusOrder {
		fmt.Fprintf(w, "- %s: %d\n", s.title, counts[s.status])
	}
}

func writeReport(w io.Writer, planned []emails.CommentEditEmail, applied bool, appURL string) error {
	e := html.EscapeString
	var b strings.Builder
	mode := "Dry run: nothing was written."
	if applied {
		mode = "Applied."
	}
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Comment edits</title><style>
:root { --bg:#f7f7f5; --card:#fff; --text:#1d1d1b; --muted:#6b6b66; --line:#e4e3df; --accent:#2f6fdf; --del:#fde2e1; --ins:#dff3e4; }
@media (prefers-color-scheme: dark) { :root { --bg:#161615; --card:#1f1f1d; --text:#ecebe7; --muted:#9a9a94; --line:#33332f; --accent:#6a9cf5; --del:#5a2a28; --ins:#21452b; } }
* { box-sizing:border-box; } body { margin:0; background:var(--bg); color:var(--text); font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; }
main { max-width:960px; margin:0 auto; padding:32px 16px 80px; } h1 { font-size:24px; margin:0 0 4px; } .muted { color:var(--muted); }
nav a { margin-right:12px; } section { margin-top:28px; } h2 { font-size:18px; margin:0; } h3 { font-size:15px; margin:14px 0 4px; }
.email { background:var(--card); border:1px solid var(--line); border-radius:10px; padding:12px 16px; margin-top:10px; }
.edit { border-top:1px solid var(--line); padding:8px 0; } .edit:first-of-type { border-top:none; }
del { background:var(--del); text-decoration:none; } ins { background:var(--ins); text-decoration:none; }
.field { font:12px ui-monospace,Menlo,monospace; color:var(--muted); } .after { white-space:pre-wrap; color:var(--muted); font-size:13px; margin-top:4px; }
a { color:var(--accent); }
</style></head><body><main>`)
	fmt.Fprintf(&b, `<h1>Comment edits</h1><p class="muted">%s %d emails with open comments.</p><nav>`, e(mode), len(planned))
	counts := countByStatus(planned)
	for _, s := range statusOrder {
		if counts[s.status] > 0 {
			fmt.Fprintf(&b, `<a href="#%s">%s (%d)</a>`, s.status, e(s.title), counts[s.status])
		}
	}
	b.WriteString(`</nav>`)
	var failed []emails.CommentEditEmail
	for _, email := range planned {
		if email.Error != "" {
			failed = append(failed, email)
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(&b, `<section id="failed"><h2>Could not save · %d emails</h2><p class="muted">The save refused these emails, so none of their comments were applied; they are listed under Applied as they would have been.</p>`, len(failed))
		for _, email := range failed {
			fmt.Fprintf(&b, `<div class="email"><b>%s</b> <span class="muted">%s</span><div class="muted">%s</div></div>`, emailLink(email, appURL), e(email.Title), e(email.Error))
		}
		b.WriteString(`</section>`)
	}
	for _, s := range statusOrder {
		if counts[s.status] == 0 {
			continue
		}
		fmt.Fprintf(&b, `<section id="%s"><h2>%s · %d</h2><p class="muted">%s</p>`, s.status, e(s.title), counts[s.status], e(s.hint))
		for _, email := range planned {
			var edits []emails.CommentEdit
			for _, edit := range email.Edits {
				if edit.Status == s.status {
					edits = append(edits, edit)
				}
			}
			if len(edits) == 0 {
				continue
			}
			link := emailLink(email, appURL)
			state := ""
			if email.Ready {
				state += " · ready"
			}
			if email.Live {
				state += " · live"
			}
			if email.Error != "" {
				state += " · could not save"
			}
			fmt.Fprintf(&b, `<div class="email"><b>%s</b> <span class="muted">%s%s</span>`, link, e(email.Title), state)
			shown := map[string]bool{}
			for _, edit := range edits {
				fmt.Fprintf(&b, `<div class="edit"><span class="field">%s</span><br><del>%s</del> → <ins>%s</ins>`, e(edit.Field), e(edit.Selected), e(edit.Body))
				if len(edit.Found) > 0 {
					fmt.Fprintf(&b, `<div class="muted">found in: %s</div>`, e(strings.Join(edit.Found, ", ")))
				}
				// The field after every applied edit of it, once per field.
				if edit.After != "" && !shown[edit.Field] {
					shown[edit.Field] = true
					fmt.Fprintf(&b, `<div class="after">%s</div>`, e(edit.After))
				}
				b.WriteString(`</div>`)
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</section>`)
	}
	b.WriteString(`</main></body></html>`)
	_, err := io.WriteString(w, b.String())
	return err
}

func emailLink(email emails.CommentEditEmail, appURL string) string {
	e := html.EscapeString
	if appURL == "" {
		return e(email.Place)
	}
	return fmt.Sprintf(`<a href="%s/emails/%s/review" target="_blank" rel="noopener">%s</a>`, e(strings.TrimRight(appURL, "/")), e(email.ID), e(email.Place))
}
