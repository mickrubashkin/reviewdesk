package auth

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

const (
	otpRequestWindow    = 10 * time.Minute
	otpRequestsPerEmail = 3
	otpRequestsPerIP    = 10
	// A demo visit seeds a whole board; an office behind one address still
	// fits.
	demoLoginsPerIP = 20
	demoLoginWindow = time.Hour
)

// otpRequestLimiter caps how many login codes can be requested. Counters live
// in memory, so they are per API instance and reset on restart; that is
// enough for a single-instance deployment.
type otpRequestLimiter struct {
	byEmail *httprate.RateLimiter
	byIP    *httprate.RateLimiter
}

func newDemoLoginLimiter() *httprate.RateLimiter {
	return httprate.NewRateLimiter(demoLoginsPerIP, demoLoginWindow)
}

func newOTPRequestLimiter() otpRequestLimiter {
	return otpRequestLimiter{
		byEmail: httprate.NewRateLimiter(otpRequestsPerEmail, otpRequestWindow),
		byIP:    httprate.NewRateLimiter(otpRequestsPerIP, otpRequestWindow),
	}
}

// ipLimited counts every request-code call from the client address, including
// ones for unknown or disallowed emails, so one address cannot cycle through
// many mailboxes.
func (limiter otpRequestLimiter) ipLimited(w http.ResponseWriter, r *http.Request) bool {
	return limiter.byIP.OnLimit(w, r, "ip:"+RequestIPAddress(r))
}

// emailLimited protects a single mailbox from being flooded with codes,
// whatever addresses the requests come from.
func (limiter otpRequestLimiter) emailLimited(w http.ResponseWriter, r *http.Request, email string) bool {
	return limiter.byEmail.OnLimit(w, r, "email:"+email)
}

func writeTooManyOTPRequests(w http.ResponseWriter) {
	http.Error(w, "too many login code requests, try again later", http.StatusTooManyRequests)
}
