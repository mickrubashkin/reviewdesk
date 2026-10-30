package demo

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
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

// visitHistorySQL gives, for the demo_visits row v, which visit of its
// browser it is (1 for the first, and for a browser that keeps no id) and
// when that browser first came.
const visitHistorySQL = `
	CASE WHEN v.visitor_id = '' THEN 1 ELSE (
		SELECT count(*) FROM demo_visits p WHERE p.visitor_id = v.visitor_id AND p.created_at <= v.created_at
	) END AS number,
	CASE WHEN v.visitor_id = '' THEN v.created_at ELSE (
		SELECT min(created_at) FROM demo_visits p WHERE p.visitor_id = v.visitor_id
	) END AS first_at`

const maxVisitSourceLength = 100

var (
	visitorIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)
	timeZonePattern  = regexp.MustCompile(`^[A-Za-z0-9_+/-]{1,64}$`)
	languagePattern  = regexp.MustCompile(`^[A-Za-z0-9-]{1,35}$`)
)

// VisitEntry is what the browser tells about a demo entry.
type VisitEntry struct {
	Referrer  string
	UTMSource string
	UserAgent string
	// VisitorID is a random id the browser keeps across visits.
	VisitorID string
	// OwnerKey is DEMO_OWNER_KEY in the owner's own browser.
	OwnerKey string
	TimeZone string
	Language string
}

// RecordVisit adds the visit of a new sandbox visitor and returns it, with
// which visit of the browser it is.
func RecordVisit(ctx context.Context, tx pgx.Tx, visitor Visitor, entry VisitEntry) (Visit, error) {
	visit := Visit{
		Source:   visitSource(entry),
		Mobile:   isMobile(entry.UserAgent),
		Owner:    isOwner(entry.OwnerKey),
		TimeZone: browserValue(entry.TimeZone, timeZonePattern),
		Language: browserValue(entry.Language, languagePattern),
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO demo_visits (visitor_email, source, mobile, visitor_id, owner, time_zone, language)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at;
	`, visitor.Email, visit.Source, visit.Mobile, browserValue(entry.VisitorID, visitorIDPattern),
		visit.Owner, visit.TimeZone, visit.Language).Scan(&visit.CreatedAt)
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT `+visitHistorySQL+` FROM demo_visits v WHERE v.visitor_email = $1;`,
			visitor.Email).Scan(&visit.Number, &visit.FirstAt)
	}
	if err != nil {
		return Visit{}, fmt.Errorf("record demo visit: %w", err)
	}
	return visit, nil
}

// isOwner reports whether key is the owner's DEMO_OWNER_KEY.
func isOwner(key string) bool {
	ownerKey := strings.TrimSpace(os.Getenv("DEMO_OWNER_KEY"))
	return ownerKey != "" && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(key)), []byte(ownerKey)) == 1
}

// browserValue is what the browser sent when it looks like what is asked
// for, else empty.
func browserValue(value string, pattern *regexp.Regexp) string {
	value = strings.TrimSpace(value)
	if !pattern.MatchString(value) {
		return ""
	}
	return value
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
	// Owner marks the owner's own entry.
	Owner    bool
	TimeZone string
	Language string
	// Number is which visit of the browser this is, 1 for the first;
	// FirstAt is when the browser first came.
	Number  int
	FirstAt time.Time
	Actions map[string]int
}

// Returning reports whether the browser has been to the demo before.
func (v Visit) Returning() bool {
	return v.Number > 1
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

// ListVisits returns the visits since the given time, oldest first, without
// the owner's own.
func ListVisits(ctx context.Context, db *pgxpool.Pool, since time.Time) ([]Visit, error) {
	rows, err := db.Query(ctx, `
		SELECT v.created_at, COALESCE(v.last_seen_at, u.last_seen_at), v.source, v.mobile,
			v.time_zone, v.language, `+visitHistorySQL+`,
			COALESCE(v.actions, `+visitActionsSQL+`)
		FROM demo_visits v
		LEFT JOIN users u ON u.email = v.visitor_email
		WHERE v.created_at >= $1 AND NOT v.owner
		ORDER BY v.created_at;
	`, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Visit, error) {
		var visit Visit
		var actions []byte
		if err := row.Scan(&visit.CreatedAt, &visit.LastSeenAt, &visit.Source, &visit.Mobile,
			&visit.TimeZone, &visit.Language, &visit.Number, &visit.FirstAt, &actions); err != nil {
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
	Returning   int
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
		{&c.Returning, visit.Returning()},
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
