package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// A comment decision is what a person decided for comments the planner held
// back: exact replacements in an email's fields, blocks to take out, and the
// comments it settles. Each decision is one new version of its email.
type CommentDecision struct {
	EmailID string `json:"email_id"`
	// Note says, for the report, what was decided.
	Note         string               `json:"note"`
	Replace      []CommentFieldChange `json:"replace,omitempty"`
	RemoveBlocks []string             `json:"remove_blocks,omitempty"`
	// Resolve are the comments the decision settles; Reply is said on each.
	Resolve []string `json:"resolve"`
	Reply   string   `json:"reply"`
	// Error is why the email could not be saved; nothing of it was written.
	Error string `json:"error,omitempty"`
}

// CommentFieldChange replaces From with To in one text: a text or link field, the subject
// or the preheader. From must occur exactly once, or at least once with All.
type CommentFieldChange struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
	All   bool   `json:"all,omitempty"`
}

// ApplyCommentDecisions writes each decision as one new version of its email
// and resolves its comments with the reply. A decision whose text is not
// where it says, or whose save is refused, gets Error and is left out; the
// others go on.
func ApplyCommentDecisions(ctx context.Context, tx pgx.Tx, actor AuthUser, decisions []CommentDecision) error {
	for i := range decisions {
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		err = applyCommentDecision(ctx, savepoint, actor, decisions[i])
		var contentErr emailContentError
		var decisionErr commentDecisionError
		switch {
		case errors.As(err, &contentErr):
			decisions[i].Error = contentErr.message
		case errors.As(err, &decisionErr):
			decisions[i].Error = decisionErr.Error()
		case err != nil:
			return fmt.Errorf("decision for %s: %w", decisions[i].EmailID, err)
		}
		if decisions[i].Error != "" {
			if err := savepoint.Rollback(ctx); err != nil {
				return err
			}
			continue
		}
		if err := savepoint.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

type commentDecisionError string

func (e commentDecisionError) Error() string { return string(e) }

func applyCommentDecision(ctx context.Context, tx pgx.Tx, actor AuthUser, decision CommentDecision) error {
	var subject, preheader string
	var fieldsJSON, layoutJSON []byte
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(subject, ''), coalesce(preheader, ''), editable_fields, layout
		FROM emails WHERE id = $1 AND archived_at IS NULL FOR UPDATE;
	`, decision.EmailID).Scan(&subject, &preheader, &fieldsJSON, &layoutJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return commentDecisionError("no such active email")
		}
		return err
	}
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return err
	}
	texts := commentEditTexts{Fields: fields, Subject: subject, Preheader: preheader}

	// The save takes the whole set of fields: an email not built from
	// blocks would lose the ones left out.
	update := emailContentUpdate{EditableFields: texts.Fields}
	for _, change := range decision.Replace {
		value, ok := texts.value(change.Field)
		if field, exists := texts.Fields[change.Field]; !ok && exists && field.Type == emailedit.FieldTypeURL {
			// A link a reviewer sent for a button.
			value, ok = field.Value.(string)
		}
		if !ok {
			return commentDecisionError(fmt.Sprintf("%s is not a text of the email", change.Field))
		}
		count := strings.Count(value, change.From)
		if count == 0 || (count > 1 && !change.All) {
			return commentDecisionError(fmt.Sprintf("%q occurs %d times in %s", change.From, count, change.Field))
		}
		value = strings.ReplaceAll(value, change.From, change.To)
		switch change.Field {
		case subjectFieldKey:
			texts.Subject = value
			update.Subject = &texts.Subject
		case preheaderFieldKey:
			texts.Preheader = value
			update.Preheader = &texts.Preheader
		default:
			field := texts.Fields[change.Field]
			field.Value = value
			texts.Fields[change.Field] = field
		}
	}
	if len(decision.RemoveBlocks) > 0 {
		layout, err := parseLayout(layoutJSON)
		if err != nil {
			return err
		}
		if layout == nil {
			return commentDecisionError("the email is not built from blocks")
		}
		for _, id := range decision.RemoveBlocks {
			kept := emailblocks.Layout{}
			for _, item := range layout {
				if item.ID() != id {
					kept = append(kept, item)
				}
			}
			if len(kept) == len(layout) {
				return commentDecisionError(fmt.Sprintf("the email has no block %s", id))
			}
			layout = kept
		}
		update.Layout = layout
	}

	if _, err := saveEmailContent(ctx, tx, actor, decision.EmailID, update, emailContentSaveOptions{
		Source:   "manual",
		Metadata: map[string]any{"comment_decision": decision.Note},
	}); err != nil {
		return err
	}
	for _, id := range decision.Resolve {
		tag, err := tx.Exec(ctx, `
			UPDATE comments SET status = 'resolved', resolved_at = now(), resolved_by = $3
			WHERE id = $1 AND email_id = $2 AND status = 'open';
		`, id, decision.EmailID, actor.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return commentDecisionError(fmt.Sprintf("comment %s is not an open comment of the email", id))
		}
		if _, err := tx.Exec(ctx, `INSERT INTO comment_messages (comment_id, user_id, body) VALUES ($1, $2, $3);`,
			id, actor.ID, decision.Reply); err != nil {
			return err
		}
	}
	return nil
}
