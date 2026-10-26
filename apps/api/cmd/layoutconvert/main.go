// Command layoutconvert switches the emails of one board from their own
// template to blocks of the library (phase 3, step 4). The blocks come from
// the email's fields; the email built from them is rendered and compared
// with the old one. An email and its translations switch together, only when
// every one of them renders the same HTML (with -allow-markup: the same text,
// links and images, while the markup may differ). The rest stay as they are
// and are listed for manual work.
// It prints the plan and changes nothing unless -apply is given; then
// everything happens in one transaction.
//
//	go run ./cmd/layoutconvert -board onboarding              # dry run, reads DATABASE_URL
//	go run ./cmd/layoutconvert -board onboarding -apply -actor you@company.com
//	go run ./cmd/layoutconvert -board onboarding -allow-markup  # also those whose markup differs
//	go run ./cmd/layoutconvert -board onboarding -allow-markup -keep-upload -apply -actor you@company.com
//	                                   # live emails keep their upload mark: no re-upload
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
	board := flag.String("board", "", "board key (required)")
	apply := flag.Bool("apply", false, "write the changes; without it this is a dry run")
	actor := flag.String("actor", "", "email of the admin recorded as the author of the new versions (required with -apply)")
	allowMarkup := flag.Bool("allow-markup", false, "also switch emails with the same text, links and images whose markup differs")
	keepUpload := flag.Bool("keep-upload", false, "move the upload mark of emails whose robot has their latest version to the new one, instead of asking for a re-upload")
	flag.Parse()

	options := emails.LayoutConversionOptions{Strict: !*allowMarkup, KeepUpload: *keepUpload}
	if err := run(strings.TrimSpace(*board), *apply, strings.TrimSpace(*actor), options); err != nil {
		fmt.Fprintf(os.Stderr, "layoutconvert: %v\n", err)
		os.Exit(1)
	}
}

func run(board string, apply bool, actorEmail string, options emails.LayoutConversionOptions) error {
	strict := options.Strict
	if board == "" {
		return fmt.Errorf("-board is required")
	}
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

	access := pgx.ReadOnly
	if apply {
		access = pgx.ReadWrite
	}
	tx, err := dbpool.BeginTx(ctx, pgx.TxOptions{AccessMode: access})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM boards WHERE key = $1);`, board).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("there is no board %q", board)
	}

	families, err := emails.PlanLayoutConversion(ctx, tx, board)
	if err != nil {
		return err
	}
	switched, keptUpload := 0, 0
	if apply {
		actor, err := loadActor(ctx, tx, actorEmail)
		if err != nil {
			return err
		}
		if switched, keptUpload, err = emails.ApplyLayoutConversion(ctx, tx, actor, families, options); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	WriteReport(os.Stdout, board, families, strict, apply, switched, keptUpload)
	return nil
}

func loadActor(ctx context.Context, tx pgx.Tx, email string) (auth.AuthUser, error) {
	var user auth.AuthUser
	err := tx.QueryRow(ctx, `
		SELECT id, email, role FROM users WHERE lower(email) = lower($1);
	`, email).Scan(&user.ID, &user.Email, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return user, fmt.Errorf("there is no user %s", email)
	}
	if err != nil {
		return user, err
	}
	if user.Role != "admin" && user.Role != "super_admin" {
		return user, fmt.Errorf("%s is not an admin", email)
	}
	return user, nil
}
