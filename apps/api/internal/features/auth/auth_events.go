package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthEventFilters struct {
	Email     string
	EventType string
	Success   string
	Limit     int
}

type AuthEvent struct {
	UserID    *string
	Email     string
	EventType string
	Success   bool
}

func insertAuthEvent(ctx context.Context, dbpool *pgxpool.Pool, r *http.Request, event AuthEvent) error {
	_, err := dbpool.Exec(ctx, `
		INSERT INTO auth_events (
			user_id,
			email,
			event_type,
			success,
			ip_address,
			user_agent
		)
		VALUES ($1, $2, $3, $4, $5, $6);
	`,
		event.UserID,
		event.Email,
		event.EventType,
		event.Success,
		RequestIPAddress(r),
		r.UserAgent(),
	)

	return err
}

func listAuthEvents(ctx context.Context, dbpool *pgxpool.Pool, filters AuthEventFilters) ([]AuthEventItem, error) {
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	where := []string{}
	args := []any{}

	if filters.Email != "" {
		args = append(args, "%"+strings.ToLower(filters.Email)+"%")
		where = append(where, fmt.Sprintf("lower(email) LIKE $%d", len(args)))
	}
	if filters.EventType != "" {
		args = append(args, filters.EventType)
		where = append(where, fmt.Sprintf("event_type = $%d", len(args)))
	}
	if filters.Success != "" {
		args = append(args, filters.Success == "true")
		where = append(where, fmt.Sprintf("success = $%d", len(args)))
	}

	query := `
		SELECT
			id,
			user_id,
			email,
			event_type,
			success,
			ip_address,
			user_agent,
			created_at
		FROM auth_events
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

	events := []AuthEventItem{}
	for rows.Next() {
		var event AuthEventItem
		if err := rows.Scan(
			&event.ID,
			&event.UserID,
			&event.Email,
			&event.EventType,
			&event.Success,
			&event.IPAddress,
			&event.UserAgent,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}

		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func RequestIPAddress(r *http.Request) string {
	for _, headerName := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(headerName))
		if value == "" {
			continue
		}

		ip, _, _ := strings.Cut(value, ",")
		return strings.TrimSpace(ip)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
