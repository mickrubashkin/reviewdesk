package emailblocks

import (
	"strings"
	"testing"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

func TestCheck(t *testing.T) {
	library := Defaults()
	fields := func(values map[string]string) emailedit.EditableFields {
		result := emailedit.EditableFields{}
		for key, value := range values {
			result[key] = emailedit.EditableField{Type: "text", Value: value}
		}
		return result
	}
	good := fields(map[string]string{
		"intro_body": "Hello", "primary_cta_text": "Open", "primary_cta_url": "https://example.com",
	})
	layout := Layout{{Block: "intro_body"}, {Block: "primary_cta"}}

	if issues := Check(CheckInput{Library: library, Layout: layout, Fields: good, Subject: "Hi"}); len(issues) != 0 {
		t.Fatalf("a good email has no issues: %+v", issues)
	}

	t.Run("buttons", func(t *testing.T) {
		none := Check(CheckInput{Library: library, Layout: Layout{{Block: "intro_body"}}, Fields: good})
		if len(none) != 1 || none[0].Level != IssueError || !strings.Contains(none[0].Message, "no button") {
			t.Fatalf("no button is an error: %+v", none)
		}
		two := Check(CheckInput{Library: library, Fields: good,
			Layout: Layout{{Block: "primary_cta"}, {Block: "primary_cta", Suffix: "2"}}})
		if len(two) == 0 || !strings.Contains(two[0].Message, "2 buttons") {
			t.Fatalf("two buttons is an error: %+v", two)
		}
		// The repeat has its own fields, empty here.
		if len(two) != 3 || two[1].Field != "primary_cta_text_2" || two[2].Field != "primary_cta_url_2" {
			t.Fatalf("the repeat's empty fields are errors under its keys: %+v", two)
		}
	})

	t.Run("required and limits", func(t *testing.T) {
		issues := Check(CheckInput{Library: library, Layout: layout,
			Fields: fields(map[string]string{
				"intro_body": "  \n", "primary_cta_text": strings.Repeat("я", 31), "primary_cta_url": "https://example.com",
			}),
			Subject:   strings.Repeat("s", SubjectMaxLength+1),
			Preheader: strings.Repeat("p", PreheaderMaxLength),
		})
		want := []Issue{
			{Level: IssueError, Field: "intro_body", Message: "Main text: Text is empty"},
			{Level: IssueWarning, Field: "subject", Message: "Subject is 91 characters, the limit is 90"},
			{Level: IssueWarning, Field: "primary_cta_text", Message: "Button: Button text is 31 characters, the limit is 30"},
		}
		if len(issues) != len(want) {
			t.Fatalf("got %+v", issues)
		}
		for i := range want {
			if issues[i] != want[i] {
				t.Fatalf("issue %d: got %+v, want %+v", i, issues[i], want[i])
			}
		}
	})

	t.Run("old format", func(t *testing.T) {
		issues := Check(CheckInput{Fields: emailedit.EditableFields{}, Preheader: strings.Repeat("p", PreheaderMaxLength+1)})
		if len(issues) != 1 || issues[0].Field != "preheader" {
			t.Fatalf("an email in the old format gets only the subject and preheader limits: %+v", issues)
		}
	})
}

func TestNewErrors(t *testing.T) {
	empty := Issue{Level: IssueError, Field: "intro_body", Message: "Main text: Text is empty"}
	long := Issue{Level: IssueWarning, Field: "subject", Message: "Subject is 91 characters, the limit is 90"}
	noButton := Issue{Level: IssueError, Message: "The email has no button: add the Button block"}

	if added := NewErrors([]Issue{empty}, []Issue{empty, long}); len(added) != 0 {
		t.Fatalf("an error the email had and a warning are not new: %+v", added)
	}
	if added := NewErrors([]Issue{empty}, []Issue{noButton, empty}); len(added) != 1 || added[0] != noButton {
		t.Fatalf("expected the missing button, got %+v", added)
	}
}
