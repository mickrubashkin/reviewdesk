// Command oldcleanup sorts out the "old" email variants of a board using the
// portal sync: unsent ones are archived, sent ones without a new version
// become the main version, and sent ones with a new version stay marked as
// still in the Bitrix24 robot. It prints the plan and changes nothing unless
// -apply is given; then everything happens in one transaction and each email
// gets an event in its history.
//
//	go run ./cmd/oldcleanup                        # dry run, reads DATABASE_URL
//	go run ./cmd/oldcleanup -apply -actor you@company.com
//	go run ./cmd/oldcleanup -archive-ids id1,id2   # also archive these emails
//	go run ./cmd/oldcleanup -promote-ids id1 -promote-note "Sent from the partner admin panel"  # also promote these
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

func main() {
	board := flag.String("board", "onboarding", "board key")
	apply := flag.Bool("apply", false, "write the changes; without it this is a dry run")
	actor := flag.String("actor", "", "super admin email recorded as the one who made the changes (required with -apply)")
	archiveIDs := flag.String("archive-ids", "", "comma-separated ids of other emails to archive")
	promoteIDs := flag.String("promote-ids", "", "comma-separated ids of other old emails to make the main version and mark live")
	promoteNote := flag.String("promote-note", "", "live note for -promote-ids, e.g. where the email is sent from")
	flag.Parse()

	extra := extraSteps{archiveIDs: splitIDs(*archiveIDs), promoteIDs: splitIDs(*promoteIDs), promoteNote: strings.TrimSpace(*promoteNote)}
	if len(extra.promoteIDs) > 0 && extra.promoteNote == "" {
		fmt.Fprintln(os.Stderr, "oldcleanup: -promote-note is required with -promote-ids")
		os.Exit(1)
	}
	if err := run(*board, *apply, *actor, extra); err != nil {
		fmt.Fprintf(os.Stderr, "oldcleanup: %v\n", err)
		os.Exit(1)
	}
}

func splitIDs(value string) []string {
	var ids []string
	for _, id := range strings.Split(value, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

type extraSteps struct {
	archiveIDs  []string
	promoteIDs  []string
	promoteNote string
}

func run(board string, apply bool, actorEmail string, extra extraSteps) error {
	_ = godotenv.Load("../../.env")
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer dbpool.Close()

	boardEmails, err := loadEmails(ctx, dbpool, board)
	if err != nil {
		return err
	}
	toArchive, err := loadByIDs(ctx, dbpool, extra.archiveIDs)
	if err != nil {
		return err
	}
	toPromote, err := loadByIDs(ctx, dbpool, extra.promoteIDs)
	if err != nil {
		return err
	}
	listed := map[string]bool{}
	for _, email := range append(toArchive, toPromote...) {
		listed[email.ID] = true
	}
	var steps []Step
	for _, step := range BuildPlan(boardEmails) {
		if !listed[step.Email.ID] {
			steps = append(steps, step)
		}
	}
	for _, email := range toArchive {
		steps = append(steps, Step{Email: email, Action: ActionArchive})
	}
	for _, email := range toPromote {
		steps = append(steps, Step{Email: email, Action: ActionPromote, Note: extra.promoteNote})
	}
	WriteReport(os.Stdout, steps)
	if !apply {
		fmt.Println("\nDry run: nothing was changed. Run with -apply -actor <email> to make these changes.")
		return nil
	}
	if actorEmail == "" {
		return fmt.Errorf("-actor is required with -apply")
	}
	return applySteps(ctx, dbpool, steps, actorEmail)
}

const emailColumns = `
	e.id, e.slug, e.title, coalesce(e.stage, ''), e.sort_order, e.language, e.variant, coalesce(e.adaptation_key, ''),
	coalesce(s.sends, 0), s.last_sent, e.live_marked_at IS NOT NULL`

const sendsJoin = `
	LEFT JOIN (
		SELECT coalesce(decision_email_id, match_email_id) AS email_id, sum(send_count)::int AS sends, max(last_sent_at) AS last_sent
		FROM portal_email_groups
		WHERE (decision IS NULL AND match_status IN ('matched', 'differs')) OR decision IN ('confirmed', 'created')
		GROUP BY 1
	) s ON s.email_id = e.id`

func loadEmails(ctx context.Context, dbpool *pgxpool.Pool, board string) ([]Email, error) {
	return queryEmails(ctx, dbpool, `SELECT `+emailColumns+` FROM emails e `+sendsJoin+`
		WHERE e.sequence = $1 AND e.archived_at IS NULL AND e.variant IN ('old', 'new')
		ORDER BY e.sort_order, e.language, e.variant;`, board)
}

func loadByIDs(ctx context.Context, dbpool *pgxpool.Pool, ids []string) ([]Email, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	found, err := queryEmails(ctx, dbpool, `SELECT `+emailColumns+` FROM emails e `+sendsJoin+`
		WHERE e.id::text = ANY($1) AND e.archived_at IS NULL;`, ids)
	if err != nil {
		return nil, err
	}
	if len(found) != len(ids) {
		return nil, fmt.Errorf("found %d of %d -archive-ids among active emails", len(found), len(ids))
	}
	return found, nil
}

func queryEmails(ctx context.Context, dbpool *pgxpool.Pool, query string, arg any) ([]Email, error) {
	rows, err := dbpool.Query(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Email
	for rows.Next() {
		var e Email
		if err := rows.Scan(&e.ID, &e.Slug, &e.Title, &e.Stage, &e.SortOrder, &e.Language, &e.Variant, &e.AdaptationKey, &e.Sends, &e.LastSent, &e.Live); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func applySteps(ctx context.Context, dbpool *pgxpool.Pool, steps []Step, actorEmail string) error {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var actorID string
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1) AND role = 'super_admin';`, actorEmail).Scan(&actorID); err != nil {
		return fmt.Errorf("actor %s is not a super admin: %w", actorEmail, err)
	}

	applied := 0
	for _, step := range steps {
		if step.Action == ActionCheck {
			continue
		}
		applied++
		e := step.Email
		var query, action, note string
		var args []any
		switch step.Action {
		case ActionArchive:
			query = `UPDATE emails SET archived_at = now(), archived_by = $2 WHERE id = $1 AND archived_at IS NULL;`
			action = "email_archived"
			args = []any{e.ID, actorID}
		case ActionPromote:
			query = `UPDATE emails SET variant = 'new', live_marked_at = now(), live_marked_by = $2, live_note = $3 WHERE id = $1 AND archived_at IS NULL AND variant = 'old';`
			action, note = "email_live_marked", promotedNote
			if step.Note != "" {
				note = step.Note
			}
			args = []any{e.ID, actorEmail, note}
		case ActionStillInRobot:
			query = `UPDATE emails SET live_marked_at = now(), live_marked_by = $2, live_note = $3 WHERE id = $1 AND archived_at IS NULL AND variant = 'old';`
			action, note = "email_live_marked", stillInRobotNote
			args = []any{e.ID, actorEmail, note}
		}
		tag, err := tx.Exec(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("%s %s: %w", step.Action, e.ID, err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("%s %s: email changed since the plan was made", step.Action, e.ID)
		}
		id, slug, title := e.ID, e.Slug, e.Title
		if err := emails.InsertEmailEvent(ctx, tx, emails.EmailEventParam{
			ActorUserID: actorID,
			ActorEmail:  actorEmail,
			Action:      action,
			EmailID:     &id,
			EmailSlug:   &slug,
			EmailTitle:  &title,
			Metadata:    map[string]any{"source": "oldcleanup", "step": string(step.Action), "note": note, "portal_sends": e.Sends},
		}); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("\nApplied %d changes.\n", applied)
	return nil
}

func WriteReport(w io.Writer, steps []Step) {
	order := []Action{ActionArchive, ActionPromote, ActionStillInRobot, ActionCheck}
	titles := map[Action]string{
		ActionArchive:      "Archive (not sent in the synced window and replaced by a new version, or listed by id)",
		ActionCheck:        "Left as is: not seen in the portal and the only version of its slot (check by hand)",
		ActionPromote:      "Becomes the main version and is marked live (only version, still sent)",
		ActionStillInRobot: "Stays, marked live as still in the Bitrix24 robot (a new version exists)",
	}
	byAction := map[Action][]Step{}
	for _, step := range steps {
		byAction[step.Action] = append(byAction[step.Action], step)
	}
	fmt.Fprintln(w, "# Old variants cleanup")
	fmt.Fprintln(w)
	for _, action := range order {
		fmt.Fprintf(w, "- %s: %d\n", titles[action], len(byAction[action]))
	}
	for _, action := range order {
		list := byAction[action]
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Email.SortOrder != list[j].Email.SortOrder {
				return list[i].Email.SortOrder < list[j].Email.SortOrder
			}
			return list[i].Email.Language < list[j].Email.Language
		})
		fmt.Fprintf(w, "\n## %s\n\n", titles[action])
		for _, step := range list {
			e := step.Email
			line := fmt.Sprintf("- %s #%d %s «%s»", e.Stage, e.SortOrder, strings.ToUpper(e.Language), e.Title)
			if e.AdaptationKey != "" && e.AdaptationKey != "default" {
				line += " [" + e.AdaptationKey + "]"
			}
			if e.Variant != "old" {
				line += " (" + e.Variant + ")"
			}
			if e.Sends > 0 {
				line += fmt.Sprintf(" — %d sends, last %s", e.Sends, e.LastSent.Format("2006-01-02"))
			}
			if step.New != nil {
				line += fmt.Sprintf("; new version: %d sends", step.New.Sends)
			}
			fmt.Fprintln(w, line)
		}
	}
}
