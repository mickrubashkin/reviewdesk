package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

// WriteReport prints the plan as Markdown: what switches to blocks and what
// stays in the old format, and why.
func WriteReport(w io.Writer, board string, families []emails.LayoutFamily, strict, applied bool, switched, keptUpload int) {
	fmt.Fprintf(w, "# Switching emails to blocks — %s\n\n", board)
	if applied {
		fmt.Fprintf(w, "Applied: %d emails switched to blocks; %d kept their upload mark.\n\n", switched, keptUpload)
	} else {
		fmt.Fprintf(w, "Dry run: nothing was changed. Run with `-apply -actor you@company.com` to switch the emails below.\n\n")
	}

	var ready, stay []emails.LayoutFamily
	readyEmails, readyLive, readyMarkup, stayEmails := 0, 0, 0, 0
	for _, family := range families {
		members := family.Members()
		if family.Blocker(strict) != "" {
			stay = append(stay, family)
			stayEmails += len(members)
			continue
		}
		ready = append(ready, family)
		for _, member := range members {
			readyEmails++
			if member.Live {
				readyLive++
			}
			if member.Match == emailblocks.MatchContent {
				readyMarkup++
			}
		}
	}

	fmt.Fprintf(w, "| | Emails |\n|---|---|\n")
	fmt.Fprintf(w, "| Switch to blocks | %d |\n", readyEmails)
	fmt.Fprintf(w, "| … same HTML | %d |\n", readyEmails-readyMarkup)
	fmt.Fprintf(w, "| … same text, links and images, markup differs | %d |\n", readyMarkup)
	fmt.Fprintf(w, "| … live (get a new version: re-upload) | %d |\n", readyLive)
	fmt.Fprintf(w, "| Stay in the old format | %d |\n\n", stayEmails)
	fmt.Fprintf(w, "An email and its translations switch together. Statuses stay; every switched email gets a new version.\n\n")

	if len(ready) > 0 {
		fmt.Fprintf(w, "## Switch to blocks\n\n")
		fmt.Fprintf(w, "| Email | Title | Blocks | Render | Notes |\n|---|---|---|---|---|\n")
		for _, family := range ready {
			for _, member := range family.Members() {
				fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n", member.Place, cell(member.Title),
					strings.Join(member.Layout.Keys(), " → "), member.Match, cell(notes(member)))
			}
		}
		fmt.Fprintln(w)
	}

	if len(stay) > 0 {
		fmt.Fprintf(w, "## Stay in the old format — to do by hand\n\n")
		fmt.Fprintf(w, "| Email | Title | Live | Why |\n|---|---|---|---|\n")
		for _, family := range stay {
			fmt.Fprintf(w, "| %s | %s | %s | %s |\n", familyPlace(family), cell(family.Master.Title),
				yes(family.Master.Live), cell(family.Blocker(strict)))
		}
		fmt.Fprintln(w)
	}
}

func notes(member emails.LayoutConversion) string {
	parts := []string{}
	if member.Live {
		parts = append(parts, "live")
	}
	if len(member.Recovered) > 0 {
		parts = append(parts, "from the old template: "+strings.Join(member.Recovered, ", "))
	}
	if member.MovedComments > 0 {
		parts = append(parts, fmt.Sprintf("%d comments move to the new blocks", member.MovedComments))
	}
	if member.Match == emailblocks.MatchContent {
		parts = append(parts, member.Reason)
	}
	return strings.Join(parts, "; ")
}

func familyPlace(family emails.LayoutFamily) string {
	place := family.Master.Place
	if len(family.Translations) > 0 {
		languages := []string{}
		for _, translation := range family.Translations {
			languages = append(languages, strings.ToUpper(translation.Language))
		}
		place += " + " + strings.Join(languages, ", ")
	}
	return place
}

func yes(value bool) string {
	if value {
		return "yes"
	}
	return ""
}

func cell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", `\|`)
}
