package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

// runChange drops, sets and relinks shared texts (-drop, -set, -relink). The
// dry run does the same inside a transaction it rolls back, so the report
// shows exactly what -apply would write.
func runChange(ctx context.Context, dbpool *pgxpool.Pool, change emails.SharedTextsChange, apply bool, actorEmail string) error {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

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
	result, err := emails.ApplySharedTextsChange(ctx, tx, actor, change)
	if err != nil {
		return err
	}
	if apply {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	writeChange(os.Stdout, result, apply)
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
	if user.Role != "super_admin" {
		return user, fmt.Errorf("%s is not a super admin", email)
	}
	return user, nil
}

// readSetFile reads -set: a JSON array of {key, language, value}.
func readSetFile(path string) ([]emails.SharedTextValue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var values []emails.SharedTextValue
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return values, nil
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

func writeChange(w io.Writer, result emails.SharedTextsResult, apply bool) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format, args...) }
	p("# Shared texts change\n\n")
	if apply {
		p("Applied.\n\n")
	} else {
		p("Dry run: nothing was changed. Run with `-apply -actor you@company.com` to write it.\n\n")
	}
	changed, live := 0, 0
	for _, email := range result.Emails {
		if email.Changed {
			changed++
			if email.Live {
				live++
			}
		}
	}
	p("- Shared texts dropped: %d (fields unlinked in %d emails, values kept)\n", result.Dropped, result.Unlinked)
	p("- Shared texts with a new value: %d\n", result.Set)
	p("- Emails with a new version: %d (live, to re-upload: %d)\n\n", changed, live)
	if len(result.Emails) == 0 {
		return
	}
	p("| Email | Live | New version | Linked to the shared text |\n|---|---|---|---|\n")
	for _, email := range result.Emails {
		p("| %s | %s | %s | %s |\n", email.Place, yes(email.Live), yes(email.Changed), strings.Join(email.Relinked, ", "))
	}
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return ""
}
