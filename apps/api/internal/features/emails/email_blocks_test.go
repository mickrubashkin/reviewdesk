package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
)

func resetEmailBlocks(t *testing.T, dbpool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := dbpool.Exec(ctx, `DELETE FROM email_blocks;`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDefaultBlocks(ctx, dbpool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM email_blocks;`)
		_ = EnsureDefaultBlocks(context.Background(), dbpool)
	})
}

func blockRequest(t *testing.T, dbpool *pgxpool.Pool, user AuthUser, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	RegisterEmailRoutes(router, dbpool)
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &payload)
	request = request.WithContext(auth.WithUser(request.Context(), user))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestEmailBlocksListAndSave(t *testing.T) {
	dbpool := testDBPool(t)
	resetEmailBlocks(t, dbpool)
	superAdmin := AuthUser{ID: "blocks-super", Email: "blocks-super@example.com", Role: "super_admin"}

	if err := EnsureDefaultBlocks(context.Background(), dbpool); err != nil {
		t.Fatalf("adding the defaults again must be a no-op: %v", err)
	}
	response := blockRequest(t, dbpool, superAdmin, http.MethodGet, "/api/blocks", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list: %d %s", response.Code, response.Body.String())
	}
	var list struct{ Blocks []EmailBlock }
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Blocks) != len(emailblocks.Defaults()) || list.Blocks[0].Key != emailblocks.FrameKey {
		t.Fatalf("the library starts with the defaults, frame first: %+v", list.Blocks)
	}
	if !strings.Contains(response.Body.String(), `"key":"frame","name":"Frame"`) || strings.Contains(response.Body.String(), `"fields":null`) {
		t.Errorf("every block lists its fields as an array: %s", response.Body.String()[:200])
	}
	cta, _ := findEmailBlock(list.Blocks, "primary_cta")

	// The button gets a second line of text: the new field is added, the
	// known ones keep their descriptions.
	html := strings.Replace(cta.HTML, `</a>`, `</a><br><span data-edit-text="primary_cta_note">No card needed</span>`, 1)
	response = blockRequest(t, dbpool, superAdmin, http.MethodPut, "/api/blocks/primary_cta",
		emailBlockRequest{HTML: html, Version: cta.Version})
	if response.Code != http.StatusOK {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	var saved struct{ Block EmailBlock }
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Block.Version != cta.Version+1 || saved.Block.UpdatedBy != superAdmin.Email {
		t.Errorf("a save bumps the version and records who: %+v", saved.Block)
	}
	fields := map[string]emailblocks.Field{}
	for _, field := range saved.Block.Fields {
		fields[field.Key] = field
	}
	if fields["primary_cta_text"].MaxLength != 30 || !fields["primary_cta_text"].Required {
		t.Errorf("a known field keeps its description: %+v", fields["primary_cta_text"])
	}
	if fields["primary_cta_note"].Type != "text" {
		t.Errorf("a new field comes from the HTML: %+v", saved.Block.Fields)
	}

	response = blockRequest(t, dbpool, superAdmin, http.MethodPut, "/api/blocks/primary_cta",
		emailBlockRequest{HTML: cta.HTML, Version: cta.Version})
	if response.Code != http.StatusConflict {
		t.Errorf("a save from an old version must be refused: %d", response.Code)
	}

	response = blockRequest(t, dbpool, superAdmin, http.MethodPut, "/api/blocks/primary_cta",
		emailBlockRequest{HTML: `<p>Hi</p>`, Version: saved.Block.Version})
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "table row") {
		t.Errorf("broken HTML must be refused with the reason: %d %s", response.Code, response.Body.String())
	}
}

func TestEmailBlockPreview(t *testing.T) {
	dbpool := testDBPool(t)
	resetEmailBlocks(t, dbpool)
	superAdmin := AuthUser{ID: "blocks-super", Email: "blocks-super@example.com", Role: "super_admin"}

	response := blockRequest(t, dbpool, superAdmin, http.MethodPost, "/api/blocks/intro_body/preview",
		emailBlockRequest{HTML: `<tr><td data-review-block="intro_body" data-edit-text="intro_body">Unsaved text</td></tr>`})
	if response.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", response.Code, response.Body.String())
	}
	var preview struct{ HTML string }
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.HTML, "Unsaved text") || !strings.Contains(preview.HTML, "<!DOCTYPE html>") {
		t.Errorf("a block is previewed inside the frame with its own text: %s", preview.HTML)
	}

	frame := emailblocks.Defaults()[0]
	response = blockRequest(t, dbpool, superAdmin, http.MethodPost, "/api/blocks/frame/preview",
		emailBlockRequest{HTML: strings.Replace(frame.HTML, "#d7e9f8", "#ffeeee", 1)})
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.HTML, "#ffeeee") || !strings.Contains(preview.HTML, `data-review-block="footer_logo"`) {
		t.Errorf("the frame is previewed with every block in it: %d", response.Code)
	}
}

func TestEmailBlocksAreForSuperAdmins(t *testing.T) {
	dbpool := testDBPool(t)
	resetEmailBlocks(t, dbpool)
	admin := AuthUser{ID: "blocks-admin", Email: "blocks-admin@example.com", Role: "admin"}

	if response := blockRequest(t, dbpool, admin, http.MethodGet, "/api/blocks", nil); response.Code != http.StatusForbidden {
		t.Errorf("list: %d", response.Code)
	}
	if response := blockRequest(t, dbpool, admin, http.MethodPut, "/api/blocks/intro_body",
		emailBlockRequest{HTML: "<tr></tr>", Version: 1}); response.Code != http.StatusForbidden {
		t.Errorf("save: %d", response.Code)
	}
}

func TestBlockCatalogIsForEveryone(t *testing.T) {
	dbpool := testDBPool(t)
	resetEmailBlocks(t, dbpool)
	reviewer := AuthUser{ID: "blocks-reviewer", Email: "blocks-reviewer@example.com", Role: "reviewer"}

	response := blockRequest(t, dbpool, reviewer, http.MethodGet, "/api/block-catalog", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("catalog: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"html"`) {
		t.Error("the catalog carries the HTML of the blocks")
	}
	var body struct {
		Blocks []BlockCatalogItem `json:"blocks"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	var cta BlockCatalogItem
	for _, block := range body.Blocks {
		keys = append(keys, block.Key)
		if block.Key == "primary_cta" {
			cta = block
		}
	}
	if len(keys) == 0 || slices.Contains(keys, emailblocks.FrameKey) {
		t.Errorf("blocks: %v", keys)
	}
	limited := false
	for _, field := range cta.Fields {
		if field.Key == "primary_cta_text" && field.MaxLength == 30 {
			limited = true
		}
	}
	if !limited {
		t.Errorf("button fields: %+v", cta.Fields)
	}
}
