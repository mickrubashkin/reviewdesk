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

// EmailSends are an email's sends per day and text over the last Days days,
// counted in the viewer's time zone. Since and Until are the first and last
// day of the period.
type EmailSends struct {
	Days  int          `json:"days"`
	Since string       `json:"since"`
	Until string       `json:"until"`
	Texts []SentText   `json:"texts"`
	Daily []DailySends `json:"daily"`
}

// LoadEmailSends counts the sends of the texts tied to the email the same way
// the board's "Live" count does: confirmed by a person, or matched with no
// decision yet. Texts come oldest first.
func LoadEmailSends(ctx context.Context, db *pgxpool.Pool, emailID string, days int, loc *time.Location, now time.Time) (EmailSends, error) {
	local := now.In(loc)
	since := time.Date(local.Year(), local.Month(), local.Day()-days+1, 0, 0, 0, 0, loc)
	result := EmailSends{
		Days:  days,
		Since: since.Format(time.DateOnly),
		Until: local.Format(time.DateOnly),
		Texts: []SentText{},
		Daily: []DailySends{},
	}

	rows, err := db.Query(ctx, `
		SELECT g.id::text, g.board_key, g.subject, g.first_sent_at, g.last_sent_at,
			to_char(s.sent_at AT TIME ZONE $3, 'YYYY-MM-DD') AS day, count(*)::int
		FROM portal_email_groups g
		JOIN portal_sent_emails s
			ON s.board_key = g.board_key AND s.fingerprint = g.fingerprint AND s.sent_at >= $2
		WHERE (g.decision IN ('confirmed', 'created') AND g.decision_email_id = $1)
			OR (g.decision IS NULL AND g.match_status = 'matched' AND g.match_email_id = $1)
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
	return result, rows.Err()
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
