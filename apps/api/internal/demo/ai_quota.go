package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The demo backend has a real OpenAI key so visitors can see the AI review,
// which means every visitor could spend it. Each visitor gets a few calls and
// the whole demo a daily budget; cached results do not count.
const (
	visitorAICallLimit = 5
	dailyAICallLimit   = 200
)

var (
	ErrVisitorAILimit = fmt.Errorf("this demo copy has used its %d AI requests", visitorAICallLimit)
	ErrDailyAILimit   = errors.New("the demo has used today's AI requests, try again tomorrow")
)

// IsVisitorEmail reports whether email belongs to a sandbox visitor.
func IsVisitorEmail(email string) bool {
	return strings.HasPrefix(email, visitorEmailPrefix) && strings.HasSuffix(email, visitorEmailDomain)
}

// ReserveAICall records an OpenAI call by a demo visitor before it is made,
// or returns ErrVisitorAILimit or ErrDailyAILimit when it is over a limit.
// Other users are never limited and nothing is written for them.
func ReserveAICall(ctx context.Context, db *pgxpool.Pool, email string) error {
	if !IsVisitorEmail(email) {
		return nil
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// One reservation at a time, so parallel requests cannot all pass the
	// count before any of them is written.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('demo_ai_calls'));`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM demo_ai_calls WHERE created_at < now() - interval '2 days';`); err != nil {
		return err
	}

	var visitorCalls, dailyCalls int
	if err := tx.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE visitor_email = $1),
			COUNT(*) FILTER (WHERE created_at >= now() - interval '24 hours')
		FROM demo_ai_calls;
	`, email).Scan(&visitorCalls, &dailyCalls); err != nil {
		return err
	}
	switch {
	case visitorCalls >= visitorAICallLimit:
		return ErrVisitorAILimit
	case dailyCalls >= dailyAICallLimit:
		return ErrDailyAILimit
	}

	if _, err := tx.Exec(ctx, `INSERT INTO demo_ai_calls (visitor_email) VALUES ($1);`, email); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
