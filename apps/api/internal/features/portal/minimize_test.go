package portal

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRedactPartnerData(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"greeting name", `<p>Hi Maria,</p>`, `<p>Hi [name],</p>`},
		{"two-word name", `<p>Dear John Smith,</p>`, `<p>Dear [name],</p>`},
		{"portuguese", `<td>Olá João!</td>`, `<td>Olá [name]!</td>`},
		{"polish", `<p>Dzień dobry Katarzyna,</p>`, `<p>Dzień dobry [name],</p>`},
		{"generic there", `<p>Hi there,</p>`, `<p>Hi there,</p>`},
		{"generic partner", `<p>Dear Partner,</p>`, `<p>Dear Partner,</p>`},
		{"bitrix expression kept", `<p>Hi {=Document:CONTACT_NAME},</p>`, `<p>Hi {=Document:CONTACT_NAME},</p>`},
		{"partner email", `<a href="mailto:alice@gmail.com">alice@gmail.com</a>`, `<a href="mailto:[email]">[email]</a>`},
		{"company email kept", `<a href="mailto:partners@bitrix24.com">partners@bitrix24.com</a>`, `<a href="mailto:partners@bitrix24.com">partners@bitrix24.com</a>`},
		{"no greeting", `<p>Book a call, please.</p>`, `<p>Book a call, please.</p>`},
		{"subject", `Hi Samir, I'm Alice from the Partner Network`, `Hi [name], I'm Alice from the Partner Network`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RedactPartnerData(test.in); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRedactPartnerDataIsIdempotent(t *testing.T) {
	body := `<p>Hi Maria,</p><p>Write to maria@example.org</p>`
	once := RedactPartnerData(body)
	if twice := RedactPartnerData(once); twice != once {
		t.Fatalf("second pass changed the body:\n%q\n%q", once, twice)
	}
}

func TestMinimizeSentEmailsKeepsOnlyRedactedSamples(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for portal integration tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(db.Close)

	const board = "minimize-test"
	cleanup := func() {
		_, _ = db.Exec(ctx, `DELETE FROM portal_email_groups WHERE board_key = $1;`, board)
		_, _ = db.Exec(ctx, `DELETE FROM portal_sent_emails WHERE board_key = $1;`, board)
	}
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now().UTC()
	since := now.AddDate(0, 0, -90)
	body := `<p>Hi Maria,</p><p>Reply to maria@example.org</p>`
	insert := func(id int64, sentAt time.Time) {
		t.Helper()
		if _, err := db.Exec(ctx, `
			INSERT INTO portal_sent_emails (activity_id, board_key, deal_id, sent_at, subject, body_html, fingerprint)
			VALUES ($1, $2, 1, $3, 'Hi Maria, welcome', $4, 'fp');
		`, id, board, sentAt, body); err != nil {
			t.Fatalf("insert %d: %v", id, err)
		}
	}
	insert(9000001, now.AddDate(0, 0, -200)) // old sample: kept, redacted
	insert(9000002, now.AddDate(0, 0, -150)) // old copy: deleted
	insert(9000003, now.AddDate(0, 0, -10))  // recent copy: body cleared

	if _, err := db.Exec(ctx, `
		INSERT INTO portal_email_groups (board_key, fingerprint, subject, sample_activity_id, first_sent_at, last_sent_at)
		VALUES ($1, 'fp', 'Hi Maria, welcome', 9000001, $2, $2);
	`, board, now); err != nil {
		t.Fatalf("insert group: %v", err)
	}

	if err := minimizeSentEmails(ctx, db, board, since); err != nil {
		t.Fatalf("minimize: %v", err)
	}

	bodies := map[int64]string{}
	rows, err := db.Query(ctx, `SELECT activity_id, body_html FROM portal_sent_emails WHERE board_key = $1;`, board)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for rows.Next() {
		var id int64
		var b string
		if err := rows.Scan(&id, &b); err != nil {
			t.Fatalf("scan: %v", err)
		}
		bodies[id] = b
	}
	rows.Close()

	if _, ok := bodies[9000002]; ok {
		t.Fatal("expected the old non-sample copy to be deleted")
	}
	if got := bodies[9000003]; got != "" {
		t.Fatalf("expected the recent non-sample body to be cleared, got %q", got)
	}
	sample, ok := bodies[9000001]
	if !ok {
		t.Fatal("expected the sample to be kept even though it is old")
	}
	if strings.Contains(sample, "Maria") || strings.Contains(sample, "maria@example.org") {
		t.Fatalf("expected the sample to be redacted, got %q", sample)
	}

	var sentSubject, groupSubject string
	if err := db.QueryRow(ctx, `SELECT subject FROM portal_sent_emails WHERE activity_id = 9000003;`).Scan(&sentSubject); err != nil {
		t.Fatalf("read sent subject: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT subject FROM portal_email_groups WHERE board_key = $1;`, board).Scan(&groupSubject); err != nil {
		t.Fatalf("read group subject: %v", err)
	}
	for _, subject := range []string{sentSubject, groupSubject} {
		if subject != "Hi [name], welcome" {
			t.Fatalf("expected redacted subject, got %q", subject)
		}
	}
}
