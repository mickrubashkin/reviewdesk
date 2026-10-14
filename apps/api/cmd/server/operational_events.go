package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	featureauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/features/auth"
)

type operationalEventContextKey string

const requestIDContextKey operationalEventContextKey = "request_id"

type operationalEventFilters struct {
	Level     string
	EventType string
	UserEmail string
	RequestID string
	Path      string
	Limit     int
}

type operationalEvent struct {
	Level      string
	EventType  string
	Message    string
	UserID     *string
	UserEmail  *string
	RequestID  *string
	Method     *string
	Path       *string
	StatusCode *int
	DurationMS *int
	Metadata   any
}

func registerOperationalRoutes(r chi.Router, dbpool *pgxpool.Pool) {
	r.Get("/api/admin/operational-events", listOperationalEventsHandler(dbpool))
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDContextKey, requestID)))
	})
}

func operationalEventMiddleware(dbpool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			startedAt := time.Now()
			next.ServeHTTP(recorder, r)

			statusCode := recorder.statusCode
			if statusCode < http.StatusInternalServerError && statusCode != http.StatusForbidden {
				return
			}

			durationMS := int(time.Since(startedAt).Milliseconds())
			level := "warn"
			if statusCode >= http.StatusInternalServerError {
				level = "error"
			}
			userID, userEmail := operationalEventUser(r)
			requestID := requestIDFromContext(r.Context())
			method := r.Method
			path := r.URL.Path
			message := fmt.Sprintf("%s %s returned %d", method, path, statusCode)

			logOperationalEvent(r.Context(), dbpool, operationalEvent{
				Level:      level,
				EventType:  "api_request_failed",
				Message:    message,
				UserID:     userID,
				UserEmail:  userEmail,
				RequestID:  stringPointerIfNotEmpty(requestID),
				Method:     &method,
				Path:       &path,
				StatusCode: &statusCode,
				DurationMS: &durationMS,
				Metadata: map[string]any{
					"query":      r.URL.RawQuery,
					"user_agent": r.UserAgent(),
					"ip_address": featureauth.RequestIPAddress(r),
				},
			})
		})
	}
}

func listOperationalEventsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !featureauth.RequireAdmin(w, r) {
			return
		}

		filters := operationalEventFilters{
			Level:     strings.TrimSpace(r.URL.Query().Get("level")),
			EventType: strings.TrimSpace(r.URL.Query().Get("event_type")),
			UserEmail: strings.TrimSpace(r.URL.Query().Get("user_email")),
			RequestID: strings.TrimSpace(r.URL.Query().Get("request_id")),
			Path:      strings.TrimSpace(r.URL.Query().Get("path")),
			Limit:     parseOperationalEventsLimit(r.URL.Query().Get("limit")),
		}
		events, err := listOperationalEvents(r.Context(), dbpool, filters)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list operational events: %v\n", err)
			http.Error(w, "failed to load operational events", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(events)
	}
}

func insertOperationalEvent(ctx context.Context, dbpool *pgxpool.Pool, event operationalEvent) error {
	metadataJSON, err := json.Marshal(emptyMapIfNil(event.Metadata))
	if err != nil {
		return err
	}

	_, err = dbpool.Exec(ctx, `
		INSERT INTO operational_events (
			level,
			event_type,
			message,
			user_id,
			user_email,
			request_id,
			method,
			path,
			status_code,
			duration_ms,
			metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb);
	`,
		event.Level,
		event.EventType,
		event.Message,
		event.UserID,
		event.UserEmail,
		event.RequestID,
		event.Method,
		event.Path,
		event.StatusCode,
		event.DurationMS,
		metadataJSON,
	)

	return err
}

func logOperationalEvent(ctx context.Context, dbpool *pgxpool.Pool, event operationalEvent) {
	if err := insertOperationalEvent(ctx, dbpool, event); err != nil {
		if isOperationalEventsTableMissing(err) || featureauth.IsRequestCanceledError(err) {
			return
		}
		fmt.Fprintf(os.Stderr, "failed to insert operational event %s: %v\n", event.EventType, err)
	}
}

func listOperationalEvents(ctx context.Context, dbpool *pgxpool.Pool, filters operationalEventFilters) ([]OperationalEventItem, error) {
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	where := []string{}
	args := []any{}

	if filters.Level != "" {
		args = append(args, filters.Level)
		where = append(where, fmt.Sprintf("level = $%d", len(args)))
	}
	if filters.EventType != "" {
		args = append(args, filters.EventType)
		where = append(where, fmt.Sprintf("event_type = $%d", len(args)))
	}
	if filters.UserEmail != "" {
		args = append(args, "%"+strings.ToLower(filters.UserEmail)+"%")
		where = append(where, fmt.Sprintf("lower(coalesce(user_email, '')) LIKE $%d", len(args)))
	}
	if filters.RequestID != "" {
		args = append(args, filters.RequestID)
		where = append(where, fmt.Sprintf("request_id = $%d", len(args)))
	}
	if filters.Path != "" {
		args = append(args, "%"+strings.ToLower(filters.Path)+"%")
		where = append(where, fmt.Sprintf("lower(coalesce(path, '')) LIKE $%d", len(args)))
	}

	query := `
		SELECT
			id,
			level,
			event_type,
			message,
			user_id,
			user_email,
			request_id,
			method,
			path,
			status_code,
			duration_ms,
			metadata,
			created_at
		FROM operational_events
	`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d;", len(args))

	rows, err := dbpool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []OperationalEventItem{}
	for rows.Next() {
		var event OperationalEventItem
		var metadata []byte
		if err := rows.Scan(
			&event.ID,
			&event.Level,
			&event.EventType,
			&event.Message,
			&event.UserID,
			&event.UserEmail,
			&event.RequestID,
			&event.Method,
			&event.Path,
			&event.StatusCode,
			&event.DurationMS,
			&metadata,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		event.Metadata = json.RawMessage(metadata)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func parseOperationalEventsLimit(value string) int {
	if value == "" {
		return 100
	}
	var limit int
	if _, err := fmt.Sscanf(value, "%d", &limit); err != nil {
		return 100
	}
	return limit
}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey).(string)
	return value
}

func operationalEventUser(r *http.Request) (*string, *string) {
	user, ok := auth.FromRequest(r)
	if !ok {
		return nil, nil
	}
	return &user.ID, &user.Email
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}

func stringPointerIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func isOperationalEventsTableMissing(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (recorder *statusRecorder) WriteHeader(statusCode int) {
	recorder.statusCode = statusCode
	recorder.ResponseWriter.WriteHeader(statusCode)
}

func (recorder *statusRecorder) Flush() {
	flusher, ok := recorder.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}
