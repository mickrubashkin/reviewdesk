// Command sharedtexts creates the shared texts (footer, privacy link, logo,
// login block, signature) from the values emails already have, one per key
// and language, and links the email fields that have exactly that value. No
// email changes its HTML, so nothing needs a re-upload. It prints the plan and
// changes nothing unless -apply is given; then everything happens in one
// transaction.
//
//	go run ./cmd/sharedtexts                                  # dry run, reads DATABASE_URL
//	go run ./cmd/sharedtexts -apply -actor you@company.com
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	apply := flag.Bool("apply", false, "write the changes; without it this is a dry run")
	actor := flag.String("actor", "", "super admin email recorded as the author of new shared texts (required with -apply)")
	flag.Parse()

	if err := run(*apply, strings.TrimSpace(*actor)); err != nil {
		fmt.Fprintf(os.Stderr, "sharedtexts: %v\n", err)
		os.Exit(1)
	}
}

func run(apply bool, actor string) error {
	if apply && actor == "" {
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

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	emails, err := loadEmails(ctx, tx)
	if err != nil {
		return err
	}
	existing, err := loadExisting(ctx, tx)
	if err != nil {
		return err
	}
	plan := BuildPlan(emails, existing)
	if apply {
		if err := applyPlan(ctx, tx, plan, actor); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	WritePlan(os.Stdout, plan, apply)
	return nil
}

func loadEmails(ctx context.Context, tx pgx.Tx) ([]Email, error) {
	// Live as on the board: matched portal sends or a manual mark. FOR UPDATE
	// keeps the values from changing between the plan and the links.
	rows, err := tx.Query(ctx, `
		SELECT e.id, e.sequence, e.stage, e.sort_order, lower(e.language), coalesce(e.variant, ''),
			e.editable_fields,
			e.live_marked_at IS NOT NULL OR EXISTS (
				SELECT 1 FROM portal_email_groups g
				WHERE (g.decision IN ('confirmed', 'created') AND g.decision_email_id = e.id)
					OR (g.decision IS NULL AND g.match_status = 'matched' AND g.match_email_id = e.id)
			)
		FROM emails e
		WHERE e.archived_at IS NULL
		ORDER BY e.sequence, e.stage, e.sort_order, e.language, e.created_at
		FOR UPDATE OF e;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []Email
	for rows.Next() {
		var e Email
		var board, stage, variant string
		var sortOrder int
		var fields []byte
		if err := rows.Scan(&e.ID, &board, &stage, &sortOrder, &e.Language, &variant, &fields, &e.Live); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(fields, &e.Fields); err != nil {
			return nil, fmt.Errorf("email %s editable fields: %w", e.ID, err)
		}
		e.Place = fmt.Sprintf("%s · %s #%d · %s", board, stage, sortOrder, strings.ToUpper(e.Language))
		if variant != "" && variant != "new" {
			e.Place += " · " + variant
		}
		emails = append(emails, e)
	}
	return emails, rows.Err()
}

func loadExisting(ctx context.Context, tx pgx.Tx) (map[string]Existing, error) {
	rows, err := tx.Query(ctx, `SELECT key, language, type, value FROM shared_texts WHERE board_key = '';`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	existing := map[string]Existing{}
	for rows.Next() {
		var key, language string
		var text Existing
		if err := rows.Scan(&key, &language, &text.Type, &text.Value); err != nil {
			return nil, err
		}
		existing[key+"\x1f"+language] = text
	}
	return existing, rows.Err()
}

func applyPlan(ctx context.Context, tx pgx.Tx, plan Plan, actor string) error {
	for _, item := range plan.Items {
		if !item.Exists {
			if _, err := tx.Exec(ctx, `
				INSERT INTO shared_texts (key, language, type, value, en_version, updated_by_email)
				VALUES ($1, $2, $3, $4, CASE WHEN $2 = 'en' THEN NULL ELSE 1 END, $5);
			`, item.Key, item.Language, item.Type, item.Value, actor); err != nil {
				return fmt.Errorf("create shared text %s %s: %w", item.Key, item.Language, err)
			}
		}
		for _, email := range item.Link {
			if _, err := tx.Exec(ctx, `
				UPDATE emails
				SET editable_fields = jsonb_set(editable_fields, ARRAY[$2::text, 'shared'], 'true'::jsonb)
				WHERE id = $1;
			`, email.ID, item.Key); err != nil {
				return fmt.Errorf("link %s in email %s: %w", item.Key, email.ID, err)
			}
		}
	}
	return nil
}
