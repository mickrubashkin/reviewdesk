package backup

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	objects map[string][]byte
	deleted []string
}

func (s *fakeStore) Put(_ context.Context, key string, body []byte) error {
	s.objects[key] = body
	return nil
}

func (s *fakeStore) Keys(_ context.Context, prefix string) ([]string, error) {
	keys := []string{}
	for key := range s.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (s *fakeStore) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	s.deleted = append(s.deleted, key)
	return nil
}

func TestExpired(t *testing.T) {
	keys := []string{
		"b/2026-11-08.sql.gz", "b/2026-11-10.sql.gz", "b/notes.txt", "b/2026-11-09.sql.gz", "b/2026-11-07.sql.gz",
	}
	got := Expired(keys, "b/", 2)
	if want := []string{"b/2026-11-08.sql.gz", "b/2026-11-07.sql.gz"}; !slices.Equal(got, want) {
		t.Errorf("expired %v, want %v", got, want)
	}
	if got := Expired(keys, "b/", 4); len(got) != 0 {
		t.Errorf("nothing beyond keep, got %v", got)
	}
}

func TestRunDue(t *testing.T) {
	store := &fakeStore{objects: map[string][]byte{
		"b/2026-11-08.sql.gz": {1},
		"b/2026-11-09.sql.gz": {1},
	}}
	dumps := 0
	job := Job{Store: store, Prefix: "b/", Keep: 2, Dump: func(context.Context) ([]byte, error) {
		dumps++
		return []byte("dump"), nil
	}}
	now := time.Date(2026, 11, 10, 6, 0, 0, 0, time.UTC)

	for range 2 {
		if err := job.RunDue(context.Background(), now); err != nil {
			t.Fatal(err)
		}
	}
	if dumps != 1 {
		t.Errorf("dumped %d times, want once a day", dumps)
	}
	if string(store.objects["b/2026-11-10.sql.gz"]) != "dump" {
		t.Errorf("today's backup missing: %v", store.objects)
	}
	if !slices.Equal(store.deleted, []string{"b/2026-11-08.sql.gz"}) {
		t.Errorf("deleted %v", store.deleted)
	}

	job.Dump = func(context.Context) ([]byte, error) { return nil, errors.New("pg_dump: no connection") }
	if err := job.RunDue(context.Background(), now.AddDate(0, 0, 1)); err == nil || !strings.Contains(err.Error(), "no connection") {
		t.Errorf("a failed dump: %v", err)
	}
}

func TestVibeStorage(t *testing.T) {
	var uploadedKey, uploadedFile, deletedPath, listCursors string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/storage/objects/upload":
			uploadedKey = r.FormValue("key")
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			body, _ := io.ReadAll(file)
			uploadedFile = string(body)
			_, _ = w.Write([]byte(`{"object":{"key":"x"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/storage/objects":
			listCursors += r.URL.Query().Get("cursor") + ";"
			if r.URL.Query().Get("cursor") == "" {
				_, _ = w.Write([]byte(`{"data":[{"key":"b/1"}],"cursor":"next"}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[{"key":"b/2"}],"cursor":null}`))
			}
		case r.Method == http.MethodDelete:
			deletedPath = r.URL.EscapedPath()
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	store := &VibeStorage{APIKey: "k", BaseURL: server.URL}
	ctx := context.Background()

	if err := store.Put(ctx, "b/2026-11-10.sql.gz", []byte("dump")); err != nil {
		t.Fatal(err)
	}
	if uploadedKey != "b/2026-11-10.sql.gz" || uploadedFile != "dump" {
		t.Errorf("uploaded %q %q", uploadedKey, uploadedFile)
	}
	keys, err := store.Keys(ctx, "b/")
	if err != nil || !slices.Equal(keys, []string{"b/1", "b/2"}) || listCursors != ";next;" {
		t.Errorf("keys %v, cursors %q, err %v", keys, listCursors, err)
	}
	if err := store.Delete(ctx, "b/2026-11-08.sql.gz"); err != nil || deletedPath != "/storage/objects/b%2F2026-11-08.sql.gz" {
		t.Errorf("deleted %q, err %v", deletedPath, err)
	}
	if err := (&VibeStorage{APIKey: "wrong", BaseURL: server.URL}).Delete(ctx, "b/1"); err == nil {
		t.Error("a refusal is an error")
	}
}
