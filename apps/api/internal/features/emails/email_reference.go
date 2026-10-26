package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
)

const (
	emailEventBasedOnLinked    = "email_based_on_linked"
	emailEventBasedOnConfirmed = "email_based_on_confirmed"
)

// fieldDifference is a field whose value is not the reference email's.
// Empty on one side when only the other has the field.
type fieldDifference struct {
	Key       string `json:"key"`
	Reference string `json:"reference"`
	Own       string `json:"own"`
}

type timingDifference struct {
	Reference string `json:"reference"`
	Own       string `json:"own"`
}

// basedOnStatus tells a region's email what changed in the reference email
// since it last matched it, and how it differs from the reference now.
type basedOnStatus struct {
	ReferenceID            string `json:"reference_id"`
	ReferenceTitle         string `json:"reference_title"`
	ReferenceLanguage      string `json:"reference_language"`
	ReferenceStage         string `json:"reference_stage"`
	ReferenceSortOrder     int    `json:"reference_sort_order"`
	BasedOnVersion         int    `json:"based_on_version"`
	ReferenceLatestVersion int    `json:"reference_latest_version"`
	// Stale is true when the reference's text changed since; Changes and
	// BlockChanges say how.
	Stale        bool                      `json:"stale"`
	Changes      []translationChange       `json:"changes"`
	BlockChanges []emailblocks.BlockChange `json:"block_changes"`
	CheckedAt    *time.Time                `json:"checked_at"`
	CheckedBy    *string                   `json:"checked_by"`

	// What differs now. Fields only when both are in the same language:
	// another language differs in every text. Blocks: "removed" is only in
	// the reference, "added" only in this email; empty unless both are built
	// from blocks.
	FieldDifferences []fieldDifference         `json:"field_differences"`
	BlockDifferences []emailblocks.BlockChange `json:"block_differences"`
	TimingDifference *timingDifference         `json:"timing_difference"`
}

type basedOnLinkRequest struct {
	// ReferenceID is an email of the board's reference region; null removes
	// the link.
	ReferenceID *string `json:"reference_id"`
}

// basedOnHandler shows how a region's email stands against the reference
// email it adapts; 204 when it adapts none.
func basedOnHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := loadBasedOnStatus(r.Context(), dbpool, chi.URLParam(r, "id"))
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load the reference of %s: %v\n", chi.URLParam(r, "id"), err)
			http.Error(w, "failed to load the reference", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func loadBasedOnStatus(ctx context.Context, db *pgxpool.Pool, id string) (basedOnStatus, error) {
	var status basedOnStatus
	var fromVersion *int
	var own, reference emailNow
	err := db.QueryRow(ctx, `
		SELECT m.id, m.title, m.language, m.stage, m.sort_order, e.based_on_version, e.based_on_checked_at, e.based_on_checked_by,
			coalesce((SELECT max(version_number) FROM email_versions WHERE email_id = m.id), 0),
			e.language, e.subject, e.preheader, e.editable_fields, e.layout, e.send_timing, e.send_condition,
			m.subject, m.preheader, m.editable_fields, m.layout, m.send_timing, m.send_condition
		FROM emails e
		JOIN emails m ON m.id = e.based_on
		WHERE e.id = $1;
	`, id).Scan(&status.ReferenceID, &status.ReferenceTitle, &status.ReferenceLanguage, &status.ReferenceStage,
		&status.ReferenceSortOrder, &fromVersion, &status.CheckedAt, &status.CheckedBy, &status.ReferenceLatestVersion,
		&own.language, &own.subject, &own.preheader, &own.fields, &own.layout, &own.timing, &own.condition,
		&reference.subject, &reference.preheader, &reference.fields, &reference.layout, &reference.timing, &reference.condition)
	if err != nil {
		return status, err
	}
	reference.language = status.ReferenceLanguage
	if fromVersion != nil {
		status.BasedOnVersion = *fromVersion
	}
	status.Changes = []translationChange{}
	status.BlockChanges = []emailblocks.BlockChange{}
	status.FieldDifferences = []fieldDifference{}
	status.BlockDifferences = []emailblocks.BlockChange{}

	// What changed in the reference since, as for a translation.
	before, errBefore := loadVersionText(ctx, db, status.ReferenceID, status.BasedOnVersion)
	after, errAfter := loadVersionText(ctx, db, status.ReferenceID, status.ReferenceLatestVersion)
	if errBefore == nil && errAfter == nil {
		for key := range sharedFieldKeys(own.fields) {
			delete(before.fields, key)
			delete(after.fields, key)
		}
		status.Changes = diffVersionText(before, after)
		status.Stale = len(status.Changes) > 0
		if before.layout != nil {
			status.BlockChanges = emailblocks.Diff(before.layout, after.layout)
		}
	}

	status.FieldDifferences, status.BlockDifferences, status.TimingDifference = diffFromReference(reference, own)

	if len(status.BlockChanges) > 0 || len(status.BlockDifferences) > 0 {
		library, err := loadBlockLibrary(ctx, db)
		if err != nil {
			return status, err
		}
		names := map[string]string{}
		for _, block := range library {
			names[block.Key] = block.Name
		}
		for i := range status.BlockChanges {
			status.BlockChanges[i].Name = names[status.BlockChanges[i].Block]
		}
		for i := range status.BlockDifferences {
			status.BlockDifferences[i].Name = names[status.BlockDifferences[i].Block]
		}
	}
	return status, nil
}

// diffFromReference tells how an email differs from its reference now:
// fields when both are in the same language, blocks when both are built
// from blocks, and the slot's timing.
func diffFromReference(reference, own emailNow) ([]fieldDifference, []emailblocks.BlockChange, *timingDifference) {
	fields := []fieldDifference{}
	if own.language == reference.language {
		fields = diffEmailFields(reference, own)
	}
	blocks := []emailblocks.BlockChange{}
	referenceLayout, errReference := parseLayout(reference.layout)
	ownLayout, errOwn := parseLayout(own.layout)
	if errReference == nil && errOwn == nil && referenceLayout != nil && ownLayout != nil {
		blocks = emailblocks.Diff(referenceLayout, ownLayout)
	}
	var timing *timingDifference
	if slotTiming(reference) != slotTiming(own) {
		timing = &timingDifference{Reference: slotTiming(reference), Own: slotTiming(own)}
	}
	return fields, blocks, timing
}

// referenceDifference sums up for the board how a region's email differs
// from its reference.
type referenceDifference struct {
	Fields int                       `json:"fields"`
	Blocks []emailblocks.BlockChange `json:"blocks"`
	Timing *timingDifference         `json:"timing"`
}

// referenceDifferencesHandler sums up, for every active email of a board
// that adapts a reference email, how it differs from it now: the map of
// regions on the board shows it per cell. Keyed by email id.
func referenceDifferencesHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		board := strings.TrimSpace(r.URL.Query().Get("board"))
		if board == "" {
			http.Error(w, "board is required", http.StatusBadRequest)
			return
		}
		differences, err := loadReferenceDifferences(r.Context(), dbpool, board)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load reference differences of %s: %v\n", board, err)
			http.Error(w, "failed to load the differences", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"emails": differences})
	}
}

func loadReferenceDifferences(ctx context.Context, db *pgxpool.Pool, board string) (map[string]referenceDifference, error) {
	rows, err := db.Query(ctx, `
		SELECT e.id, e.language, e.subject, e.preheader, e.editable_fields, e.layout, e.send_timing, e.send_condition,
			m.language, m.subject, m.preheader, m.editable_fields, m.layout, m.send_timing, m.send_condition
		FROM emails e
		JOIN emails m ON m.id = e.based_on
		WHERE e.sequence = $1 AND e.archived_at IS NULL;
	`, board)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	differences := map[string]referenceDifference{}
	for rows.Next() {
		var id string
		var own, reference emailNow
		if err := rows.Scan(&id, &own.language, &own.subject, &own.preheader, &own.fields, &own.layout, &own.timing, &own.condition,
			&reference.language, &reference.subject, &reference.preheader, &reference.fields, &reference.layout, &reference.timing, &reference.condition); err != nil {
			return nil, err
		}
		fields, blocks, timing := diffFromReference(reference, own)
		differences[id] = referenceDifference{Fields: len(fields), Blocks: blocks, Timing: timing}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(differences) == 0 {
		return differences, nil
	}
	library, err := loadBlockLibrary(ctx, db)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, block := range library {
		names[block.Key] = block.Name
	}
	for _, difference := range differences {
		for i := range difference.Blocks {
			difference.Blocks[i].Name = names[difference.Blocks[i].Block]
		}
	}
	return differences, nil
}

// emailNow is the content of an email as it is now.
type emailNow struct {
	language           string
	subject, preheader *string
	fields, layout     []byte
	timing, condition  *string
}

func slotTiming(email emailNow) string {
	timing, condition := stringFromPointer(email.timing), stringFromPointer(email.condition)
	if condition == "" {
		return timing
	}
	return timing + " · " + condition
}

// diffEmailFields lists the subject, preheader and fields, links and images
// included, whose values differ between two emails in the same language.
func diffEmailFields(reference, own emailNow) []fieldDifference {
	differences := []fieldDifference{}
	add := func(key, a, b string) {
		if a != b {
			differences = append(differences, fieldDifference{Key: key, Reference: a, Own: b})
		}
	}
	add("subject", stringFromPointer(reference.subject), stringFromPointer(own.subject))
	add("preheader", stringFromPointer(reference.preheader), stringFromPointer(own.preheader))

	referenceFields, ownFields := fieldValues(reference.fields), fieldValues(own.fields)
	keys := []string{}
	for key := range referenceFields {
		keys = append(keys, key)
	}
	for key := range ownFields {
		if _, ok := referenceFields[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		add(key, referenceFields[key], ownFields[key])
	}
	return differences
}

func fieldValues(fieldsJSON []byte) map[string]string {
	var fields map[string]struct {
		Value any `json:"value"`
	}
	values := map[string]string{}
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return values
	}
	for key, field := range fields {
		if field.Value != nil {
			values[key] = fmt.Sprint(field.Value)
		} else {
			values[key] = ""
		}
	}
	return values
}

// confirmBasedOnHandler marks a region's email as matching the reference's
// current version: someone took from it what the region needs.
func confirmBasedOnHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		if canEdit, err := CanEditEmail(r.Context(), dbpool, user, id); err != nil || !canEdit {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to update the email", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		var slug, title string
		var version int
		err = tx.QueryRow(r.Context(), `
			UPDATE emails e
			SET based_on_version = coalesce((SELECT max(version_number) FROM email_versions WHERE email_id = e.based_on), 0),
				based_on_checked_at = now(),
				based_on_checked_by = $2
			WHERE e.id = $1 AND e.based_on IS NOT NULL AND e.archived_at IS NULL
			RETURNING slug, title, based_on_version;
		`, id, user.Email).Scan(&slug, &title, &version)
		if err != nil {
			http.Error(w, "the email is not based on a reference email", http.StatusNotFound)
			return
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventBasedOnConfirmed,
			EmailID:     &id,
			EmailSlug:   &slug,
			EmailTitle:  &title,
			Metadata:    map[string]any{"reference_version": version},
		}); err != nil {
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update the email", http.StatusInternalServerError)
			return
		}
		status, err := loadBasedOnStatus(r.Context(), dbpool, id)
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

// basedOnLinkHandler sets by hand which reference email a region's email
// adapts, e.g. one in another slot.
func basedOnLinkHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		if !requireManageEmail(w, r, dbpool, user, id) {
			return
		}
		var request basedOnLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to save the link", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		err = setBasedOn(r.Context(), tx, user, id, request.ReferenceID)
		var contentErr emailContentError
		switch {
		case errors.As(err, &contentErr):
			http.Error(w, contentErr.message, contentErr.status)
			return
		case err != nil:
			fmt.Fprintf(os.Stderr, "failed to link %s to its reference: %v\n", id, err)
			http.Error(w, "failed to save the link", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to save the link", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func setBasedOn(ctx context.Context, tx pgx.Tx, user AuthUser, id string, referenceID *string) error {
	var slug, title, board string
	var translationOf *string
	var adapting bool
	err := tx.QueryRow(ctx, `
		SELECT e.slug, e.title, coalesce(e.sequence, ''), e.translation_of, coalesce(NOT r.reference, false)
		FROM emails e
		LEFT JOIN board_regions r ON r.board_key = e.sequence AND r.key = e.region_key
		WHERE e.id = $1 AND e.archived_at IS NULL
		FOR UPDATE OF e;
	`, id).Scan(&slug, &title, &board, &translationOf, &adapting)
	if errors.Is(err, pgx.ErrNoRows) {
		return emailContentError{http.StatusNotFound, "email not found"}
	}
	if err != nil {
		return err
	}
	if referenceID == nil {
		if _, err := tx.Exec(ctx, `UPDATE emails SET based_on = NULL, based_on_version = NULL WHERE id = $1;`, id); err != nil {
			return err
		}
	} else {
		if translationOf != nil {
			return emailContentError{http.StatusBadRequest, "the email is a translation: it follows its EN email, remove that link first"}
		}
		if !adapting {
			return emailContentError{http.StatusBadRequest, "put the email in a region other than the reference first"}
		}
		var inReference bool
		err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM emails m
				JOIN board_regions r ON r.board_key = m.sequence AND r.key = m.region_key AND r.reference
				WHERE m.id::text = $1 AND m.sequence = $2 AND m.archived_at IS NULL
			);
		`, *referenceID, board).Scan(&inReference)
		if err != nil {
			return err
		}
		if !inReference {
			return emailContentError{http.StatusBadRequest, "the reference must be an email of the board's reference region"}
		}
		if err := linkBasedOn(ctx, tx, id, *referenceID); err != nil {
			return err
		}
	}
	return InsertEmailEvent(ctx, tx, EmailEventParam{
		ActorUserID: user.ID,
		ActorEmail:  user.Email,
		Action:      emailEventBasedOnLinked,
		EmailID:     &id,
		EmailSlug:   &slug,
		EmailTitle:  &title,
		Metadata:    map[string]any{"reference_id": referenceID},
	})
}

// linkBasedOn ties a region's email to a reference email at the reference's
// current version.
func linkBasedOn(ctx context.Context, db emailEventExecutor, id, referenceID string) error {
	_, err := db.Exec(ctx, `
		UPDATE emails
		SET based_on = $2,
			based_on_version = coalesce((SELECT max(version_number) FROM email_versions WHERE email_id = $2), 0),
			based_on_checked_at = NULL,
			based_on_checked_by = NULL
		WHERE id = $1;
	`, id, referenceID)
	return err
}

// adaptsReference reports whether an email follows the reference as a
// region's own version rather than as a translation: it is linked so, or
// it is in a region other than the reference and translates nothing.
func adaptsReference(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	var adapts bool
	err := tx.QueryRow(ctx, `
		SELECT e.based_on IS NOT NULL OR (e.translation_of IS NULL AND coalesce(NOT r.reference, false))
		FROM emails e
		LEFT JOIN board_regions r ON r.board_key = e.sequence AND r.key = e.region_key
		WHERE e.id = $1;
	`, id).Scan(&adapts)
	return adapts, err
}

// linkToSlotReference ties an email of a region other than the reference to
// the reference email of its slot, keeping an existing link to it. Without
// one in the slot the email is the region's own; in the reference region or
// in none it adapts nothing. A translation is left as it is.
func linkToSlotReference(ctx context.Context, tx pgx.Tx, id string) error {
	var current *string
	var translation, adapting bool
	var board, stage, variant string
	var sortOrder int
	err := tx.QueryRow(ctx, `
		SELECT e.based_on, e.translation_of IS NOT NULL, coalesce(NOT r.reference, false),
			coalesce(e.sequence, ''), e.stage, e.sort_order, e.variant
		FROM emails e
		LEFT JOIN board_regions r ON r.board_key = e.sequence AND r.key = e.region_key
		WHERE e.id = $1;
	`, id).Scan(&current, &translation, &adapting, &board, &stage, &sortOrder, &variant)
	if err != nil || translation {
		return err
	}
	var referenceID string
	if adapting {
		err = tx.QueryRow(ctx, `
			SELECT m.id FROM emails m
			JOIN board_regions r ON r.board_key = m.sequence AND r.key = m.region_key AND r.reference
			WHERE m.sequence = $1 AND m.stage = $2 AND m.sort_order = $3 AND m.archived_at IS NULL AND m.id <> $4
			ORDER BY (m.variant = $5) DESC, (m.variant <> 'old') DESC, (m.adaptation_key = 'default') DESC, m.created_at DESC
			LIMIT 1;
		`, board, stage, sortOrder, id, variant).Scan(&referenceID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	switch {
	case referenceID == "":
		_, err = tx.Exec(ctx, `UPDATE emails SET based_on = NULL, based_on_version = NULL WHERE id = $1 AND based_on IS NOT NULL;`, id)
		return err
	case current != nil && *current == referenceID:
		return nil
	default:
		return linkBasedOn(ctx, tx, id, referenceID)
	}
}
