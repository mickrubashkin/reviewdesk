package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

// AI translation calls run as background jobs (#199): on VibeCode the
// platform gateway drops a request the app has not answered in time, and
// the dropped connection used to cancel the model call with it. A job runs
// detached from the request; the client polls it.

const (
	translationJobCreate  = "create"
	translationJobSuggest = "suggest"

	// translationJobTimeout bounds a job, so none stays "running" forever.
	translationJobTimeout = 5 * time.Minute
	// finishedJobShown keeps a finished job in the board's list for a
	// while, so a client that polled it running sees how it ended.
	finishedJobShown = 10 * time.Minute

	translationFailedMessage = "The AI translation failed, try again."
)

// translationJob is one AI translation call and how it went.
type translationJob struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	EmailID        string          `json:"email_id"`
	Language       string          `json:"language"`
	MasterVersion  *int            `json:"master_version"`
	Status         string          `json:"status"`
	Result         json.RawMessage `json:"result"`
	Error          string          `json:"error"`
	CreatedEmailID *string         `json:"created_email_id"`
	StartedBy      *string         `json:"started_by"`
	StartedByEmail string          `json:"started_by_email"`
	StartedAt      time.Time       `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at"`
}

const translationJobColumns = `id, kind, email_id, language, master_version, status, result, error, created_email_id, started_by, started_by_email, started_at, finished_at`

func scanTranslationJob(row pgx.Row) (translationJob, error) {
	var job translationJob
	var result []byte
	err := row.Scan(&job.ID, &job.Kind, &job.EmailID, &job.Language, &job.MasterVersion, &job.Status, &result,
		&job.Error, &job.CreatedEmailID, &job.StartedBy, &job.StartedByEmail, &job.StartedAt, &job.FinishedAt)
	if len(result) > 0 {
		job.Result = result
	}
	return job, err
}

// jobError is a failure whose message the person may read as is.
type jobError struct{ message string }

func (e jobError) Error() string { return e.message }

// translationWork is what a job does; it returns the job's result and, for
// a created translation, the new email.
type translationWork func(ctx context.Context) (result any, createdEmailID *string, err error)

// startTranslationJob records a running job and runs work in the background.
// When the same kind of job already runs for the email and language, it
// returns that one and does not start another.
func startTranslationJob(ctx context.Context, dbpool *pgxpool.Pool, kind, emailID, language string, masterVersion *int, user coreauth.AuthUser, work translationWork) (translationJob, error) {
	job, err := scanTranslationJob(dbpool.QueryRow(ctx, `
		INSERT INTO translation_jobs (kind, email_id, language, sequence, master_version, started_by, started_by_email)
		SELECT $1, e.id, $3, coalesce(e.sequence, ''), $4, $5, $6
		FROM emails e WHERE e.id = $2
		ON CONFLICT DO NOTHING
		RETURNING `+translationJobColumns+`;
	`, kind, emailID, language, masterVersion, user.ID, user.Email))
	if errors.Is(err, pgx.ErrNoRows) {
		return scanTranslationJob(dbpool.QueryRow(ctx, `
			SELECT `+translationJobColumns+` FROM translation_jobs
			WHERE kind = $1 AND email_id = $2 AND language = $3 AND status = 'running';
		`, kind, emailID, language))
	}
	if err != nil {
		return translationJob{}, err
	}
	go runTranslationJob(dbpool, job.ID, work)
	return job, nil
}

func runTranslationJob(dbpool *pgxpool.Pool, jobID string, work translationWork) {
	var result any
	var createdEmailID *string
	var err error
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic: %v", recovered)
		}
		finishTranslationJob(dbpool, jobID, result, createdEmailID, err)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), translationJobTimeout)
	defer cancel()
	result, createdEmailID, err = work(ctx)
}

func finishTranslationJob(dbpool *pgxpool.Pool, jobID string, result any, createdEmailID *string, workErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	status, message := "done", ""
	var resultJSON []byte
	if workErr != nil {
		status, message = "failed", translationFailedMessage
		var userErr jobError
		if errors.As(workErr, &userErr) {
			message = userErr.message
		}
		fmt.Fprintf(os.Stderr, "translation job %s failed: %v\n", jobID, workErr)
	} else if result != nil {
		var err error
		if resultJSON, err = json.Marshal(result); err != nil {
			status, message = "failed", translationFailedMessage
			fmt.Fprintf(os.Stderr, "translation job %s: encode result: %v\n", jobID, err)
		}
	}
	if _, err := dbpool.Exec(ctx, `
		UPDATE translation_jobs
		SET status = $2, result = $3, error = $4, created_email_id = $5, finished_at = now()
		WHERE id = $1 AND status = 'running';
	`, jobID, status, resultJSON, message, createdEmailID); err != nil {
		fmt.Fprintf(os.Stderr, "translation job %s: save the outcome: %v\n", jobID, err)
	}
}

// FailInterruptedTranslationJobs closes the jobs a restart cut off, so no
// cell stays "translating".
func FailInterruptedTranslationJobs(ctx context.Context, dbpool *pgxpool.Pool) (int64, error) {
	tag, err := dbpool.Exec(ctx, `
		UPDATE translation_jobs
		SET status = 'failed', error = 'The app restarted during the translation, try again.', finished_at = now()
		WHERE status = 'running';
	`)
	return tag.RowsAffected(), err
}

// logTranslationCall records the model call on the new email, or on the
// email the job started from when nothing was created.
func logTranslationCall(dbpool *pgxpool.Pool, emailID string, user coreauth.AuthUser, metrics AIAnalysisMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := insertAIAnalysisLog(ctx, dbpool, emailID, &user, metrics, false); err != nil {
		fmt.Fprintf(os.Stderr, "failed to log translation call: %v\n", err)
	}
}

func writeTranslationJob(w http.ResponseWriter, status int, job translationJob) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(job)
}

// listBoardTranslationJobsHandler lists the latest "create" job of each
// email and language on a board that is running, failed and not closed, or
// finished a moment ago; the matrix shows them in the empty cells.
func listBoardTranslationJobsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := coreauth.FromRequest(r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		board := strings.TrimSpace(r.URL.Query().Get("board"))
		if board == "" {
			http.Error(w, "board is required", http.StatusBadRequest)
			return
		}
		rows, err := dbpool.Query(r.Context(), `
			SELECT `+translationJobColumns+` FROM (
				SELECT DISTINCT ON (email_id, language) *
				FROM translation_jobs
				WHERE sequence = $1 AND kind = 'create'
				ORDER BY email_id, language, started_at DESC
			) latest
			WHERE dismissed_at IS NULL
				AND (status IN ('running', 'failed') OR finished_at > now() - make_interval(secs => $2))
			ORDER BY started_at;
		`, board, finishedJobShown.Seconds())
		if err != nil {
			http.Error(w, "failed to load the translations", http.StatusInternalServerError)
			return
		}
		jobs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (translationJob, error) { return scanTranslationJob(row) })
		if err != nil {
			http.Error(w, "failed to load the translations", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jobs": jobs})
	}
}

// latestSuggestionHandler returns the translation's latest suggestion job
// the person has not closed, or 204.
func latestSuggestionHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		if canEdit, err := emails.CanEditEmail(r.Context(), dbpool, user, id); err != nil || !canEdit {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		job, err := scanTranslationJob(dbpool.QueryRow(r.Context(), `
			SELECT `+translationJobColumns+` FROM translation_jobs
			WHERE email_id = $1 AND kind = 'suggest' AND dismissed_at IS NULL
			ORDER BY started_at DESC LIMIT 1;
		`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			http.Error(w, "failed to load the suggestion", http.StatusInternalServerError)
			return
		}
		writeTranslationJob(w, http.StatusOK, job)
	}
}

// dismissTranslationJobHandler closes a finished job: a failure the person
// has read, or a suggestion applied or discarded.
func dismissTranslationJobHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var kind, emailID, language string
		err := dbpool.QueryRow(r.Context(), `SELECT kind, email_id, language FROM translation_jobs WHERE id = $1;`,
			chi.URLParam(r, "jobID")).Scan(&kind, &emailID, &language)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to load the job", http.StatusInternalServerError)
			return
		}
		var allowed bool
		if kind == translationJobCreate {
			allowed, err = emails.CanTranslateInto(r.Context(), dbpool, user, emailID, language)
		} else {
			allowed, err = emails.CanEditEmail(r.Context(), dbpool, user, emailID)
		}
		if err != nil || !allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if _, err := dbpool.Exec(r.Context(), `
			UPDATE translation_jobs SET dismissed_at = now() WHERE id = $1 AND status <> 'running';
		`, chi.URLParam(r, "jobID")); err != nil {
			http.Error(w, "failed to close the job", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
