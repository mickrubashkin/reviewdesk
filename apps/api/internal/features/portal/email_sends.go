package portal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultSendDays = 90

// SentText is one text the portal sent for an email: a portal group, told
// apart by its fingerprint. A re-upload with a changed text starts a new one.
type SentText struct {
	GroupID  string `json:"group_id"`
	BoardKey string `json:"board_key"`
	Subject  string `json:"subject"`
	// FirstSentAt and LastSentAt span every send the sync saw, also before
	// the period asked for.
	FirstSentAt time.Time `json:"first_sent_at"`
	LastSentAt  time.Time `json:"last_sent_at"`
	// Sends counts those within the period.
	Sends int `json:"sends"`
}

type DailySends struct {
	Date    string `json:"date"`
	GroupID string `json:"group_id"`
	Sends   int    `json:"sends"`
}

// StageSends counts an email's sends from one deal stage of the portal, the
// stage the deal was in when the email went out.
type StageSends struct {
	StageID string `json:"stage_id"`
	Name    string `json:"name"`
	Sends   int    `json:"sends"`
	// Other marks a stage whose sends on the board mostly go to emails of
	// another board stage, when this email also went out from one of its own.
	Other bool `json:"other"`
}

// SendDeals counts the deals behind the sends in the period.
type SendDeals struct {
	Deals int `json:"deals"`
	// Repeated counts deals that got the email more than once: the deal came
	// back to the stage, or a robot repeats the email.
	Repeated int `json:"repeated"`
	// Doubled counts deals that got the same text twice within a day, which
	// two robots sending the same email would do.
	Doubled int `json:"doubled"`
}

// EmailSends are an email's sends per day and text over the last Days days,
// counted in the viewer's time zone. Since and Until are the first and last
// day of the period.
type EmailSends struct {
	Days   int          `json:"days"`
	Since  string       `json:"since"`
	Until  string       `json:"until"`
	Texts  []SentText   `json:"texts"`
	Daily  []DailySends `json:"daily"`
	Stages []StageSends `json:"stages"`
	Deals  SendDeals    `json:"deals"`
}

// tiedEmailID is the email a portal group's sends count for, the same way the
// board's "Live" count ties them: confirmed by a person, or matched with no
// decision yet.
const tiedEmailID = `CASE
	WHEN g.decision IN ('confirmed', 'created') THEN g.decision_email_id
	WHEN g.decision IS NULL AND g.match_status = 'matched' THEN g.match_email_id
END`

// LoadEmailSends counts the sends of the texts tied to the email (see
// tiedEmailID). Texts come oldest first, stages in the portal's order.
func LoadEmailSends(ctx context.Context, db *pgxpool.Pool, emailID string, days int, loc *time.Location, now time.Time) (EmailSends, error) {
	local := now.In(loc)
	since := time.Date(local.Year(), local.Month(), local.Day()-days+1, 0, 0, 0, 0, loc)
	result := EmailSends{
		Days:   days,
		Since:  since.Format(time.DateOnly),
		Until:  local.Format(time.DateOnly),
		Texts:  []SentText{},
		Daily:  []DailySends{},
		Stages: []StageSends{},
	}

	rows, err := db.Query(ctx, `
		SELECT g.id::text, g.board_key, g.subject, g.first_sent_at, g.last_sent_at,
			to_char(s.sent_at AT TIME ZONE $3, 'YYYY-MM-DD') AS day, count(*)::int
		FROM portal_email_groups g
		JOIN portal_sent_emails s
			ON s.board_key = g.board_key AND s.fingerprint = g.fingerprint AND s.sent_at >= $2
		WHERE `+tiedEmailID+` = $1
		GROUP BY g.id, day
		ORDER BY g.first_sent_at, g.id, day;
	`, emailID, since, loc.String())
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var text SentText
		var day DailySends
		if err := rows.Scan(&text.GroupID, &text.BoardKey, &text.Subject, &text.FirstSentAt, &text.LastSentAt,
			&day.Date, &day.Sends); err != nil {
			return result, err
		}
		day.GroupID = text.GroupID
		if n := len(result.Texts); n == 0 || result.Texts[n-1].GroupID != text.GroupID {
			result.Texts = append(result.Texts, text)
		}
		result.Texts[len(result.Texts)-1].Sends += day.Sends
		result.Daily = append(result.Daily, day)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if result.Stages, err = loadSendStages(ctx, db, emailID, since); err != nil {
		return result, err
	}
	result.Deals, err = loadSendDeals(ctx, db, emailID, since)
	return result, err
}

// loadSendStages counts the email's sends since then by the deal's stage at
// send time. Board and portal stages don't map one to one (one board stage
// can cover several portal stages), so a portal stage belongs to the board
// stage whose emails on the email's board got most of its sends. Sends from a
// stage that belongs elsewhere are marked only when the email also went out
// from a stage of its own: an email sent only from elsewhere more likely sits
// in a board stage named differently.
func loadSendStages(ctx context.Context, db *pgxpool.Pool, emailID string, since time.Time) ([]StageSends, error) {
	rows, err := db.Query(ctx, `
		WITH target AS (
			SELECT sequence, stage FROM emails WHERE id = $1
		), tied AS (
			SELECT s.board_key, s.stage_id, s.sent_at, e.id AS email_id, e.stage
			FROM portal_email_groups g
			JOIN emails e ON e.id = `+tiedEmailID+`
			JOIN portal_sent_emails s ON s.board_key = g.board_key AND s.fingerprint = g.fingerprint
			WHERE e.sequence IS NOT DISTINCT FROM (SELECT sequence FROM target)
				AND (e.archived_at IS NULL OR e.id = $1)
		), owners AS (
			SELECT DISTINCT ON (board_key, stage_id) board_key, stage_id, stage
			FROM tied
			WHERE stage_id IS NOT NULL
			GROUP BY board_key, stage_id, stage
			ORDER BY board_key, stage_id, count(*) DESC, stage
		)
		SELECT COALESCE(t.stage_id, ''), COALESCE(ps.name, t.stage_id, ''), count(*)::int,
			COALESCE(o.stage = (SELECT stage FROM target), false) AS own
		FROM tied t
		LEFT JOIN owners o ON o.board_key = t.board_key AND o.stage_id = t.stage_id
		LEFT JOIN portal_stages ps ON ps.board_key = t.board_key AND ps.stage_id = t.stage_id
		WHERE t.email_id = $1 AND t.sent_at >= $2
		GROUP BY t.stage_id, ps.name, ps.sort, o.stage
		ORDER BY ps.sort NULLS LAST, t.stage_id;
	`, emailID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stages := []StageSends{}
	var own []bool
	anyOwn := false
	for rows.Next() {
		var stage StageSends
		var isOwn bool
		if err := rows.Scan(&stage.StageID, &stage.Name, &stage.Sends, &isOwn); err != nil {
			return nil, err
		}
		stages = append(stages, stage)
		own = append(own, isOwn)
		anyOwn = anyOwn || isOwn
	}
	for i := range stages {
		stages[i].Other = anyOwn && !own[i] && stages[i].StageID != ""
	}
	return stages, rows.Err()
}

// loadSendDeals counts the deals the email went to since then.
func loadSendDeals(ctx context.Context, db *pgxpool.Pool, emailID string, since time.Time) (SendDeals, error) {
	var deals SendDeals
	err := db.QueryRow(ctx, `
		WITH sends AS (
			SELECT s.deal_id,
				s.sent_at - lag(s.sent_at) OVER (PARTITION BY s.deal_id, s.fingerprint ORDER BY s.sent_at) < interval '1 day' AS doubled
			FROM portal_email_groups g
			JOIN portal_sent_emails s ON s.board_key = g.board_key AND s.fingerprint = g.fingerprint AND s.sent_at >= $2
			WHERE `+tiedEmailID+` = $1
		), per_deal AS (
			SELECT count(*) AS sends, COALESCE(bool_or(doubled), false) AS doubled FROM sends GROUP BY deal_id
		)
		SELECT count(*)::int, (count(*) FILTER (WHERE sends > 1))::int, (count(*) FILTER (WHERE doubled))::int
		FROM per_deal;
	`, emailID, since).Scan(&deals.Deals, &deals.Repeated, &deals.Doubled)
	return deals, err
}

// parseSendDays reads ?days=, at most the days the sync keeps sends.
func parseSendDays(value string) int {
	days, err := strconv.Atoi(value)
	if err != nil || days <= 0 {
		return defaultSendDays
	}
	return min(days, SendRetentionDays)
}

// parseTimeZone reads ?tz=, an IANA name from the browser; UTC otherwise.
func parseTimeZone(value string) *time.Location {
	if value == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(value)
	if err != nil {
		return time.UTC
	}
	return loc
}

func emailSendsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		emailID := chi.URLParam(r, "id")
		var found bool
		if err := dbpool.QueryRow(r.Context(), `SELECT true FROM emails WHERE id::text = $1;`, emailID).Scan(&found); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "email not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to load sends", http.StatusInternalServerError)
			return
		}
		query := r.URL.Query()
		sends, err := LoadEmailSends(r.Context(), dbpool, emailID, parseSendDays(query.Get("days")),
			parseTimeZone(query.Get("tz")), time.Now())
		if err != nil {
			fmt.Fprintf(os.Stderr, "email sends: %v\n", err)
			http.Error(w, "failed to load sends", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, sends)
	}
}
