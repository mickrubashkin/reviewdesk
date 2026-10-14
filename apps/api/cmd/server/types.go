package main

import (
	"encoding/json"
	"time"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)


type AuthUser = auth.AuthUser

type OperationalEventItem struct {
	ID         string          `json:"id"`
	Level      string          `json:"level"`
	EventType  string          `json:"event_type"`
	Message    string          `json:"message"`
	UserID     *string         `json:"user_id"`
	UserEmail  *string         `json:"user_email"`
	RequestID  *string         `json:"request_id"`
	Method     *string         `json:"method"`
	Path       *string         `json:"path"`
	StatusCode *int            `json:"status_code"`
	DurationMS *int            `json:"duration_ms"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
}
