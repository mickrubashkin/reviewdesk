package ai

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/demo"
)

// demoAIQuotaRefusal takes one call from a demo visitor's AI limits before a
// model call. It returns an empty message when the call may go ahead, or the
// message and status to refuse it with.
func demoAIQuotaRefusal(r *http.Request, dbpool *pgxpool.Pool) (string, int) {
	user, ok := auth.FromRequest(r)
	if !ok {
		return "", 0
	}

	err := demo.ReserveAICall(r.Context(), dbpool, user.Email)
	switch {
	case err == nil:
		return "", 0
	case errors.Is(err, demo.ErrVisitorAILimit), errors.Is(err, demo.ErrDailyAILimit):
		return err.Error(), http.StatusTooManyRequests
	default:
		fmt.Fprintf(os.Stderr, "failed to check the demo ai limit: %v\n", err)
		return "failed to check the demo AI limit", http.StatusInternalServerError
	}
}
