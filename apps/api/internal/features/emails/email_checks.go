package emails

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// checkEmail lists the rules of form an email breaks; library is loaded
// only for an email built from blocks and may be passed in when the caller
// has it.
func checkEmail(ctx context.Context, db emailEventExecutor, library []emailblocks.Block, layout emailblocks.Layout, fields emailedit.EditableFields, subject, preheader *string) ([]emailblocks.Issue, error) {
	if layout != nil && library == nil {
		var err error
		if library, err = loadBlockLibrary(ctx, db); err != nil {
			return nil, err
		}
	}
	return emailblocks.Check(emailblocks.CheckInput{
		Library:   library,
		Layout:    layout,
		Fields:    fields,
		Subject:   stringFromPointer(subject),
		Preheader: stringFromPointer(preheader),
	}), nil
}

// checkStoredEmail is checkEmail for the layout and fields as stored.
func checkStoredEmail(ctx context.Context, db emailEventExecutor, layoutJSON, fieldsJSON []byte, subject, preheader *string) ([]emailblocks.Issue, error) {
	layout, err := parseLayout(layoutJSON)
	if err != nil {
		return nil, err
	}
	fields := emailedit.EditableFields{}
	if len(fieldsJSON) > 0 {
		if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
			return nil, err
		}
	}
	return checkEmail(ctx, db, nil, layout, fields, subject, preheader)
}

func issueMessages(issues []emailblocks.Issue) string {
	messages := make([]string, len(issues))
	for i, issue := range issues {
		messages[i] = issue.Message
	}
	return strings.Join(messages, "; ")
}
