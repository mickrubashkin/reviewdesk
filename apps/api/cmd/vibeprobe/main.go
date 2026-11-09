// Command vibeprobe is the VibeCode pilot's stand-in for the API (#171): it
// checks that a static Go binary runs on the platform and that its SSE reaches
// the browser through the Node wrapper and the gateway unbuffered.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"
)

const version = "vibeprobe 1"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	flag.Parse()

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
