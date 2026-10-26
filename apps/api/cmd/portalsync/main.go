// Command portalsync pulls the emails CRM robots actually sent for one deal
// category and matches them against a board. Read-only towards Bitrix24.
//
//	go run ./cmd/portalsync -board onboarding -category 10 -days 90
//	go run ./cmd/portalsync -categories   # list deal categories
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/portal"
)

func main() {
	board := flag.String("board", "onboarding", "board key to match against")
	category := flag.Int("category", 10, "Bitrix24 deal category (funnel) ID")
	days := flag.Int("days", 90, "how many days back to load sent emails")
	listCategories := flag.Bool("categories", false, "list deal categories and exit")
	flag.Parse()

	_ = godotenv.Load("../../.env")
	ctx := context.Background()

	client, err := portal.NewClientFromEnv()
	if err != nil {
		fail(err)
	}
	if *listCategories {
		categories, err := client.DealCategories(ctx)
		if err != nil {
			fail(err)
		}
		for _, c := range categories {
			fmt.Printf("%4d  %s\n", c.ID, c.Name)
		}
		return
	}

	dbpool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fail(err)
	}
	defer dbpool.Close()

	options := portal.SyncOptions{BoardKey: *board, CategoryID: *category, Days: *days}
	runID, err := portal.BeginRun(ctx, dbpool, options, "cli")
	if err != nil {
		fail(err)
	}

	fmt.Printf("sync %s: board=%s category=%d days=%d\n", runID, *board, *category, *days)
	if err := portal.Sync(ctx, dbpool, client, runID, options); err != nil {
		fail(err)
	}

	var seen, stored, groups int
	_ = dbpool.QueryRow(ctx, `SELECT activities_seen, emails_stored, groups_count FROM portal_sync_runs WHERE id = $1;`, runID).Scan(&seen, &stored, &groups)
	fmt.Printf("done: %d emails checked, %d from this funnel, %d distinct emails\n", seen, stored, groups)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "portalsync: %v\n", err)
	os.Exit(1)
}
