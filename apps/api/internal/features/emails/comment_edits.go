package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// A comment edit is an open comment whose body is the text to put in place
// of the text it selects, the way reviewers suggested wording on the board.
// Only an exact, single match in the field the comment is on is applied;
// everything else is left open for a person, with the reason.
const (
	CommentEditApply     = "apply"
	CommentEditAmbiguous = "ambiguous" // the selected text occurs more than once
	CommentEditConflict  = "conflict"  // another edit changes the same text
	CommentEditStale     = "stale"     // the selected text is no longer there
	CommentEditImage     = "image"     // text on an image: it needs a new image
	CommentEditShared    = "shared"    // the field takes its value from a shared text
	CommentEditNotText   = "not_text"  // a link or a fragment, not replacement text
	CommentEditQuestion  = "question"  // reads as a request or a question
	CommentEditElsewhere = "elsewhere" // the text is in another field than the comment's
)

// Subject and preheader are columns of the email, not editable fields; the
// plan uses these keys for them.
const (
	subjectFieldKey   = "subject"
	preheaderFieldKey = "preheader"
)

type CommentEditInput struct {
	ID          string
	ReviewBlock string
	Selected    string
	Body        string
	CreatedAt   time.Time
}

type CommentEdit struct {
	CommentID string `json:"comment_id"`
	Status    string `json:"status"`
	// Field is the field the edit changes, or the one the comment is on.
	Field    string `json:"field"`
	Selected string `json:"selected"`
	Body     string `json:"body"`
	// Before and After are the field's whole value around the edit.
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	// Found lists the fields the selected text is in, when it is not in the
	// comment's own field.
	Found []string `json:"found,omitempty"`
	start int
}

// commentEditTexts are an email's texts a comment can be on, by field key.
type commentEditTexts struct {
	Fields    emailedit.EditableFields
	Subject   string
	Preheader string
}

func (t commentEditTexts) value(key string) (string, bool) {
	switch key {
	case subjectFieldKey:
		return t.Subject, true
	case preheaderFieldKey:
		return t.Preheader, true
	}
	field, ok := t.Fields[key]
	if !ok || field.Type != emailedit.FieldTypeText {
		return "", false
	}
	value, ok := field.Value.(string)
	return value, ok
}

// keys lists every text, subject and preheader first, then the fields in
// order.
func (t commentEditTexts) keys() []string {
	keys := []string{subjectFieldKey, preheaderFieldKey}
	var fields []string
	for key, field := range t.Fields {
		if field.Type == emailedit.FieldTypeText {
			fields = append(fields, key)
		}
	}
	sort.Slice(fields, func(i, j int) bool {
		a, b := t.Fields[fields[i]], t.Fields[fields[j]]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return fields[i] < fields[j]
	})
	return append(keys, fields...)
}

// commentField is the field a comment's review block stands for: the block
// is named after the field, with "_text" or "_alt" for a link or an image.
func commentField(block string, texts commentEditTexts) (string, bool) {
	for _, key := range []string{block, block + "_text", block + "_alt"} {
		if _, ok := texts.value(key); ok {
			return key, true
		}
	}
	return "", false
}

var (
	linkOnly = regexp.MustCompile(`^https?://\S+$`)
	// Words a reviewer writes when asking for a change rather than giving
	// the new text.
	requestWords = regexp.MustCompile(`(?i)\b(please|bitte|por favor|proszę|change|replace|delete|remove|translate|why)\b`)
	// A note to the editor in front of the new text ("Надо: Hallo,"); the
	// text that follows it is the replacement.
	editorNote = regexp.MustCompile(`(?i)^(надо|нужно|should be)\s*:\s*`)
	// Words in Cyrillic left after the note are an instruction to the
	// editor: the emails are not in Russian.
	cyrillic = regexp.MustCompile(`\p{Cyrillic}`)
)

// planCommentEdit decides what one comment can do to the email's texts on
// its own; planEmailCommentEdits then checks the edits of an email against
// each other.
func planCommentEdit(comment CommentEditInput, texts commentEditTexts) CommentEdit {
	body := editorNote.ReplaceAllString(strings.TrimSpace(comment.Body), "")
	edit := CommentEdit{CommentID: comment.ID, Selected: comment.Selected, Body: strings.TrimSpace(body), start: -1}
	field, ok := commentField(comment.ReviewBlock, texts)
	if ok {
		edit.Field = field
	} else {
		edit.Field = comment.ReviewBlock
	}

	switch {
	case linkOnly.MatchString(edit.Body) || len([]rune(edit.Body)) < 3:
		edit.Status = CommentEditNotText
		return edit
	case cyrillic.MatchString(edit.Body) && !cyrillic.MatchString(comment.Selected),
		requestWords.MatchString(edit.Body) && !requestWords.MatchString(comment.Selected),
		strings.Contains(edit.Body, "?") && !strings.Contains(comment.Selected, "?"):
		edit.Status = CommentEditQuestion
		return edit
	}

	value, _ := texts.value(edit.Field)
	switch count := strings.Count(value, comment.Selected); {
	case !ok || count == 0:
		for _, key := range texts.keys() {
			if other, _ := texts.value(key); comment.Selected != "" && strings.Contains(other, comment.Selected) {
				edit.Found = append(edit.Found, key)
			}
		}
		if len(edit.Found) > 0 {
			edit.Status = CommentEditElsewhere
		} else {
			edit.Status = CommentEditStale
		}
		return edit
	case count > 1:
		edit.Status = CommentEditAmbiguous
		return edit
	}

	if strings.HasSuffix(edit.Field, "_alt") {
		// The alt text of a banner: the words people read are drawn on the
		// image, so changing the alt alone would not change them.
		edit.Status = CommentEditImage
		return edit
	}
	if texts.Fields[edit.Field].Shared {
		edit.Status = CommentEditShared
		return edit
	}
	edit.Status = CommentEditApply
	edit.start = strings.Index(value, comment.Selected)
	return edit
}

// planEmailCommentEdits plans every open comment of one email. Edits that
// change overlapping text of the same field are all held back; the others
// in a field are applied together, so Before and After show the field with
// all of them.
func planEmailCommentEdits(comments []CommentEditInput, texts commentEditTexts) []CommentEdit {
	edits := make([]CommentEdit, len(comments))
	byField := map[string][]int{}
	for i, comment := range comments {
		edits[i] = planCommentEdit(comment, texts)
		if edits[i].Status == CommentEditApply {
			byField[edits[i].Field] = append(byField[edits[i].Field], i)
		}
	}
	for field, indexes := range byField {
		sort.Slice(indexes, func(a, b int) bool { return edits[indexes[a]].start < edits[indexes[b]].start })
		for n := 1; n < len(indexes); n++ {
			prev, cur := &edits[indexes[n-1]], &edits[indexes[n]]
			if cur.start < prev.start+len(prev.Selected) {
				prev.Status, cur.Status = CommentEditConflict, CommentEditConflict
			}
		}
		before, _ := texts.value(field)
		after := before
		// From the end, so earlier positions stay valid.
		for n := len(indexes) - 1; n >= 0; n-- {
			edit := edits[indexes[n]]
			if edit.Status != CommentEditApply {
				continue
			}
			after = after[:edit.start] + edit.Body + after[edit.start+len(edit.Selected):]
		}
		for _, i := range indexes {
			edits[i].Before = before
			if edits[i].Status == CommentEditApply {
				edits[i].After = after
			}
		}
	}
	return edits
}

// CommentEditEmail is one email with the edits its open comments make.
type CommentEditEmail struct {
	ID       string        `json:"id"`
	Place    string        `json:"place"`
	Language string        `json:"language"`
	Title    string        `json:"title"`
	Ready    bool          `json:"ready"`
	Live     bool          `json:"live"`
	Edits    []CommentEdit `json:"edits"`
	Changed  bool          `json:"changed"`
	// Error is why the email could not be saved; its comments stay open.
	Error string `json:"error,omitempty"`
}

// CommentEditsOptions narrows the emails the edits are planned for.
type CommentEditsOptions struct {
	Board      string
	SkipBoards []string
	// NoLock plans without locking the emails, for a read-only look; the
	// plan may then be out of date by the time anything is applied.
	NoLock bool
}

// PlanCommentEdits plans the edits of every open comment on the active
// emails, oldest comment first in each email. It locks the emails (unless
// NoLock), so the plan stays what ApplyCommentEdits writes in the same
// transaction.
func PlanCommentEdits(ctx context.Context, tx pgx.Tx, options CommentEditsOptions) ([]CommentEditEmail, error) {
	lock := "FOR UPDATE OF e"
	if options.NoLock {
		lock = ""
	}
	rows, err := tx.Query(ctx, `
		SELECT e.id, coalesce(e.sequence, ''), e.stage, e.sort_order, lower(e.language), coalesce(e.variant, ''), e.title,
			e.review_status = 'ready', `+emailLiveSQL+`, coalesce(e.subject, ''), coalesce(e.preheader, ''), e.editable_fields,
			c.id, c.review_block, c.selected_text, c.body, c.created_at
		FROM emails e
		JOIN comments c ON c.email_id = e.id AND c.status = 'open'
		WHERE e.archived_at IS NULL
			AND ($1 = '' OR e.sequence = $1)
			AND NOT coalesce(e.sequence, '') = ANY($2)
		ORDER BY e.sequence, e.stage, e.sort_order, e.language, e.variant, e.id, c.created_at, c.id
		`+lock+`;
	`, options.Board, append([]string{}, options.SkipBoards...))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CommentEditEmail
	var texts []commentEditTexts
	var comments [][]CommentEditInput
	for rows.Next() {
		var email CommentEditEmail
		var board, stage, variant, subject, preheader string
		var sortOrder int
		var fieldsJSON []byte
		var comment CommentEditInput
		if err := rows.Scan(&email.ID, &board, &stage, &sortOrder, &email.Language, &variant, &email.Title,
			&email.Ready, &email.Live, &subject, &preheader, &fieldsJSON,
			&comment.ID, &comment.ReviewBlock, &comment.Selected, &comment.Body, &comment.CreatedAt); err != nil {
			return nil, err
		}
		if n := len(result); n == 0 || result[n-1].ID != email.ID {
			var fields emailedit.EditableFields
			if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
				return nil, fmt.Errorf("email %s editable fields: %w", email.ID, err)
			}
			email.Place = fmt.Sprintf("%s · %s #%d · %s", board, stage, sortOrder, strings.ToUpper(email.Language))
			if variant != "" && variant != "new" {
				email.Place += " · " + variant
			}
			result = append(result, email)
			texts = append(texts, commentEditTexts{Fields: fields, Subject: subject, Preheader: preheader})
			comments = append(comments, nil)
		}
		comments[len(comments)-1] = append(comments[len(comments)-1], comment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range result {
		result[i].Edits = planEmailCommentEdits(comments[i], texts[i])
	}
	return result, nil
}

// ApplyCommentEdits writes the planned edits of each email as one new
// version, then resolves the applied comments with a reply saying what
// changed. Comments held back stay open. It marks Changed on the emails it
// saved; an email the save refuses (its fields no longer fit its template,
// say) gets Error and is left as it was, and the others go on.
func ApplyCommentEdits(ctx context.Context, tx pgx.Tx, actor AuthUser, emails []CommentEditEmail) error {
	for i := range emails {
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		changed, err := applyEmailCommentEdits(ctx, savepoint, actor, emails[i])
		var contentErr emailContentError
		switch {
		case errors.As(err, &contentErr):
			emails[i].Error = contentErr.message
			if err := savepoint.Rollback(ctx); err != nil {
				return err
			}
		case err != nil:
			return fmt.Errorf("%s: %w", emails[i].Place, err)
		default:
			emails[i].Changed = changed
			if err := savepoint.Commit(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyEmailCommentEdits(ctx context.Context, tx pgx.Tx, actor AuthUser, email CommentEditEmail) (bool, error) {
	update := emailContentUpdate{}
	var applied []CommentEdit
	for _, edit := range email.Edits {
		if edit.Status != CommentEditApply {
			continue
		}
		applied = append(applied, edit)
		after := edit.After
		switch edit.Field {
		case subjectFieldKey:
			update.Subject = &after
		case preheaderFieldKey:
			update.Preheader = &after
		default:
			if update.EditableFields == nil {
				var fieldsJSON []byte
				if err := tx.QueryRow(ctx, `SELECT editable_fields FROM emails WHERE id = $1;`, email.ID).Scan(&fieldsJSON); err != nil {
					return false, err
				}
				if err := json.Unmarshal(fieldsJSON, &update.EditableFields); err != nil {
					return false, err
				}
			}
			field := update.EditableFields[edit.Field]
			field.Value = after
			update.EditableFields[edit.Field] = field
		}
	}
	if len(applied) == 0 {
		return false, nil
	}
	changed, err := saveEmailContent(ctx, tx, actor, email.ID, update, emailContentSaveOptions{
		Source:   "manual",
		Metadata: map[string]any{"applied_comments": len(applied)},
	})
	if err != nil {
		return false, err
	}
	for _, edit := range applied {
		if _, err := tx.Exec(ctx, `
			UPDATE comments SET status = 'resolved', resolved_at = now(), resolved_by = $2 WHERE id = $1;
		`, edit.CommentID, actor.ID); err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO comment_messages (comment_id, user_id, body) VALUES ($1, $2, $3);
		`, edit.CommentID, actor.ID, fmt.Sprintf("Applied: %q → %q", edit.Selected, edit.Body)); err != nil {
			return false, err
		}
	}
	return changed, nil
}
