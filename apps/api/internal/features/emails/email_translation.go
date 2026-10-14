package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

const emailEventTranslationConfirmed = "email_translation_confirmed"

type translationChange struct {
	Key    string `json:"key"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type translationStatus struct {
	MasterID              string              `json:"master_id"`
	MasterTitle           string              `json:"master_title"`
	MasterLanguage        string              `json:"master_language"`
	TranslatedFromVersion int                 `json:"translated_from_version"`
	MasterLatestVersion   int                 `json:"master_latest_version"`
	Stale                 bool                `json:"stale"`
	Changes               []translationChange `json:"changes"`
	CheckedAt             *time.Time          `json:"checked_at"`
	CheckedBy             *string             `json:"checked_by"`
}

type versionText struct {
	subject, preheader string
	fields             map[string]string
}

// translationHandler shows whether a translation is behind its EN master and
// what changed in the master's text since it was translated.
func translationHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := loadTranslationStatus(r.Context(), dbpool, chi.URLParam(r, "id"))
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent) // not a translation
			return
		}
		if err != nil {
			http.Error(w, "failed to load translation status", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func loadTranslationStatus(ctx context.Context, db *pgxpool.Pool, id string) (translationStatus, error) {
	var status translationStatus
	var fromVersion *int
	err := db.QueryRow(ctx, `
		SELECT m.id, m.title, m.language, t.translated_from_version, t.translation_checked_at, t.translation_checked_by,
			coalesce((SELECT max(version_number) FROM email_versions WHERE email_id = m.id), 0)
		FROM emails t
		JOIN emails m ON m.id = t.translation_of
		WHERE t.id = $1;
	`, id).Scan(&status.MasterID, &status.MasterTitle, &status.MasterLanguage, &fromVersion,
		&status.CheckedAt, &status.CheckedBy, &status.MasterLatestVersion)
	if err != nil {
		return status, err
	}
	if fromVersion != nil {
		status.TranslatedFromVersion = *fromVersion
	}
	status.Changes = []translationChange{}

	before, errBefore := loadVersionText(ctx, db, status.MasterID, status.TranslatedFromVersion)
	after, errAfter := loadVersionText(ctx, db, status.MasterID, status.MasterLatestVersion)
	if errBefore != nil || errAfter != nil {
		// No snapshot to compare with; the board shows such translations as
		// up to date too.
		return status, nil
	}
	status.Changes = diffVersionText(before, after)
	status.Stale = len(status.Changes) > 0
	return status, nil
}

func loadVersionText(ctx context.Context, db *pgxpool.Pool, emailID string, version int) (versionText, error) {
	var subject, preheader *string
	var fieldsJSON []byte
	if err := db.QueryRow(ctx, `
		SELECT subject, preheader, editable_fields FROM email_versions WHERE email_id = $1 AND version_number = $2;
	`, emailID, version).Scan(&subject, &preheader, &fieldsJSON); err != nil {
		return versionText{}, err
	}
	var fields map[string]struct {
		Type  string `json:"type"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return versionText{}, err
	}
	text := versionText{subject: stringFromPointer(subject), preheader: stringFromPointer(preheader), fields: map[string]string{}}
	for key, field := range fields {
		if field.Type == "text" {
			text.fields[key] = fmt.Sprint(field.Value)
		}
	}
	return text, nil
}

// diffVersionText lists what a translator needs to redo: subject, preheader
// and text fields that differ between two master versions.
func diffVersionText(before, after versionText) []translationChange {
	changes := []translationChange{}
	add := func(key, a, b string) {
		// Exact comparison, like email_text_hash in the database, so the
		// board's "outdated" flag and this list agree.
		if a != b {
			changes = append(changes, translationChange{Key: key, Before: a, After: b})
		}
	}
	add("subject", before.subject, after.subject)
	add("preheader", before.preheader, after.preheader)

	keys := map[string]bool{}
	for key := range before.fields {
		keys[key] = true
	}
	for key := range after.fields {
		keys[key] = true
	}
	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)
	for _, key := range sorted {
		add(key, before.fields[key], after.fields[key])
	}
	return changes
}

// confirmTranslationHandler marks a translation as matching the master's
// current text, after someone updated (or checked) it.
func confirmTranslationHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok || !auth.IsAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		id := chi.URLParam(r, "id")

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to update translation", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		var slug, title string
		var version int
		err = tx.QueryRow(r.Context(), `
			UPDATE emails t
			SET translated_from_version = coalesce((SELECT max(version_number) FROM email_versions WHERE email_id = t.translation_of), 0),
				translation_checked_at = now(),
				translation_checked_by = $2
			WHERE t.id = $1 AND t.translation_of IS NOT NULL AND t.archived_at IS NULL
			RETURNING slug, title, translated_from_version;
		`, id, user.Email).Scan(&slug, &title, &version)
		if err != nil {
			http.Error(w, "translation not found", http.StatusNotFound)
			return
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventTranslationConfirmed,
			EmailID:     &id,
			EmailSlug:   &slug,
			EmailTitle:  &title,
			Metadata:    map[string]any{"master_version": version},
		}); err != nil {
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update translation", http.StatusInternalServerError)
			return
		}

		status, err := loadTranslationStatus(r.Context(), dbpool, id)
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

// linkTranslation ties a newly created email to its EN master at the master's
// current version.
func linkTranslation(ctx context.Context, db emailEventExecutor, translationID, masterID string) error {
	_, err := db.Exec(ctx, `
		UPDATE emails
		SET translation_of = $2,
			translated_from_version = coalesce((SELECT max(version_number) FROM email_versions WHERE email_id = $2), 0)
		WHERE id = $1;
	`, translationID, masterID)
	return err
}
