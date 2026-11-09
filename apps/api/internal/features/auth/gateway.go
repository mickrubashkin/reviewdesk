package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// On VibeCode (AUTH_MODE=vibe) there is no sign-in page: the platform's
// gateway signs people in with their Bitrix24 account and passes the user's ID
// in X-Vibe-User-Id on every request. The first time an ID comes in, its
// email is looked up in the platform API (slow: seconds) and the ID is linked
// to the ReviewDesk user with that email or alias. Someone new is created as
// with a one-time code, which needs an allowed domain. Sessions and cookies
// are not used.

var (
	errAccessPending = errors.New("access pending")
	errNoAccess      = errors.New("no access")
)

var portalUserIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

// How long /api/auth/me waits for a first link before the page asks again,
// and how long a refused ID is not looked up again.
const (
	gatewayLinkWait      = 20 * time.Second
	gatewayLinkTimeout   = 5 * time.Minute
	gatewayDeniedRecheck = 10 * time.Minute
)

func GatewayMode() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_MODE")), "vibe")
}

type portalUser struct {
	Email string
	Name  string
}

// lookupPortalUser finds a Bitrix24 user's email and name; tests replace it.
var lookupPortalUser = fetchPortalUser

func fetchPortalUser(ctx context.Context, portalUserID string) (portalUser, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("BITRIX_API_BASE_URL")), "/")
	if base == "" {
		base = "https://vibecode.bitrix24.tech/v1"
	}
	key := strings.TrimSpace(os.Getenv("BITRIX_API_KEY"))
	if key == "" {
		return portalUser{}, errors.New("BITRIX_API_KEY is not set")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/users/"+url.PathEscape(portalUserID), nil)
	if err != nil {
		return portalUser{}, err
	}
	request.Header.Set("X-Api-Key", key)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return portalUser{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return portalUser{}, err
	}
	if response.StatusCode != http.StatusOK {
		return portalUser{}, fmt.Errorf("platform users API returned %d: %.300s", response.StatusCode, body)
	}
	return parsePortalUser(body)
}

// parsePortalUser reads the user from the answer, which wraps it in data.
func parsePortalUser(body []byte) (portalUser, error) {
	type fields struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		LastName string `json:"lastName"`
	}
	var answer struct {
		Data *fields `json:"data"`
		fields
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return portalUser{}, err
	}
	user := answer.fields
	if answer.Data != nil {
		user = *answer.Data
	}
	return portalUser{
		Email: normalizeEmail(user.Email),
		Name:  strings.TrimSpace(strings.TrimSpace(user.Name) + " " + strings.TrimSpace(user.LastName)),
	}, nil
}

// gatewayIdentity is the Bitrix24 user ID the gateway passed. It is trusted
// only from the gateway's tunnel, which reaches the app on the loopback.
func gatewayIdentity(r *http.Request) (string, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return "", false
	}
	id := strings.TrimSpace(r.Header.Get("X-Vibe-User-Id"))
	return id, portalUserIDPattern.MatchString(id)
}

func gatewayUserFromRequest(ctx context.Context, dbpool *pgxpool.Pool, r *http.Request) (AuthUser, error) {
	portalUserID, ok := gatewayIdentity(r)
	if !ok {
		return AuthUser{}, fmt.Errorf("missing gateway identity")
	}

	var user AuthUser
	err := dbpool.QueryRow(ctx, `
		SELECT id, email, role FROM users WHERE portal_user_id = $1;
	`, portalUserID).Scan(&user.ID, &user.Email, &user.Role)
	if err == nil {
		if isGenericAddress(user.Email) {
			return AuthUser{}, errGenericAddress
		}
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AuthUser{}, err
	}

	if gatewayLinks.refused(portalUserID) {
		return AuthUser{}, errNoAccess
	}
	gatewayLinks.start(dbpool, r, portalUserID)
	return AuthUser{}, errAccessPending
}

// waitForGatewayLink lets /api/auth/me hold a first visit until the link is
// made, so the page usually opens without asking again.
func waitForGatewayLink(r *http.Request) {
	if portalUserID, ok := gatewayIdentity(r); ok {
		gatewayLinks.wait(r.Context(), portalUserID, gatewayLinkWait)
	}
}

type gatewayLinker struct {
	mu       sync.Mutex
	inFlight map[string]chan struct{}
	denied   map[string]time.Time
}

var gatewayLinks = &gatewayLinker{
	inFlight: map[string]chan struct{}{},
	denied:   map[string]time.Time{},
}

func (l *gatewayLinker) refused(portalUserID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	until, ok := l.denied[portalUserID]
	if ok && time.Now().After(until) {
		delete(l.denied, portalUserID)
		return false
	}
	return ok
}

// start looks the ID up once at a time; later requests wait for that one.
func (l *gatewayLinker) start(dbpool *pgxpool.Pool, r *http.Request, portalUserID string) {
	l.mu.Lock()
	if _, ok := l.inFlight[portalUserID]; ok {
		l.mu.Unlock()
		return
	}
	done := make(chan struct{})
	l.inFlight[portalUserID] = done
	l.mu.Unlock()

	// The request is gone by the time the lookup ends; the auth log keeps its
	// address and browser.
	request := r.Clone(context.Background())
	go func() {
		linked := linkPortalUser(dbpool, request, portalUserID)
		l.mu.Lock()
		delete(l.inFlight, portalUserID)
		if !linked {
			l.denied[portalUserID] = time.Now().Add(gatewayDeniedRecheck)
		}
		l.mu.Unlock()
		close(done)
	}()
}

func (l *gatewayLinker) wait(ctx context.Context, portalUserID string, timeout time.Duration) {
	l.mu.Lock()
	done, ok := l.inFlight[portalUserID]
	l.mu.Unlock()
	if !ok {
		return
	}
	select {
	case <-done:
	case <-ctx.Done():
	case <-time.After(timeout):
	}
}

// linkPortalUser finds the Bitrix24 user's email and links the ID to the
// ReviewDesk user with it. A failed lookup is not a refusal of access, but it
// is retried only after gatewayDeniedRecheck all the same.
func linkPortalUser(dbpool *pgxpool.Pool, r *http.Request, portalUserID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), gatewayLinkTimeout)
	defer cancel()

	found, err := lookupPortalUser(ctx, portalUserID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to look up Bitrix24 user %s: %v\n", portalUserID, err)
		return false
	}
	known, err := knownEmail(ctx, dbpool, found.Email)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to check gateway user %s: %v\n", found.Email, err)
		return false
	}
	if found.Email == "" || isGenericAddress(found.Email) || (!known && !isAllowedAuthEmail(found.Email)) {
		logAuthEvent(ctx, dbpool, r, AuthEvent{Email: found.Email, EventType: "gateway_denied", Success: false})
		announceGatewayRefusal(found, portalUserID)
		return false
	}

	user, err := upsertAuthUser(ctx, dbpool, found.Email)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to upsert gateway user %s: %v\n", found.Email, err)
		return false
	}
	if _, err := dbpool.Exec(ctx, `
		UPDATE users
		SET portal_user_id = $2,
			name = CASE WHEN name = '' THEN $3 ELSE name END,
			updated_at = now()
		WHERE id = $1;
	`, user.ID, portalUserID, found.Name); err != nil {
		fmt.Fprintf(os.Stderr, "failed to link Bitrix24 user %s to %s: %v\n", portalUserID, user.Email, err)
		return false
	}
	logAuthEvent(ctx, dbpool, r, AuthEvent{UserID: &user.ID, Email: user.Email, EventType: "gateway_link", Success: true})
	return true
}

// knownEmail says whether a user or an alias already has the address: an
// administrator may have added someone outside the allowed domains.
func knownEmail(ctx context.Context, dbpool *pgxpool.Pool, email string) (bool, error) {
	var known bool
	err := dbpool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE email = $1)
			OR EXISTS (SELECT 1 FROM user_email_aliases WHERE email = $1);
	`, email).Scan(&known)
	return known, err
}

// announceGatewayRefusal tells the owner's chat that someone came in from the
// portal and got no access, so an administrator can add them.
func announceGatewayRefusal(found portalUser, portalUserID string) {
	if Notifier == nil {
		return
	}
	who := found.Email
	if who == "" {
		who = "Bitrix24 user " + portalUserID + " (no email)"
	}
	if found.Name != "" {
		who = found.Name + " <" + who + ">"
	}
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	if err := notifyOwner(ctx, "No access to ReviewDesk for "+who+": the address is not in an allowed domain. Add them in Settings → Users (or as an alias of their user) if they should get in."); err != nil {
		fmt.Fprintf(os.Stderr, "gateway refusal of %s: %v\n", who, err)
	}
}
