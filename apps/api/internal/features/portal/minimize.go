package portal

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sent copies carry partner personal data: the name in the greeting and,
// sometimes, partner email addresses. Only one copy per group is needed (to
// preview, diff and import it), so after a sync every other body is dropped
// and the kept sample is redacted.

const (
	redactedName  = "[name]"
	redactedEmail = "[email]"
)

var emailAddressPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@([A-Za-z0-9-]+\.)+[A-Za-z]{2,}`)

// Addresses on these domains belong to the company, not to partners.
var companyEmailDomain = regexp.MustCompile(`(?i)@(bitrix24|bitrx24|bitrixsoft)\.`)

// A greeting word followed by up to four words and a comma or "!". The words
// are replaced unless they are a generic salutation such as "Hi there,".
var greetingPattern = regexp.MustCompile(`(?i)((?:^|[\s>])(?:dear|hello|hi|hey|hola|estimad[oa]|olá|ola|prezad[oa]|hallo|liebe[rs]?|sehr geehrte[rs]?|witaj|cześć|dzień dobry|szanown[aiy]))(\s+)([\p{L}][\p{L}.'-]*(?:\s+[\p{L}][\p{L}.'-]*){0,3})(\s*[,!])`)

var genericSalutations = map[string]bool{
	"there": true, "partner": true, "partners": true, "team": true, "colleague": true, "colleagues": true,
	"socio": true, "socia": true, "parceiro": true, "parceira": true, "partnerin": true,
	"partnerze": true, "partnerko": true, "panie": true, "pani": true, "państwo": true,
	"todos": true, "equipo": true, "equipe": true, "zusammen": true,
	"[name]": true,
}

// RedactPartnerData replaces partner names in greetings and non-company email
// addresses with placeholders. Template markup and company addresses stay.
func RedactPartnerData(body string) string {
	body = emailAddressPattern.ReplaceAllStringFunc(body, func(address string) string {
		if companyEmailDomain.MatchString(address) {
			return address
		}
		return redactedEmail
	})

	return greetingPattern.ReplaceAllStringFunc(body, func(match string) string {
		parts := greetingPattern.FindStringSubmatch(match)
		name := strings.ToLower(strings.TrimSpace(parts[3]))
		if genericSalutations[name] || strings.Contains(name, "{") {
			return match
		}
		return parts[1] + parts[2] + redactedName + parts[4]
	})
}

// minimizeSentEmails runs at the end of a sync. It deletes copies sent before
// the sync window, clears the bodies of all copies but each group's sample, and
// redacts the samples. Samples are kept even when old: groups and their
// decisions point at them.
func minimizeSentEmails(ctx context.Context, db *pgxpool.Pool, boardKey string, since time.Time) error {
	if _, err := db.Exec(ctx, `
		DELETE FROM portal_sent_emails s
		WHERE s.board_key = $1
			AND s.sent_at < $2
			AND NOT EXISTS (SELECT 1 FROM portal_email_groups g WHERE g.sample_activity_id = s.activity_id);
	`, boardKey, since); err != nil {
		return err
	}

	if _, err := db.Exec(ctx, `
		UPDATE portal_sent_emails s
		SET body_html = ''
		WHERE s.board_key = $1
			AND s.body_html <> ''
			AND NOT EXISTS (SELECT 1 FROM portal_email_groups g WHERE g.sample_activity_id = s.activity_id);
	`, boardKey); err != nil {
		return err
	}

	// Subjects can greet the partner by name too ("Hi Samir, …"), so they are
	// redacted on every copy and group, not only on samples.
	for _, step := range []struct{ query, update string }{
		{
			`SELECT s.activity_id, s.body_html FROM portal_sent_emails s
				JOIN portal_email_groups g ON g.sample_activity_id = s.activity_id
				WHERE s.board_key = $1;`,
			`UPDATE portal_sent_emails SET body_html = $2 WHERE activity_id = $1;`,
		},
		{
			`SELECT activity_id, subject FROM portal_sent_emails WHERE board_key = $1;`,
			`UPDATE portal_sent_emails SET subject = $2 WHERE activity_id = $1;`,
		},
		{
			`SELECT id, subject FROM portal_email_groups WHERE board_key = $1;`,
			`UPDATE portal_email_groups SET subject = $2 WHERE id = $1;`,
		},
	} {
		if err := redactColumn(ctx, db, boardKey, step.query, step.update); err != nil {
			return err
		}
	}
	return nil
}

// redactColumn runs RedactPartnerData over the (id, text) rows of query and
// writes back the ones that changed with update.
func redactColumn(ctx context.Context, db *pgxpool.Pool, boardKey, query, update string) error {
	rows, err := db.Query(ctx, query, boardKey)
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for rows.Next() {
		var id any
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			rows.Close()
			return err
		}
		if redacted := RedactPartnerData(text); redacted != text {
			batch.Queue(update, id, redacted)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	return sendBatch(ctx, db, batch)
}
