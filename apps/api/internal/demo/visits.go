package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Every demo entry is recorded in demo_visits, so the demo has a history of
// who came from where and what they did, while sandboxes and their events
// are deleted after a day. The actions are copied into the visit when its
// sandbox goes; until then they are read from the live events.

// visitActionsSQL counts what the visitor of the demo_visits row v did: email
// events by action, marked_ready for a status change to ready (the last step
// of the demo guide), and ai_calls for OpenAI calls.
const visitActionsSQL = `(
	SELECT COALESCE(jsonb_object_agg(kind, n), '{}'::jsonb) FROM (
		SELECT
			CASE WHEN action = 'email_review_status_updated' AND changes->'review_status'->>'after' = 'ready'
				THEN 'marked_ready' ELSE action END AS kind,
			count(*) AS n
		FROM email_events WHERE actor_email = v.visitor_email
		GROUP BY 1
		UNION ALL
		SELECT 'ai_calls', count(*) FROM demo_ai_calls WHERE visitor_email = v.visitor_email
		HAVING count(*) > 0
	) counts
)`

const maxVisitSourceLength = 100

// VisitEntry is what the browser tells about a demo entry.
type VisitEntry struct {
	Referrer  string
	UTMSource string
	UserAgent string
}

// RecordVisit adds the visit of a new sandbox visitor.
func RecordVisit(ctx context.Context, tx pgx.Tx, visitor Visitor, entry VisitEntry) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO demo_visits (visitor_email, source, mobile) VALUES ($1, $2, $3);
	`, visitor.Email, visitSource(entry), isMobile(entry.UserAgent))
	if err != nil {
		return fmt.Errorf("record demo visit: %w", err)
	}
	return nil
}

// visitSource names where a visitor came from: utm_source when the link has
// one, else the referring site, with every LinkedIn address as "linkedin".
// Empty means a direct visit.
func visitSource(entry VisitEntry) string {
	source := strings.ToLower(strings.TrimSpace(entry.UTMSource))
	if source == "" {
		if parsed, err := url.Parse(strings.TrimSpace(entry.Referrer)); err == nil {
			source = strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
		}
	}
	if strings.Contains(source, "linkedin") || source == "lnkd.in" {
		return "linkedin"
	}
	if len(source) > maxVisitSourceLength {
		source = source[:maxVisitSourceLength]
	}
	return source
}

func isMobile(userAgent string) bool {
	return strings.Contains(userAgent, "Mobi") || strings.Contains(userAgent, "Android")
}

// keepVisitActions copies into their visits what the visitors of the given
// sandboxes did, before the sandboxes and their events are deleted.
func keepVisitActions(ctx context.Context, tx pgx.Tx, keys []string) error {
	_, err := tx.Exec(ctx, `
		UPDATE demo_visits v
		SET actions = `+visitActionsSQL+`, last_seen_at = u.last_seen_at
		FROM users u
		WHERE u.email = v.visitor_email
			AND v.actions IS NULL
			AND u.board_scoped AND u.managed_boards <@ $1;
	`, keys)
	if err != nil {
		return fmt.Errorf("keep demo visit actions: %w", err)
	}
	return nil
}

// Visit is one demo entry with what the visitor did.
type Visit struct {
	CreatedAt  time.Time
	LastSeenAt *time.Time
	Source     string
	Mobile     bool
	Actions    map[string]int
}

// Active reports whether the visitor changed anything or used the AI.
func (v Visit) Active() bool {
	return len(v.Actions) > 0
}

func (v Visit) Commented() bool {
	return v.Actions["comment_created"]+v.Actions["comment_replied"] > 0
}

func (v Visit) Edited() bool {
	return v.Actions["email_updated"] > 0
}

func (v Visit) Resolved() bool {
	return v.Actions["comment_resolved"] > 0
}

// MarkedReady reports whether the visitor reached the last step of the guide.
func (v Visit) MarkedReady() bool {
	return v.Actions["marked_ready"] > 0
}

func (v Visit) UsedAI() bool {
	return v.Actions["ai_calls"] > 0
}

// ListVisits returns the visits since the given time, oldest first.
func ListVisits(ctx context.Context, db *pgxpool.Pool, since time.Time) ([]Visit, error) {
	rows, err := db.Query(ctx, `
		SELECT v.created_at, COALESCE(v.last_seen_at, u.last_seen_at), v.source, v.mobile,
			COALESCE(v.actions, `+visitActionsSQL+`)
		FROM demo_visits v
		LEFT JOIN users u ON u.email = v.visitor_email
		WHERE v.created_at >= $1
		ORDER BY v.created_at;
	`, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Visit, error) {
		var visit Visit
		var actions []byte
		if err := row.Scan(&visit.CreatedAt, &visit.LastSeenAt, &visit.Source, &visit.Mobile, &actions); err != nil {
			return Visit{}, err
		}
		if err := json.Unmarshal(actions, &visit.Actions); err != nil {
			return Visit{}, err
		}
		return visit, nil
	})
}

// VisitCounts sums up a group of visits: how many there were and how many
// went how far.
type VisitCounts struct {
	Label       string
	Visitors    int
	LinkedIn    int
	Mobile      int
	Active      int
	Edited      int
	Commented   int
	Resolved    int
	MarkedReady int
	UsedAI      int
}

func (c *VisitCounts) add(visit Visit) {
	c.Visitors++
	for _, counted := range []struct {
		count *int
		yes   bool
	}{
		{&c.LinkedIn, visit.Source == "linkedin"},
		{&c.Mobile, visit.Mobile},
		{&c.Active, visit.Active()},
		{&c.Edited, visit.Edited()},
		{&c.Commented, visit.Commented()},
		{&c.Resolved, visit.Resolved()},
		{&c.MarkedReady, visit.MarkedReady()},
		{&c.UsedAI, visit.UsedAI()},
	} {
		if counted.yes {
			*counted.count++
		}
	}
}

// CountVisitsByDay groups visits by their day in loc, newest day first, and
// adds a total.
func CountVisitsByDay(visits []Visit, loc *time.Location) (days []VisitCounts, total VisitCounts) {
	total.Label = "Total"
	byDay := map[string]*VisitCounts{}
	for _, visit := range visits {
		day := visit.CreatedAt.In(loc).Format("2006-01-02 Mon")
		if byDay[day] == nil {
			byDay[day] = &VisitCounts{Label: day}
		}
		byDay[day].add(visit)
		total.add(visit)
	}
	for _, counts := range byDay {
		days = append(days, *counts)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Label > days[j].Label })
	return days, total
}

// CountVisitsBySource returns how many visits each source brought, most first.
func CountVisitsBySource(visits []Visit) []VisitCounts {
	bySource := map[string]*VisitCounts{}
	for _, visit := range visits {
		source := visit.Source
		if source == "" {
			source = "direct"
		}
		if bySource[source] == nil {
			bySource[source] = &VisitCounts{Label: source}
		}
		bySource[source].add(visit)
	}
	sources := make([]VisitCounts, 0, len(bySource))
	for _, counts := range bySource {
		sources = append(sources, *counts)
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Visitors != sources[j].Visitors {
			return sources[i].Visitors > sources[j].Visitors
		}
		return sources[i].Label < sources[j].Label
	})
	return sources
}
