// Command vibeprobe is the VibeCode pilot's stand-in for the API (#171): it
// checks that a static Go binary runs on the platform and that its SSE reaches
// the browser through the Node wrapper and the gateway unbuffered. With --db it
// measures the database in DATABASE_URL instead (Galaxy has no psql).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

const version = "vibeprobe 1"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	checkDB := flag.Bool("db", false, "measure the database in DATABASE_URL, print JSON and exit")
	flag.Parse()

	if *checkDB {
		result := measureDatabase(os.Getenv("DATABASE_URL"))
		_ = json.NewEncoder(os.Stdout).Encode(result)
		return
	}

	if *showVersion {
		fmt.Printf("%s %s/%s %s\n", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","version":%q}`, version)
	})
	mux.HandleFunc("GET /api/probe/sse", streamTicks)

	fmt.Fprintf(os.Stderr, "%s listening on %s\n", version, *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// streamTicks sends ten events a second apart, each with the server time, so
// the client can tell chunks arriving one by one from one buffered burst.
func streamTicks(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for i := 1; i <= 10; i++ {
		fmt.Fprintf(w, "event: tick\ndata: {\"n\":%d,\"sentAt\":%d}\n\n", i, time.Now().UnixMilli())
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Second):
		}
	}
	fmt.Fprint(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
}

type databaseResult struct {
	OK            bool    `json:"ok"`
	Error         string  `json:"error,omitempty"`
	ConnectMs     int64   `json:"connectMs"`
	ServerVersion string  `json:"serverVersion,omitempty"`
	GooseVersion  *int64  `json:"gooseVersion,omitempty"`
	RoundTripsMs  []int64 `json:"roundTripsMs,omitempty"`
	MedianMs      int64   `json:"medianMs"`
	MaxMs         int64   `json:"maxMs"`
}

// measureDatabase times the connection and twenty SELECT 1 round trips: a page
// of the app makes a few dozen queries, so the median shows what a remote
// database adds to each.
func measureDatabase(databaseURL string) databaseResult {
	var result databaseResult
	if databaseURL == "" {
		result.Error = "DATABASE_URL is not set"
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	started := time.Now()
	conn, err := pgx.Connect(ctx, databaseURL)
	result.ConnectMs = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer conn.Close(context.Background())

	if err := conn.QueryRow(ctx, "SELECT current_setting('server_version')").Scan(&result.ServerVersion); err != nil {
		result.Error = err.Error()
		return result
	}
	var goose int64
	if err := conn.QueryRow(ctx, "SELECT max(version_id) FROM goose_db_version").Scan(&goose); err == nil {
		result.GooseVersion = &goose
	}

	for range 20 {
		started := time.Now()
		var one int
		if err := conn.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
			result.Error = err.Error()
			return result
		}
		result.RoundTripsMs = append(result.RoundTripsMs, time.Since(started).Milliseconds())
	}
	sorted := append([]int64(nil), result.RoundTripsMs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	result.MedianMs = sorted[len(sorted)/2]
	result.MaxMs = sorted[len(sorted)-1]
	result.OK = true
	return result
}
