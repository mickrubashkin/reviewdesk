package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

// Imported emails are live production emails that are not sorted into slots
// yet; the variant says where they came from.
const importedVariant = "portal"

// A group needs at least this many sends to be imported: single sends are
// mostly tests and one-off emails written by hand.
const minImportSends = 2

// Tests and manual replies or forwards by managers are not robot emails.
var (
	testSubject  = regexp.MustCompile(`(?i)^\s*(test|teste|тест)(?:[^\p{L}]|$)`)
	replySubject = regexp.MustCompile(`(?i)^\s*(re|fw|fwd|aw)\s*:`)
)

// IsManualOrTestSubject reports subjects that are tests, replies or forwards.
func IsManualOrTestSubject(subject string) bool {
	return testSubject.MatchString(subject) || replySubject.MatchString(subject)
}

type importResult struct {
	BoardKey  string `json:"board_key"`
	BoardName string `json:"board_name"`
	Created   int    `json:"created"`
	Linked    int    `json:"linked"`
	// WithoutFields were imported as sent, because no board template fits.
	WithoutFields int             `json:"without_fields"`
	Skipped       []importSkipped `json:"skipped"`
}

type importSkipped struct {
	GroupID string `json:"group_id"`
	Subject string `json:"subject"`
	Reason  string `json:"reason"`
}

type importCandidate struct {
	id, language, subject string
	sendCount             int
	stages                map[string]int
}

// ImportBoardKey is where emails sent on the portal but missing from the
// board are imported, kept apart from the board's own emails until sorted.
func ImportBoardKey(boardKey string) string {
	return boardKey + "-portal"
}

func importMissingHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireSuperAdmin(w, r)
		if !ok {
			return
		}
		result, err := importMissing(r, dbpool, user, chi.URLParam(r, "boardKey"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "portal import: %v\n", err)
			http.Error(w, "import failed", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func importMissing(r *http.Request, db *pgxpool.Pool, user auth.AuthUser, boardKey string) (importResult, error) {
	ctx := r.Context()
	result := importResult{BoardKey: ImportBoardKey(boardKey), Skipped: []importSkipped{}}

	candidates, err := importCandidates(ctx, db, boardKey)
	if err != nil {
		return result, err
	}

	stageNames, stageOrder, err := loadPortalStages(ctx, db, boardKey)
	if err != nil {
		return result, err
	}
	result.BoardName, err = ensureImportBoard(ctx, db, boardKey, stageOrder, stageNames)
	if err != nil {
		return result, err
	}

	// The same email sent by different managers differs only in the
	// signature; import it once and tie the other variants to it.
	primaryByKey := map[string]string{} // dedupe key -> created email ID
	for _, c := range candidates {
		portalStage := topStage(c.stages)
		stage := stageKey(stageNames[portalStage])
		if stage == "" {
			stage = "unknown-stage"
		}
		dedupe := strings.Join([]string{stage, c.language, strings.ToLower(importTitle(c.subject))}, "|")

		if emailID, ok := primaryByKey[dedupe]; ok {
			if err := markGroupCreated(ctx, db, c.id, emailID, user.Email); err != nil {
				return result, err
			}
			result.Linked++
			continue
		}

		rebuilt, err := rebuildFromGroup(ctx, db, c.id, "")
		if err != nil {
			// No template fits (e.g. an older layout): import the sent copy as
			// it is, so the email is at least visible; fields can come later.
			rebuilt, err = rawImport(ctx, db, c)
			if err != nil {
				result.Skipped = append(result.Skipped, importSkipped{GroupID: c.id, Subject: c.subject, Reason: err.Error()})
				continue
			}
			result.WithoutFields++
		}

		created, err := createImportedEmail(r, db, user, result.BoardKey, stage, c, rebuilt)
		if err != nil {
			result.Skipped = append(result.Skipped, importSkipped{GroupID: c.id, Subject: c.subject, Reason: err.Error()})
			continue
		}
		primaryByKey[dedupe] = created
		result.Created++
	}
	return result, nil
}

func importCandidates(ctx context.Context, db *pgxpool.Pool, boardKey string) ([]importCandidate, error) {
	rows, err := db.Query(ctx, `
		SELECT id, language, subject, send_count, stages
		FROM portal_email_groups
		WHERE board_key = $1 AND decision IS NULL AND match_status = 'unmatched'
			AND language <> '' AND send_count >= $2
		ORDER BY send_count DESC, last_sent_at DESC;
	`, boardKey, minImportSends)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []importCandidate
	for rows.Next() {
		var c importCandidate
		var stagesJSON []byte
		if err := rows.Scan(&c.id, &c.language, &c.subject, &c.sendCount, &stagesJSON); err != nil {
			return nil, err
		}
		if IsManualOrTestSubject(c.subject) {
			continue
		}
		_ = json.Unmarshal(stagesJSON, &c.stages)
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

func loadPortalStages(ctx context.Context, db *pgxpool.Pool, boardKey string) (map[string]string, []string, error) {
	rows, err := db.Query(ctx, `SELECT stage_id, name FROM portal_stages WHERE board_key = $1 ORDER BY sort;`, boardKey)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	names := map[string]string{}
	var order []string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, err
		}
		names[id] = name
		order = append(order, id)
	}
	return names, order, rows.Err()
}

// ensureImportBoard creates the import board with the portal's stages as
// columns, or adds stages that appeared since.
func ensureImportBoard(ctx context.Context, db *pgxpool.Pool, boardKey string, stageOrder []string, stageNames map[string]string) (string, error) {
	var sourceName string
	if err := db.QueryRow(ctx, `SELECT name FROM boards WHERE key = $1;`, boardKey).Scan(&sourceName); err != nil {
		return "", err
	}
	name := "Portal: live, not sorted (" + sourceName + ")"

	stages := []string{}
	for _, id := range stageOrder {
		if key := stageKey(stageNames[id]); key != "" {
			stages = append(stages, key)
		}
	}
	stages = append(stages, "unknown-stage")

	var existing []byte
	err := db.QueryRow(ctx, `SELECT stages FROM boards WHERE key = $1;`, ImportBoardKey(boardKey)).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		stagesJSON, _ := json.Marshal(stages)
		_, err = db.Exec(ctx, `INSERT INTO boards (key, name, stages) VALUES ($1, $2, $3::jsonb);`,
			ImportBoardKey(boardKey), name, string(stagesJSON))
		return name, err
	}
	if err != nil {
		return "", err
	}

	var current []string
	if err := json.Unmarshal(existing, &current); err != nil {
		return "", err
	}
	known := map[string]bool{}
	for _, s := range current {
		known[s] = true
	}
	for _, s := range stages {
		if !known[s] {
			current = append(current, s)
		}
	}
	stagesJSON, _ := json.Marshal(current)
	_, err = db.Exec(ctx, `UPDATE boards SET stages = $2::jsonb, updated_at = now() WHERE key = $1;`, ImportBoardKey(boardKey), string(stagesJSON))
	return name, err
}

func createImportedEmail(r *http.Request, db *pgxpool.Pool, user auth.AuthUser, boardKey, stage string, c importCandidate, rebuilt reconstructResponse) (string, error) {
	ctx := r.Context()
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var maxSort *int
	if err := tx.QueryRow(ctx, `
		SELECT max(sort_order) FROM emails WHERE sequence = $1 AND stage = $2 AND archived_at IS NULL;
	`, boardKey, stage).Scan(&maxSort); err != nil {
		return "", err
	}
	sortOrder := 1
	if maxSort != nil {
		sortOrder = *maxSort + 1
	}

	subject := c.subject
	title := importTitle(c.subject)
	var created emails.EmailDetail
	// Two different emails can shorten to the same title; keep both.
	for attempt := 1; attempt <= 5; attempt++ {
		candidateTitle := title
		if attempt > 1 {
			candidateTitle = fmt.Sprintf("%s (%d)", title, attempt)
		}
		// A savepoint, so a duplicate title does not abort the transaction.
		savepoint, spErr := tx.Begin(ctx)
		if spErr != nil {
			return "", spErr
		}
		created, err = emails.CreateEmailTx(r, db, savepoint, user, emails.NewEmailParams{
			Sequence:     boardKey,
			Title:        candidateTitle,
			Subject:      &subject,
			Stage:        stage,
			SortOrder:    sortOrder,
			Language:     c.language,
			Variant:      importedVariant,
			OriginalHTML: rebuilt.HTML,
		})
		if err == nil {
			if err = savepoint.Commit(ctx); err != nil {
				return "", err
			}
			break
		}
		_ = savepoint.Rollback(ctx)
		if !strings.Contains(err.Error(), "already exists") {
			break
		}
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE portal_email_groups
		SET decision = 'created', decision_email_id = $2, decided_by_email = $3, decided_at = now()
		WHERE id = $1;
	`, c.id, created.ID, user.Email); err != nil {
		return "", err
	}
	return created.ID, tx.Commit(ctx)
}

func markGroupCreated(ctx context.Context, db *pgxpool.Pool, groupID, emailID, by string) error {
	_, err := db.Exec(ctx, `
		UPDATE portal_email_groups
		SET decision = 'created', decision_email_id = $2, decided_by_email = $3, decided_at = now()
		WHERE id = $1;
	`, groupID, emailID, by)
	return err
}

func topStage(stages map[string]int) string {
	best, bestCount := "", -1
	keys := make([]string, 0, len(stages))
	for key := range stages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if stages[key] > bestCount {
			best, bestCount = key, stages[key]
		}
	}
	return best
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// stageKey turns a portal stage name into a board stage key.
func stageKey(name string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// importTitle shortens a subject to a board title: subjects repeat the
// programme name ("Partner Program | Welcome Bonus Activated",
// "Bitrix24: You haven't started yet"), which adds nothing on a card.
func importTitle(subject string) string {
	title := strings.TrimSpace(subject)
	if i := strings.LastIndex(title, " | "); i >= 0 {
		title = title[i+3:]
	} else if rest, ok := strings.CutPrefix(title, "Bitrix24:"); ok {
		title = rest
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = strings.TrimSpace(subject)
	}
	if utf8.RuneCountInString(title) > 80 {
		title = string([]rune(title)[:80])
	}
	return title
}

// rawImport wraps the repaired sent copy in a minimal document with the
// subject as title, for emails no template can rebuild.
func rawImport(ctx context.Context, db *pgxpool.Pool, c importCandidate) (reconstructResponse, error) {
	var body string
	if err := db.QueryRow(ctx, `
		SELECT s.body_html FROM portal_email_groups g
		JOIN portal_sent_emails s ON s.activity_id = g.sample_activity_id
		WHERE g.id = $1;
	`, c.id).Scan(&body); err != nil {
		return reconstructResponse{}, err
	}
	lang := c.language
	if mapped, ok := htmlLangByLanguage[lang]; ok {
		lang = mapped
	}
	html := "<!doctype html><html lang=\"" + lang + "\"><head><meta charset=\"utf-8\"><title>" +
		htmlEscape(c.subject) + "</title></head><body>" + RepairTimelineHTML(body) + "</body></html>"
	return reconstructResponse{Reconstruction: Reconstruction{HTML: html}, Subject: c.subject, Language: c.language}, nil
}

func htmlEscape(value string) string { return stdhtml.EscapeString(value) }
