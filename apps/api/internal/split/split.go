// Package split tells an email whose robots send different texts: each
// manager of a region has a robot of their own, and when one of them still
// holds an older text, the email goes out in more than one text at once.
package split

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WindowDays is how far back the sends are looked at. A robot sends only to
// deals entering the stage after it was set up, so the window must hold
// several sends.
const WindowDays = 14

// trail is how long the replaced text may still go out after the new one
// started: deals the robot took in before the re-upload.
const trail = 24 * time.Hour

// Text is one text (portal group) of an email that went out in the window.
type Text struct {
	GroupID string `json:"group_id"`
	// FirstSentAt and LastSentAt span every send the sync saw, also before
	// the window.
	FirstSentAt time.Time `json:"first_sent_at"`
	LastSentAt  time.Time `json:"last_sent_at"`
	// Sends counts those within the window.
	Sends int `json:"sends"`
}

// Texts returns the texts that still go out side by side, newest first, or
// nil when the email is not split. A text whose last send came before the
// newest text started (give or take the trail) was replaced, not left behind.
func Texts(texts []Text) []Text {
	if len(texts) < 2 {
		return nil
	}
	newest := texts[0].FirstSentAt
	for _, text := range texts[1:] {
		if text.FirstSentAt.After(newest) {
			newest = text.FirstSentAt
		}
	}
	kept := []Text{}
	for _, text := range texts {
		if !text.LastSentAt.Before(newest.Add(trail)) || text.FirstSentAt.Equal(newest) {
			kept = append(kept, text)
		}
	}
	if len(kept) < 2 {
		return nil
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].FirstSentAt.After(kept[j].FirstSentAt) })
	return kept
}

// RecentTextsSQL is a lateral subquery for a query over emails: the texts of
// the email that went out in the window, as a JSON array (NULL when none).
// Sends count for the email the same way the board's "Live" count ties them:
// confirmed by a person, or matched with no decision yet.
var RecentTextsSQL = `
	SELECT jsonb_agg(jsonb_build_object(
		'group_id', g.id, 'first_sent_at', g.first_sent_at, 'last_sent_at', g.last_sent_at, 'sends', w.sends
	) ORDER BY g.first_sent_at, g.id) AS texts
	FROM portal_email_groups g
	CROSS JOIN LATERAL (
		SELECT count(*)::int AS sends
		FROM portal_sent_emails s
		WHERE s.board_key = g.board_key
			AND s.fingerprint = g.fingerprint
			AND s.sent_at >= now() - interval '` + windowInterval + `'
	) w
	WHERE w.sends > 0
		AND ((g.decision IN ('confirmed', 'created') AND g.decision_email_id = emails.id)
			OR (g.decision IS NULL AND g.match_status = 'matched' AND g.match_email_id = emails.id))`

var windowInterval = strconv.Itoa(WindowDays) + " days"

// FromJSON decodes the texts RecentTextsSQL gives and keeps those that go out
// side by side (see Texts).
func FromJSON(raw []byte) ([]Text, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var texts []Text
	if err := json.Unmarshal(raw, &texts); err != nil {
		return nil, err
	}
	return Texts(texts), nil
}

// Email is an active email that goes out in more than one text.
type Email struct {
	ID        string
	Title     string
	BoardName string
	Texts     []Text
}

// Load finds the active emails that go out in more than one text, by board
// name and then in the board's order.
func Load(ctx context.Context, db *pgxpool.Pool) ([]Email, error) {
	rows, err := db.Query(ctx, `
		SELECT emails.id::text, emails.title, COALESCE(b.name, emails.sequence), recent.texts
		FROM emails
		LEFT JOIN boards b ON b.key = emails.sequence
		CROSS JOIN LATERAL (`+RecentTextsSQL+`) recent
		WHERE emails.archived_at IS NULL AND jsonb_array_length(recent.texts) > 1
		ORDER BY 3, emails.sort_order, emails.created_at;
	`)
	if err != nil {
		return nil, err
	}
	var emails []Email
	var email Email
	var raw []byte
	_, err = pgx.ForEachRow(rows, []any{&email.ID, &email.Title, &email.BoardName, &raw}, func() error {
		texts, err := FromJSON(raw)
		if err != nil || texts == nil {
			return err
		}
		email.Texts = texts
		emails = append(emails, email)
		return nil
	})
	return emails, err
}
