package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

type markEmailUploadedRequest struct {
	Uploaded bool `json:"uploaded"`
	// Version is the version the super admin copied. When it is set and the
	// email was saved again after the copy, the mark is refused.
	Version *int `json:"version"`
	// Robots are the robots the email was pasted into or taken out of; none
	// means every robot of the email's region.
	Robots []string `json:"robots"`
}

// RobotUpload is one robot of an email and the version pasted into it, if
// any. Robot is a manager of the email's region, or empty for the one robot
// of an email whose region lists no managers.
type RobotUpload struct {
	Robot           string     `json:"robot"`
	Version         *int       `json:"version"`
	UploadedAt      *time.Time `json:"uploaded_at"`
	UploadedByEmail *string    `json:"uploaded_by_email"`
}

type markEmailUploadedResponse struct {
	UploadedVersion *int          `json:"uploaded_version"`
	UploadedAt      *time.Time    `json:"uploaded_at"`
	UploadRobots    []RobotUpload `json:"upload_robots"`
}

// markEmailUploadedHandler records that a super admin pasted the current
// version of a ready email into its Bitrix24 robots, or takes the mark off.
// A robot whose version is older than the email's shows as needing a
// re-upload.
func markEmailUploadedHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok || !auth.IsSuperAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var request markEmailUploadedRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		id := chi.URLParam(r, "id")
		var slug, title, reviewStatus string
		var latestVersion *int
		err = tx.QueryRow(r.Context(), `
			SELECT slug, title, review_status,
				(SELECT max(version_number) FROM email_versions WHERE email_id = emails.id)
			FROM emails
			WHERE id = $1 AND archived_at IS NULL
			FOR UPDATE;
		`, id).Scan(&slug, &title, &reviewStatus, &latestVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "email not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}
		if request.Uploaded && !isApprovedEmailReviewStatus(reviewStatus) {
			http.Error(w, "mark the email ready before uploading it to Bitrix24", http.StatusConflict)
			return
		}
		if request.Uploaded && latestVersion == nil {
			http.Error(w, "the email has no saved version yet", http.StatusConflict)
			return
		}
		if request.Uploaded && request.Version != nil && *request.Version != *latestVersion {
			http.Error(w, "the email changed after it was copied, copy it again", http.StatusConflict)
			return
		}

		robots, err := emailRobots(r.Context(), tx, id)
		if err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}
		chosen := robots
		if len(request.Robots) > 0 {
			chosen = request.Robots
			for _, robot := range chosen {
				if !slices.Contains(robots, robot) {
					http.Error(w, fmt.Sprintf("%q is not a robot of the email's region", robot), http.StatusBadRequest)
					return
				}
			}
		}

		if request.Uploaded {
			_, err = tx.Exec(r.Context(), `
				INSERT INTO email_robot_uploads (email_id, robot, version, uploaded_by_email)
				SELECT $1, robot, $3, $4 FROM unnest($2::text[]) AS robot
				ON CONFLICT (email_id, robot) DO UPDATE
				SET version = EXCLUDED.version, uploaded_at = now(), uploaded_by_email = EXCLUDED.uploaded_by_email;
			`, id, chosen, *latestVersion, user.Email)
		} else if len(request.Robots) > 0 {
			_, err = tx.Exec(r.Context(), `DELETE FROM email_robot_uploads WHERE email_id = $1 AND robot = ANY($2);`, id, chosen)
		} else {
			// Also the marks of robots no longer in the region.
			_, err = tx.Exec(r.Context(), `DELETE FROM email_robot_uploads WHERE email_id = $1;`, id)
		}
		if err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}

		action := emailEventUploadUnmarked
		metadata := map[string]any{}
		if request.Uploaded {
			action = emailEventUploaded
			metadata["version"] = strconv.Itoa(*latestVersion)
		}
		if names := robotNames(chosen); names != "" {
			metadata["robots"] = names
		}
		if err := InsertEmailEvent(r.Context(), tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      action,
			EmailID:     &id,
			EmailSlug:   &slug,
			EmailTitle:  &title,
			Metadata:    metadata,
		}); err != nil {
			http.Error(w, "failed to record email event", http.StatusInternalServerError)
			return
		}

		var response markEmailUploadedResponse
		var robotsJSON []byte
		if err := tx.QueryRow(r.Context(), `
			SELECT uploaded_version, uploaded_at, robots FROM email_upload_summary WHERE email_id = $1;
		`, id).Scan(&response.UploadedVersion, &response.UploadedAt, &robotsJSON); err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(robotsJSON, &response.UploadRobots); err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "failed to update email", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

// emailRobots lists the robots an email goes into, in the region's order.
func emailRobots(ctx context.Context, q pgx.Tx, emailID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT robot FROM email_robots WHERE email_id = $1 ORDER BY sort;`, emailID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// robotNames names the robots for the activity log; the one unnamed robot
// of a region without managers needs no name.
func robotNames(robots []string) string {
	named := slices.DeleteFunc(slices.Clone(robots), func(robot string) bool { return robot == "" })
	return strings.Join(named, ", ")
}
