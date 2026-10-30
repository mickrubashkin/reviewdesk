// Command demostats prints who visited the public demo and how far they went:
// visits per day and per source, and with -visits each visit. The owner's own
// visits (DEMO_OWNER_KEY) are left out.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/demo"
)

func main() {
	days := flag.Int("days", 14, "how many days back to count")
	zone := flag.String("tz", "Europe/Madrid", "time zone for the days")
	visits := flag.Bool("visits", false, "also list each visit")
	flag.Parse()

	if err := run(*days, *zone, *visits); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to count demo visits: %v\n", err)
		os.Exit(1)
	}
}

func run(days int, zone string, listVisits bool) error {
	_ = godotenv.Load("../../.env")

	loc, err := time.LoadLocation(zone)
	if err != nil {
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

	now := time.Now().In(loc)
	since := time.Date(now.Year(), now.Month(), now.Day()-days+1, 0, 0, 0, 0, loc)
	visitList, err := demo.ListVisits(ctx, dbpool, since)
	if err != nil {
		return err
	}

	fmt.Printf("Demo visits since %s (%s)\n\n", since.Format("2006-01-02"), zone)
	byDay, total := demo.CountVisitsByDay(visitList, loc)
	printCounts(os.Stdout, "Day", append(byDay, total))
	fmt.Println()
	printCounts(os.Stdout, "Source", demo.CountVisitsBySource(visitList))

	if listVisits {
		fmt.Println()
		printVisits(os.Stdout, visitList, loc)
	}
	return nil
}

func printCounts(out io.Writer, label string, rows []demo.VisitCounts) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "%s\tVisitors\tReturning\tLinkedIn\tMobile\tActive\tEdited\tCommented\tResolved\tReady\tAI\t\n", label)
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t\n",
			row.Label, row.Visitors, row.Returning, row.LinkedIn, row.Mobile, row.Active,
			row.Edited, row.Commented, row.Resolved, row.MarkedReady, row.UsedAI)
	}
	_ = w.Flush()
}

func printVisits(out io.Writer, visits []demo.Visit, loc *time.Location) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Entered\tStayed\tVisit\tSource\tDevice\tPlace\tActions")
	for i := len(visits) - 1; i >= 0; i-- {
		visit := visits[i]
		stayed := "-"
		if visit.LastSeenAt != nil {
			stayed = visit.LastSeenAt.Sub(visit.CreatedAt).Round(time.Minute).String()
		}
		source, device := visit.Source, "desktop"
		if source == "" {
			source = "direct"
		}
		if visit.Mobile {
			device = "mobile"
		}
		place := strings.Trim(visit.TimeZone+" "+visit.Language, " ")
		if place == "" {
			place = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			visit.CreatedAt.In(loc).Format("2006-01-02 15:04"), stayed, visit.Number, source, device, place, formatActions(visit.Actions))
	}
	_ = w.Flush()
}

func formatActions(actions map[string]int) string {
	if len(actions) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(actions))
	for action, count := range actions {
		parts = append(parts, fmt.Sprintf("%s %d", action, count))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}
