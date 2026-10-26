// Command calendlyplan swaps managers' personal Calendly links for the
// responsible person's booking link variable. It prints a Markdown report
// grouped by link, with the link text and the sentence around each use, and
// changes nothing unless -apply is given: then every affected email gets the
// expression in its template hrefs and URL fields (a new version each, in one
// transaction) and the "Booking link" Bitrix variable gets the expression.
//
//	go run ./cmd/calendlyplan                     # every board, reads DATABASE_URL
//	go run ./cmd/calendlyplan -board onboarding
//	go run ./cmd/calendlyplan -apply -actor you@company.com
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

func main() {
	board := flag.String("board", "", "board key to analyse; empty means every board")
	expression := flag.String("expression", "{{Responsible person: Calendly onboarding link}}", "Bitrix24 expression that replaces the links")
	apply := flag.Bool("apply", false, "write the changes; without it this is a dry run")
	actor := flag.String("actor", "", "super admin email recorded as the author of the new versions (required with -apply)")
	flag.Parse()

	emails, err := loadEmails(*board)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calendlyplan: %v\n", err)
		os.Exit(1)
	}
	uses, err := FindUses(emails)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calendlyplan: %v\n", err)
		os.Exit(1)
	}
	WriteReport(os.Stdout, uses, *expression)
	if !*apply {
		fmt.Println("\nRun with -apply -actor <email> to make these changes.")
		return
	}
	if *actor == "" {
		fmt.Fprintln(os.Stderr, "calendlyplan: -actor is required with -apply")
		os.Exit(1)
	}
	if err := applyReplacement(uses, *expression, *actor); err != nil {
		fmt.Fprintf(os.Stderr, "calendlyplan: %v\n", err)
		os.Exit(1)
	}
}

// replaceLinks swaps every personal Calendly link in value for expression.
func replaceLinks(value, expression string) string {
	return calendlyPattern.ReplaceAllStringFunc(value, func(link string) string {
		if keepLink(link) {
			return link
		}
		return expression
	})
}

func applyReplacement(uses []Use, expression, actorEmail string) error {
	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer dbpool.Close()
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	actor := emails.LinkActor{Email: actorEmail}
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1) AND role = 'super_admin';`, actorEmail).Scan(&actor.UserID); err != nil {
		return fmt.Errorf("actor %s is not a super admin: %w", actorEmail, err)
	}

	seen := map[string]bool{}
	changedEmails, changedLinks := 0, 0
	for _, use := range uses {
		if !keepLink(use.Link) {
			changedLinks++
		}
	}
	for _, use := range uses {
		if seen[use.Email.ID] || keepLink(use.Link) {
			continue
		}
		seen[use.Email.ID] = true
		changed, err := emails.ReplaceEmailLinks(ctx, tx, actor, use.Email.ID, func(value string) string {
			return replaceLinks(value, expression)
		})
		if err != nil {
			return err
		}
		if changed {
			changedEmails++
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE bitrix_variables
		SET expression = $1, updated_by_email = $2, updated_at = now()
		WHERE label = 'Booking link' AND expression <> $1;
	`, expression, actorEmail)
	if err != nil {
		return fmt.Errorf("update the Booking link variable: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("\nApplied: %d links in %d emails; Booking link variable updated: %t.\n", changedLinks, changedEmails, tag.RowsAffected() == 1)
	return nil
}

func loadEmails(board string) ([]Email, error) {
	_ = godotenv.Load("../../.env")
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	defer dbpool.Close()

	rows, err := dbpool.Query(ctx, `
		SELECT id, coalesce(sequence, ''), coalesce(stage, ''), sort_order, title, language, variant, template_html, editable_fields
		FROM emails
		WHERE ($1 = '' OR sequence = $1) AND archived_at IS NULL
		ORDER BY sequence, sort_order, language, variant;
	`, board)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []Email
	for rows.Next() {
		var e Email
		var fields []byte
		if err := rows.Scan(&e.ID, &e.Board, &e.Stage, &e.SortOrder, &e.Title, &e.Language, &e.Variant, &e.TemplateHTML, &fields); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(fields, &e.Fields); err != nil {
			return nil, fmt.Errorf("email %s fields: %w", e.ID, err)
		}
		emails = append(emails, e)
	}
	return emails, rows.Err()
}
