package demo

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReserveAICallLimitsVisitors(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for demo AI limit tests")
	}
	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dbpool.Close)

	visitor := visitorEmailPrefix + "quota-test-a" + visitorEmailDomain
	other := visitorEmailPrefix + "quota-test-b" + visitorEmailDomain
	cleanup := func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM demo_ai_calls WHERE visitor_email LIKE $1;`, visitorEmailPrefix+"quota-test-%")
	}
	cleanup()
	t.Cleanup(cleanup)

	if err := ReserveAICall(ctx, dbpool, "admin@example.com"); err != nil {
		t.Fatalf("a regular user was limited: %v", err)
	}

	for i := 0; i < visitorAICallLimit; i++ {
		if err := ReserveAICall(ctx, dbpool, visitor); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if err := ReserveAICall(ctx, dbpool, visitor); !errors.Is(err, ErrVisitorAILimit) {
		t.Fatalf("call over the visitor limit: got %v, want ErrVisitorAILimit", err)
	}
	if err := ReserveAICall(ctx, dbpool, other); err != nil {
		t.Fatalf("another visitor was limited: %v", err)
	}

	// Fill the rest of the demo-wide budget for today.
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO demo_ai_calls (visitor_email)
		SELECT $1 FROM generate_series(1, $2);
	`, visitorEmailPrefix+"quota-test-filler"+visitorEmailDomain, dailyAICallLimit); err != nil {
		t.Fatal(err)
	}
	fresh := visitorEmailPrefix + "quota-test-c" + visitorEmailDomain
	if err := ReserveAICall(ctx, dbpool, fresh); !errors.Is(err, ErrDailyAILimit) {
		t.Fatalf("call over the daily limit: got %v, want ErrDailyAILimit", err)
	}
}
