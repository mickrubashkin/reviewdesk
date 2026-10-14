package auth

import (
	"context"
	"net/http"
)

type AuthUser struct {
	ID    string `json:"-"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type authContextKey string

const authUserContextKey authContextKey = "auth_user"

func FromContext(ctx context.Context) (AuthUser, bool) {
	user, ok := ctx.Value(authUserContextKey).(AuthUser)
	return user, ok
}

func FromRequest(r *http.Request) (AuthUser, bool) {
	return FromContext(r.Context())
}

func WithUser(ctx context.Context, user AuthUser) context.Context {
	return context.WithValue(ctx, authUserContextKey, user)
}

func IsAdmin(user AuthUser) bool {
	return user.Role == "admin" || user.Role == "super_admin"
}

func IsSuperAdmin(user AuthUser) bool {
	return user.Role == "super_admin"
}

func IsValidRole(role string) bool {
	return role == "super_admin" || role == "admin" || role == "reviewer"
}
