package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

const (
	bulkActionOwner   = "owner"
	bulkActionStatus  = "status"
	bulkActionSlot    = "slot"
	bulkActionTiming  = "timing"
	bulkActionArchive = "archive"

	maxBulkEmails = 500
)

// bulkEmailRequest applies one action to emails selected on a board. Each
// action reads only its own fields.
type bulkEmailRequest struct {
	Sequence string   `json:"sequence"`
	EmailIDs []string `json:"email_ids"`
	Action   string   `json:"action"`

	// owner: null or "" clears the owner.
	OwnerEmail *string `json:"owner_email"`
	// status: draft or ready.
	ReviewStatus string `json:"review_status"`
	// slot: the slot to move the emails to; no sort_order makes a new slot
	// at the end of the stage.
	Stage     string `json:"stage"`
	SortOrder *int   `json:"sort_order"`
	// timing: applies to the whole slots of the emails, every language.
	// Nil stays as it is, "" clears it.
	SendTiming    *string `json:"send_timing"`
	SendCondition *string `json:"send_condition"`
}

type bulkEmailResponse struct {
	Updated int                `json:"updated"`
	Skipped []bulkSkippedEmail `json:"skipped"`
	// Slots is how many slots a timing change touched.
	Slots int `json:"slots,omitempty"`
	// Stage and SortOrder are where a slot move put the emails.
	Stage     string `json:"stage,omitempty"`
	SortOrder *int   `json:"sort_order,omitempty"`
}

type bulkSkippedEmail struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Language string `json:"language"`
	Reason   string `json:"reason"`
}

type bulkEmail struct {
	ID        string
	Slug      string
	Title     string
	Language  string
	Stage     string
	SortOrder int
	Owner     *string
}

func bulkEmailHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var request bulkEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !requireManageBoard(w, r, dbpool, user, strings.TrimSpace(request.Sequence)) {
			return
		}

		response, err := applyBulkEmailAction(r.Context(), dbpool, user, request)
		var inputErr createEmailInputError
		switch {
		case err == nil:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		case errors.As(err, &inputErr):
			http.Error(w, inputErr.Error(), http.StatusBadRequest)
		case errors.Is(err, errSlotNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, errSlotStale), errors.Is(err, errSlotPositionTaken), errors.Is(err, errSlotLanguageTaken):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			fmt.Fprintf(os.Stderr, "bulk %s: %v\n", request.Action, err)
			http.Error(w, "failed to update the emails", http.StatusInternalServerError)
		}
	}
}

// applyBulkEmailAction runs in one transaction: the selection changes as a
// whole or not at all. The one exception is marking emails ready, where
// emails held back by the approval gates are skipped and listed.
func applyBulkEmailAction(ctx context.Context, db *pgxpool.Pool, user AuthUser, request bulkEmailRequest) (bulkEmailResponse, error) {
	request.Sequence = strings.TrimSpace(request.Sequence)
	if request.Sequence == "" {
		return bulkEmailResponse{}, createEmailInputError("sequence is required")
	}
	ids := uniqueStrings(request.EmailIDs)
	if len(ids) == 0 {
		return bulkEmailResponse{}, createEmailInputError("select at least one email")
	}
	if len(ids) > maxBulkEmails {
		return bulkEmailResponse{}, createEmailInputError(fmt.Sprintf("select at most %d emails at once", maxBulkEmails))
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return bulkEmailResponse{}, err
	}
	defer tx.Rollback(ctx)

	emails, err := lockBulkEmails(ctx, tx, request.Sequence, ids)
	if err != nil {
		return bulkEmailResponse{}, err
	}

	response := bulkEmailResponse{Skipped: []bulkSkippedEmail{}}
	switch request.Action {
	case bulkActionOwner:
		err = bulkSetOwner(ctx, tx, user, emails, trimmedOptionalString(request.OwnerEmail))
	case bulkActionStatus:
		response.Skipped, err = bulkSetStatus(ctx, tx, user, emails, strings.TrimSpace(request.ReviewStatus))
	case bulkActionSlot:
		var place slotPlaceResponse
		place, err = bulkMoveToSlot(ctx, tx, user, emails, request)
		response.Stage, response.SortOrder = place.Stage, &place.SortOrder
	case bulkActionTiming:
		response.Slots, err = bulkSetTiming(ctx, tx, user, emails, request)
	case bulkActionArchive:
		for _, email := range emails {
			if err = archiveEmail(ctx, tx, user, email.ID); err != nil {
				break
			}
		}
	default:
		err = createEmailInputError("unknown action")
	}
	if err != nil {
		return bulkEmailResponse{}, err
	}
	response.Updated = len(emails) - len(response.Skipped)
	return response, tx.Commit(ctx)
}

// lockBulkEmails loads the selection, EN first so that translations moved
// along with their master link to it. Any email missing from the board means
// the board changed since it was loaded.
func lockBulkEmails(ctx context.Context, tx pgx.Tx, sequence string, ids []string) ([]bulkEmail, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, slug, title, language, stage, sort_order, owner_email FROM emails
		WHERE id = ANY($1::uuid[]) AND sequence = $2 AND archived_at IS NULL
		ORDER BY (language = 'en') DESC, language, created_at
		FOR UPDATE;
	`, ids, sequence)
	if err != nil {
		return nil, err
	}
	emails, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (bulkEmail, error) {
		var email bulkEmail
		err := row.Scan(&email.ID, &email.Slug, &email.Title, &email.Language, &email.Stage, &email.SortOrder, &email.Owner)
		return email, err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "22P02" {
		return nil, createEmailInputError("invalid email id")
	}
	if err != nil {
		return nil, err
	}
	if len(emails) != len(ids) {
		return nil, errSlotStale
	}
	return emails, nil
}

func bulkSetOwner(ctx context.Context, tx pgx.Tx, user AuthUser, emails []bulkEmail, owner *string) error {
	if owner != nil {
		var known bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1));`, *owner).Scan(&known); err != nil {
			return err
		}
		if !known {
			return createEmailInputError("the owner must be a ReviewDesk user")
		}
	}
	for _, email := range emails {
		changes := map[string]any{}
		addStringChange(changes, "owner_email", email.Owner, owner)
		if len(changes) == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE emails SET owner_email = $2, updated_at = now() WHERE id = $1;`, email.ID, owner); err != nil {
			return err
		}
		if err := InsertEmailEvent(ctx, tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventPlanningUpdated,
			EmailID:     &email.ID,
			EmailSlug:   &email.Slug,
			EmailTitle:  &email.Title,
			Changes:     changes,
		}); err != nil {
			return err
		}
	}
	return nil
}

func bulkSetStatus(ctx context.Context, tx pgx.Tx, user AuthUser, emails []bulkEmail, status string) ([]bulkSkippedEmail, error) {
	if !isValidEmailReviewStatus(status) {
		return nil, createEmailInputError("review_status must be draft or ready")
	}
	skipped := []bulkSkippedEmail{}
	for _, email := range emails {
		err := setEmailReviewStatus(ctx, tx, user, email.ID, status)
		var blocked emailStatusBlockedError
		if errors.As(err, &blocked) {
			skipped = append(skipped, bulkSkippedEmail{ID: email.ID, Title: email.Title, Language: email.Language, Reason: blocked.Error()})
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	return skipped, nil
}

func bulkMoveToSlot(ctx context.Context, tx pgx.Tx, user AuthUser, emails []bulkEmail, request bulkEmailRequest) (slotPlaceResponse, error) {
	stage := strings.TrimSpace(request.Stage)
	if stage == "" {
		return slotPlaceResponse{}, createEmailInputError("stage is required")
	}
	if err := requireStageOnBoard(ctx, tx, request.Sequence, stage); err != nil {
		return slotPlaceResponse{}, err
	}
	sortOrder, err := resolveSlotSortOrder(ctx, tx, createSlotRequest{Sequence: request.Sequence, Stage: stage, SortOrder: request.SortOrder})
	// An existing slot is the usual target: its position is taken on purpose.
	if errors.Is(err, errSlotPositionTaken) {
		sortOrder, err = *request.SortOrder, nil
	}
	if err != nil {
		return slotPlaceResponse{}, err
	}

	for _, email := range emails {
		if email.Stage == stage && email.SortOrder == sortOrder {
			continue
		}
		_, err := placeEmailTx(ctx, tx, user, email.ID, placeEmailRequest{Sequence: request.Sequence, Stage: stage, SortOrder: &sortOrder})
		if errors.Is(err, errSlotLanguageTaken) {
			return slotPlaceResponse{}, fmt.Errorf("%s · %s: %w", email.Title, strings.ToUpper(email.Language), err)
		}
		if err != nil {
			return slotPlaceResponse{}, err
		}
	}
	return slotPlaceResponse{Stage: stage, SortOrder: sortOrder}, nil
}

// bulkSetTiming sets timing and condition on every slot the selection
// touches: they are shared by all languages of a slot.
func bulkSetTiming(ctx context.Context, tx pgx.Tx, user AuthUser, emails []bulkEmail, request bulkEmailRequest) (int, error) {
	if request.SendTiming == nil && request.SendCondition == nil {
		return 0, createEmailInputError("set the timing, the condition or both")
	}
	type slotKey struct {
		stage     string
		sortOrder int
	}
	seen := map[slotKey]bool{}
	for _, email := range emails {
		key := slotKey{email.Stage, email.SortOrder}
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err := updateSlotTx(ctx, tx, user, updateSlotRequest{
			Sequence:      request.Sequence,
			Stage:         email.Stage,
			SortOrder:     email.SortOrder,
			SendTiming:    request.SendTiming,
			SendCondition: request.SendCondition,
		}, nil); err != nil {
			return 0, err
		}
	}
	return len(seen), nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
