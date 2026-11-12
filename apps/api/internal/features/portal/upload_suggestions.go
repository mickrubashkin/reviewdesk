package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// UploadSuggestion: the portal already sends the current version of a ready
// email that has no upload mark for it. A super admin confirms the mark; it
// is never set automatically.
type UploadSuggestion struct {
	EmailID    string    `json:"email_id"`
	Version    int       `json:"version"`
	Sends      int       `json:"sends"`
	LastSentAt time.Time `json:"last_sent_at"`
	// Robots are the email's robots whose paired portal user sent the
	// version; UnknownSends the sends no robot of the email is paired with.
	Robots       []RobotSends `json:"robots"`
	UnknownSends int          `json:"unknown_sends"`
}

type RobotSends struct {
	Robot string `json:"robot"`
	Sends int    `json:"sends"`
}

// sentCopyIsCurrent decides from text alone whether a sent copy is the
// current version rather than the one before it (the uploaded version, or
// the previous one when nothing was marked). Sent copies keep only visible
// text, so versions that differ in links or markup cannot be told apart:
// those get no suggestion.
func sentCopyIsCurrent(sent, current, previous string, hasPrevious bool) bool {
	currentScore := Similarity(sent, current)
	if currentScore < matchedThreshold {
		return false
	}
	if !hasPrevious {
		return true
	}
	if current == previous {
		return false
	}
	return currentScore > Similarity(sent, previous)
}

func renderVersionText(template string, fieldsJSON []byte, preheader string) (string, error) {
	var fields emailedit.EditableFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return "", err
	}
	rendered, err := emailedit.RenderEditableHTMLWithMetadata(template, fields, emailedit.RenderMetadata{Preheader: preheader})
	if err != nil {
		return "", err
	}
	return NormalizedText(rendered), nil
}

type suggestionCandidate struct {
	emailID          string
	version          int
	versionCreatedAt time.Time
	current          string
	previous         string
	hasPrevious      bool
}

func LoadUploadSuggestions(ctx context.Context, db *pgxpool.Pool) ([]UploadSuggestion, error) {
	// Ready emails with a robot that does not have the latest version, with
	// the version to compare against: the oldest one those robots have, else
	// the one before. Robots are looked at one by one: a robot with no mark
	// lags behind even when the others have the latest version (the
	// summary's min skips it).
	rows, err := db.Query(ctx, `
		SELECT e.id, v.version_number, v.created_at, v.template_html, v.editable_fields, COALESCE(v.preheader, ''),
			p.template_html, p.editable_fields, COALESCE(p.preheader, '')
		FROM emails e
		JOIN LATERAL (
			SELECT * FROM email_versions WHERE email_id = e.id ORDER BY version_number DESC LIMIT 1
		) v ON true
		JOIN LATERAL (
			SELECT count(*) AS robots, min(ru.version) AS oldest
			FROM email_robots r
			LEFT JOIN email_robot_uploads ru ON ru.email_id = r.email_id AND ru.robot = r.robot
			WHERE r.email_id = e.id AND (ru.version IS NULL OR ru.version < v.version_number)
		) behind ON behind.robots > 0
		LEFT JOIN email_versions p
			ON p.email_id = e.id AND p.version_number = COALESCE(behind.oldest, v.version_number - 1)
		WHERE e.archived_at IS NULL AND e.review_status = 'ready';
	`)
	if err != nil {
		return nil, err
	}
	candidates := map[string]suggestionCandidate{}
	var ids []string
	for rows.Next() {
		var c suggestionCandidate
		var template, preheader string
		var fields []byte
		var previousTemplate, previousPreheader *string
		var previousFields []byte
		if err := rows.Scan(&c.emailID, &c.version, &c.versionCreatedAt, &template, &fields, &preheader,
			&previousTemplate, &previousFields, &previousPreheader); err != nil {
			rows.Close()
			return nil, err
		}
		if c.current, err = renderVersionText(template, fields, preheader); err != nil {
			continue
		}
		if previousTemplate != nil {
			if c.previous, err = renderVersionText(*previousTemplate, previousFields, *previousPreheader); err != nil {
				continue
			}
			c.hasPrevious = true
		}
		candidates[c.emailID] = c
		ids = append(ids, c.emailID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []UploadSuggestion{}, nil
	}

	// Sent texts tied to those emails, by a person's decision or an
	// undisputed match, with the sends made after the version was saved, by
	// the email's robot their sender is paired with ('' when none is).
	rows, err = db.Query(ctx, `
		SELECT COALESCE(g.decision_email_id, g.match_email_id)::text, g.normalized_text,
			COALESCE(er.robot, ''), count(s.activity_id), max(s.sent_at)
		FROM portal_email_groups g
		JOIN emails e ON e.id = COALESCE(g.decision_email_id, g.match_email_id)
		JOIN LATERAL (
			SELECT max(version_number) AS version FROM email_versions WHERE email_id = e.id
		) latest ON true
		JOIN email_versions v ON v.email_id = e.id AND v.version_number = latest.version
		JOIN portal_sent_emails s
			ON s.board_key = g.board_key AND s.fingerprint = g.fingerprint AND s.sent_at > v.created_at
		LEFT JOIN portal_robot_users ru ON ru.board_key = s.board_key AND ru.portal_user_id = s.responsible_id
		LEFT JOIN email_robots er ON er.email_id = e.id AND er.robot = ru.robot
		WHERE COALESCE(g.decision_email_id, g.match_email_id)::text = ANY($1)
			AND (
				(g.decision IN ('confirmed', 'created') AND g.decision_email_id IS NOT NULL)
				OR (g.decision IS NULL AND g.match_status = 'matched')
			)
		GROUP BY 1, 2, 3
		ORDER BY 1, 3;
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byEmail := map[string]*UploadSuggestion{}
	for rows.Next() {
		var emailID, text, robot string
		var sends int
		var lastSentAt time.Time
		if err := rows.Scan(&emailID, &text, &robot, &sends, &lastSentAt); err != nil {
			return nil, err
		}
		c := candidates[emailID]
		if !sentCopyIsCurrent(text, c.current, c.previous, c.hasPrevious) {
			continue
		}
		s := byEmail[emailID]
		if s == nil {
			s = &UploadSuggestion{EmailID: emailID, Version: c.version, Robots: []RobotSends{}}
			byEmail[emailID] = s
		}
		s.Sends += sends
		if robot == "" {
			s.UnknownSends += sends
		} else if n := len(s.Robots); n > 0 && s.Robots[n-1].Robot == robot {
			s.Robots[n-1].Sends += sends
		} else {
			s.Robots = append(s.Robots, RobotSends{Robot: robot, Sends: sends})
		}
		if lastSentAt.After(s.LastSentAt) {
			s.LastSentAt = lastSentAt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	suggestions := make([]UploadSuggestion, 0, len(byEmail))
	for _, s := range byEmail {
		suggestions = append(suggestions, *s)
	}
	return suggestions, nil
}

func uploadSuggestionsHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSuperAdmin(w, r); !ok {
			return
		}
		suggestions, err := LoadUploadSuggestions(r.Context(), dbpool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "upload suggestions: %v\n", err)
			http.Error(w, "failed to load upload suggestions", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, suggestions)
	}
}
