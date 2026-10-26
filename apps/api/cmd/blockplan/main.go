// Command blockplan is a read-only audit of email templates before moving
// emails to a shared block library: layouts, emails without editable fields,
// texts repeated across emails, CTA counts and text lengths. It prints a
// Markdown report and never writes to the database.
//
//	go run ./cmd/blockplan                    # all boards, reads DATABASE_URL
//	go run ./cmd/blockplan -board onboarding  # one board
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	board := flag.String("board", "", "board key to audit; empty audits every board")
	flag.Parse()

	emails, err := loadEmails(*board)
	if err != nil {
		fmt.Fprintf(os.Stderr, "blockplan: %v\n", err)
		os.Exit(1)
	}

	WriteReport(os.Stdout, BuildAudit(emails))
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

	// The transaction only guards against accidental writes.
	tx, err := dbpool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Live as on the board: matched portal sends or a manual mark.
	rows, err := tx.Query(ctx, `
		SELECT e.id, e.sequence, e.stage, e.sort_order, e.title, lower(e.language), coalesce(e.variant, ''),
			coalesce(e.adaptation_key, ''), coalesce(e.subject, ''), coalesce(e.preheader, ''),
			coalesce(e.template_html, ''), e.editable_fields,
			coalesce(portal.send_count, 0), e.live_marked_at IS NOT NULL
		FROM emails e
		LEFT JOIN LATERAL (
			SELECT sum(g.send_count)::int AS send_count
			FROM portal_email_groups g
			WHERE (g.decision IN ('confirmed', 'created') AND g.decision_email_id = e.id)
				OR (g.decision IS NULL AND g.match_status = 'matched' AND g.match_email_id = e.id)
		) portal ON true
		WHERE e.archived_at IS NULL AND ($1 = '' OR e.sequence = $1)
		ORDER BY e.sequence, e.stage, e.sort_order, e.language, e.created_at;
	`, board)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []Email
	for rows.Next() {
		var e Email
		var fields []byte
		if err := rows.Scan(&e.ID, &e.Board, &e.Stage, &e.SortOrder, &e.Title, &e.Language, &e.Variant,
			&e.Adaptation, &e.Subject, &e.Preheader, &e.TemplateHTML, &fields,
			&e.PortalSends, &e.LiveMarked); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(fields, &e.Fields); err != nil {
			return nil, fmt.Errorf("email %s editable fields: %w", e.ID, err)
		}
		emails = append(emails, e)
	}
	return emails, rows.Err()
}
