// Package digest sends the owner a morning message about the day before:
// who was in, from when to when, and what they did on which board.
package digest

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sender posts a message to the owner's chat.
type Sender interface {
	Send(ctx context.Context, text string) error
}

const (
	checkEvery    = 10 * time.Minute
	sendTimeout   = 15 * time.Second
	keepPresence  = 90 * 24 * time.Hour
	maxTextLength = 4000 // Telegram takes up to 4096 characters.
)

// Run sends, until ctx ends, the digest of each day once the next day has
// reached hour in loc.
func Run(ctx context.Context, db *pgxpool.Pool, sender Sender, loc *time.Location, hour int) {
	ticker := time.NewTicker(checkEvery)
	defer ticker.Stop()
	for {
		if err := SendDue(ctx, db, sender, time.Now(), loc, hour); err != nil {
			fmt.Fprintf(os.Stderr, "activity digest: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// SendDue sends the digest of the day before now once now has reached hour
// in loc, unless it went out already. A digest that fails to send is tried
// again next time.
func SendDue(ctx context.Context, db *pgxpool.Pool, sender Sender, now time.Time, loc *time.Location, hour int) error {
	local := now.In(loc)
	if local.Hour() < hour {
		return nil
	}
	end := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	start := end.AddDate(0, 0, -1)
	day := start.Format(time.DateOnly)

	// Claim the day first, so that a second instance does not send it too.
	claimed, err := db.Exec(ctx, `INSERT INTO activity_digests (day) VALUES ($1) ON CONFLICT DO NOTHING;`, day)
	if err != nil {
		return err
	}
	if claimed.RowsAffected() == 0 {
		return nil
	}
	err = func() error {
		people, err := LoadDay(ctx, db, start, end)
		if err != nil {
			return err
		}
		sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
		defer cancel()
		return sender.Send(sendCtx, Format(start, people, loc))
	}()
	if err != nil {
		if _, unclaimErr := db.Exec(context.WithoutCancel(ctx), `DELETE FROM activity_digests WHERE day = $1;`, day); unclaimErr != nil {
			return fmt.Errorf("%w (and the day stays marked sent: %v)", err, unclaimErr)
		}
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM user_presence WHERE seen_at < $1;`, now.Add(-keepPresence))
	return err
}

// Person is what one person did in the day.
type Person struct {
	Email   string
	Name    string
	FirstAt time.Time
	LastAt  time.Time
	// Boards holds the counts of email events by action, per board name.
	Boards map[string]map[string]int
}

func (p *Person) seen(at time.Time) {
	if p.FirstAt.IsZero() || at.Before(p.FirstAt) {
		p.FirstAt = at
	}
	if at.After(p.LastAt) {
		p.LastAt = at
	}
}

// LoadDay returns everyone seen or acting between start and end, earliest
// first: their first and last sign of life and their actions per board.
func LoadDay(ctx context.Context, db *pgxpool.Pool, start, end time.Time) ([]Person, error) {
	byEmail := map[string]*Person{}
	person := func(email, name string) *Person {
		if byEmail[email] == nil {
			byEmail[email] = &Person{Email: email, Boards: map[string]map[string]int{}}
		}
		if name != "" {
			byEmail[email].Name = name
		}
		return byEmail[email]
	}

	rows, err := db.Query(ctx, `
		SELECT u.email, u.name, min(p.seen_at), max(p.seen_at)
		FROM user_presence p
		JOIN users u ON u.id = p.user_id
		WHERE p.seen_at >= $1 AND p.seen_at < $2
		GROUP BY u.email, u.name;
	`, start, end)
	if err != nil {
		return nil, err
	}
	var email, name string
	var first, last time.Time
	if _, err := pgx.ForEachRow(rows, []any{&email, &name, &first, &last}, func() error {
		p := person(email, name)
		p.seen(first)
		p.seen(last)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load presence: %w", err)
	}

	// An email event belongs to its email's board, a board event to the
	// board in its metadata.
	rows, err = db.Query(ctx, `
		SELECT ev.actor_email, COALESCE(u.name, ''),
			COALESCE(b.name, NULLIF(board_key, ''), 'no board'),
			ev.action, count(*), min(ev.created_at), max(ev.created_at)
		FROM email_events ev
		LEFT JOIN emails e ON e.id = ev.email_id
		CROSS JOIN LATERAL (
			SELECT COALESCE(NULLIF(trim(e.sequence), ''), ev.metadata->>'board_key') AS board_key
		) keys
		LEFT JOIN boards b ON b.key = keys.board_key
		LEFT JOIN users u ON u.email = ev.actor_email
		WHERE ev.created_at >= $1 AND ev.created_at < $2
		GROUP BY 1, 2, 3, 4;
	`, start, end)
	if err != nil {
		return nil, err
	}
	var board, action string
	var count int
	if _, err := pgx.ForEachRow(rows, []any{&email, &name, &board, &action, &count, &first, &last}, func() error {
		p := person(email, name)
		p.seen(first)
		p.seen(last)
		if p.Boards[board] == nil {
			p.Boards[board] = map[string]int{}
		}
		p.Boards[board][action] += count
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load email events: %w", err)
	}

	people := make([]Person, 0, len(byEmail))
	for _, p := range byEmail {
		people = append(people, *p)
	}
	sort.Slice(people, func(i, j int) bool {
		if !people[i].FirstAt.Equal(people[j].FirstAt) {
			return people[i].FirstAt.Before(people[j].FirstAt)
		}
		return people[i].Email < people[j].Email
	})
	return people, nil
}

// actionLabels names the email events, in the order they are listed.
var actionLabels = []struct {
	label   string
	actions []string
}{
	{"edited", []string{"email_updated", "email_planning_updated"}},
	{"commented", []string{"comment_created", "comment_replied"}},
	{"resolved", []string{"comment_resolved"}},
	{"created", []string{"email_created", "email_duplicated", "email_adaptation_created"}},
	{"changed status", []string{"email_review_status_updated", "email_area_approval_updated"}},
	{"uploaded", []string{"email_uploaded"}},
	{"marked live", []string{"email_live_marked"}},
	{"archived", []string{"email_archived"}},
}

// Format is the digest of the day that starts at day: one block per person,
// with times in loc.
func Format(day time.Time, people []Person, loc *time.Location) string {
	title := "ReviewDesk · " + day.In(loc).Format("Mon, Jan 2")
	if len(people) == 0 {
		return title + "\nNobody was in."
	}
	lines := []string{fmt.Sprintf("%s · %d %s", title, len(people), plural(len(people), "person", "people"))}
	for i, p := range people {
		block := []string{"", personLine(p, loc)}
		boards := make([]string, 0, len(p.Boards))
		for board := range p.Boards {
			boards = append(boards, board)
		}
		sort.Strings(boards)
		for _, board := range boards {
			block = append(block, "  "+board+": "+describeActions(p.Boards[board]))
		}
		if len(boards) == 0 {
			block = append(block, "  Looked around, changed nothing.")
		}
		if length(lines)+length(block) > maxTextLength {
			lines = append(lines, "", fmt.Sprintf("…and %d more.", len(people)-i))
			break
		}
		lines = append(lines, block...)
	}
	return strings.Join(lines, "\n")
}

func personLine(p Person, loc *time.Location) string {
	who := p.Email
	if p.Name != "" {
		who = p.Name
	}
	first, last := p.FirstAt.In(loc).Format("15:04"), p.LastAt.In(loc).Format("15:04")
	if first == last {
		return who + " · " + first
	}
	return who + " · " + first + "–" + last
}

func describeActions(actions map[string]int) string {
	parts := []string{}
	named := map[string]bool{}
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
	return strings.Join(parts, " · ")
}

func length(lines []string) int {
	n := 0
	for _, line := range lines {
		n += len([]rune(line)) + 1
	}
	return n
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
