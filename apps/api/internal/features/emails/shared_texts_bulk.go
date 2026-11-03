package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// SharedTextValue is a new value for the shared text of one key and language
// for all boards.
type SharedTextValue struct {
	Key      string `json:"key"`
	Language string `json:"language"`
	Value    string `json:"value"`
}

// SharedTextsChange is one pass over the shared texts, for a command: drop
// some texts, give others new values, and link the fields that keep their own
// text to the shared one.
type SharedTextsChange struct {
	// Drop removes the shared texts with these keys, in every language and
	// board. Their fields stop being shared and keep their values.
	Drop []string
	// Set gives texts for all boards new values.
	Set []SharedTextValue
	// Relink makes every field with a shared text shared, so it takes the
	// shared value, except on SkipBoards.
	Relink     bool
	SkipBoards []string
}

// SharedTextsEmail is an email whose content the change touches.
type SharedTextsEmail struct {
	ID    string
	Place string
	Live  bool
	// Relinked are the fields that kept their own text and now take the
	// shared one.
	Relinked []string
	// Changed: the email got a new version (only after apply).
	Changed bool
}

type SharedTextsResult struct {
	Dropped  int // shared texts removed
	Unlinked int // fields that stopped being shared
	Set      int // shared texts whose value changed
	Emails   []SharedTextsEmail
}

// ApplySharedTextsChange runs change inside tx. Every email it touches is
// saved once, with all its shared fields resolved, and keeps its status: a
// ready email stays ready and waits for a re-upload.
func ApplySharedTextsChange(ctx context.Context, tx pgx.Tx, actor AuthUser, change SharedTextsChange) (SharedTextsResult, error) {
	var result SharedTextsResult
	if len(change.Drop) > 0 {
		tag, err := tx.Exec(ctx, `
			UPDATE emails e
			SET editable_fields = (
				SELECT jsonb_object_agg(f.key, CASE WHEN f.key = ANY($1) THEN f.value - 'shared' ELSE f.value END)
				FROM jsonb_each(e.editable_fields) f
			)
			WHERE EXISTS (
				SELECT 1 FROM jsonb_each(e.editable_fields) f
				WHERE f.key = ANY($1) AND f.value ->> 'shared' = 'true'
			);
		`, change.Drop)
		if err != nil {
			return result, fmt.Errorf("unlink dropped texts: %w", err)
		}
		result.Unlinked = int(tag.RowsAffected())
		tag, err = tx.Exec(ctx, `DELETE FROM shared_texts WHERE key = ANY($1);`, change.Drop)
		if err != nil {
			return result, fmt.Errorf("drop shared texts: %w", err)
		}
		result.Dropped = int(tag.RowsAffected())
	}

	// EN first: a translation saved after it is current, not "EN changed".
	set := slices.Clone(change.Set)
	slices.SortStableFunc(set, func(a, b SharedTextValue) int {
		return boolInt(strings.ToLower(a.Language) != slotMasterLanguage) - boolInt(strings.ToLower(b.Language) != slotMasterLanguage)
	})
	for _, text := range set {
		changed, err := setSharedTextValue(ctx, tx, actor, text)
		if err != nil {
			return result, fmt.Errorf("set %s %s: %w", text.Key, text.Language, err)
		}
		if changed {
			result.Set++
		}
	}

	emails, err := loadSharedTextsEmails(ctx, tx, change)
	if err != nil {
		return result, err
	}
	for i, email := range emails {
		changed, err := saveEmailContent(ctx, tx, actor, email.ID, emailContentUpdate{EditableFields: email.fields}, emailContentSaveOptions{
			Source:     "shared_text",
			KeepStatus: true,
			Metadata:   map[string]any{"shared_texts": "bulk"},
		})
		if err != nil {
			return result, fmt.Errorf("%s: %w", email.Place, err)
		}
		emails[i].Changed = changed
	}
	for _, email := range emails {
		if email.Changed || len(email.Relinked) > 0 {
			result.Emails = append(result.Emails, email.SharedTextsEmail)
		}
	}
	return result, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// setSharedTextValue is updateSharedText without writing the emails: the
// caller saves each email once at the end.
func setSharedTextValue(ctx context.Context, tx pgx.Tx, actor AuthUser, text SharedTextValue) (bool, error) {
	language := strings.ToLower(text.Language)
	var fieldType, current string
	err := tx.QueryRow(ctx, `
		SELECT type, value FROM shared_texts
		WHERE key = $1 AND language = $2 AND board_key = ''
		FOR UPDATE;
	`, text.Key, language).Scan(&fieldType, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, errors.New("no such shared text")
	}
	if err != nil {
		return false, err
	}
	value, err := normalizeSharedValue(fieldType, text.Value)
	if err != nil {
		return false, err
	}
	enVersion, err := sharedENVersion(ctx, tx, text.Key, language, "")
	if err != nil {
		return false, err
	}
	changed := value != current
	_, err = tx.Exec(ctx, `
		UPDATE shared_texts
		SET value = $3,
			version = version + CASE WHEN $4 THEN 1 ELSE 0 END,
			en_version = $5,
			updated_by_email = $6,
			updated_at = now()
		WHERE key = $1 AND language = $2 AND board_key = '';
	`, text.Key, language, value, changed, enVersion, actor.Email)
	return changed, err
}

type sharedTextsEmail struct {
	SharedTextsEmail
	fields emailedit.EditableFields
}

// loadSharedTextsEmails lists the active emails the change touches (a shared
// field with a new value, or with Relink a field to link), with the fields to
// save.
func loadSharedTextsEmails(ctx context.Context, tx pgx.Tx, change SharedTextsChange) ([]sharedTextsEmail, error) {
	rows, err := tx.Query(ctx, `
		SELECT e.id, coalesce(e.sequence, ''), e.stage, e.sort_order, lower(e.language), e.editable_fields, `+emailLiveSQL+`
		FROM emails e
		WHERE e.archived_at IS NULL
		ORDER BY e.sequence, e.stage, e.sort_order, e.language
		FOR UPDATE OF e;
	`)
	if err != nil {
		return nil, err
	}
	type row struct {
		id, board, stage, language string
		sortOrder                  int
		fieldsJSON                 []byte
		live                       bool
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.board, &r.stage, &r.sortOrder, &r.language, &r.fieldsJSON, &r.live); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var emails []sharedTextsEmail
	for _, r := range all {
		var fields emailedit.EditableFields
		if err := json.Unmarshal(r.fieldsJSON, &fields); err != nil {
			return nil, fmt.Errorf("email %s editable fields: %w", r.id, err)
		}
		email := sharedTextsEmail{
			SharedTextsEmail: SharedTextsEmail{
				ID:    r.id,
				Place: fmt.Sprintf("%s · %s #%d · %s", r.board, r.stage, r.sortOrder, strings.ToUpper(r.language)),
				Live:  r.live,
			},
			fields: fields,
		}
		if change.Relink && !slices.Contains(change.SkipBoards, r.board) {
			values, err := loadSharedValues(ctx, tx, r.language, r.board)
			if err != nil {
				return nil, err
			}
			for key, field := range fields {
				if shared, ok := values[key]; ok && !field.Shared && shared.Type == field.Type {
					field.Shared = true
					fields[key] = field
					email.Relinked = append(email.Relinked, key)
				}
			}
			slices.Sort(email.Relinked)
		}
		if len(email.Relinked) > 0 || usesSetText(fields, r.language, change.Set) {
			emails = append(emails, email)
		}
	}
	return emails, nil
}

func usesSetText(fields emailedit.EditableFields, language string, set []SharedTextValue) bool {
	for _, text := range set {
		if field, ok := fields[text.Key]; ok && field.Shared && strings.ToLower(text.Language) == language {
			return true
		}
	}
	return false
}
