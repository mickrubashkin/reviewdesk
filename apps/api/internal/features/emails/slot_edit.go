package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

const emailEventSlotUpdated = "slot_updated"

var (
	errSlotNotFound = errors.New("slot not found: the board may have changed, reload it")
	errSlotStale    = errors.New("the board changed meanwhile, reload it and try again")
)

// updateSlotRequest edits a slot as a whole: every language of it gets the
// same name, place, timing and condition. Nil fields stay as they are; an
// empty timing or condition clears it.
type updateSlotRequest struct {
	Sequence  string `json:"sequence"`
	Stage     string `json:"stage"`
	SortOrder int    `json:"sort_order"`

	Title *string `json:"title"`
	// ToStage and ToSortOrder move the slot. A new stage without a position
	// puts it at the end of that stage.
	ToStage       *string `json:"to_stage"`
	ToSortOrder   *int    `json:"to_sort_order"`
	SendTiming    *string `json:"send_timing"`
	SendCondition *string `json:"send_condition"`
}

// moveSlotRequest is a drag in the matrix: the slot goes right before
// another one of the target stage, or to its end.
type moveSlotRequest struct {
	Sequence        string `json:"sequence"`
	Stage           string `json:"stage"`
	SortOrder       int    `json:"sort_order"`
	ToStage         string `json:"to_stage"`
	BeforeSortOrder *int   `json:"before_sort_order"`
}

type slotPlaceResponse struct {
	Stage     string `json:"stage"`
	SortOrder int    `json:"sort_order"`
}

type slotMove struct {
	FromStage     string
	FromSortOrder int
	ToStage       string
	ToSortOrder   int
}

type slotEmail struct {
	ID            string
	Slug          string
	Title         string
	SendTiming    *string
	SendCondition *string
}

func updateSlotHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var request updateSlotRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !requireManageBoard(w, r, dbpool, user, strings.TrimSpace(request.Sequence)) {
			return
		}
		response, err := updateSlot(r.Context(), dbpool, user, request)
		writeSlotResult(w, response, err)
	}
}

func moveSlotHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var request moveSlotRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !requireManageBoard(w, r, dbpool, user, strings.TrimSpace(request.Sequence)) {
			return
		}
		response, err := moveSlot(r.Context(), dbpool, user, request)
		writeSlotResult(w, response, err)
	}
}

func writeSlotResult(w http.ResponseWriter, response slotPlaceResponse, err error) {
	var inputErr createEmailInputError
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	case errors.As(err, &inputErr):
		http.Error(w, inputErr.Error(), http.StatusBadRequest)
	case errors.Is(err, errSlotNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, errSlotPositionTaken), errors.Is(err, errSlotStale):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		fmt.Fprintf(os.Stderr, "update slot: %v\n", err)
		http.Error(w, "failed to update slot", http.StatusInternalServerError)
	}
}

func updateSlot(ctx context.Context, db *pgxpool.Pool, user AuthUser, request updateSlotRequest) (slotPlaceResponse, error) {
	request.Sequence = strings.TrimSpace(request.Sequence)
	request.Stage = strings.TrimSpace(request.Stage)
	if request.Sequence == "" || request.Stage == "" {
		return slotPlaceResponse{}, createEmailInputError("sequence and stage are required")
	}
	title := trimmedOptionalString(request.Title)
	if request.Title != nil && title == nil {
		return slotPlaceResponse{}, createEmailInputError("the slot name cannot be empty")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return slotPlaceResponse{}, err
	}
	defer tx.Rollback(ctx)

	response, err := updateSlotTx(ctx, tx, user, request, title)
	if err != nil {
		return slotPlaceResponse{}, err
	}
	return response, tx.Commit(ctx)
}

func updateSlotTx(ctx context.Context, tx pgx.Tx, user AuthUser, request updateSlotRequest, title *string) (slotPlaceResponse, error) {
	emails, err := lockSlotEmails(ctx, tx, request.Sequence, request.Stage, request.SortOrder)
	if err != nil {
		return slotPlaceResponse{}, err
	}

	toStage := request.Stage
	if request.ToStage != nil && strings.TrimSpace(*request.ToStage) != "" {
		toStage = strings.TrimSpace(*request.ToStage)
	}
	toSortOrder := request.SortOrder
	moved := toStage != request.Stage || (request.ToSortOrder != nil && *request.ToSortOrder != request.SortOrder)
	if moved {
		if err := requireStageOnBoard(ctx, tx, request.Sequence, toStage); err != nil {
			return slotPlaceResponse{}, err
		}
		target := createSlotRequest{Sequence: request.Sequence, Stage: toStage, SortOrder: request.ToSortOrder}
		if toSortOrder, err = resolveSlotSortOrder(ctx, tx, target); err != nil {
			return slotPlaceResponse{}, err
		}
		if err := applySlotMoves(ctx, tx, request.Sequence, []slotMove{{request.Stage, request.SortOrder, toStage, toSortOrder}}); err != nil {
			return slotPlaceResponse{}, err
		}
	}

	sendTiming := trimmedOptionalString(request.SendTiming)
	sendCondition := trimmedOptionalString(request.SendCondition)
	for _, email := range emails {
		changes := map[string]any{}
		nextTitle, nextTiming, nextCondition := email.Title, email.SendTiming, email.SendCondition
		if title != nil {
			nextTitle = *title
			addStringChange(changes, "title", &email.Title, &nextTitle)
		}
		if request.SendTiming != nil {
			nextTiming = sendTiming
			addStringChange(changes, "send_timing", email.SendTiming, nextTiming)
		}
		if request.SendCondition != nil {
			nextCondition = sendCondition
			addStringChange(changes, "send_condition", email.SendCondition, nextCondition)
		}
		if len(changes) > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE emails SET title = $2, send_timing = $3, send_condition = $4, updated_at = now()
				WHERE id = $1;
			`, email.ID, nextTitle, nextTiming, nextCondition); err != nil {
				return slotPlaceResponse{}, err
			}
		}
		if len(changes) == 0 && !moved {
			continue
		}
		event := EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventSlotUpdated,
			EmailID:     &email.ID,
			EmailSlug:   &email.Slug,
			EmailTitle:  &nextTitle,
			Changes:     changes,
		}
		if moved {
			event.Metadata = slotMoveMetadata(request.Stage, request.SortOrder, toStage, toSortOrder)
		}
		if err := InsertEmailEvent(ctx, tx, event); err != nil {
			return slotPlaceResponse{}, err
		}
	}

	return slotPlaceResponse{Stage: toStage, SortOrder: toSortOrder}, nil
}

func moveSlot(ctx context.Context, db *pgxpool.Pool, user AuthUser, request moveSlotRequest) (slotPlaceResponse, error) {
	request.Sequence = strings.TrimSpace(request.Sequence)
	request.Stage = strings.TrimSpace(request.Stage)
	request.ToStage = strings.TrimSpace(request.ToStage)
	if request.Sequence == "" || request.Stage == "" || request.ToStage == "" {
		return slotPlaceResponse{}, createEmailInputError("sequence, stage and to_stage are required")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return slotPlaceResponse{}, err
	}
	defer tx.Rollback(ctx)

	emails, err := lockSlotEmails(ctx, tx, request.Sequence, request.Stage, request.SortOrder)
	if err != nil {
		return slotPlaceResponse{}, err
	}
	if err := requireStageOnBoard(ctx, tx, request.Sequence, request.ToStage); err != nil {
		return slotPlaceResponse{}, err
	}

	rows, err := tx.Query(ctx, `
		SELECT DISTINCT sort_order FROM emails
		WHERE sequence = $1 AND stage = $2 AND archived_at IS NULL
		ORDER BY sort_order;
	`, request.Sequence, request.ToStage)
	if err != nil {
		return slotPlaceResponse{}, err
	}
	targetOrders, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return slotPlaceResponse{}, err
	}

	from := slotMove{FromStage: request.Stage, FromSortOrder: request.SortOrder}
	var moves []slotMove
	if request.ToStage == request.Stage {
		moves, err = planSlotReorder(targetOrders, from, request.BeforeSortOrder)
	} else {
		firstOrder := 0
		if len(targetOrders) == 0 {
			if firstOrder, err = resolveSlotSortOrder(ctx, tx, createSlotRequest{Sequence: request.Sequence, Stage: request.ToStage}); err != nil {
				return slotPlaceResponse{}, err
			}
		}
		moves, err = planSlotInsert(targetOrders, from, request.ToStage, request.BeforeSortOrder, firstOrder)
	}
	if err != nil {
		return slotPlaceResponse{}, err
	}
	if len(moves) == 0 {
		return slotPlaceResponse{Stage: request.Stage, SortOrder: request.SortOrder}, nil
	}
	if err := applySlotMoves(ctx, tx, request.Sequence, moves); err != nil {
		return slotPlaceResponse{}, err
	}

	// Only the dragged slot gets history entries; the ones that shifted to
	// make room keep their place relative to each other.
	var to slotMove
	for _, move := range moves {
		if move.FromStage == from.FromStage && move.FromSortOrder == from.FromSortOrder {
			to = move
		}
	}
	for _, email := range emails {
		if err := InsertEmailEvent(ctx, tx, EmailEventParam{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      emailEventSlotUpdated,
			EmailID:     &email.ID,
			EmailSlug:   &email.Slug,
			EmailTitle:  &email.Title,
			Metadata:    slotMoveMetadata(to.FromStage, to.FromSortOrder, to.ToStage, to.ToSortOrder),
		}); err != nil {
			return slotPlaceResponse{}, err
		}
	}
	return slotPlaceResponse{Stage: to.ToStage, SortOrder: to.ToSortOrder}, tx.Commit(ctx)
}

// planSlotReorder moves a slot within its stage. The stage keeps its set of
// positions; only the slots between the old and the new place shift by one.
func planSlotReorder(orders []int, from slotMove, before *int) ([]slotMove, error) {
	index := sort.SearchInts(orders, from.FromSortOrder)
	if index == len(orders) || orders[index] != from.FromSortOrder {
		return nil, errSlotStale
	}
	sequence := append(append([]int{}, orders[:index]...), orders[index+1:]...)
	insertAt, err := insertIndex(sequence, before)
	if err != nil {
		return nil, err
	}
	sequence = append(sequence[:insertAt], append([]int{from.FromSortOrder}, sequence[insertAt:]...)...)

	var moves []slotMove
	for i, order := range sequence {
		if order != orders[i] {
			moves = append(moves, slotMove{from.FromStage, order, from.FromStage, orders[i]})
		}
	}
	return moves, nil
}

// planSlotInsert moves a slot into another stage before a given slot: it takes
// that slot's position and the following slots shift down only as far as
// needed. At the end it gets the last position + 1; an empty stage starts at
// firstOrder.
func planSlotInsert(orders []int, from slotMove, toStage string, before *int, firstOrder int) ([]slotMove, error) {
	insertAt, err := insertIndex(orders, before)
	if err != nil {
		return nil, err
	}
	target := firstOrder
	if insertAt < len(orders) {
		target = orders[insertAt]
	} else if len(orders) > 0 {
		target = orders[len(orders)-1] + 1
	}
	moves := []slotMove{{from.FromStage, from.FromSortOrder, toStage, target}}
	next := target
	for _, order := range orders[insertAt:] {
		next = max(next+1, order)
		if next == order {
			break
		}
		moves = append(moves, slotMove{toStage, order, toStage, next})
	}
	return moves, nil
}

func insertIndex(orders []int, before *int) (int, error) {
	if before == nil {
		return len(orders), nil
	}
	for i, order := range orders {
		if order == *before {
			return i, nil
		}
	}
	return 0, errSlotStale
}

// applySlotMoves changes the place of whole slots, archived emails included
// so that restoring one brings it back to its slot. Positions go through
// negative values first: the slots swap places inside one transaction and
// the unique index must not see two slots on one place.
func applySlotMoves(ctx context.Context, tx pgx.Tx, sequence string, moves []slotMove) error {
	fromStages := make([]string, len(moves))
	fromOrders := make([]int, len(moves))
	toStages := make([]string, len(moves))
	toOrders := make([]int, len(moves))
	for i, move := range moves {
		fromStages[i], fromOrders[i], toStages[i], toOrders[i] = move.FromStage, move.FromSortOrder, move.ToStage, move.ToSortOrder
	}
	rows, err := tx.Query(ctx, `
		UPDATE emails e
		SET stage = m.to_stage, sort_order = -m.to_order - 1
		FROM unnest($2::text[], $3::int[], $4::text[], $5::int[]) AS m(from_stage, from_order, to_stage, to_order)
		WHERE e.sequence = $1 AND e.stage = m.from_stage AND e.sort_order = m.from_order
		RETURNING e.id;
	`, sequence, fromStages, fromOrders, toStages, toOrders)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return slotConflict(err)
	}
	_, err = tx.Exec(ctx, `UPDATE emails SET sort_order = -sort_order - 1 WHERE id = ANY($1);`, ids)
	return slotConflict(err)
}

func slotConflict(err error) error {
	if err != nil && isEmailCreationConflict(err) {
		return errSlotPositionTaken
	}
	return err
}

func lockSlotEmails(ctx context.Context, tx pgx.Tx, sequence, stage string, sortOrder int) ([]slotEmail, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, slug, title, send_timing, send_condition FROM emails
		WHERE sequence = $1 AND stage = $2 AND sort_order = $3 AND archived_at IS NULL
		ORDER BY (language = 'en') DESC, language, created_at
		FOR UPDATE;
	`, sequence, stage, sortOrder)
	if err != nil {
		return nil, err
	}
	emails, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (slotEmail, error) {
		var email slotEmail
		err := row.Scan(&email.ID, &email.Slug, &email.Title, &email.SendTiming, &email.SendCondition)
		return email, err
	})
	if err != nil {
		return nil, err
	}
	if len(emails) == 0 {
		return nil, errSlotNotFound
	}
	return emails, nil
}

func slotMoveMetadata(fromStage string, fromSortOrder int, toStage string, toSortOrder int) map[string]any {
	return map[string]any{
		"from": map[string]any{"stage": fromStage, "sort_order": fromSortOrder},
		"to":   map[string]any{"stage": toStage, "sort_order": toSortOrder},
	}
}
