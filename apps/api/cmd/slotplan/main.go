// Command slotplan is a read-only dry run of migrating emails to the
// slot → master → translations/overrides/branches model. It prints a Markdown
// report and never writes to the database.
//
//	go run ./cmd/slotplan -board onboarding          # reads DATABASE_URL
//	go run ./cmd/slotplan -input emails.json          # reads an exported JSON array
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	board := flag.String("board", "onboarding", "board key to analyse when reading from the database")
	input := flag.String("input", "", "JSON file with an array of emails instead of the database")
	flag.Parse()

	emails, err := loadEmails(*board, *input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "slotplan: %v\n", err)
		os.Exit(1)
	}

	WriteReport(os.Stdout, BuildPlan(emails))
}

func loadEmails(board, input string) ([]Email, error) {
	if input != "" {
		data, err := os.ReadFile(input)
		if err != nil {
			return nil, err
		}
		var emails []Email
		if err := json.Unmarshal(data, &emails); err != nil {
			return nil, fmt.Errorf("parse %s: %w", input, err)
		}
		return emails, nil
	}

	_ = godotenv.Load("../../.env")
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required without -input")
	}

	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	defer dbpool.Close()

	rows, err := dbpool.Query(ctx, `
		SELECT id, sequence, stage, sort_order, title, language, variant,
			adaptation_key, adaptation_label, review_status, owner_email, editable_fields
		FROM emails
		WHERE sequence = $1 AND archived_at IS NULL
		ORDER BY sort_order, created_at;
	`, board)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []Email
	for rows.Next() {
		var e Email
		var fields []byte
		if err := rows.Scan(&e.ID, &e.Sequence, &e.Stage, &e.SortOrder, &e.Title, &e.Language, &e.Variant,
			&e.AdaptationKey, &e.AdaptationLabel, &e.ReviewStatus, &e.OwnerEmail, &fields); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(fields, &e.EditableFields); err != nil {
			return nil, fmt.Errorf("email %s editable fields: %w", e.ID, err)
		}
		emails = append(emails, e)
	}
	return emails, rows.Err()
}
