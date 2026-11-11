package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PortalSender is a CRM portal user who sent the board's emails.
type PortalSender struct {
	UserID     int64     `json:"user_id"`
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	Sends      int       `json:"sends"`
	LastSentAt time.Time `json:"last_sent_at"`
}

// RobotSender is a robot of the board (a region manager) with the portal
// user it sends for. Marks counts the robot's upload marks on the board's
// emails; MarksSeenSent those with a send of the email by its user, at any
// version. A mark with no send is not proof the robot lacks the email: the
// stage may have had no deals of that manager.
type RobotSender struct {
	Robot         string `json:"robot"`
	PortalUserID  *int64 `json:"portal_user_id"`
	SuggestedID   *int64 `json:"suggested_user_id"`
	Sends         int    `json:"sends"`
	Marks         int    `json:"marks"`
	MarksSeenSent int    `json:"marks_seen_sent"`
}

type RobotSenders struct {
	Robots  []RobotSender  `json:"robots"`
	Senders []PortalSender `json:"senders"`
	// UnpairedSends counts sends whose sender is no robot's user, or unknown
	// (sends synced before senders were kept).
	UnpairedSends int `json:"unpaired_sends"`
}

func LoadRobotSenders(ctx context.Context, db *pgxpool.Pool, boardKey string) (RobotSenders, error) {
	result := RobotSenders{Robots: []RobotSender{}, Senders: []PortalSender{}}

	rows, err := db.Query(ctx, `
		SELECT s.responsible_id, COALESCE(u.name, ''), COALESCE(u.email, ''), count(*)::int, max(s.sent_at)
		FROM portal_sent_emails s
		LEFT JOIN portal_users u ON u.user_id = s.responsible_id
		WHERE s.board_key = $1 AND s.responsible_id IS NOT NULL
		GROUP BY 1, 2, 3
		ORDER BY 4 DESC;
	`, boardKey)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var s PortalSender
		if err := rows.Scan(&s.UserID, &s.Name, &s.Email, &s.Sends, &s.LastSentAt); err != nil {
			rows.Close()
			return result, err
		}
		result.Senders = append(result.Senders, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, err
	}

	// The board's robots in the regions' order, each once.
	rows, err = db.Query(ctx, `
		WITH robots AS (
			SELECT m.robot, min(r.sort_order * 1000 + m.ord) AS sort
			FROM board_regions r
			CROSS JOIN LATERAL unnest(r.managers) WITH ORDINALITY AS m(robot, ord)
			WHERE r.board_key = $1 AND m.robot <> ''
			GROUP BY m.robot
		)
		SELECT robots.robot, ru.portal_user_id,
			(SELECT count(*)::int FROM portal_sent_emails s
				WHERE s.board_key = $1 AND s.responsible_id = ru.portal_user_id),
			(SELECT count(*)::int FROM email_robot_uploads up
				JOIN emails e ON e.id = up.email_id
				WHERE e.sequence = $1 AND e.archived_at IS NULL AND up.robot = robots.robot),
			(SELECT count(*)::int FROM email_robot_uploads up
				JOIN emails e ON e.id = up.email_id
				WHERE e.sequence = $1 AND e.archived_at IS NULL AND up.robot = robots.robot
					AND EXISTS (
						SELECT 1 FROM portal_email_groups g
						JOIN portal_sent_emails s ON s.board_key = g.board_key AND s.fingerprint = g.fingerprint
						WHERE g.board_key = $1 AND s.responsible_id = ru.portal_user_id
							AND (`+tiedEmailID+`) = up.email_id
					))
		FROM robots
		LEFT JOIN portal_robot_users ru ON ru.board_key = $1 AND ru.robot = robots.robot
		ORDER BY robots.sort;
	`, boardKey)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var r RobotSender
		if err := rows.Scan(&r.Robot, &r.PortalUserID, &r.Sends, &r.Marks, &r.MarksSeenSent); err != nil {
			rows.Close()
			return result, err
		}
		result.Robots = append(result.Robots, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, err
	}

	// Suggest only users no robot has yet.
	paired := map[int64]bool{}
	for _, r := range result.Robots {
		if r.PortalUserID != nil {
			paired[*r.PortalUserID] = true
		}
	}
	var free []portalUser
	pairedSends := 0
	for _, s := range result.Senders {
		if paired[s.UserID] {
			pairedSends += s.Sends
			continue
		}
		free = append(free, portalUser{ID: s.UserID, Name: s.Name, Email: s.Email})
	}
	for i := range result.Robots {
		if result.Robots[i].PortalUserID == nil {
			result.Robots[i].SuggestedID = suggestRobotUser(result.Robots[i].Robot, free)
		}
	}

	var total int
	if err := db.QueryRow(ctx, `SELECT count(*)::int FROM portal_sent_emails WHERE board_key = $1;`, boardKey).Scan(&total); err != nil {
		return result, err
	}
	result.UnpairedSends = total - pairedSends
	return result, nil
}

var (
	errNotBoardRobot = errors.New("not a robot of this board")
	errUserPaired    = errors.New("this portal user is already paired with another robot")
)

// SetRobotSender pairs a robot with a portal user, or unpairs it when
// userID is nil.
func SetRobotSender(ctx context.Context, db *pgxpool.Pool, boardKey, robot string, userID *int64, byEmail string) error {
	var known bool
	if err := db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM board_regions WHERE board_key = $1 AND $2 = ANY(managers)) AND $2 <> '';
	`, boardKey, robot).Scan(&known); err != nil {
		return err
	}
	if !known {
		return errNotBoardRobot
	}
	if userID == nil {
		_, err := db.Exec(ctx, `DELETE FROM portal_robot_users WHERE board_key = $1 AND robot = $2;`, boardKey, robot)
		return err
	}
	_, err := db.Exec(ctx, `
		INSERT INTO portal_robot_users (board_key, robot, portal_user_id, set_by_email) VALUES ($1, $2, $3, $4)
		ON CONFLICT (board_key, robot) DO UPDATE SET
			portal_user_id = EXCLUDED.portal_user_id, set_by_email = EXCLUDED.set_by_email, set_at = now();
	`, boardKey, robot, *userID, byEmail)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errUserPaired
	}
	return err
}

func robotSendersHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSuperAdmin(w, r); !ok {
			return
		}
		result, err := LoadRobotSenders(r.Context(), dbpool, chi.URLParam(r, "boardKey"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "robot senders: %v\n", err)
			http.Error(w, "failed to load robots", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func putRobotSenderHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireSuperAdmin(w, r)
		if !ok {
			return
		}
		var body struct {
			Robot        string `json:"robot"`
			PortalUserID *int64 `json:"portal_user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		err := SetRobotSender(r.Context(), dbpool, chi.URLParam(r, "boardKey"), body.Robot, body.PortalUserID, user.Email)
		switch {
		case errors.Is(err, errNotBoardRobot):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, errUserPaired):
			http.Error(w, err.Error(), http.StatusConflict)
		case err != nil:
			fmt.Fprintf(os.Stderr, "set robot sender: %v\n", err)
			http.Error(w, "failed to save the robot's sender", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}
