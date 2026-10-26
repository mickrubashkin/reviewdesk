package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/demo"
)

func main() {
	reset := flag.Bool("reset", false, "reset demo-owned data before seeding")
	flag.Parse()

	if err := run(*reset); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to seed demo data: %v\n", err)
		os.Exit(1)
	}
}

func run(reset bool) error {
	_ = godotenv.Load("../../.env")

	if err := demo.ValidateSeedEnv(reset); err != nil {
		return err
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("create connection pool: %w", err)
	}
	defer dbpool.Close()

	if err := dbpool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if reset {
		if err := demo.Reset(ctx, tx); err != nil {
			return err
		}
	}
	count, err := demo.Seed(ctx, tx, demo.SharedBoard)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	fmt.Printf("Seeded demo board %q with %d emails\n", demo.SharedBoard.Key, count)
	return nil
}
