package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

const maxSharedTextLength = 5000

// SharedText is a text many emails share, one per key and language: footer,
// privacy link, logo, signature. Emails point at it from a field with the
// same key marked "shared". A board can have its own version of a text (a
// regional footer); its emails take that one instead.
type SharedText struct {
	Key      string `json:"key"`
	Language string `json:"language"`
	// Board is the key of the board this version is for, "" for all boards.
	Board     string    `json:"board"`
	Type      string    `json:"type"`
	Value     string    `json:"value"`
	Version   int       `json:"version"`
	ENVersion *int      `json:"en_version"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`

	// Emails in this language (on this board, or on boards without their
	// own version) that point at the text, and how many of them are live
	// (sent by the portal or marked live).
	LinkedEmails int `json:"linked_emails"`
	LinkedLive   int `json:"linked_live"`
	// Emails in this language with the same field that keep their own,
	// different text.
	OwnEmails int `json:"own_emails"`
	// ENChanged: the EN text changed after this translation was last saved
	// or confirmed. A board's translation follows the board's EN text if
	// there is one.
	ENChanged bool `json:"en_changed"`
}

type sharedValue struct {
	Type  string
	Value string
}

// loadSharedValues returns the shared texts an email of one language on board
// uses, by key: the board's own version where there is one.
func loadSharedValues(ctx context.Context, db emailEventExecutor, language, board string) (map[string]sharedValue, error) {
	rows, err := db.Query(ctx, `
		SELECT key, type, value FROM shared_texts
		WHERE language = $1 AND board_key IN ('', $2)
		ORDER BY board_key <> '';
	`, strings.ToLower(language), board)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]sharedValue{}
	for rows.Next() {
		var key string
		var value sharedValue
		if err := rows.Scan(&key, &value.Type, &value.Value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

func hasSharedField(fields emailedit.EditableFields) bool {
	for _, field := range fields {
		if field.Shared {
			return true
		}
	}
	return false
}

// resolveSharedFields gives every shared field the current value of its
// shared text in language on board. A field with no shared text (or of
// another type) stops being shared and keeps its value.
func resolveSharedFields(ctx context.Context, db emailEventExecutor, language, board string, fields emailedit.EditableFields) (emailedit.EditableFields, error) {
	if !hasSharedField(fields) {
		return fields, nil
	}
	values, err := loadSharedValues(ctx, db, language, board)
	if err != nil {
		return nil, err
	}
	return applySharedValues(fields, values), nil
}

func applySharedValues(fields emailedit.EditableFields, values map[string]sharedValue) emailedit.EditableFields {
	resolved := make(emailedit.EditableFields, len(fields))
	for key, field := range fields {
		if field.Shared {
			if shared, ok := values[key]; ok && shared.Type == field.Type {
				field.Value = shared.Value
			} else {
				field.Shared = false
			}
		}
		resolved[key] = field
	}
	return resolved
}

// linkSharedFields sets the shared mark from the values without changing any
// of them: a shared field whose value differs from the shared text stops
// being shared, and with autoLink a field equal to it becomes shared.
func linkSharedFields(ctx context.Context, db emailEventExecutor, language, board string, fields emailedit.EditableFields, autoLink bool) (emailedit.EditableFields, error) {
	if !autoLink && !hasSharedField(fields) {
		return fields, nil
	}
	values, err := loadSharedValues(ctx, db, language, board)
	if err != nil {
		return nil, err
	}
	return markSharedFields(fields, values, autoLink), nil
}

func markSharedFields(fields emailedit.EditableFields, values map[string]sharedValue, autoLink bool) emailedit.EditableFields {
	linked := make(emailedit.EditableFields, len(fields))
	for key, field := range fields {
		shared, ok := values[key]
		matches := ok && shared.Type == field.Type && fmt.Sprint(field.Value) == shared.Value
		if field.Shared && !matches {
			field.Shared = false
		}
		if !field.Shared && matches && autoLink {
			field.Shared = true
		}
		linked[key] = field
	}
	return linked
}

// linkSharedFieldsJSON is linkSharedFields over stored editable_fields JSON.
func linkSharedFieldsJSON(ctx context.Context, db emailEventExecutor, language, board string, fieldsJSON []byte, autoLink bool) ([]byte, error) {
	if len(fieldsJSON) == 0 {
		return fieldsJSON, nil
	}
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return nil, err
	}
	linked, err := linkSharedFields(ctx, db, language, board, fields, autoLink)
	if err != nil {
		return nil, err
	}
	return linked.JSON()
}

// sharedFieldKeys lists the fields of stored editable_fields JSON that are
// shared.
func sharedFieldKeys(fieldsJSON []byte) map[string]bool {
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return nil
	}
	keys := map[string]bool{}
	for key, field := range fields {
		if field.Shared {
			keys[key] = true
		}
	}
	return keys
}

// emailLiveSQL is true for an email the portal saw sent (confirmed or
// matched) or that someone marked live, as on the board.
const emailLiveSQL = `(e.live_marked_at IS NOT NULL OR EXISTS (
	SELECT 1 FROM portal_email_groups g
	WHERE (g.decision IN ('confirmed', 'created') AND g.decision_email_id = e.id)
		OR (g.decision IS NULL AND g.match_status = 'matched' AND g.match_email_id = e.id)
))`

func listSharedTexts(ctx context.Context, db emailEventExecutor) ([]SharedText, error) {
	rows, err := db.Query(ctx, `
		SELECT key, language, board_key, type, value, version, en_version, updated_by_email, updated_at
		FROM shared_texts
		ORDER BY key, language, board_key;
	`)
	if err != nil {
		return nil, err
	}
	texts := []SharedText{}
	byPlace := map[string]int{}
	enVersions := map[string]int{}
	for rows.Next() {
		var text SharedText
		if err := rows.Scan(&text.Key, &text.Language, &text.Board, &text.Type, &text.Value, &text.Version, &text.ENVersion,
			&text.UpdatedBy, &text.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if text.Language == "en" {
			enVersions[text.Key+"\x1f"+text.Board] = text.Version
		}
		byPlace[sharedPlace(text.Key, text.Language, text.Board)] = len(texts)
		texts = append(texts, text)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range texts {
		enVersion, ok := enVersions[texts[i].Key+"\x1f"+texts[i].Board]
		if !ok {
			enVersion, ok = enVersions[texts[i].Key+"\x1f"]
		}
		texts[i].ENChanged = texts[i].Language != "en" && ok &&
			(texts[i].ENVersion == nil || *texts[i].ENVersion < enVersion)
	}

	emailRows, err := db.Query(ctx, `
		SELECT lower(e.language), coalesce(e.sequence, ''), e.editable_fields, `+emailLiveSQL+`
		FROM emails e
		WHERE e.archived_at IS NULL;
	`)
	if err != nil {
		return nil, err
	}
	defer emailRows.Close()
	for emailRows.Next() {
		var language, board string
		var fieldsJSON []byte
		var live bool
		if err := emailRows.Scan(&language, &board, &fieldsJSON, &live); err != nil {
			return nil, err
		}
		var fields emailedit.EditableFields
		if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
			continue
		}
		for key, field := range fields {
			i, ok := byPlace[sharedPlace(key, language, board)]
			if !ok {
				i, ok = byPlace[sharedPlace(key, language, "")]
			}
			if !ok || field.Type != texts[i].Type {
				continue
			}
			switch {
			case field.Shared:
				texts[i].LinkedEmails++
				if live {
					texts[i].LinkedLive++
				}
			case fmt.Sprint(field.Value) != texts[i].Value:
				texts[i].OwnEmails++
			}
		}
	}
	return texts, emailRows.Err()
}

func sharedPlace(key, language, board string) string {
	return key + "\x1f" + language + "\x1f" + board
}

func findSharedText(ctx context.Context, db emailEventExecutor, key, language, board string) (SharedText, error) {
	texts, err := listSharedTexts(ctx, db)
	if err != nil {
		return SharedText{}, err
	}
	for _, text := range texts {
		if text.Key == key && text.Language == language && text.Board == board {
			return text, nil
		}
	}
	return SharedText{}, pgx.ErrNoRows
}

// canEditSharedText: the texts for all boards are for super admins, a
// board's own version also for the people who manage the board.
func canEditSharedText(ctx context.Context, db emailEventExecutor, user AuthUser, board string) (bool, error) {
	if auth.IsSuperAdmin(user) {
		return true, nil
	}
	if board == "" {
		return false, nil
	}
	return auth.CanManageBoard(ctx, db, user, board)
}

func listSharedTextsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		texts, err := listSharedTexts(r.Context(), dbpool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load shared texts: %v\n", err)
			http.Error(w, "failed to load shared texts", http.StatusInternalServerError)
			return
		}
		visible, scoped, err := auth.ScopedBoards(r.Context(), dbpool, user)
		if err != nil {
			http.Error(w, "failed to load shared texts", http.StatusInternalServerError)
			return
		}
		if scoped {
			// The common texts, and the own versions of the user's boards.
			texts = slices.DeleteFunc(texts, func(text SharedText) bool {
				return text.Board != "" && !slices.Contains(visible, text.Board)
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"texts": texts})
	}
}

type updateSharedTextRequest struct {
	Value string `json:"value"`
	// Version is the version the editor started from; a newer one means
	// someone else saved in between.
	Version int `json:"version"`
}

type updateSharedTextResponse struct {
	Text          SharedText `json:"text"`
	UpdatedEmails int        `json:"updated_emails"`
	LiveEmails    int        `json:"live_emails"`
}

// sharedTextChange is one change to a shared text inside a transaction.
type sharedTextChange func(ctx context.Context, tx pgx.Tx, user AuthUser, key, language, board string) (updateSharedTextResponse, error)

// sharedTextHandler runs change for the text in the URL; ?board= picks a
// board's own version.
func sharedTextHandler(dbpool *pgxpool.Pool, change sharedTextChange) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		board := strings.TrimSpace(r.URL.Query().Get("board"))
		allowed, err := canEditSharedText(r.Context(), dbpool, user, board)
		if err != nil {
			http.Error(w, "failed to check permissions", http.StatusInternalServerError)
			return
		}
		if !allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to save the shared text", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		response, err := change(r.Context(), tx, user,
			chi.URLParam(r, "key"), strings.ToLower(chi.URLParam(r, "language")), board)
		var contentErr emailContentError
		if errors.As(err, &contentErr) {
			http.Error(w, contentErr.message, contentErr.status)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to save shared text: %v\n", err)
			http.Error(w, "failed to save the shared text", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to save the shared text", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

func decodeSharedTextRequest(r *http.Request) (updateSharedTextRequest, error) {
	var request updateSharedTextRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return request, emailContentError{http.StatusBadRequest, "invalid request body"}
	}
	return request, nil
}

// updateSharedTextHandler saves a shared text and writes a new version of
// every active email that points at it. Saving a translation with the same
// value confirms it is still right after an EN change.
func updateSharedTextHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, err := decodeSharedTextRequest(r)
		sharedTextHandler(dbpool, func(ctx context.Context, tx pgx.Tx, user AuthUser, key, language, board string) (updateSharedTextResponse, error) {
			if err != nil {
				return updateSharedTextResponse{}, err
			}
			return updateSharedText(ctx, tx, user, key, language, board, request)
		})(w, r)
	}
}

// createBoardSharedTextHandler gives a board its own version of a shared
// text; the board's emails that point at the text switch to it.
func createBoardSharedTextHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, err := decodeSharedTextRequest(r)
		sharedTextHandler(dbpool, func(ctx context.Context, tx pgx.Tx, user AuthUser, key, language, board string) (updateSharedTextResponse, error) {
			if err != nil {
				return updateSharedTextResponse{}, err
			}
			return createBoardSharedText(ctx, tx, user, key, language, board, request.Value)
		})(w, r)
	}
}

// deleteBoardSharedTextHandler removes a board's own version; its emails go
// back to the text for all boards.
func deleteBoardSharedTextHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return sharedTextHandler(dbpool, deleteBoardSharedText)
}

func updateSharedText(ctx context.Context, tx pgx.Tx, user AuthUser, key, language, board string, request updateSharedTextRequest) (updateSharedTextResponse, error) {
	var current SharedText
	err := tx.QueryRow(ctx, `
		SELECT key, language, type, value, version FROM shared_texts
		WHERE key = $1 AND language = $2 AND board_key = $3
		FOR UPDATE;
	`, key, language, board).Scan(&current.Key, &current.Language, &current.Type, &current.Value, &current.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return updateSharedTextResponse{}, emailContentError{http.StatusNotFound, "shared text not found"}
	}
	if err != nil {
		return updateSharedTextResponse{}, err
	}
	if request.Version != current.Version {
		return updateSharedTextResponse{}, emailContentError{http.StatusConflict,
			"someone changed this shared text in the meantime, reload and try again"}
	}
	value, err := normalizeSharedValue(current.Type, request.Value)
	if err != nil {
		return updateSharedTextResponse{}, emailContentError{http.StatusBadRequest, err.Error()}
	}
	enVersion, err := sharedENVersion(ctx, tx, key, language, board)
	if err != nil {
		return updateSharedTextResponse{}, err
	}

	changed := value != current.Value
	if _, err := tx.Exec(ctx, `
		UPDATE shared_texts
		SET value = $4,
			version = version + CASE WHEN $5 THEN 1 ELSE 0 END,
			en_version = $6,
			updated_by_email = $7,
			updated_at = now()
		WHERE key = $1 AND language = $2 AND board_key = $3;
	`, key, language, board, value, changed, enVersion, user.Email); err != nil {
		return updateSharedTextResponse{}, err
	}

	response := updateSharedTextResponse{}
	if changed {
		response.UpdatedEmails, response.LiveEmails, err = propagateSharedText(ctx, tx, user, key, language, board,
			map[string]any{"version": current.Version + 1})
		if err != nil {
			return updateSharedTextResponse{}, err
		}
	}
	response.Text, err = findSharedText(ctx, tx, key, language, board)
	return response, err
}

func createBoardSharedText(ctx context.Context, tx pgx.Tx, user AuthUser, key, language, board, value string) (updateSharedTextResponse, error) {
	if board == "" {
		return updateSharedTextResponse{}, emailContentError{http.StatusBadRequest, "board is required"}
	}
	var boardExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM boards WHERE key = $1);`, board).Scan(&boardExists); err != nil {
		return updateSharedTextResponse{}, err
	}
	if !boardExists {
		return updateSharedTextResponse{}, emailContentError{http.StatusNotFound, "board not found"}
	}
	var fieldType string
	err := tx.QueryRow(ctx, `
		SELECT type FROM shared_texts WHERE key = $1 AND language = $2 AND board_key = '';
	`, key, language).Scan(&fieldType)
	if errors.Is(err, pgx.ErrNoRows) {
		return updateSharedTextResponse{}, emailContentError{http.StatusNotFound, "shared text not found"}
	}
	if err != nil {
		return updateSharedTextResponse{}, err
	}
	value, err = normalizeSharedValue(fieldType, value)
	if err != nil {
		return updateSharedTextResponse{}, emailContentError{http.StatusBadRequest, err.Error()}
	}
	enVersion, err := sharedENVersion(ctx, tx, key, language, board)
	if err != nil {
		return updateSharedTextResponse{}, err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO shared_texts (key, language, board_key, type, value, en_version, updated_by_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT DO NOTHING;
	`, key, language, board, fieldType, value, enVersion, user.Email)
	if err != nil {
		return updateSharedTextResponse{}, err
	}
	if tag.RowsAffected() == 0 {
		return updateSharedTextResponse{}, emailContentError{http.StatusConflict,
			"this board already has its own version of the text, reload and try again"}
	}

	response := updateSharedTextResponse{}
	response.UpdatedEmails, response.LiveEmails, err = propagateSharedText(ctx, tx, user, key, language, board,
		map[string]any{"version": 1, "created": true})
	if err != nil {
		return updateSharedTextResponse{}, err
	}
	response.Text, err = findSharedText(ctx, tx, key, language, board)
	return response, err
}

func deleteBoardSharedText(ctx context.Context, tx pgx.Tx, user AuthUser, key, language, board string) (updateSharedTextResponse, error) {
	if board == "" {
		return updateSharedTextResponse{}, emailContentError{http.StatusBadRequest, "the text for all boards cannot be removed"}
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM shared_texts WHERE key = $1 AND language = $2 AND board_key = $3;
	`, key, language, board)
	if err != nil {
		return updateSharedTextResponse{}, err
	}
	if tag.RowsAffected() == 0 {
		return updateSharedTextResponse{}, emailContentError{http.StatusNotFound, "shared text not found"}
	}

	response := updateSharedTextResponse{}
	response.UpdatedEmails, response.LiveEmails, err = propagateSharedText(ctx, tx, user, key, language, board,
		map[string]any{"removed": true})
	if err != nil {
		return updateSharedTextResponse{}, err
	}
	// The board's emails now use the text for all boards.
	response.Text, err = findSharedText(ctx, tx, key, language, "")
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return response, err
}

// sharedENVersion is the version of the EN text a translation on board is
// checked against: the board's own EN version if there is one, else the one
// for all boards. It is nil for EN itself.
func sharedENVersion(ctx context.Context, tx pgx.Tx, key, language, board string) (*int, error) {
	if language == "en" {
		return nil, nil
	}
	var version *int
	err := tx.QueryRow(ctx, `
		SELECT version FROM shared_texts
		WHERE key = $1 AND language = 'en' AND board_key IN ('', $2)
		ORDER BY board_key <> '' DESC
		LIMIT 1;
	`, key, board).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return version, err
}

// propagateSharedText writes a new version of every active email in
// language whose field key points at the shared text: the emails of board,
// or for the text for all boards the emails of boards without their own
// version. metadata goes into each version's event.
func propagateSharedText(ctx context.Context, tx pgx.Tx, user AuthUser, key, language, board string, metadata map[string]any) (int, int, error) {
	rows, err := tx.Query(ctx, `
		SELECT e.id, e.title, `+emailLiveSQL+`
		FROM emails e
		WHERE e.archived_at IS NULL
			AND lower(e.language) = $1
			AND e.editable_fields -> $2 ->> 'shared' = 'true'
			AND CASE WHEN $3 = '' THEN NOT EXISTS (
				SELECT 1 FROM shared_texts o
				WHERE o.key = $2 AND o.language = $1 AND o.board_key <> '' AND o.board_key = e.sequence
			) ELSE e.sequence = $3 END
		ORDER BY e.id;
	`, language, key, board)
	if err != nil {
		return 0, 0, err
	}
	type linkedEmail struct {
		id, title string
		live      bool
	}
	linked := []linkedEmail{}
	for rows.Next() {
		var email linkedEmail
		if err := rows.Scan(&email.id, &email.title, &email.live); err != nil {
			rows.Close()
			return 0, 0, err
		}
		linked = append(linked, email)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	sharedText := map[string]any{"key": key, "language": language}
	if board != "" {
		sharedText["board"] = board
	}
	for name, value := range metadata {
		sharedText[name] = value
	}
	updated, live := 0, 0
	for _, email := range linked {
		var fieldsJSON []byte
		if err := tx.QueryRow(ctx, `SELECT editable_fields FROM emails WHERE id = $1;`, email.id).Scan(&fieldsJSON); err != nil {
			return 0, 0, err
		}
		var fields emailedit.EditableFields
		if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
			return 0, 0, err
		}
		// saveEmailContent takes the new value from the shared text.
		changed, err := saveEmailContent(ctx, tx, user, email.id, emailContentUpdate{EditableFields: fields}, emailContentSaveOptions{
			Source:     "shared_text",
			KeepStatus: true,
			Metadata:   map[string]any{"shared_text": sharedText},
		})
		var contentErr emailContentError
		if errors.As(err, &contentErr) {
			return 0, 0, emailContentError{contentErr.status, fmt.Sprintf("%s: %s", email.title, contentErr.message)}
		}
		if err != nil {
			return 0, 0, err
		}
		if changed {
			updated++
			if email.live {
				live++
			}
		}
	}
	return updated, live, nil
}

func normalizeSharedValue(fieldType, value string) (string, error) {
	if fieldType == emailedit.FieldTypeText {
		value = strings.TrimRight(strings.ReplaceAll(value, "\r\n", "\n"), " \n\t")
	} else {
		value = strings.TrimSpace(value)
	}
	if strings.TrimSpace(value) == "" {
		return "", errors.New("the text is empty")
	}
	if len(value) > maxSharedTextLength {
		return "", fmt.Errorf("the text is longer than %d characters", maxSharedTextLength)
	}
	if err := emailedit.ValidateFieldValue(emailedit.EditableField{Type: fieldType, Value: value}); err != nil {
		return "", errors.New("not a valid link: use http, https or a Bitrix24 field")
	}
	return value, nil
}
