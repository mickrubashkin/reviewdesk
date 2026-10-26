package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/boards"
)

func TestCreateBoardCopiesEnglishEmailsAsDrafts(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	user := createTestUserWithRole(t, dbpool, "admin")
	sourceKey := createTestBoard(t, dbpool, "copy-source-board", []string{"registered", "qualified"})

	masterID := insertBoardCopyTestEmail(t, dbpool, sourceKey, "registered", 101, "en", "new", "default")
	insertBoardCopyTestEmail(t, dbpool, sourceKey, "qualified", 201, "EN", "new", "default")
	// Left behind: translations, old variants, branches and archived emails.
	insertBoardCopyTestEmail(t, dbpool, sourceKey, "registered", 101, "de", "new", "default")
	insertBoardCopyTestEmail(t, dbpool, sourceKey, "registered", 101, "en", "old", "default")
	insertBoardCopyTestEmail(t, dbpool, sourceKey, "registered", 101, "en", "new", "india")
	archivedID := insertBoardCopyTestEmail(t, dbpool, sourceKey, "qualified", 202, "en", "new", "default")
	if _, err := dbpool.Exec(ctx, `UPDATE emails SET archived_at = now() WHERE id = $1;`, archivedID); err != nil {
		t.Fatalf("failed to archive test email: %v", err)
	}
	// The master is live, uploaded, owned and discussed; the copy is not.
	if _, err := dbpool.Exec(ctx, `
		UPDATE emails
		SET review_status = 'ready', owner_email = 'owner@example.com', live_marked_at = now(),
			live_marked_by = 'admin@example.com',
			send_timing = '2 days', send_condition = 'no deal'
		WHERE id = $1;
	`, masterID); err != nil {
		t.Fatalf("failed to mark test email: %v", err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO email_robot_uploads (email_id, robot, version, uploaded_by_email) VALUES ($1, '', 1, 'test');`, masterID); err != nil {
		t.Fatalf("failed to mark test email uploaded: %v", err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO comments (email_id, user_id, review_block, selected_text, start_offset, end_offset, body)
		VALUES ($1, $2, 'body-001', 'copy', 0, 4, 'a comment');
	`, masterID, user.ID); err != nil {
		t.Fatalf("failed to create test comment: %v", err)
	}

	boards.CopyEmails = CopyBoardEmailsTx
	t.Cleanup(func() { boards.CopyEmails = nil })
	router := chi.NewRouter()
	boards.RegisterBoardRoutes(router, dbpool)
	request := httptest.NewRequest(http.MethodPost, "/api/boards", bytes.NewReader([]byte(
		`{ "name": "Copy Target Board", "source_board_key": "`+sourceKey+`", "copy_emails": true }`,
	)))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, withAuthUser(request, user))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	var created boards.BoardItem
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created board: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(ctx, `DELETE FROM email_events WHERE metadata->>'board_key' = $1;`, created.Key)
		_, _ = dbpool.Exec(ctx, `DELETE FROM email_events WHERE email_id IN (SELECT id FROM emails WHERE sequence = $1);`, created.Key)
		_, _ = dbpool.Exec(ctx, `DELETE FROM emails WHERE sequence = $1;`, created.Key)
		_, _ = dbpool.Exec(ctx, `DELETE FROM boards WHERE id = $1;`, created.ID)
	})
	if created.CopiedEmailCount == nil || *created.CopiedEmailCount != 2 {
		t.Fatalf("expected 2 copied emails, got %v", created.CopiedEmailCount)
	}

	rows, err := dbpool.Query(ctx, `
		SELECT id, slug, stage, sort_order, language, variant, review_status, owner_email,
			live_marked_at IS NULL, (SELECT uploaded_version FROM email_upload_summary u WHERE u.email_id = emails.id), send_timing, send_condition,
			(SELECT count(*) FROM comments c WHERE c.email_id = emails.id),
			(SELECT count(*) FROM email_versions v WHERE v.email_id = emails.id)
		FROM emails
		WHERE sequence = $1
		ORDER BY sort_order;
	`, created.Key)
	if err != nil {
		t.Fatalf("failed to load copied emails: %v", err)
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var id, slug, stage, language, variant, status string
		var sortOrder int
		var owner, timing, condition *string
		var notLive bool
		var uploaded *int
		var comments, versions int
		if err := rows.Scan(&id, &slug, &stage, &sortOrder, &language, &variant, &status, &owner,
			&notLive, &uploaded, &timing, &condition, &comments, &versions); err != nil {
			t.Fatalf("failed to scan copied email: %v", err)
		}
		slugs = append(slugs, slug)
		if language != "en" || variant != "new" || status != "draft" || owner != nil || !notLive || uploaded != nil {
			t.Fatalf("expected a clean EN draft, got %s: %s %s %s owner=%v notLive=%v uploaded=%v", slug, language, variant, status, owner, notLive, uploaded)
		}
		if comments != 0 || versions != 1 {
			t.Fatalf("expected no comments and one version for %s, got %d and %d", slug, comments, versions)
		}
		if sortOrder == 101 && (stage != "registered" || stringFromPointer(timing) != "2 days" || stringFromPointer(condition) != "no deal") {
			t.Fatalf("expected the slot to be copied, got %s %d %v %v", stage, sortOrder, timing, condition)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed to read copied emails: %v", err)
	}
	expected := []string{created.Key + "-registered-101-en", created.Key + "-qualified-201-en"}
	if len(slugs) != 2 || slugs[0] != expected[0] || slugs[1] != expected[1] {
		t.Fatalf("expected slugs %v, got %v", expected, slugs)
	}
}

func TestCreateBoardWithoutCopyKeepsBoardEmpty(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	user := createTestUserWithRole(t, dbpool, "admin")
	sourceKey := createTestBoard(t, dbpool, "copy-source-board-2", []string{"registered"})
	insertBoardCopyTestEmail(t, dbpool, sourceKey, "registered", 101, "en", "new", "default")

	boards.CopyEmails = CopyBoardEmailsTx
	t.Cleanup(func() { boards.CopyEmails = nil })
	router := chi.NewRouter()
	boards.RegisterBoardRoutes(router, dbpool)
	request := httptest.NewRequest(http.MethodPost, "/api/boards", bytes.NewReader([]byte(
		`{ "name": "Empty Target Board", "source_board_key": "`+sourceKey+`" }`,
	)))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, withAuthUser(request, user))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	var created boards.BoardItem
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created board: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(ctx, `DELETE FROM email_events WHERE metadata->>'board_key' = $1;`, created.Key)
		_, _ = dbpool.Exec(ctx, `DELETE FROM boards WHERE id = $1;`, created.ID)
	})
	if created.CopiedEmailCount != nil {
		t.Fatalf("expected no copy count, got %d", *created.CopiedEmailCount)
	}
	var count int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM emails WHERE sequence = $1;`, created.Key).Scan(&count); err != nil {
		t.Fatalf("failed to count emails: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected an empty board, got %d emails", count)
	}
}

func TestBoardCopySlug(t *testing.T) {
	for _, test := range []struct{ slug, want string }{
		{"onboarding-approved-next-step-en", "india-approved-next-step-en"},
		{"approved-next-step-en", "india-approved-next-step-en"},
	} {
		if got := boardCopySlug(test.slug, "onboarding", "india"); got != test.want {
			t.Fatalf("boardCopySlug(%q) = %q, want %q", test.slug, got, test.want)
		}
	}
}

func insertBoardCopyTestEmail(t *testing.T, dbpool *pgxpool.Pool, sequence, stage string, sortOrder int, language, variant, adaptationKey string) string {
	t.Helper()
	slug := fmt.Sprintf("%s-%s-%d-%s", sequence, stage, sortOrder, strings.ToLower(language))
	if variant != "new" {
		slug += "-" + variant
	}
	slug = emailSlugWithAdaptation(slug, adaptationKey)
	var emailID string
	err := dbpool.QueryRow(context.Background(), `
		INSERT INTO emails (
			slug, sequence, title, stage, sort_order, language, variant, adaptation_key,
			original_html, review_html, template_html, editable_fields
		)
		VALUES (
			$1, $2, 'Copy Test Email', $3, $4, $5, $6, $7,
			'<html><body>copy test</body></html>',
			'<html><body><p data-review-block="body-001">copy test</p></body></html>',
			'<html><body><p data-review-block="body-001">copy test</p></body></html>',
			'{}'::jsonb
		)
		RETURNING id;
	`, slug, sequence, stage, sortOrder, language, variant, adaptationKey).Scan(&emailID)
	if err != nil {
		t.Fatalf("failed to create copy test email: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM emails WHERE id = $1;`, emailID)
	})
	return emailID
}
