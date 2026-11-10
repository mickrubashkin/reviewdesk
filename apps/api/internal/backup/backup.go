// Package backup copies the database to Vibe Storage once a day. On VibeCode
// the database lives on the app's own VM, and the platform backs up nothing
// there.
package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Store keeps the backups.
type Store interface {
	Put(ctx context.Context, key string, body []byte) error
	Keys(ctx context.Context, prefix string) ([]string, error)
	Delete(ctx context.Context, key string) error
}

// Sender posts a message to the owner's chat.
type Sender interface {
	Send(ctx context.Context, text string) error
}

// Dumper returns a gzipped dump of the database.
type Dumper func(ctx context.Context) ([]byte, error)

type Job struct {
	Store  Store
	Dump   Dumper
	Prefix string
	// Keep is how many daily backups stay; older ones are deleted.
	Keep int
	// Notify, when set, hears about a day's first failed backup.
	Notify Sender
}

const (
	checkEvery    = time.Hour
	notifyTimeout = 45 * time.Second
	// maxUpload is the most Vibe Storage takes in one request.
	maxUpload = 10 << 20
)

var dailyName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.sql\.gz$`)

// FromEnv builds the job for this server's database: DATABASE_URL, the
// platform key BITRIX_API_KEY, and optionally DB_BACKUP_PREFIX and
// DB_BACKUP_KEEP.
func FromEnv() (Job, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Job{}, errors.New("DATABASE_URL is not set")
	}
	store, err := VibeStorageFromEnv()
	if err != nil {
		return Job{}, err
	}
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return Job{}, errors.New("pg_dump is not installed")
	}
	prefix := strings.TrimSpace(os.Getenv("DB_BACKUP_PREFIX"))
	if prefix == "" {
		prefix = "reviewdesk/backups/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	keep := 30
	if value := strings.TrimSpace(os.Getenv("DB_BACKUP_KEEP")); value != "" {
		if keep, err = strconv.Atoi(value); err != nil || keep < 1 {
			return Job{}, fmt.Errorf("DB_BACKUP_KEEP %q is not a positive number", value)
		}
	}
	return Job{Store: store, Dump: PgDump(databaseURL), Prefix: prefix, Keep: keep}, nil
}

// Run makes the day's backup, until ctx ends, and tells the owner when a
// day's backup fails the first time; it keeps trying every hour.
func Run(ctx context.Context, job Job) {
	failedDay := ""
	for {
		now := time.Now()
		if err := job.RunDue(ctx, now); err != nil {
			fmt.Fprintf(os.Stderr, "database backup: %v\n", err)
			if day := now.UTC().Format(time.DateOnly); failedDay != day && job.Notify != nil {
				failedDay = day
				notifyCtx, cancel := context.WithTimeout(ctx, notifyTimeout)
				if err := job.Notify.Send(notifyCtx, failureMessage(err)); err != nil {
					fmt.Fprintf(os.Stderr, "database backup: tell the owner: %v\n", err)
				}
				cancel()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(checkEvery):
		}
	}
}

// RunDue uploads today's backup unless the store has it, then deletes the
// backups beyond Keep.
func (j Job) RunDue(ctx context.Context, now time.Time) error {
	keys, err := j.Store.Keys(ctx, j.Prefix)
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}
	key := j.Prefix + now.UTC().Format(time.DateOnly) + ".sql.gz"
	if !slices.Contains(keys, key) {
		body, err := j.Dump(ctx)
		if err != nil {
			return fmt.Errorf("dump: %w", err)
		}
		if len(body) > maxUpload {
			return fmt.Errorf("dump is %d MB, over the %d MB one upload takes", len(body)>>20, maxUpload>>20)
		}
		if err := j.Store.Put(ctx, key, body); err != nil {
			return fmt.Errorf("upload %s: %w", key, err)
		}
		fmt.Printf("Database backup: %s, %d KB\n", key, len(body)>>10)
		keys = append(keys, key)
	}
	for _, old := range Expired(keys, j.Prefix, j.Keep) {
		if err := j.Store.Delete(ctx, old); err != nil {
			return fmt.Errorf("delete %s: %w", old, err)
		}
	}
	return nil
}

// Expired returns the daily backups beyond the newest keep. Other files
// under the prefix are not touched.
func Expired(keys []string, prefix string, keep int) []string {
	daily := []string{}
	for _, key := range keys {
		if name, ok := strings.CutPrefix(key, prefix); ok && dailyName.MatchString(name) {
			daily = append(daily, key)
		}
	}
	if len(daily) <= keep {
		return nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(daily)))
	return daily[keep:]
}

// PgDump dumps the database with pg_dump as plain SQL, without owners and
// grants, so that it restores into any database with psql.
func PgDump(databaseURL string) Dumper {
	return func(ctx context.Context) ([]byte, error) {
		var out bytes.Buffer
		zipped := gzip.NewWriter(&out)
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, "pg_dump", "--no-owner", "--no-acl", "--format=plain", "--dbname="+databaseURL)
		cmd.Stdout = zipped
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		if err := zipped.Close(); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
}

func failureMessage(err error) string {
	text := err.Error()
	if runes := []rune(text); len(runes) > 500 {
		text = string(runes[:500]) + "…"
	}
	return "ReviewDesk: today's database backup failed, trying again every hour.\n" + text
}
