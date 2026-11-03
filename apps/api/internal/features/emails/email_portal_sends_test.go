package emails

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestListEmailsCountsPortalSendsOfLast30Days(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	emailID := createTestEmail(t, dbpool)

	const board = "sends-30d-test"
	cleanup := func() {
		_, _ = dbpool.Exec(ctx, `DELETE FROM portal_email_groups WHERE board_key = $1;`, board)
		_, _ = dbpool.Exec(ctx, `DELETE FROM portal_sent_emails WHERE board_key = $1;`, board)
	}
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now().UTC()
	for i, daysAgo := range []int{1, 5, 29, 31, 120} {
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO portal_sent_emails (activity_id, board_key, deal_id, sent_at, body_html, fingerprint)
			VALUES ($1, $2, $3, $4, '', 'fp');
		`, 9100000+i, board, i, now.AddDate(0, 0, -daysAgo)); err != nil {
			t.Fatalf("insert send: %v", err)
		}
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO portal_email_groups (board_key, fingerprint, sample_activity_id, send_count, first_sent_at, last_sent_at, match_email_id, match_status)
		VALUES ($1, 'fp', 9100000, 5, $2, $3, $4, 'matched');
	`, board, now.AddDate(0, 0, -120), now.AddDate(0, 0, -1), emailID); err != nil {
		t.Fatalf("insert group: %v", err)
	}

	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/emails", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var emails []EmailListItem
	if err := json.NewDecoder(response.Body).Decode(&emails); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, email := range emails {
		if email.ID != emailID {
			continue
		}
		if email.PortalSendCount != 5 || email.PortalSends30d != 3 {
			t.Fatalf("expected 5 kept sends and 3 in 30 days, got %d and %d", email.PortalSendCount, email.PortalSends30d)
		}
		if email.PortalLastSentAt == nil || now.Sub(*email.PortalLastSentAt) > 48*time.Hour {
			t.Fatalf("expected the last send a day ago, got %v", email.PortalLastSentAt)
		}
		return
	}
	t.Fatalf("seed email %s not returned", emailID)
}
