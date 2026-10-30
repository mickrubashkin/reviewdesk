package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The owner hears about the demo in a chat: a line when someone enters, and
// a report of what they did once they have been quiet for visitQuietAfter.

// Sender posts a message to the owner's chat.
type Sender interface {
	Send(ctx context.Context, text string) error
}

// Notifier gets the demo's messages; nil sends none.
var Notifier Sender

const (
	visitQuietAfter    = 30 * time.Minute
	visitReportsEvery  = 5 * time.Minute
	visitReportsPerRun = 20
	sendTimeout        = 15 * time.Second
)

// AnnounceVisit tells the owner a visitor has just entered. It does not wait
// for the chat.
func AnnounceVisit(entry VisitEntry) {
	if Notifier == nil {
		return
	}
	text := "Demo: new visitor · " + sourceLabel(visitSource(entry)) + " · " + deviceLabel(isMobile(entry.UserAgent))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		defer cancel()
		if err := Notifier.Send(ctx, text); err != nil {
			fmt.Fprintf(os.Stderr, "demo visit announcement: %v\n", err)
		}
	}()
}

// RunVisitReports reports, every few minutes until ctx ends, the visits
// that went quiet, with times in loc.
func RunVisitReports(ctx context.Context, db *pgxpool.Pool, loc *time.Location) {
	ticker := time.NewTicker(visitReportsEvery)
	defer ticker.Stop()
	for {
		if err := ReportQuietVisits(ctx, db, time.Now(), loc); err != nil {
			fmt.Fprintf(os.Stderr, "demo visit reports: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ReportQuietVisits sends the report of each unreported visit with nothing
// new since now minus visitQuietAfter, and marks it reported. A report that
// fails to send is tried again next time.
func ReportQuietVisits(ctx context.Context, db *pgxpool.Pool, now time.Time, loc *time.Location) error {
	if Notifier == nil {
		return nil
	}
	// The visitor's last sign of life: their last request (kept every ten
	// minutes) or their last change, whichever is later.
	rows, err := db.Query(ctx, `
		SELECT visitor_email, created_at, last_at, source, mobile, actions FROM (
			SELECT v.visitor_email, v.created_at, v.source, v.mobile,
				GREATEST(v.created_at, v.last_seen_at, u.last_seen_at,
					(SELECT max(created_at) FROM email_events WHERE actor_email = v.visitor_email)) AS last_at,
				COALESCE(v.actions, `+visitActionsSQL+`) AS actions
			FROM demo_visits v
			LEFT JOIN users u ON u.email = v.visitor_email
			WHERE v.reported_at IS NULL
		) visits
		WHERE last_at < $1
		ORDER BY created_at
		LIMIT $2;
	`, now.Add(-visitQuietAfter), visitReportsPerRun)
	if err != nil {
		return err
	}
	type quietVisit struct {
		email string
		visit Visit
	}
	visits, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (quietVisit, error) {
		var quiet quietVisit
		var lastAt time.Time
		var actions []byte
		if err := row.Scan(&quiet.email, &quiet.visit.CreatedAt, &lastAt, &quiet.visit.Source, &quiet.visit.Mobile, &actions); err != nil {
			return quietVisit{}, err
		}
		quiet.visit.LastSeenAt = &lastAt
		return quiet, json.Unmarshal(actions, &quiet.visit.Actions)
	})
	if err != nil {
		return err
	}
	for _, quiet := range visits {
		sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
		err := Notifier.Send(sendCtx, VisitReport(quiet.visit, loc))
		cancel()
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `UPDATE demo_visits SET reported_at = now() WHERE visitor_email = $1;`, quiet.email); err != nil {
			return err
		}
	}
	return nil
}

// VisitReport is the message about one visit: where from, when and how long,
// and what the visitor did.
func VisitReport(visit Visit, loc *time.Location) string {
	stayed := "a moment"
	if visit.LastSeenAt != nil {
		if minutes := int(visit.LastSeenAt.Sub(visit.CreatedAt).Round(time.Minute) / time.Minute); minutes > 0 {
			stayed = fmt.Sprintf("about %d min", minutes)
		}
	}
	return strings.Join([]string{
		"Demo visit · " + sourceLabel(visit.Source) + " · " + deviceLabel(visit.Mobile),
		visit.CreatedAt.In(loc).Format("Mon 15:04") + ", " + stayed,
		describeActions(visit.Actions),
	}, "\n")
}

// actionLabels names what the visitor did, in the order the demo guide goes.
var actionLabels = []struct {
	label   string
	actions []string
}{
	{"edited", []string{"email_updated"}},
	{"commented", []string{"comment_created", "comment_replied"}},
	{"resolved", []string{"comment_resolved"}},
	{"created", []string{"email_created", "email_duplicated", "email_adaptation_created"}},
	{"changed status", []string{"email_review_status_updated"}},
	{"used AI", []string{"ai_calls"}},
}

func describeActions(actions map[string]int) string {
	if len(actions) == 0 {
		return "Looked around, changed nothing."
	}
	parts := []string{}
	named := map[string]bool{"marked_ready": true}
	for _, item := range actionLabels {
		count := 0
		for _, action := range item.actions {
			count += actions[action]
			named[action] = true
		}
		if count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", item.label, count))
		}
	}
	other := 0
	for action, count := range actions {
		if !named[action] {
			other += count
		}
	}
	if other > 0 {
		parts = append(parts, fmt.Sprintf("other %d", other))
	}
	text := strings.Join(parts, " · ")
	if actions["marked_ready"] > 0 {
		if text != "" {
			text += " · "
		}
		text += "marked an email ready"
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

func sourceLabel(source string) string {
	if source == "" {
		return "direct"
	}
	return source
}

func deviceLabel(mobile bool) string {
	if mobile {
		return "mobile"
	}
	return "desktop"
}
