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
	ID    string `json:"id"`
	Email string `json:"email"`
	// Name is how the person is shown; empty means show the email.
	Name       string     `json:"name"`
	Role       string     `json:"role"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	// Languages the person translates, e.g. ["de"]; empty for most.
	Languages []string `json:"languages"`
	// ManagedBoards are the keys of boards the person manages like an admin;
	// empty for most, admins manage every board anyway.
	ManagedBoards []string `json:"managed_boards"`
	// Aliases are other work addresses that sign in to this user.
	Aliases []string `json:"aliases"`
}

// userAdminColumns matches UserAdminItem.scanTargets.
const userAdminColumns = "id, email, name, role, created_at, updated_at, last_seen_at, languages, managed_boards"

func (item *UserAdminItem) scanTargets() []any {
	return []any{&item.ID, &item.Email, &item.Name, &item.Role, &item.CreatedAt, &item.UpdatedAt, &item.LastSeenAt, &item.Languages, &item.ManagedBoards}
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
