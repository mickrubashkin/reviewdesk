package auth

import (
	"context"
	"time"

	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type AuthUser = coreauth.AuthUser

type EmailSender interface {
	SendLoginCode(ctx context.Context, toEmail string, code string) error
}

type UserAdminItem struct {
	ID         string     `json:"id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

type AuthEventItem struct {
	ID        string    `json:"id"`
	UserID    *string   `json:"user_id"`
	Email     string    `json:"email"`
	EventType string    `json:"event_type"`
	Success   bool      `json:"success"`
	IPAddress *string   `json:"ip_address"`
	UserAgent *string   `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}


