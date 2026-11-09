package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testWebDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":         "<!doctype html><title>app</title>",
		"assets/app-1a2b.js": "console.log(1)",
		"favicon.svg":        "<svg/>",
	}
	for name, body := range files {
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestWebHandlerServesFilesAndFallsBackToIndex(t *testing.T) {
	handler := webHandler(testWebDir(t), "")

	cases := []struct {
		path   string
		status int
		body   string
		cache  string
	}{
		{"/", 200, "<title>app</title>", "no-cache"},
		{"/emails/42/review", 200, "<title>app</title>", "no-cache"},
		{"/assets/app-1a2b.js", 200, "console.log(1)", "public, max-age=31536000, immutable"},
		{"/favicon.svg", 200, "<svg/>", ""},
		{"/assets/", 200, "<title>app</title>", "no-cache"},
		{"/../../etc/passwd", 400, "invalid URL path", ""},
		{"/api/unknown", 404, "", ""},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, c.path, nil))
		if recorder.Code != c.status {
			t.Errorf("%s: status %d, want %d", c.path, recorder.Code, c.status)
		}
		if c.body != "" && !strings.Contains(recorder.Body.String(), c.body) {
			t.Errorf("%s: body %q, want %q in it", c.path, recorder.Body.String(), c.body)
		}
		if got := recorder.Header().Get("Cache-Control"); got != c.cache {
			t.Errorf("%s: Cache-Control %q, want %q", c.path, got, c.cache)
		}
	}
}

func TestWebHandlerRejectsWrites(t *testing.T) {
	recorder := httptest.NewRecorder()
	webHandler(testWebDir(t), "").ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("POST /: status %d, want 404", recorder.Code)
	}
}

func TestWebHandlerFraming(t *testing.T) {
	recorder := httptest.NewRecorder()
	webHandler(testWebDir(t), "").ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := recorder.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("without frame ancestors: X-Frame-Options %q, want DENY", got)
	}

	recorder = httptest.NewRecorder()
	webHandler(testWebDir(t), "https://example.com").ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := recorder.Header().Get("X-Frame-Options"); got != "" {
		t.Errorf("with frame ancestors: X-Frame-Options %q, want none", got)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); got != "frame-ancestors https://example.com" {
		t.Errorf("with frame ancestors: CSP %q", got)
	}
}
