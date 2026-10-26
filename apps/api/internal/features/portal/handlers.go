package portal

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

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

func RegisterRoutes(r chi.Router, dbpool *pgxpool.Pool) {
	r.Get("/api/portal/categories", listCategoriesHandler())
	r.Get("/api/bitrix-variables", getVariablesHandler(dbpool))
	r.Put("/api/bitrix-variables", putVariablesHandler(dbpool))
	r.Post("/api/boards/{boardKey}/portal/sync", startSyncHandler(dbpool))
	r.Put("/api/boards/{boardKey}/portal/settings", putSyncSettingsHandler(dbpool))
	r.Get("/api/boards/{boardKey}/portal", boardOverviewHandler(dbpool))
	r.Get("/api/portal/upload-suggestions", uploadSuggestionsHandler(dbpool))
	r.Get("/api/portal/groups/{id}", groupDetailHandler(dbpool))
	r.Patch("/api/portal/groups/{id}", decideGroupHandler(dbpool))
	r.Get("/api/portal/groups/{id}/reconstruct", reconstructGroupHandler(dbpool))
	r.Post("/api/boards/{boardKey}/portal/import-missing", importMissingHandler(dbpool))
}

func requireSuperAdmin(w http.ResponseWriter, r *http.Request) (auth.AuthUser, bool) {
	user, ok := auth.FromRequest(r)
	if !ok || !auth.IsSuperAdmin(user) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return auth.AuthUser{}, false
	}
	return user, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func listCategoriesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSuperAdmin(w, r); !ok {
			return
		}
		client, err := NewClientFromEnv()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		categories, err := client.DealCategories(r.Context())
		if err != nil {
			fmt.Fprintf(os.Stderr, "portal categories: %v\n", err)
			http.Error(w, "failed to load funnels from Bitrix24", http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, categories)
	}
}

type startSyncRequest struct {
	CategoryID int `json:"category_id"`
	Days       int `json:"days"`
}

func startSyncHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireSuperAdmin(w, r)
		if !ok {
			return
		}
		var request startSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if request.Days <= 0 || request.Days > 365 {
			http.Error(w, "days must be between 1 and 365", http.StatusBadRequest)
			return
		}
		client, err := NewClientFromEnv()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}

		options := SyncOptions{BoardKey: chi.URLParam(r, "boardKey"), CategoryID: request.CategoryID, Days: request.Days}
		runID, err := BeginRun(r.Context(), dbpool, options, user.Email)
		if errors.Is(err, ErrSyncRunning) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "failed to start sync", http.StatusInternalServerError)
			return
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
			defer cancel()
			if err := Sync(ctx, dbpool, client, runID, options); err != nil {
				fmt.Fprintf(os.Stderr, "portal sync %s failed: %v\n", runID, err)
			}
		}()

		writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID})
	}
}

func putSyncSettingsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireSuperAdmin(w, r)
		if !ok {
			return
		}
		var settings SyncSettings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := settings.validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := saveSyncSettings(r.Context(), dbpool, chi.URLParam(r, "boardKey"), settings, user.Email)
		if errors.Is(err, errBoardNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "portal sync settings: %v\n", err)
			http.Error(w, "failed to save sync settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, saved)
	}
}

type syncRun struct {
	ID             string     `json:"id"`
	CategoryID     int        `json:"category_id"`
	Days           int        `json:"days"`
	Status         string     `json:"status"`
	Progress       string     `json:"progress"`
	ActivitiesSeen int        `json:"activities_seen"`
	EmailsStored   int        `json:"emails_stored"`
	GroupsCount    int        `json:"groups_count"`
	Error          *string    `json:"error"`
	StartedBy      string     `json:"started_by_email"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

type portalStage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

type groupItem struct {
	ID            string         `json:"id"`
	Language      string         `json:"language"`
	Subject       string         `json:"subject"`
	Excerpt       string         `json:"excerpt"`
	SendCount     int            `json:"send_count"`
	FirstSentAt   time.Time      `json:"first_sent_at"`
	LastSentAt    time.Time      `json:"last_sent_at"`
	Stages        map[string]int `json:"stages"`
	MatchEmailID  *string        `json:"match_email_id"`
	MatchTitle    *string        `json:"match_email_title"`
	MatchStage    *string        `json:"match_email_stage"`
	MatchScore    float64        `json:"match_score"`
	MatchStatus   string         `json:"match_status"`
	Decision      *string        `json:"decision"`
	DecisionEmail *string        `json:"decision_email_id"`
	DecidedBy     *string        `json:"decided_by_email"`
	DecidedAt     *time.Time     `json:"decided_at"`
}

type unseenEmail struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Stage           string `json:"stage"`
	SortOrder       int    `json:"sort_order"`
	Language        string `json:"language"`
	Variant         string `json:"variant"`
	AdaptationLabel string `json:"adaptation_label"`
	// RegionName is the email's region on a board with regions, else "".
	RegionName    string  `json:"region_name"`
	SendTiming    *string `json:"send_timing"`
	SendCondition *string `json:"send_condition"`
}

type boardOverview struct {
	LatestRun *syncRun      `json:"latest_run"`
	Settings  SyncSettings  `json:"settings"`
	Stages    []portalStage `json:"stages"`
	Groups    []groupItem   `json:"groups"`
	Unseen    []unseenEmail `json:"unseen"`
	// ManualLive are emails a super admin marked live by hand.
	ManualLive []manualLiveEmail `json:"manual_live"`
}

type manualLiveEmail struct {
	unseenEmail
	MarkedAt time.Time `json:"live_marked_at"`
	MarkedBy string    `json:"live_marked_by"`
	Note     *string   `json:"live_note"`
}

func boardOverviewHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSuperAdmin(w, r); !ok {
			return
		}
		boardKey := chi.URLParam(r, "boardKey")
		overview, err := loadBoardOverview(r.Context(), dbpool, boardKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "portal overview %s: %v\n", boardKey, err)
			http.Error(w, "failed to load portal data", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, overview)
	}
}

func loadBoardOverview(ctx context.Context, db *pgxpool.Pool, boardKey string) (boardOverview, error) {
	overview := boardOverview{Stages: []portalStage{}, Groups: []groupItem{}, Unseen: []unseenEmail{}, ManualLive: []manualLiveEmail{}}

	var run syncRun
	err := db.QueryRow(ctx, `
		SELECT id, category_id, days, status, progress, activities_seen, emails_stored, groups_count,
			error, started_by_email, started_at, finished_at
		FROM portal_sync_runs WHERE board_key = $1 ORDER BY started_at DESC LIMIT 1;
	`, boardKey).Scan(&run.ID, &run.CategoryID, &run.Days, &run.Status, &run.Progress, &run.ActivitiesSeen,
		&run.EmailsStored, &run.GroupsCount, &run.Error, &run.StartedBy, &run.StartedAt, &run.FinishedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return overview, err
	}
	if err == nil {
		overview.LatestRun = &run
	}
	if overview.Settings, err = loadSyncSettings(ctx, db, boardKey); err != nil {
		return overview, err
	}

	rows, err := db.Query(ctx, `SELECT stage_id, name, sort FROM portal_stages WHERE board_key = $1 ORDER BY sort;`, boardKey)
	if err != nil {
		return overview, err
	}
	for rows.Next() {
		var s portalStage
		if err := rows.Scan(&s.ID, &s.Name, &s.Sort); err != nil {
			rows.Close()
			return overview, err
		}
		overview.Stages = append(overview.Stages, s)
	}
	rows.Close()

	rows, err = db.Query(ctx, `
		SELECT g.id, g.language, g.subject, left(g.normalized_text, 220), g.send_count, g.first_sent_at, g.last_sent_at,
			g.stages, g.match_email_id, e.title, e.stage, g.match_score, g.match_status, g.decision,
			g.decision_email_id, g.decided_by_email, g.decided_at
		FROM portal_email_groups g
		LEFT JOIN emails e ON e.id = COALESCE(g.decision_email_id, g.match_email_id)
		WHERE g.board_key = $1
		ORDER BY g.last_sent_at DESC;
	`, boardKey)
	if err != nil {
		return overview, err
	}
	for rows.Next() {
		var g groupItem
		var stagesJSON []byte
		if err := rows.Scan(&g.ID, &g.Language, &g.Subject, &g.Excerpt, &g.SendCount, &g.FirstSentAt, &g.LastSentAt,
			&stagesJSON, &g.MatchEmailID, &g.MatchTitle, &g.MatchStage, &g.MatchScore, &g.MatchStatus, &g.Decision,
			&g.DecisionEmail, &g.DecidedBy, &g.DecidedAt); err != nil {
			rows.Close()
			return overview, err
		}
		_ = json.Unmarshal(stagesJSON, &g.Stages)
		overview.Groups = append(overview.Groups, g)
	}
	rows.Close()

	// Board emails no sent email points to: either not triggered yet (the
	// robot condition has not fired) or not used anymore.
	rows, err = db.Query(ctx, `
		SELECT e.id, e.title, e.stage, e.sort_order, e.language, e.variant, e.adaptation_label,
			coalesce((SELECT r.name FROM board_regions r WHERE r.board_key = e.sequence AND r.key = e.region_key), ''), e.send_timing, e.send_condition
		FROM emails e
		WHERE e.sequence = $1 AND e.archived_at IS NULL AND e.live_marked_at IS NULL
			AND NOT EXISTS (
				SELECT 1 FROM portal_email_groups g
				WHERE g.board_key = $1 AND (
					(g.decision IN ('confirmed', 'created') AND g.decision_email_id = e.id)
					OR (g.decision IS NULL AND g.match_status = 'matched' AND g.match_email_id = e.id)
				)
			)
		ORDER BY e.sort_order, e.language;
	`, boardKey)
	if err != nil {
		return overview, err
	}
	defer rows.Close()
	for rows.Next() {
		var u unseenEmail
		if err := rows.Scan(&u.ID, &u.Title, &u.Stage, &u.SortOrder, &u.Language, &u.Variant, &u.AdaptationLabel, &u.RegionName, &u.SendTiming, &u.SendCondition); err != nil {
			return overview, err
		}
		overview.Unseen = append(overview.Unseen, u)
	}
	if err := rows.Err(); err != nil {
		return overview, err
	}
	rows.Close()

	rows, err = db.Query(ctx, `
		SELECT e.id, e.title, e.stage, e.sort_order, e.language, e.variant, e.adaptation_label,
			coalesce((SELECT r.name FROM board_regions r WHERE r.board_key = e.sequence AND r.key = e.region_key), ''), e.send_timing,
			e.send_condition, e.live_marked_at, COALESCE(e.live_marked_by, ''), e.live_note
		FROM emails e
		WHERE e.sequence = $1 AND e.archived_at IS NULL AND e.live_marked_at IS NOT NULL
		ORDER BY e.sort_order, e.language;
	`, boardKey)
	if err != nil {
		return overview, err
	}
	for rows.Next() {
		var m manualLiveEmail
		if err := rows.Scan(&m.ID, &m.Title, &m.Stage, &m.SortOrder, &m.Language, &m.Variant, &m.AdaptationLabel,
			&m.RegionName, &m.SendTiming, &m.SendCondition, &m.MarkedAt, &m.MarkedBy, &m.Note); err != nil {
			return overview, err
		}
		overview.ManualLive = append(overview.ManualLive, m)
	}
	return overview, rows.Err()
}

type groupDetail struct {
	groupItem
	SampleHTML string        `json:"sample_html"`
	SampleText string        `json:"sample_text"`
	Match      *matchedEmail `json:"match"`
}

type matchedEmail struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Language string `json:"language"`
	Stage    string `json:"stage"`
	HTML     string `json:"html"`
	Text     string `json:"text"`
}

func groupDetailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSuperAdmin(w, r); !ok {
			return
		}
		id := chi.URLParam(r, "id")

		var detail groupDetail
		var stagesJSON []byte
		var body string
		err := dbpool.QueryRow(r.Context(), `
			SELECT g.id, g.language, g.subject, g.normalized_text, g.send_count, g.first_sent_at, g.last_sent_at, g.stages,
				g.match_email_id, g.match_score, g.match_status, g.decision, g.decision_email_id, g.decided_by_email,
				g.decided_at, s.body_html
			FROM portal_email_groups g
			JOIN portal_sent_emails s ON s.activity_id = g.sample_activity_id
			WHERE g.id = $1;
		`, id).Scan(&detail.ID, &detail.Language, &detail.Subject, &detail.SampleText, &detail.SendCount,
			&detail.FirstSentAt, &detail.LastSentAt, &stagesJSON, &detail.MatchEmailID, &detail.MatchScore,
			&detail.MatchStatus, &detail.Decision, &detail.DecisionEmail, &detail.DecidedBy, &detail.DecidedAt, &body)
		if err != nil {
			http.Error(w, "group not found", http.StatusNotFound)
			return
		}
		_ = json.Unmarshal(stagesJSON, &detail.Stages)
		detail.SampleHTML = RepairTimelineHTML(body)

		emailID := detail.DecisionEmail
		if emailID == nil {
			emailID = detail.MatchEmailID
		}
		if requested := r.URL.Query().Get("email_id"); requested != "" {
			emailID = &requested
		}
		if emailID != nil {
			match, err := loadMatchedEmail(r.Context(), dbpool, *emailID)
			if err == nil {
				detail.Match = &match
			}
		}
		writeJSON(w, http.StatusOK, detail)
	}
}

func loadMatchedEmail(ctx context.Context, db *pgxpool.Pool, id string) (matchedEmail, error) {
	var m matchedEmail
	var template, preheader string
	var fieldsJSON []byte
	if err := db.QueryRow(ctx, `
		SELECT id, title, language, stage, template_html, editable_fields, COALESCE(preheader, '')
		FROM emails WHERE id = $1;
	`, id).Scan(&m.ID, &m.Title, &m.Language, &m.Stage, &template, &fieldsJSON, &preheader); err != nil {
		return m, err
	}
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return m, err
	}
	rendered, err := emailedit.RenderEditableHTMLWithMetadata(template, fields, emailedit.RenderMetadata{Preheader: preheader})
	if err != nil {
		return m, err
	}
	m.HTML, m.Text = rendered, NormalizedText(rendered)
	return m, nil
}

type decideRequest struct {
	Decision *string `json:"decision"`
	EmailID  *string `json:"email_id"`
}

func decideGroupHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireSuperAdmin(w, r)
		if !ok {
			return
		}
		var request decideRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if request.Decision != nil {
			switch *request.Decision {
			case "confirmed", "created":
				if request.EmailID == nil || strings.TrimSpace(*request.EmailID) == "" {
					http.Error(w, "email_id is required", http.StatusBadRequest)
					return
				}
			case "ignored":
				request.EmailID = nil
			default:
				http.Error(w, "decision must be confirmed, created, ignored or null", http.StatusBadRequest)
				return
			}
		} else {
			request.EmailID = nil
		}

		tag, err := dbpool.Exec(r.Context(), `
			UPDATE portal_email_groups
			SET decision = $2, decision_email_id = $3,
				decided_by_email = CASE WHEN $2::text IS NULL THEN NULL ELSE $4 END,
				decided_at = CASE WHEN $2::text IS NULL THEN NULL ELSE now() END
			WHERE id = $1;
		`, chi.URLParam(r, "id"), request.Decision, request.EmailID, user.Email)
		if err != nil {
			http.Error(w, "failed to save decision", http.StatusBadRequest)
			return
		}
		if tag.RowsAffected() == 0 {
			http.Error(w, "group not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type reconstructResponse struct {
	Reconstruction
	Subject            string `json:"subject"`
	Language           string `json:"language"`
	TemplateEmailID    string `json:"template_email_id"`
	TemplateEmailTitle string `json:"template_email_title"`
	SuggestedStage     string `json:"suggested_stage"`
}

// suggestServiceStage maps the portal stage the group was mostly sent from to
// the board stage that emails sent from that portal stage already live in.
func suggestServiceStage(ctx context.Context, db *pgxpool.Pool, groupID string) (string, error) {
	var stage string
	err := db.QueryRow(ctx, `
		WITH target AS (
			SELECT key AS portal_stage
			FROM portal_email_groups g, jsonb_each_text(g.stages)
			WHERE g.id = $1
			ORDER BY value::int DESC
			LIMIT 1
		)
		SELECT e.stage
		FROM portal_email_groups g
		CROSS JOIN target
		JOIN emails e ON e.id = COALESCE(g.decision_email_id, g.match_email_id)
		WHERE g.id <> $1
			AND g.board_key = (SELECT board_key FROM portal_email_groups WHERE id = $1)
			AND (g.decision IN ('confirmed', 'created') OR (g.decision IS NULL AND g.match_status = 'matched'))
			AND g.stages ? target.portal_stage
		GROUP BY e.stage
		ORDER BY sum((g.stages ->> target.portal_stage)::int) DESC
		LIMIT 1;
	`, groupID).Scan(&stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return stage, err
}

// reconstructGroupHandler rebuilds an editable email from a group's sent copy
// on the board email whose structure fits it best (or the one requested).
func reconstructGroupHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSuperAdmin(w, r); !ok {
			return
		}
		response, err := rebuildFromGroup(r.Context(), dbpool, chi.URLParam(r, "id"), r.URL.Query().Get("email_id"))
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "group not found", http.StatusNotFound)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "reconstruct group: %v\n", err)
			http.Error(w, "could not rebuild the email", http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, http.StatusOK, response)
	}
}

var errNoTemplate = errors.New("the board has no email with editable fields to use as a template")

// rebuildFromGroup picks the board template that fits the group's sent copy
// and rebuilds the email on it. emailID forces a specific template.
func rebuildFromGroup(ctx context.Context, db *pgxpool.Pool, groupID, emailID string) (reconstructResponse, error) {
	var boardKey, language, subject, body string
	if err := db.QueryRow(ctx, `
		SELECT g.board_key, g.language, g.subject, s.body_html
		FROM portal_email_groups g
		JOIN portal_sent_emails s ON s.activity_id = g.sample_activity_id
		WHERE g.id = $1;
	`, groupID).Scan(&boardKey, &language, &subject, &body); err != nil {
		return reconstructResponse{}, err
	}

	rows, err := db.Query(ctx, `
		SELECT id, title, language, template_html
		FROM emails
		WHERE sequence = $1 AND archived_at IS NULL AND editable_fields <> '{}'::jsonb
			AND ($2 = '' OR id::text = $2);
	`, boardKey, emailID)
	if err != nil {
		return reconstructResponse{}, err
	}
	type candidate struct {
		id, title, template string
		score               float64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var emailLanguage string
		if err := rows.Scan(&c.id, &c.title, &emailLanguage, &c.template); err != nil {
			rows.Close()
			return reconstructResponse{}, err
		}
		c.score = SkeletonSimilarity(body, c.template)
		if emailLanguage == language {
			c.score += 0.001 // same structure: prefer the same language
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if len(candidates) == 0 {
		return reconstructResponse{}, errNoTemplate
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

	// Structure alone can tie between template generations (e.g. one or
	// two signature fields), so try the closest few and keep the first
	// whose every field is found in the sent copy.
	var result Reconstruction
	var used candidate
	for i, c := range candidates {
		if i >= 8 {
			break
		}
		attempt, err := Reconstruct(body, c.template, subject, language)
		if err != nil {
			continue
		}
		if used.id == "" || (attempt.FromTemplate && !result.FromTemplate) {
			result, used = attempt, c
		}
		if attempt.FromTemplate {
			break
		}
	}
	if used.id == "" {
		return reconstructResponse{}, errors.New("no template could rebuild this email")
	}

	suggestedStage, err := suggestServiceStage(ctx, db, groupID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "suggest stage: %v\n", err)
	}
	return reconstructResponse{
		Reconstruction:     result,
		Subject:            subject,
		Language:           language,
		TemplateEmailID:    used.id,
		TemplateEmailTitle: used.title,
		SuggestedStage:     suggestedStage,
	}, nil
}
