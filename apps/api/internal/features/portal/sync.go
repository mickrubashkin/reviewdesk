package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

const (
	// Groups at or above this similarity are the same email as in the service
	// (signature names and similar small differences allowed).
	matchedThreshold = 0.9
	// Below this the email is considered absent from the service.
	differsThreshold = 0.5
)

type SyncOptions struct {
	BoardKey   string
	CategoryID int
	Days       int
}

// Sync mirrors the CRM robot emails of one deal category into the portal
// tables and matches them against the board's emails. It only reads from
// Bitrix24. Progress is written to the run row so the UI can show it.
func Sync(ctx context.Context, db *pgxpool.Pool, client *Client, runID string, options SyncOptions) error {
	err := runSync(ctx, db, client, runID, options)
	status, message := "done", ""
	if err != nil {
		status, message = "failed", err.Error()
	}
	_, updateErr := db.Exec(context.Background(), `
		UPDATE portal_sync_runs
		SET status = $2, error = NULLIF($3, ''), finished_at = now()
		WHERE id = $1;
	`, runID, status, message)
	if err != nil {
		return err
	}
	return updateErr
}

func runSync(ctx context.Context, db *pgxpool.Pool, client *Client, runID string, options SyncOptions) error {
	progress := func(format string, args ...any) {
		_, _ = db.Exec(ctx, `UPDATE portal_sync_runs SET progress = $2 WHERE id = $1;`, runID, fmt.Sprintf(format, args...))
	}

	progress("Loading stages")
	stages, err := client.DealStages(ctx, options.CategoryID)
	if err != nil {
		return err
	}
	if err := storeStages(ctx, db, options.BoardKey, stages); err != nil {
		return err
	}

	sinceTime := time.Now().AddDate(0, 0, -options.Days).UTC()
	since := sinceTime.Format(time.RFC3339)
	categoryByDeal := map[int64]int{}
	dealsWithEmails := map[int64]bool{}
	seen := 0
	var funnelActivityIDs []int64

	// Pass 1: a light list (no bodies) to find which emails belong to the
	// funnel. Most outgoing deal emails are from other funnels.
	err = client.List(ctx, "crm.activity.list", map[string]any{
		"order": map[string]any{"ID": "ASC"},
		"filter": map[string]any{
			"OWNER_TYPE_ID": 2, // deal
			"TYPE_ID":       4, // email
			"DIRECTION":     2, // outgoing
			">=CREATED":     since,
		},
		"select": []string{"ID", "OWNER_ID"},
	}, func(page json.RawMessage) error {
		var activities []struct {
			ID      string `json:"ID"`
			OwnerID string `json:"OWNER_ID"`
		}
		if err := json.Unmarshal(page, &activities); err != nil {
			return err
		}
		var unknown []int64
		for _, a := range activities {
			dealID, _ := strconv.ParseInt(a.OwnerID, 10, 64)
			if _, ok := categoryByDeal[dealID]; !ok {
				unknown = append(unknown, dealID)
			}
		}
		if err := resolveDealCategories(ctx, client, unknown, categoryByDeal); err != nil {
			return err
		}
		for _, a := range activities {
			seen++
			dealID, _ := strconv.ParseInt(a.OwnerID, 10, 64)
			if categoryByDeal[dealID] == options.CategoryID {
				activityID, _ := strconv.ParseInt(a.ID, 10, 64)
				funnelActivityIDs = append(funnelActivityIDs, activityID)
			}
		}
		progress("Step 1 of 3 · Finding this funnel's emails by ID only (no subjects or texts read): %d checked, %d from this funnel", seen, len(funnelActivityIDs))
		return nil
	})
	if err != nil {
		return err
	}

	// Pass 2: bodies only for the funnel's emails.
	stored := 0
	for start := 0; start < len(funnelActivityIDs); start += 50 {
		end := min(start+50, len(funnelActivityIDs))
		response, err := client.Call(ctx, "crm.activity.list", map[string]any{
			"filter": map[string]any{"ID": funnelActivityIDs[start:end]},
			"select": []string{"ID", "OWNER_ID", "SUBJECT", "DESCRIPTION", "CREATED"},
		})
		if err != nil {
			return err
		}
		var activities []struct {
			ID          string `json:"ID"`
			OwnerID     string `json:"OWNER_ID"`
			Subject     string `json:"SUBJECT"`
			Description string `json:"DESCRIPTION"`
			Created     string `json:"CREATED"`
		}
		if err := json.Unmarshal(response.Result, &activities); err != nil {
			return err
		}
		batch := &pgx.Batch{}
		for _, a := range activities {
			if strings.TrimSpace(a.Description) == "" {
				continue
			}
			activityID, _ := strconv.ParseInt(a.ID, 10, 64)
			dealID, _ := strconv.ParseInt(a.OwnerID, 10, 64)
			sentAt, err := time.Parse(time.RFC3339, a.Created)
			if err != nil {
				continue
			}
			batch.Queue(`
				INSERT INTO portal_sent_emails (activity_id, board_key, deal_id, sent_at, subject, body_html, fingerprint)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (activity_id) DO UPDATE SET
					subject = EXCLUDED.subject,
					body_html = EXCLUDED.body_html,
					fingerprint = EXCLUDED.fingerprint,
					synced_at = now();
			`, activityID, options.BoardKey, dealID, sentAt, RedactPartnerData(a.Subject), a.Description, Fingerprint(NormalizedText(a.Description)))
			dealsWithEmails[dealID] = true
			stored++
		}
		if err := sendBatch(ctx, db, batch); err != nil {
			return err
		}
		progress("Step 2 of 3 · Loading subjects and texts of this funnel's emails only: %d of %d", end, len(funnelActivityIDs))
	}
	_, _ = db.Exec(ctx, `UPDATE portal_sync_runs SET activities_seen = $2, emails_stored = $3 WHERE id = $1;`, runID, seen, stored)

	progress("Loading stage history for %d deals", len(dealsWithEmails))
	if err := assignStagesAtSend(ctx, db, client, options.BoardKey, dealsWithEmails, progress); err != nil {
		return err
	}

	progress("Grouping and matching")
	groups, err := rebuildGroups(ctx, db, options.BoardKey)
	if err != nil {
		return err
	}
	if err := matchGroups(ctx, db, options.BoardKey); err != nil {
		return err
	}
	progress("Removing partner data from stored copies")
	if err := minimizeSentEmails(ctx, db, options.BoardKey, sinceTime); err != nil {
		return err
	}
	_, err = db.Exec(ctx, `UPDATE portal_sync_runs SET groups_count = $2, progress = 'Done' WHERE id = $1;`, runID, groups)
	return err
}

func storeStages(ctx context.Context, db *pgxpool.Pool, boardKey string, stages []Stage) error {
	batch := &pgx.Batch{}
	for _, stage := range stages {
		batch.Queue(`
			INSERT INTO portal_stages (board_key, stage_id, name, sort) VALUES ($1, $2, $3, $4)
			ON CONFLICT (board_key, stage_id) DO UPDATE SET name = EXCLUDED.name, sort = EXCLUDED.sort;
		`, boardKey, stage.ID, stage.Name, stage.Sort)
	}
	return sendBatch(ctx, db, batch)
}

// sendBatch runs queued statements in one round trip. Writes go in batches
// because the production database is far enough away that one statement per
// row made a sync take minutes.
func sendBatch(ctx context.Context, db *pgxpool.Pool, batch *pgx.Batch) error {
	if batch.Len() == 0 {
		return nil
	}
	results := db.SendBatch(ctx, batch)
	for i := 0; i < batch.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			results.Close()
			return err
		}
	}
	return results.Close()
}

func resolveDealCategories(ctx context.Context, client *Client, dealIDs []int64, categoryByDeal map[int64]int) error {
	for start := 0; start < len(dealIDs); start += 50 {
		end := min(start+50, len(dealIDs))
		chunk := dealIDs[start:end]
		response, err := client.Call(ctx, "crm.deal.list", map[string]any{
			"filter": map[string]any{"ID": chunk},
			"select": []string{"ID", "CATEGORY_ID"},
		})
		if err != nil {
			return err
		}
		var deals []struct {
			ID         string `json:"ID"`
			CategoryID string `json:"CATEGORY_ID"`
		}
		if err := json.Unmarshal(response.Result, &deals); err != nil {
			return err
		}
		for _, id := range chunk {
			categoryByDeal[id] = -1 // deleted or not visible to the webhook user
		}
		for _, deal := range deals {
			id, _ := strconv.ParseInt(deal.ID, 10, 64)
			category, _ := strconv.Atoi(deal.CategoryID)
			categoryByDeal[id] = category
		}
	}
	return nil
}

type stageChange struct {
	stageID string
	at      time.Time
}

// assignStagesAtSend records the deal stage at the moment each email was
// sent: the deal may have moved on since, so its current stage is not enough.
func assignStagesAtSend(ctx context.Context, db *pgxpool.Pool, client *Client, boardKey string, deals map[int64]bool, progress func(string, ...any)) error {
	ids := make([]int64, 0, len(deals))
	for id := range deals {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	history := map[int64][]stageChange{}
	for start := 0; start < len(ids); start += 50 {
		end := min(start+50, len(ids))
		err := client.List(ctx, "crm.stagehistory.list", map[string]any{
			"entityTypeId": 2,
			"order":        map[string]any{"ID": "ASC"},
			"filter":       map[string]any{"OWNER_ID": ids[start:end]},
			"select":       []string{"OWNER_ID", "STAGE_ID", "CREATED_TIME"},
		}, func(page json.RawMessage) error {
			var result struct {
				Items []struct {
					OwnerID     json.Number `json:"OWNER_ID"`
					StageID     string      `json:"STAGE_ID"`
					CreatedTime string      `json:"CREATED_TIME"`
				} `json:"items"`
			}
			if err := json.Unmarshal(page, &result); err != nil {
				return err
			}
			for _, item := range result.Items {
				owner, _ := item.OwnerID.Int64()
				at, err := time.Parse(time.RFC3339, item.CreatedTime)
				if err != nil {
					continue
				}
				history[owner] = append(history[owner], stageChange{stageID: item.StageID, at: at})
			}
			return nil
		})
		if err != nil {
			return err
		}
		progress("Step 3 of 3 · Loading deal stage history: %d of %d deals", end, len(ids))
	}

	rows, err := db.Query(ctx, `SELECT activity_id, deal_id, sent_at FROM portal_sent_emails WHERE board_key = $1;`, boardKey)
	if err != nil {
		return err
	}
	type sent struct {
		activityID, dealID int64
		at                 time.Time
	}
	var all []sent
	for rows.Next() {
		var s sent
		if err := rows.Scan(&s.activityID, &s.dealID, &s.at); err != nil {
			rows.Close()
			return err
		}
		all = append(all, s)
	}
	rows.Close()

	var activityIDs []int64
	var stages []string
	for _, s := range all {
		changes, ok := history[s.dealID]
		if !ok {
			continue
		}
		if stage := stageAt(changes, s.at); stage != "" {
			activityIDs = append(activityIDs, s.activityID)
			stages = append(stages, stage)
		}
	}
	_, err = db.Exec(ctx, `
		UPDATE portal_sent_emails p
		SET stage_id = v.stage
		FROM unnest($1::bigint[], $2::text[]) AS v(activity_id, stage)
		WHERE p.activity_id = v.activity_id;
	`, activityIDs, stages)
	return err
}

func stageAt(changes []stageChange, at time.Time) string {
	sort.Slice(changes, func(i, j int) bool { return changes[i].at.Before(changes[j].at) })
	stage := ""
	for _, change := range changes {
		// A robot sends right after the stage changes; allow for clock skew.
		if change.at.After(at.Add(time.Minute)) {
			break
		}
		stage = change.stageID
	}
	return stage
}

// Fingerprints are computed when a copy is fetched. Bodies are not kept after
// a sync (see minimizeSentEmails), so after a change to NormalizedText run a
// sync over the full window to refresh them.
func rebuildGroups(ctx context.Context, db *pgxpool.Pool, boardKey string) (int, error) {
	rows, err := db.Query(ctx, `
		SELECT activity_id, fingerprint, subject, sent_at, COALESCE(stage_id, ''), body_html <> ''
		FROM portal_sent_emails
		WHERE board_key = $1
		ORDER BY sent_at;
	`, boardKey)
	if err != nil {
		return 0, err
	}
	type group struct {
		sample      int64
		subjects    map[string]int
		count       int
		first, last time.Time
		stages      map[string]int
	}
	groups := map[string]*group{}
	for rows.Next() {
		var activityID int64
		var fingerprint, subject, stage string
		var sentAt time.Time
		var hasBody bool
		if err := rows.Scan(&activityID, &fingerprint, &subject, &sentAt, &stage, &hasBody); err != nil {
			rows.Close()
			return 0, err
		}
		g := groups[fingerprint]
		if g == nil {
			g = &group{subjects: map[string]int{}, stages: map[string]int{}, first: sentAt}
			groups[fingerprint] = g
		}
		// The sample is the latest copy that still has its body.
		if hasBody || g.sample == 0 {
			g.sample = activityID
		}
		g.last = sentAt
		g.count++
		g.subjects[subject]++
		if stage != "" {
			g.stages[stage]++
		}
	}
	rows.Close()

	sampleIDs := make([]int64, 0, len(groups))
	for _, g := range groups {
		sampleIDs = append(sampleIDs, g.sample)
	}
	bodies := map[int64]string{}
	bodyRows, err := db.Query(ctx, `SELECT activity_id, body_html FROM portal_sent_emails WHERE activity_id = ANY($1);`, sampleIDs)
	if err != nil {
		return 0, err
	}
	for bodyRows.Next() {
		var id int64
		var body string
		if err := bodyRows.Scan(&id, &body); err != nil {
			bodyRows.Close()
			return 0, err
		}
		bodies[id] = body
	}
	bodyRows.Close()

	batch := &pgx.Batch{}
	for fingerprint, g := range groups {
		text := NormalizedText(RedactPartnerData(bodies[g.sample]))
		stagesJSON, _ := json.Marshal(g.stages)
		batch.Queue(`
			INSERT INTO portal_email_groups (
				board_key, fingerprint, language, subject, normalized_text, sample_activity_id,
				send_count, first_sent_at, last_sent_at, stages
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb)
			ON CONFLICT (board_key, fingerprint) DO UPDATE SET
				language = EXCLUDED.language,
				subject = EXCLUDED.subject,
				normalized_text = EXCLUDED.normalized_text,
				sample_activity_id = EXCLUDED.sample_activity_id,
				send_count = EXCLUDED.send_count,
				first_sent_at = EXCLUDED.first_sent_at,
				last_sent_at = EXCLUDED.last_sent_at,
				stages = EXCLUDED.stages,
				updated_at = now();
		`, boardKey, fingerprint, DetectLanguage(text), mostCommon(g.subjects), text, g.sample,
			g.count, g.first, g.last, string(stagesJSON))
	}
	if err := sendBatch(ctx, db, batch); err != nil {
		return 0, err
	}
	// Groups nobody decided on and no stored email points to any more (e.g.
	// after normalisation changed) are dropped.
	if _, err := db.Exec(ctx, `
		DELETE FROM portal_email_groups g
		WHERE g.board_key = $1 AND g.decision IS NULL
			AND NOT EXISTS (
				SELECT 1 FROM portal_sent_emails s WHERE s.board_key = g.board_key AND s.fingerprint = g.fingerprint
			);
	`, boardKey); err != nil {
		return 0, err
	}
	return len(groups), nil
}

type serviceEmail struct {
	id       string
	language string
	text     string
	grams    map[string]bool
}

// matchGroups finds the closest board email for every group. It never
// touches decisions a person already made.
func matchGroups(ctx context.Context, db *pgxpool.Pool, boardKey string) error {
	emails, err := loadServiceEmails(ctx, db, boardKey)
	if err != nil {
		return err
	}

	rows, err := db.Query(ctx, `SELECT id, language, normalized_text FROM portal_email_groups WHERE board_key = $1;`, boardKey)
	if err != nil {
		return err
	}
	type groupText struct{ id, language, text string }
	var groups []groupText
	for rows.Next() {
		var g groupText
		if err := rows.Scan(&g.id, &g.language, &g.text); err != nil {
			rows.Close()
			return err
		}
		groups = append(groups, g)
	}
	rows.Close()

	for i := range emails {
		emails[i].grams = bigrams(emails[i].text)
	}
	batch := &pgx.Batch{}
	for _, g := range groups {
		grams := bigrams(g.text)
		bestID, bestScore := "", 0.0
		for _, email := range emails {
			if g.language != "" && email.language != g.language {
				continue
			}
			if score := jaccard(grams, email.grams); score > bestScore {
				bestID, bestScore = email.id, score
			}
		}
		status := "unmatched"
		switch {
		case bestScore >= matchedThreshold:
			status = "matched"
		case bestScore >= differsThreshold:
			status = "differs"
		}
		var matchID any
		if status != "unmatched" {
			matchID = bestID
		}
		batch.Queue(`
			UPDATE portal_email_groups SET match_email_id = $2, match_score = $3, match_status = $4 WHERE id = $1;
		`, g.id, matchID, bestScore, status)
	}
	return sendBatch(ctx, db, batch)
}

func loadServiceEmails(ctx context.Context, db *pgxpool.Pool, boardKey string) ([]serviceEmail, error) {
	rows, err := db.Query(ctx, `
		SELECT id, language, template_html, editable_fields, COALESCE(preheader, '')
		FROM emails
		WHERE sequence = $1 AND archived_at IS NULL;
	`, boardKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []serviceEmail
	for rows.Next() {
		var e serviceEmail
		var template, preheader string
		var fieldsJSON []byte
		if err := rows.Scan(&e.id, &e.language, &template, &fieldsJSON, &preheader); err != nil {
			return nil, err
		}
		var fields emailedit.EditableFields
		if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
			continue
		}
		rendered, err := emailedit.RenderEditableHTMLWithMetadata(template, fields, emailedit.RenderMetadata{Preheader: preheader})
		if err != nil {
			continue
		}
		e.text = NormalizedText(rendered)
		emails = append(emails, e)
	}
	return emails, rows.Err()
}

func mostCommon(counts map[string]int) string {
	best, bestCount := "", -1
	for value, count := range counts {
		if count > bestCount || (count == bestCount && value < best) {
			best, bestCount = value, count
		}
	}
	return best
}
