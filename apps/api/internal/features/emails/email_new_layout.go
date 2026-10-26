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
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// emailFromLayout is a new email assembled from the library.
type emailFromLayout struct {
	template string
	fields   emailedit.EditableFields
	rendered string
}

// buildNewEmailFromLayout assembles a new email in language on board: the
// template from the blocks, shared fields with the shared texts of the
// language and board, the other fields with the text written in the block.
func buildNewEmailFromLayout(ctx context.Context, db emailEventExecutor, layout emailblocks.Layout, language, board string, subject, preheader *string) (*emailFromLayout, error) {
	template, fields, err := buildFromLayout(ctx, db, layout, language, nil)
	var contentErr emailContentError
	if errors.As(err, &contentErr) {
		return nil, createEmailInputError(contentErr.message)
	}
	if err != nil {
		return nil, err
	}
	if fields, err = resolveSharedFields(ctx, db, language, board, fields); err != nil {
		return nil, err
	}
	rendered, err := emailedit.RenderEditableHTMLWithMetadata(template, fields, emailedit.RenderMetadata{
		Subject:   stringFromPointer(subject),
		Preheader: stringFromPointer(preheader),
	})
	if err != nil {
		return nil, createEmailInputError("the blocks could not be rendered: " + err.Error())
	}
	return &emailFromLayout{template: template, fields: fields, rendered: rendered}, nil
}

// defaultLayout is the blocks a new email in stage of board starts with:
// the most common layout of the EN emails built from blocks in the stage,
// else on the board, else the library default.
func defaultLayout(ctx context.Context, db emailEventExecutor, board, stage string) (emailblocks.Layout, error) {
	var raw []byte
	err := db.QueryRow(ctx, `
		SELECT layout
		FROM emails
		WHERE sequence = $1
			AND layout IS NOT NULL
			AND translation_of IS NULL
			AND lower(language) = 'en'
			AND archived_at IS NULL
		GROUP BY layout
		ORDER BY count(*) FILTER (WHERE stage = $2) DESC, count(*) DESC, max(updated_at) DESC
		LIMIT 1;
	`, board, stage).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return emailblocks.DefaultLayout(), nil
	}
	if err != nil {
		return nil, err
	}
	layout, err := parseLayout(raw)
	if err != nil {
		return nil, err
	}
	// A block removed from the library since then leaves the default.
	library, err := loadBlockLibrary(ctx, db)
	if err != nil {
		return nil, err
	}
	if layout.Validate(library) != nil {
		return emailblocks.DefaultLayout(), nil
	}
	return layout, nil
}

type defaultLayoutResponse struct {
	Layout emailblocks.Layout `json:"layout"`
}

// defaultLayoutHandler answers GET /api/default-layout?sequence=…&stage=….
func defaultLayoutHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		board := strings.TrimSpace(r.URL.Query().Get("sequence"))
		if !requireManageBoard(w, r, dbpool, user, board) {
			return
		}
		layout, err := defaultLayout(r.Context(), dbpool, board, strings.TrimSpace(r.URL.Query().Get("stage")))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to find the default layout of %s: %v\n", board, err)
			http.Error(w, "failed to find the default blocks", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(defaultLayoutResponse{Layout: layout})
	}
}

type layoutPreviewRequest struct {
	Sequence  string             `json:"sequence"`
	Language  string             `json:"language"`
	Layout    emailblocks.Layout `json:"layout"`
	Subject   *string            `json:"subject"`
	Preheader *string            `json:"preheader"`
}

// layoutPreviewHandler renders an email that is not created yet, the way
// creating it from the blocks would.
func layoutPreviewHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var request layoutPreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Layout == nil {
			http.Error(w, "layout is required", http.StatusBadRequest)
			return
		}
		board := strings.TrimSpace(request.Sequence)
		if !requireManageBoard(w, r, dbpool, user, board) {
			return
		}
		language := strings.ToLower(strings.TrimSpace(request.Language))
		if language == "" {
			language = slotMasterLanguage
		}
		built, err := buildNewEmailFromLayout(r.Context(), dbpool, request.Layout, language, board, request.Subject, request.Preheader)
		var inputErr createEmailInputError
		if errors.As(err, &inputErr) {
			http.Error(w, inputErr.Error(), http.StatusBadRequest)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to preview a layout: %v\n", err)
			http.Error(w, "failed to render the blocks", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"html": built.rendered})
	}
}
