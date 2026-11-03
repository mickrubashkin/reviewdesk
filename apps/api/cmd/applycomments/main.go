// Command applycomments puts the wording reviewers suggested in comments into
// the emails: a comment whose body is the text to use in place of the text it
// selects. Only an exact, single match in the comment's own field is applied,
// each email gets one new version, and the applied comments are resolved with
// a reply. Everything else stays open, with the reason in the report.
//
// It changes nothing unless -apply is given: the dry run makes the same
// changes inside a transaction it rolls back, so the report shows exactly
// what -apply would write.
//
//	go run ./cmd/applycomments -report ../../personal-notes/comment-edits.html
//	go run ./cmd/applycomments -apply -actor you@company.com -report ...
//
// The misc board is left alone by default (-skip-board misc): it holds the
// emails people practise on.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

func main() {
	apply := flag.Bool("apply", false, "write the changes; without it this is a dry run")
	planOnly := flag.Bool("plan-only", false, "only plan, in a read-only transaction: no locks and no rehearsal of the saves, so emails the save would refuse are not found")
	actor := flag.String("actor", "", "super admin email recorded as the author of the versions and replies (required with -apply)")
	board := flag.String("board", "", "only this board")
	skipBoards := flag.String("skip-board", "misc", "comma-separated boards to leave alone")
	report := flag.String("report", "", "write an HTML report of every comment to this file")
	appURL := flag.String("app-url", os.Getenv("AUTH_APP_URL"), "the app's address, to link each email in the report")
	flag.Parse()

	if *apply && *planOnly {
		fmt.Fprintln(os.Stderr, "applycomments: -apply and -plan-only exclude each other")
		os.Exit(2)
	}
	options := emails.CommentEditsOptions{Board: strings.TrimSpace(*board), SkipBoards: splitList(*skipBoards), NoLock: *planOnly}
	if err := run(*apply, strings.TrimSpace(*actor), options, *report, *appURL); err != nil {
		fmt.Fprintf(os.Stderr, "applycomments: %v\n", err)
		os.Exit(1)
	}
}

func run(apply bool, actorEmail string, options emails.CommentEditsOptions, reportPath, appURL string) error {
	if apply && actorEmail == "" {
		return fmt.Errorf("-actor is required with -apply")
	}
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

	accessMode := pgx.ReadWrite
	if options.NoLock {
		accessMode = pgx.ReadOnly
	}
	tx, err := dbpool.BeginTx(ctx, pgx.TxOptions{AccessMode: accessMode})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if options.NoLock {
		planned, err := emails.PlanCommentEdits(ctx, tx, options)
		if err != nil {
			return err
		}
		return report(planned, false, reportPath, appURL)
	}
	if actorEmail == "" {
		// The dry run needs an author for the versions it rolls back.
		if err := tx.QueryRow(ctx, `SELECT email FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1;`).Scan(&actorEmail); err != nil {
			return fmt.Errorf("find a super admin for the dry run: %w", err)
		}
	}
	actor, err := loadSuperAdmin(ctx, tx, actorEmail)
	if err != nil {
		return err
	}
	planned, err := emails.PlanCommentEdits(ctx, tx, options)
	if err != nil {
		return err
	}
	if err := emails.ApplyCommentEdits(ctx, tx, actor, planned); err != nil {
		return err
	}
	if apply {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}

	return report(planned, apply, reportPath, appURL)
}

func report(planned []emails.CommentEditEmail, applied bool, reportPath, appURL string) error {
	writeSummary(os.Stdout, planned, applied)
	if reportPath == "" {
		return nil
	}
	file, err := os.Create(reportPath)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := writeReport(file, planned, applied, appURL); err != nil {
		return err
	}
	fmt.Printf("Report: %s\n", reportPath)
	return nil
}

func loadSuperAdmin(ctx context.Context, tx pgx.Tx, email string) (auth.AuthUser, error) {
	var user auth.AuthUser
	err := tx.QueryRow(ctx, `SELECT id, email, role FROM users WHERE lower(email) = lower($1);`, email).
		Scan(&user.ID, &user.Email, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return user, fmt.Errorf("there is no user %s", email)
	}
	if err != nil {
		return user, err
	}
	if !auth.IsSuperAdmin(user) {
		return user, fmt.Errorf("%s is not a super admin", email)
	}
	return user, nil
}

func splitList(value string) []string {
	var items []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}
