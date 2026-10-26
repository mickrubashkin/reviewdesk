package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	coreauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/demo"
)

const sessionCookieName = "email_review_session"

// Context key moved to core/auth

type authRequest struct {
	Email string `json:"email"`
}

type otpCodeRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

func RegisterAuthRoutes(r chi.Router, dbpool *pgxpool.Pool, emailSender EmailSender) {
	r.Post("/api/auth/request-code", requestOTPCodeHandler(dbpool, emailSender, newOTPRequestLimiter()))
	r.Post("/api/auth/verify-code", verifyOTPCodeHandler(dbpool))
	r.Post("/api/auth/dev-login", devLoginHandler(dbpool))
	r.Post("/api/auth/demo-login", demoLoginHandler(dbpool, newDemoLoginLimiter()))
	r.Get("/api/auth/events", listAuthEventsHandler(dbpool))
	r.Get("/api/auth/me", meHandler(dbpool))
	r.Post("/api/auth/logout", logoutHandler(dbpool))
	r.Get("/api/users/names", listUserNamesHandler(dbpool))
	r.Get("/api/admin/users", listAdminUsersHandler(dbpool))
	r.Post("/api/admin/users", createAdminUserHandler(dbpool))
	r.Patch("/api/admin/users/{id}/role", updateAdminUserRoleHandler(dbpool))
	r.Patch("/api/admin/users/{id}/languages", updateAdminUserLanguagesHandler(dbpool))
	r.Patch("/api/admin/users/{id}/boards", updateAdminUserBoardsHandler(dbpool))
	r.Patch("/api/admin/users/{id}/name", updateAdminUserNameHandler(dbpool))
	r.Post("/api/admin/users/{id}/merge", mergeUsersHandler(dbpool))
	r.Delete("/api/admin/users/{id}", deleteUserHandler(dbpool))
	r.Post("/api/admin/users/{id}/aliases", addUserAliasHandler(dbpool))
	r.Delete("/api/admin/users/{id}/aliases/{email}", removeUserAliasHandler(dbpool))
}

func Middleware(dbpool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") ||
				strings.HasPrefix(r.URL.Path, "/api/auth/") ||
				r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			user, err := currentUserFromRequest(r.Context(), dbpool, r)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			touchUserLastSeen(r.Context(), dbpool, user)

			next.ServeHTTP(w, r.WithContext(coreauth.WithUser(r.Context(), user)))
		})
	}
}

func SameOriginMutationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutatingMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		if !isAllowedRequestOrigin(r) {
			http.Error(w, "invalid request origin", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isMutatingMethod(method string) bool {
	return method != http.MethodGet &&
		method != http.MethodHead &&
		method != http.MethodOptions
}

func isAllowedRequestOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Browsers send Origin on every non-GET request. Without it, accept only
		// what Fetch Metadata marks as same-origin; requests with neither header
		// are not from a page and must not ride on the session cookie.
		return strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "same-origin")
	}

	originURL, err := url.Parse(origin)
	if err != nil || originURL.Scheme == "" || originURL.Host == "" {
		return false
	}

	if strings.EqualFold(originURL.Host, r.Host) {
		return true
	}

	for _, allowedOrigin := range allowedAuthOrigins() {
		if strings.EqualFold(origin, allowedOrigin) {
			return true
		}
	}

	return false
}

func allowedAuthOrigins() []string {
	value := strings.TrimSpace(os.Getenv("AUTH_ALLOWED_ORIGINS"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("CORS_ORIGIN"))
	}
	origins := []string{}

	for _, origin := range strings.Split(value, ",") {
		normalizedOrigin := strings.TrimRight(strings.TrimSpace(origin), "/")
		if normalizedOrigin != "" {
			origins = append(origins, normalizedOrigin)
		}
	}

	return origins
}

func touchUserLastSeen(ctx context.Context, dbpool *pgxpool.Pool, user AuthUser) {
	_, err := dbpool.Exec(ctx, `
		UPDATE users
		SET last_seen_at = now()
		WHERE id = $1
			AND (
				last_seen_at IS NULL
				OR last_seen_at < now() - interval '10 minutes'
			);
	`, user.ID)
	if err != nil {
		if IsRequestCanceledError(err) {
			return
		}
		fmt.Fprintf(os.Stderr, "failed to update last_seen_at for user %s: %v\n", user.Email, err)
	}
}

func IsRequestCanceledError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func requestOTPCodeHandler(dbpool *pgxpool.Pool, emailSender EmailSender, limiter otpRequestLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if limiter.ipLimited(w, r) {
			writeTooManyOTPRequests(w)
			return
		}

		var payload authRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeAuthOK(w)
			return
		}

		email := normalizeEmail(payload.Email)
		if !isAllowedAuthEmail(email) {
			writeAuthOK(w)
			return
		}

		if limiter.emailLimited(w, r, email) {
			logAuthEvent(r.Context(), dbpool, r, AuthEvent{
				Email:     email,
				EventType: "otp_rate_limited",
				Success:   false,
			})
			writeTooManyOTPRequests(w)
			return
		}

		user, err := upsertAuthUser(r.Context(), dbpool, email)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to upsert auth user %s: %v\n", email, err)
			writeAuthOK(w)
			return
		}
		logAuthEvent(r.Context(), dbpool, r, AuthEvent{
			UserID:    &user.ID,
			Email:     user.Email,
			EventType: "otp_requested",
			Success:   true,
		})

		code, err := randomOTPCode()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create OTP code for %s: %v\n", email, err)
			http.Error(w, "failed to create login code", http.StatusInternalServerError)
			return
		}

		if err := saveOTPCode(r.Context(), dbpool, user.Email, code); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save OTP code for %s: %v\n", email, err)
			http.Error(w, "failed to create login code", http.StatusInternalServerError)
			return
		}

		if err := emailSender.SendLoginCode(r.Context(), user.Email, code); err != nil {
			fmt.Fprintf(os.Stderr, "failed to send OTP code for %s: %v\n", user.Email, err)
		}

		writeAuthOK(w)
	}
}

func verifyOTPCodeHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload otpCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		email := normalizeEmail(payload.Email)
		if !isAllowedAuthEmail(email) || !isValidOTPCodeFormat(payload.Code) {
			if email != "" {
				logAuthEvent(r.Context(), dbpool, r, AuthEvent{
					Email:     email,
					EventType: "failed_otp",
					Success:   false,
				})
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if err := consumeOTPCode(r.Context(), dbpool, email, payload.Code); err != nil {
			logAuthEvent(r.Context(), dbpool, r, AuthEvent{
				Email:     email,
				EventType: "failed_otp",
				Success:   false,
			})
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := upsertAuthUser(r.Context(), dbpool, email)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to upsert auth user %s: %v\n", email, err)
			http.Error(w, "failed to sign in", http.StatusInternalServerError)
			return
		}
		logAuthEvent(r.Context(), dbpool, r, AuthEvent{
			UserID:    &user.ID,
			Email:     user.Email,
			EventType: "otp_login",
			Success:   true,
		})

		if err := createSessionCookie(r.Context(), dbpool, w, user); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create OTP session for %s: %v\n", user.Email, err)
			http.Error(w, "failed to sign in", http.StatusInternalServerError)
			return
		}

		writeAuthOK(w)
	}
}

func devLoginHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !devLoginEnabled() {
			http.NotFound(w, r)
			return
		}

		var payload authRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		email := normalizeEmail(payload.Email)
		if !isAllowedAuthEmail(email) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := upsertAuthUser(r.Context(), dbpool, email)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to upsert dev auth user %s: %v\n", email, err)
			http.Error(w, "failed to sign in", http.StatusInternalServerError)
			return
		}
		logAuthEvent(r.Context(), dbpool, r, AuthEvent{
			UserID:    &user.ID,
			Email:     user.Email,
			EventType: "dev_login",
			Success:   true,
		})

		if err := createSessionCookie(r.Context(), dbpool, w, user); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create dev session for %s: %v\n", user.Email, err)
			http.Error(w, "failed to sign in", http.StatusInternalServerError)
			return
		}

		writeAuthOK(w)
	}
}

// demoLoginHandler lets a visitor of the public demo in without a code:
// each visitor gets a sandbox, their own user and copy of the demo board,
// and a session that ends with it.
func demoLoginHandler(dbpool *pgxpool.Pool, limiter *httprate.RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !demoLoginEnabled() {
			http.NotFound(w, r)
			return
		}
		// Every entry seeds a board; one address cannot flood the demo.
		if limiter.OnLimit(w, r, "demo:"+RequestIPAddress(r)) {
			http.Error(w, "too many demo visits from this address, try again later", http.StatusTooManyRequests)
			return
		}

		// Where the visitor came from, for the demo visit stats. Optional.
		var payload struct {
			Referrer  string `json:"referrer"`
			UTMSource string `json:"utm_source"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&payload)

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to sign in", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())
		visitor, err := demo.CreateSandbox(r.Context(), tx)
		if err == nil {
			err = demo.RecordVisit(r.Context(), tx, visitor, demo.VisitEntry{
				Referrer:  payload.Referrer,
				UTMSource: payload.UTMSource,
				UserAgent: r.UserAgent(),
			})
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create demo sandbox: %v\n", err)
			http.Error(w, "failed to sign in", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to sign in", http.StatusInternalServerError)
			return
		}
		user := AuthUser{ID: visitor.ID, Email: visitor.Email, Role: visitor.Role}
		logAuthEvent(r.Context(), dbpool, r, AuthEvent{
			UserID:    &user.ID,
			Email:     user.Email,
			EventType: "demo_login",
			Success:   true,
		})

		if err := createSessionCookieUntil(r.Context(), dbpool, w, user, time.Now().Add(demo.SandboxLifetime)); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create demo session for %s: %v\n", user.Email, err)
			http.Error(w, "failed to sign in", http.StatusInternalServerError)
			return
		}

		writeAuthOK(w)
	}
}

func meHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := currentUserFromRequest(r.Context(), dbpool, r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		touchUserLastSeen(r.Context(), dbpool, user)

		response := meResponse{AuthUser: user}
		if err := dbpool.QueryRow(r.Context(), `
			SELECT languages, managed_boards FROM users WHERE id = $1;
		`, user.ID).Scan(&response.Languages, &response.ManagedBoards); err != nil {
			http.Error(w, "failed to load user", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

func logoutHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			if user, userErr := currentUserFromRequest(r.Context(), dbpool, r); userErr == nil {
				logAuthEvent(r.Context(), dbpool, r, AuthEvent{
					UserID:    &user.ID,
					Email:     user.Email,
					EventType: "logout",
					Success:   true,
				})
			}
			_, _ = dbpool.Exec(r.Context(), `
				DELETE FROM sessions
				WHERE session_hash = $1;
			`, hashToken(cookie.Value))
		}

		http.SetCookie(w, expiredSessionCookie())
		writeAuthOK(w)
	}
}

func listAuthEventsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := currentUserFromRequest(r.Context(), dbpool, r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !isAdminUser(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		filters := AuthEventFilters{
			Email:     strings.TrimSpace(r.URL.Query().Get("email")),
			EventType: strings.TrimSpace(r.URL.Query().Get("event_type")),
			Success:   strings.TrimSpace(r.URL.Query().Get("success")),
			Limit:     ParseLimit(r.URL.Query().Get("limit")),
		}

		events, err := listAuthEvents(r.Context(), dbpool, filters)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list auth events: %v\n", err)
			http.Error(w, "failed to load auth events", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(events)
	}
}

func ParseLimit(value string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}

	return limit
}

func logAuthEvent(ctx context.Context, dbpool *pgxpool.Pool, r *http.Request, event AuthEvent) {
	if err := insertAuthEvent(ctx, dbpool, r, event); err != nil {
		fmt.Fprintf(os.Stderr, "failed to insert auth event %s for %s: %v\n", event.EventType, event.Email, err)
	}
}

func currentUserFromRequest(ctx context.Context, dbpool *pgxpool.Pool, r *http.Request) (AuthUser, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return AuthUser{}, fmt.Errorf("missing session")
	}

	var user AuthUser
	err = dbpool.QueryRow(ctx, `
		SELECT u.id, u.email, u.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.session_hash = $1
			AND s.expires_at > now()
		LIMIT 1;
	`, hashToken(cookie.Value)).Scan(&user.ID, &user.Email, &user.Role)
	if err != nil {
		return AuthUser{}, err
	}
	// Shared mailboxes lose access even if they signed in before the ban.
	if isGenericAddress(user.Email) {
		return AuthUser{}, errGenericAddress
	}

	return user, nil
}

func createSessionCookie(ctx context.Context, dbpool *pgxpool.Pool, w http.ResponseWriter, user AuthUser) error {
	return createSessionCookieUntil(ctx, dbpool, w, user, time.Now().Add(30*24*time.Hour))
}

func createSessionCookieUntil(ctx context.Context, dbpool *pgxpool.Pool, w http.ResponseWriter, user AuthUser, expiresAt time.Time) error {
	sessionToken, err := randomToken()
	if err != nil {
		return err
	}

	_, err = dbpool.Exec(ctx, `
		INSERT INTO sessions (session_hash, user_id, expires_at)
		VALUES ($1, $2, $3);
	`, hashToken(sessionToken), user.ID, expiresAt)
	if err != nil {
		return err
	}

	http.SetCookie(w, sessionCookie(sessionToken, expiresAt))
	return nil
}

func upsertAuthUser(ctx context.Context, dbpool *pgxpool.Pool, email string) (AuthUser, error) {
	if user, ok, err := resolveAliasUser(ctx, dbpool, email); err != nil || ok {
		return user, err
	}

	role := "reviewer"
	if email == bootstrapSuperAdminEmail() {
		role = "super_admin"
	}

	var user AuthUser
	err := dbpool.QueryRow(ctx, `
		INSERT INTO users (email, role)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET
			role = CASE
				WHEN users.role <> 'super_admin' AND EXCLUDED.role = 'super_admin' THEN 'super_admin'
				ELSE users.role
			END,
			updated_at = now()
		RETURNING id, email, role;
	`, email, role).Scan(&user.ID, &user.Email, &user.Role)

	return user, err
}

type updateUserRoleRequest struct {
	Role string `json:"role"`
}

type createUserRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func listAdminUsersHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Board managers read the list too: owner pickers on their board use it.
		if manages, err := coreauth.ManagesAnyBoard(r.Context(), dbpool, user); err != nil || !manages {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		rows, err := dbpool.Query(r.Context(), `
			SELECT `+userAdminColumns+`
			FROM users
			ORDER BY email;
		`)
		if err != nil {
			http.Error(w, "failed to load users", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		users := []UserAdminItem{}
		for rows.Next() {
			var item UserAdminItem
			if err := rows.Scan(item.scanTargets()...); err != nil {
				http.Error(w, "failed to read users", http.StatusInternalServerError)
				return
			}
			users = append(users, item)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "failed to read users", http.StatusInternalServerError)
			return
		}
		if err := attachAliases(r.Context(), dbpool, users); err != nil {
			http.Error(w, "failed to read users", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(users)
	}
}

func createAdminUserHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !isAdminUser(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var request createUserRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		email := normalizeEmail(request.Email)
		if email == "" || !strings.Contains(email, "@") {
			http.Error(w, "invalid email", http.StatusBadRequest)
			return
		}
		if isGenericAddress(email) {
			http.Error(w, errGenericAddress.Error(), http.StatusBadRequest)
			return
		}
		role := strings.ToLower(strings.TrimSpace(request.Role))
		if !isValidUserRole(role) {
			http.Error(w, "invalid role", http.StatusBadRequest)
			return
		}
		if !canCreateUserRole(user, role) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		_, isAlias, err := resolveAliasUser(r.Context(), dbpool, email)
		if err != nil {
			http.Error(w, "failed to create user", http.StatusInternalServerError)
			return
		}
		if isAlias {
			http.Error(w, "this address is an alias of another user", http.StatusConflict)
			return
		}

		var createdUser UserAdminItem
		err = dbpool.QueryRow(r.Context(), `
			INSERT INTO users (email, role)
			VALUES ($1, $2)
			ON CONFLICT (email) DO NOTHING
			RETURNING `+userAdminColumns+`;
		`, email, role).Scan(createdUser.scanTargets()...)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "user already exists", http.StatusConflict)
				return
			}
			http.Error(w, "failed to create user", http.StatusInternalServerError)
			return
		}

		logAuthEvent(r.Context(), dbpool, r, AuthEvent{
			UserID:    &createdUser.ID,
			Email:     createdUser.Email,
			EventType: "admin_user_created",
			Success:   true,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createdUser)
	}
}

func updateAdminUserRoleHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := coreauth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !isSuperAdminUser(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		targetUserID := chi.URLParam(r, "id")
		var request updateUserRoleRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		role := strings.ToLower(strings.TrimSpace(request.Role))
		if !isValidUserRole(role) {
			http.Error(w, "invalid role", http.StatusBadRequest)
			return
		}
		if targetUserID == user.ID && role != "super_admin" {
			http.Error(w, "cannot change your own super admin role", http.StatusBadRequest)
			return
		}

		var updatedUser UserAdminItem
		err := dbpool.QueryRow(r.Context(), `
			UPDATE users
			SET role = $2,
				updated_at = now()
			WHERE id = $1
			RETURNING `+userAdminColumns+`;
		`, targetUserID, role).Scan(updatedUser.scanTargets()...)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updatedUser)
	}
}

func isAdminUser(user AuthUser) bool {
	return coreauth.IsAdmin(user)
}

func isSuperAdminUser(user AuthUser) bool {
	return coreauth.IsSuperAdmin(user)
}

func isValidUserRole(role string) bool {
	return coreauth.IsValidRole(role)
}

func canCreateUserRole(actor AuthUser, role string) bool {
	if isSuperAdminUser(actor) {
		return true
	}
	return actor.Role == "admin" && role == "reviewer"
}

func bootstrapSuperAdminEmail() string {
	if email := normalizeEmail(os.Getenv("AUTH_BOOTSTRAP_SUPER_ADMIN_EMAIL")); email != "" {
		return email
	}
	return normalizeEmail(os.Getenv("AUTH_BOOTSTRAP_ADMIN_EMAIL"))
}

func saveOTPCode(ctx context.Context, dbpool *pgxpool.Pool, email string, code string) error {
	expiresAt := time.Now().Add(10 * time.Minute)
	_, err := dbpool.Exec(ctx, `
		UPDATE auth_otp_codes
		SET used_at = now()
		WHERE email = $1
			AND used_at IS NULL;
	`, email)
	if err != nil {
		return err
	}

	_, err = dbpool.Exec(ctx, `
		INSERT INTO auth_otp_codes (email, code_hash, expires_at)
		VALUES ($1, $2, $3);
	`, email, hashToken(code), expiresAt)

	return err
}

func consumeOTPCode(ctx context.Context, dbpool *pgxpool.Pool, email string, code string) error {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var id string
	var codeHash string
	var attempts int
	err = tx.QueryRow(ctx, `
		SELECT id, code_hash, attempts
		FROM auth_otp_codes
		WHERE email = $1
			AND used_at IS NULL
			AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE;
	`, email).Scan(&id, &codeHash, &attempts)
	if err != nil {
		return err
	}

	if attempts >= 5 {
		return pgx.ErrNoRows
	}

	if codeHash != hashToken(strings.TrimSpace(code)) {
		_, _ = tx.Exec(ctx, `
			UPDATE auth_otp_codes
			SET attempts = attempts + 1
			WHERE id = $1;
		`, id)
		if err := tx.Commit(ctx); err != nil {
			return err
		}

		return pgx.ErrNoRows
	}

	_, err = tx.Exec(ctx, `
		UPDATE auth_otp_codes
		SET used_at = now(),
			attempts = attempts + 1
		WHERE id = $1;
	`, id)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func isAllowedAuthEmail(email string) bool {
	allowedDomains := allowedAuthDomains()
	if len(allowedDomains) == 0 || email == "" || isGenericAddress(email) {
		return false
	}

	_, domain, ok := strings.Cut(email, "@")
	if !ok {
		return false
	}

	for _, allowedDomain := range allowedDomains {
		if domain == allowedDomain {
			return true
		}
	}

	return false
}

func allowedAuthDomains() []string {
	value := strings.TrimSpace(os.Getenv("AUTH_ALLOWED_DOMAINS"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("AUTH_ALLOWED_DOMAIN"))
	}

	domains := []string{}
	for _, domain := range strings.Split(value, ",") {
		normalizedDomain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "@")
		if normalizedDomain != "" {
			domains = append(domains, normalizedDomain)
		}
	}

	return domains
}

func isValidOTPCodeFormat(code string) bool {
	normalizedCode := strings.TrimSpace(code)
	if len(normalizedCode) != 6 {
		return false
	}

	for _, char := range normalizedCode {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func randomOTPCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%06d", value.Int64()), nil
}

func hashToken(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func sessionCookie(value string, expiresAt time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   authCookieSecure(),
	}
}

func expiredSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   authCookieSecure(),
	}
}

func authCookieSecure() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_COOKIE_SECURE")), "true")
}

func devLoginEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_DEV_LOGIN_ENABLED")), "true")
}

func demoLoginEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_DEMO_LOGIN_ENABLED")), "true")
}

func writeAuthOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func RequireAdmin(w http.ResponseWriter, r *http.Request) bool {
	user, ok := coreauth.FromRequest(r)
	if !ok || !coreauth.IsAdmin(user) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}
