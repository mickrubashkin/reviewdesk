package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

const maxBlockHTMLLength = 200_000

// EmailBlock is a block of the library as stored and served.
type EmailBlock struct {
	emailblocks.Block
	Version   int       `json:"version"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
	// Active emails built with the block and how many of them are live: a
	// save writes a new version of each.
	Emails     int `json:"emails"`
	LiveEmails int `json:"live_emails"`
}

// EnsureDefaultBlocks adds the blocks of the starting library that are not in
// the database yet; blocks already there keep their HTML.
func EnsureDefaultBlocks(ctx context.Context, db emailEventExecutor) error {
	for i, block := range emailblocks.Defaults() {
		fields, err := json.Marshal(block.Fields)
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
			INSERT INTO email_blocks (key, name, description, html, fields, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (key) DO NOTHING;
		`, block.Key, block.Name, block.Description, block.HTML, fields, i); err != nil {
			return err
		}
	}
	return nil
}

func listEmailBlocks(ctx context.Context, db emailEventExecutor) ([]EmailBlock, error) {
	rows, err := db.Query(ctx, `
		SELECT key, name, description, html, fields, version, updated_by_email, updated_at
		FROM email_blocks
		ORDER BY sort_order, key;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	blocks := []EmailBlock{}
	for rows.Next() {
		var block EmailBlock
		var fields []byte
		if err := rows.Scan(&block.Key, &block.Name, &block.Description, &block.HTML, &fields,
			&block.Version, &block.UpdatedBy, &block.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(fields, &block.Fields); err != nil {
			return nil, fmt.Errorf("block %s fields: %w", block.Key, err)
		}
		if block.Fields == nil {
			block.Fields = []emailblocks.Field{}
		}
		blocks = append(blocks, block)
	}
	return blocks, rows.Err()
}

func findEmailBlock(blocks []EmailBlock, key string) (EmailBlock, bool) {
	for _, block := range blocks {
		if block.Key == key {
			return block, true
		}
	}
	return EmailBlock{}, false
}

// blockLibraryHandler lets super admins in and loads the library for next.
func blockLibraryHandler(dbpool *pgxpool.Pool, next func(w http.ResponseWriter, r *http.Request, user AuthUser, blocks []EmailBlock)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.FromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.IsSuperAdmin(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		blocks, err := listEmailBlocks(r.Context(), dbpool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load blocks: %v\n", err)
			http.Error(w, "failed to load blocks", http.StatusInternalServerError)
			return
		}
		next(w, r, user, blocks)
	}
}

// countBlockUsage fills in how many emails use each block.
func countBlockUsage(ctx context.Context, db emailEventExecutor, blocks []EmailBlock) error {
	for i := range blocks {
		emails, err := emailsWithBlock(ctx, db, blocks[i].Key)
		if err != nil {
			return err
		}
		blocks[i].Emails, blocks[i].LiveEmails = len(emails), 0
		for _, email := range emails {
			if email.Live {
				blocks[i].LiveEmails++
			}
		}
	}
	return nil
}

func listEmailBlocksHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return blockLibraryHandler(dbpool, func(w http.ResponseWriter, r *http.Request, _ AuthUser, blocks []EmailBlock) {
		if err := countBlockUsage(r.Context(), dbpool, blocks); err != nil {
			fmt.Fprintf(os.Stderr, "failed to count block usage: %v\n", err)
			http.Error(w, "failed to load blocks", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"blocks": blocks})
	})
}

// BlockCatalogItem is a block as the email editor offers it: what it is and
// its fields, without the HTML.
type BlockCatalogItem struct {
	Key         string              `json:"key"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Fields      []emailblocks.Field `json:"fields"`
}

// listBlockCatalogHandler lists the blocks an email can be built from, for
// everyone signed in: managers of a board pick them, translators see what
// an email is made of. The frame is not on the list, it is always there.
func listBlockCatalogHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.FromRequest(r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		blocks, err := listEmailBlocks(r.Context(), dbpool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load blocks: %v\n", err)
			http.Error(w, "failed to load blocks", http.StatusInternalServerError)
			return
		}
		catalog := []BlockCatalogItem{}
		for _, block := range blocks {
			if block.Key == emailblocks.FrameKey {
				continue
			}
			catalog = append(catalog, BlockCatalogItem{
				Key: block.Key, Name: block.Name, Description: block.Description, Fields: block.Fields,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"blocks": catalog})
	}
}

type emailBlockRequest struct {
	HTML string `json:"html"`
	// Version is the version the editor started from; a newer one means
	// someone else saved in between.
	Version int `json:"version"`
}

func decodeEmailBlockRequest(r *http.Request) (emailBlockRequest, error) {
	var request emailBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return request, emailContentError{http.StatusBadRequest, "invalid request body"}
	}
	if len(request.HTML) > maxBlockHTMLLength {
		return request, emailContentError{http.StatusBadRequest, "the HTML is too long"}
	}
	return request, nil
}

// blockTemplate is the template html makes for the block with key: the
// block in the saved frame, or for the frame every other block in it.
func blockTemplate(blocks []EmailBlock, key, html string) (string, error) {
	frame, ok := findEmailBlock(blocks, emailblocks.FrameKey)
	if !ok {
		return "", fmt.Errorf("the library has no frame")
	}
	if err := emailblocks.Validate(key, html, frame.HTML); err != nil {
		return "", emailContentError{http.StatusBadRequest, err.Error()}
	}
	if key != emailblocks.FrameKey {
		return emailblocks.Assemble(frame.HTML, html)
	}
	others := []string{}
	for _, block := range blocks {
		if block.Key != emailblocks.FrameKey {
			others = append(others, block.HTML)
		}
	}
	return emailblocks.Assemble(html, others...)
}

// previewEmailBlockHandler renders HTML for a block, saved or not, with the
// values written in its markup.
func previewEmailBlockHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return blockLibraryHandler(dbpool, func(w http.ResponseWriter, r *http.Request, _ AuthUser, blocks []EmailBlock) {
		key := chi.URLParam(r, "key")
		if _, ok := findEmailBlock(blocks, key); !ok {
			http.Error(w, "block not found", http.StatusNotFound)
			return
		}
		request, err := decodeEmailBlockRequest(r)
		if err == nil {
			var rendered string
			rendered, err = renderBlockPreview(blocks, key, request.HTML)
			if err == nil {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"html": rendered})
				return
			}
		}
		writeEmailBlockError(w, err)
	})
}

func renderBlockPreview(blocks []EmailBlock, key, html string) (string, error) {
	template, err := blockTemplate(blocks, key, html)
	if err != nil {
		return "", err
	}
	fields, err := emailedit.ExtractEditableFields(template)
	if err != nil {
		return "", emailContentError{http.StatusBadRequest, err.Error()}
	}
	// A field written in Markdown in the block (a list) shows formatted, as
	// in a new email.
	for _, block := range blocks {
		for _, described := range block.Fields {
			if field, ok := fields[described.Key]; ok && described.Format != "" && field.Type == emailedit.FieldTypeText {
				field.Format = described.Format
				fields[described.Key] = field
			}
		}
	}
	rendered, err := emailedit.RenderEditableHTMLWithMetadata(template, fields, emailedit.RenderMetadata{Subject: "Block preview"})
	if err != nil {
		return "", emailContentError{http.StatusBadRequest, err.Error()}
	}
	return rendered, nil
}

type updateEmailBlockResponse struct {
	Block         EmailBlock `json:"block"`
	UpdatedEmails int        `json:"updated_emails"`
	LiveEmails    int        `json:"live_emails"`
}

// updateEmailBlockHandler saves a block's HTML; its fields follow the HTML
// and keep their descriptions. Every email built with the block gets a new
// version in the same transaction.
func updateEmailBlockHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return blockLibraryHandler(dbpool, func(w http.ResponseWriter, r *http.Request, user AuthUser, blocks []EmailBlock) {
		request, err := decodeEmailBlockRequest(r)
		if err != nil {
			writeEmailBlockError(w, err)
			return
		}
		tx, err := dbpool.Begin(r.Context())
		if err != nil {
			writeEmailBlockError(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		key := chi.URLParam(r, "key")
		var response updateEmailBlockResponse
		response.Block, err = updateEmailBlock(r.Context(), tx, user, blocks, key, request)
		if err == nil {
			current, _ := findEmailBlock(blocks, key)
			if response.Block.Version != current.Version {
				response.UpdatedEmails, response.LiveEmails, err = rebuildEmailsWithBlock(r.Context(), tx, user, key, response.Block.Version)
			}
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			writeEmailBlockError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
}

func updateEmailBlock(ctx context.Context, db emailEventExecutor, user AuthUser, blocks []EmailBlock, key string, request emailBlockRequest) (EmailBlock, error) {
	current, ok := findEmailBlock(blocks, key)
	if !ok {
		return EmailBlock{}, emailContentError{http.StatusNotFound, "block not found"}
	}
	if _, err := blockTemplate(blocks, key, request.HTML); err != nil {
		return EmailBlock{}, err
	}
	fields := []emailblocks.Field{}
	if key != emailblocks.FrameKey {
		frame, _ := findEmailBlock(blocks, emailblocks.FrameKey)
		var err error
		fields, err = emailblocks.Fields(frame.HTML, request.HTML, current.Fields)
		if err != nil {
			return EmailBlock{}, emailContentError{http.StatusBadRequest, err.Error()}
		}
	}
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return EmailBlock{}, err
	}

	// The version check sits in the WHERE so a save in between is never lost.
	var saved EmailBlock
	var savedFields []byte
	err = db.QueryRow(ctx, `
		UPDATE email_blocks
		SET html = $3, fields = $4,
			version = version + CASE WHEN html <> $3 THEN 1 ELSE 0 END,
			updated_by_email = CASE WHEN html <> $3 THEN $5 ELSE updated_by_email END,
			updated_at = CASE WHEN html <> $3 THEN now() ELSE updated_at END
		WHERE key = $1 AND version = $2
		RETURNING key, name, description, html, fields, version, updated_by_email, updated_at;
	`, key, request.Version, request.HTML, fieldsJSON, user.Email).Scan(&saved.Key, &saved.Name, &saved.Description,
		&saved.HTML, &savedFields, &saved.Version, &saved.UpdatedBy, &saved.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return EmailBlock{}, emailContentError{http.StatusConflict,
			"someone changed this block in the meantime, reload and try again"}
	}
	if err != nil {
		return EmailBlock{}, err
	}
	if err := json.Unmarshal(savedFields, &saved.Fields); err != nil {
		return EmailBlock{}, err
	}
	return saved, nil
}

func writeEmailBlockError(w http.ResponseWriter, err error) {
	var contentErr emailContentError
	if errors.As(err, &contentErr) {
		http.Error(w, contentErr.message, contentErr.status)
		return
	}
	fmt.Fprintf(os.Stderr, "failed to save block: %v\n", err)
	http.Error(w, "failed to save the block", http.StatusInternalServerError)
}
