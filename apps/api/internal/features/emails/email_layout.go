package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// parseLayout reads a stored layout; NULL is an email in the old format.
func parseLayout(raw []byte) (emailblocks.Layout, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var layout emailblocks.Layout
	if err := json.Unmarshal(raw, &layout); err != nil {
		return nil, err
	}
	return layout, nil
}

// layoutJSON is the value stored for layout: NULL for the old format.
func layoutJSON(layout emailblocks.Layout) ([]byte, error) {
	if layout == nil {
		return nil, nil
	}
	return json.Marshal(layout)
}

func sameLayout(a, b emailblocks.Layout) bool {
	return (a == nil) == (b == nil) && slices.Equal(a, b)
}

func loadBlockLibrary(ctx context.Context, db emailEventExecutor) ([]emailblocks.Block, error) {
	stored, err := listEmailBlocks(ctx, db)
	if err != nil {
		return nil, err
	}
	library := make([]emailblocks.Block, len(stored))
	for i, block := range stored {
		library[i] = block.Block
	}
	return library, nil
}

// buildFromLayout assembles the template of an email in language built from
// layout and gives it its fields: the values in fields where the key is
// still there, the block's own for new ones.
func buildFromLayout(ctx context.Context, db emailEventExecutor, layout emailblocks.Layout, language string, fields emailedit.EditableFields) (string, emailedit.EditableFields, error) {
	library, err := loadBlockLibrary(ctx, db)
	if err != nil {
		return "", nil, err
	}
	template, err := layout.Build(library)
	if err != nil {
		return "", nil, emailContentError{http.StatusBadRequest, err.Error()}
	}
	template = emailblocks.WithLanguage(template, language)
	built, err := layout.Fields(library, template, fields)
	if err != nil {
		return "", nil, emailContentError{http.StatusBadRequest, err.Error()}
	}
	return template, built, nil
}

type updateEmailLayoutRequest struct {
	Layout emailblocks.Layout `json:"layout"`
}

// updateEmailLayoutHandler sets the blocks an EN email is built from; its
// translations follow. An email in the old format switches to blocks: fields
// with the same key keep their values.
func updateEmailLayoutHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !requireManageEmail(w, r, dbpool, user, id) {
			return
		}
		var request updateEmailLayoutRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Layout == nil {
			http.Error(w, "layout is required", http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to save the blocks", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		err = updateEmailLayout(r.Context(), tx, user, id, request.Layout)
		var contentErr emailContentError
		if errors.As(err, &contentErr) {
			http.Error(w, contentErr.message, contentErr.status)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to save the layout of email %s: %v\n", id, err)
			http.Error(w, "failed to save the blocks", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to save the blocks", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func updateEmailLayout(ctx context.Context, tx pgx.Tx, user AuthUser, id string, layout emailblocks.Layout) error {
	var translationOf *string
	err := tx.QueryRow(ctx, `
		SELECT translation_of FROM emails WHERE id = $1 AND archived_at IS NULL;
	`, id).Scan(&translationOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return emailContentError{http.StatusNotFound, "email not found"}
	}
	if err != nil {
		return err
	}
	if translationOf != nil {
		return emailContentError{http.StatusBadRequest, "a translation takes its blocks from the EN email"}
	}
	if _, err := saveEmailContent(ctx, tx, user, id, emailContentUpdate{Layout: layout}, emailContentSaveOptions{
		Source: "manual",
		Check:  true,
	}); err != nil {
		return err
	}
	return syncTranslationLayouts(ctx, tx, user, id)
}

// syncTranslationLayouts gives the active translations of master its
// layout. A block new to a translation comes with the master's text, so the
// translation shows as outdated with what to translate; removed blocks take
// their fields with them.
func syncTranslationLayouts(ctx context.Context, tx pgx.Tx, user AuthUser, masterID string) error {
	var masterLayoutJSON, masterFieldsJSON []byte
	if err := tx.QueryRow(ctx, `
		SELECT layout, editable_fields FROM emails WHERE id = $1;
	`, masterID).Scan(&masterLayoutJSON, &masterFieldsJSON); err != nil {
		return err
	}
	masterLayout, err := parseLayout(masterLayoutJSON)
	if err != nil || masterLayout == nil {
		return err
	}
	var masterFields emailedit.EditableFields
	if err := json.Unmarshal(masterFieldsJSON, &masterFields); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT id, title, layout, editable_fields FROM emails
		WHERE translation_of = $1 AND archived_at IS NULL
		ORDER BY id;
	`, masterID)
	if err != nil {
		return err
	}
	type translation struct {
		id, title string
		fields    emailedit.EditableFields
	}
	behind := []translation{}
	for rows.Next() {
		var item translation
		var layoutRaw, fieldsRaw []byte
		if err := rows.Scan(&item.id, &item.title, &layoutRaw, &fieldsRaw); err != nil {
			rows.Close()
			return err
		}
		layout, err := parseLayout(layoutRaw)
		if err != nil {
			rows.Close()
			return err
		}
		if sameLayout(layout, masterLayout) {
			continue
		}
		if err := json.Unmarshal(fieldsRaw, &item.fields); err != nil {
			rows.Close()
			return err
		}
		behind = append(behind, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range behind {
		fields := emailedit.EditableFields{}
		for key, field := range masterFields {
			fields[key] = field
		}
		for key, field := range item.fields {
			fields[key] = field
		}
		_, err := saveEmailContent(ctx, tx, user, item.id, emailContentUpdate{Layout: masterLayout, EditableFields: fields},
			emailContentSaveOptions{Source: "manual", Metadata: map[string]any{"layout_from": masterID}})
		var contentErr emailContentError
		if errors.As(err, &contentErr) {
			return emailContentError{contentErr.status, fmt.Sprintf("%s: %s", item.title, contentErr.message)}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// rebuildEmailsWithBlock writes a new version of every active email built
// with the block after the block changed. Their status stays: the block was
// checked on its own screen. It returns how many emails changed and how many
// of them are live.
func rebuildEmailsWithBlock(ctx context.Context, tx pgx.Tx, user AuthUser, key string, version int) (int, int, error) {
	emails, err := emailsWithBlock(ctx, tx, key)
	if err != nil {
		return 0, 0, err
	}
	updated, live := 0, 0
	for _, email := range emails {
		changed, err := saveEmailContent(ctx, tx, user, email.ID, emailContentUpdate{}, emailContentSaveOptions{
			Source:     "block",
			KeepStatus: true,
			Metadata:   map[string]any{"block": map[string]any{"key": key, "version": version}},
		})
		var contentErr emailContentError
		if errors.As(err, &contentErr) {
			return 0, 0, emailContentError{contentErr.status, fmt.Sprintf("%s: %s", email.Title, contentErr.message)}
		}
		if err != nil {
			return 0, 0, err
		}
		if changed {
			updated++
			if email.Live {
				live++
			}
		}
	}
	return updated, live, nil
}

type emailWithBlock struct {
	ID    string
	Title string
	Live  bool
}

// emailsWithBlock lists the active emails built with the block; every email
// built from blocks has the frame.
func emailsWithBlock(ctx context.Context, db emailEventExecutor, key string) ([]emailWithBlock, error) {
	filter, err := json.Marshal([]map[string]string{{"block": key}})
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `
		SELECT e.id, e.title, `+emailLiveSQL+`
		FROM emails e
		WHERE e.archived_at IS NULL
			AND e.layout IS NOT NULL
			AND ($1 = $2 OR e.layout @> $3::jsonb)
		ORDER BY e.id;
	`, key, emailblocks.FrameKey, filter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	emails := []emailWithBlock{}
	for rows.Next() {
		var email emailWithBlock
		if err := rows.Scan(&email.ID, &email.Title, &email.Live); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

// layoutChange is how a layout change goes into an event: the blocks before
// and after (nil for the old format) and what was added or removed.
func layoutChange(before, after emailblocks.Layout) map[string]any {
	keys := func(layout emailblocks.Layout) []string {
		if layout == nil {
			return nil
		}
		return layout.Keys()
	}
	return map[string]any{
		"before": keys(before),
		"after":  keys(after),
		"blocks": emailblocks.Diff(before, after),
	}
}
