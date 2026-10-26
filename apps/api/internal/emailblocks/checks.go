package emailblocks

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// ButtonKey is the email's button. An email built from blocks has exactly
// one; links inside the text are not buttons.
const ButtonKey = "primary_cta"

// Length limits of the subject and preheader, near the 95th percentile of
// the emails in all languages (audit, 17 Oct 2026).
const (
	SubjectMaxLength   = 90
	PreheaderMaxLength = 110
)

// Issue levels: an error stops a save that brings it in; both stop the
// email from being marked ready.
const (
	IssueError   = "error"
	IssueWarning = "warning"
)

// Issue is a rule of form the email breaks.
type Issue struct {
	Level string `json:"level"`
	// Field is the key of the field it is about, "subject" or "preheader";
	// empty for the email as a whole.
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// CheckInput is the email as the checks see it. Layout is nil for an email
// in the old format: it gets only the subject and preheader limits, its
// fields are not described by any block.
type CheckInput struct {
	Library   []Block
	Layout    Layout
	Fields    emailedit.EditableFields
	Subject   string
	Preheader string
}

// Check lists the rules the email breaks: errors first, then warnings, each
// in email order.
func Check(input CheckInput) []Issue {
	errors, warnings := []Issue{}, []Issue{}
	warnings = appendLengthIssue(warnings, "subject", "Subject", input.Subject, SubjectMaxLength)
	warnings = appendLengthIssue(warnings, "preheader", "Preheader", input.Preheader, PreheaderMaxLength)

	if input.Layout != nil {
		byKey := map[string]Block{}
		for _, block := range input.Library {
			byKey[block.Key] = block
		}
		buttons := 0
		for _, item := range input.Layout {
			if item.Block == ButtonKey {
				buttons++
			}
			block := byKey[item.Block]
			for _, field := range InstanceFields(block, item.Suffix) {
				// A field the email lacks counts as empty; a number has no
				// length to check.
				raw := input.Fields[field.Key].Value
				value, isText := raw.(string)
				if !isText && raw != nil {
					continue
				}
				if field.Required && strings.TrimSpace(value) == "" {
					errors = append(errors, Issue{Level: IssueError, Field: field.Key,
						Message: fmt.Sprintf("%s: %s is empty", block.Name, field.Label)})
				}
				warnings = appendLengthIssue(warnings, field.Key, block.Name+": "+field.Label, value, field.MaxLength)
			}
		}
		switch {
		case buttons == 0:
			errors = append([]Issue{{Level: IssueError, Message: "The email has no button: add the Button block"}}, errors...)
		case buttons > 1:
			errors = append([]Issue{{Level: IssueError,
				Message: fmt.Sprintf("The email has %d buttons, it needs exactly one: put other links in the text", buttons)}}, errors...)
		}
	}
	return append(errors, warnings...)
}

func appendLengthIssue(issues []Issue, key, label, value string, limit int) []Issue {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	if limit <= 0 || length <= limit {
		return issues
	}
	return append(issues, Issue{Level: IssueWarning, Field: key,
		Message: fmt.Sprintf("%s is %d characters, the limit is %d", label, length, limit)})
}

// NewErrors lists the errors in after that before does not have: a save may
// keep an error the email already had, but not bring in a new one.
func NewErrors(before, after []Issue) []Issue {
	had := map[Issue]bool{}
	for _, issue := range before {
		had[issue] = true
	}
	added := []Issue{}
	for _, issue := range after {
		if issue.Level == IssueError && !had[issue] {
			added = append(added, issue)
		}
	}
	return added
}
