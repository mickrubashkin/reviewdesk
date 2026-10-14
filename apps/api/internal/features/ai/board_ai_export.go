package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

const maxBoardAIExportEmails = 1000

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type boardAIExportRequest struct {
	EmailIDs         []string `json:"email_ids"`
	IncludeBoard     bool     `json:"include_board"`
	IncludeAISetup   bool     `json:"include_ai_setup"`
	EmailSections    []string `json:"email_sections"`
	OpenCommentsOnly bool     `json:"open_comments_only"`
}

type boardAIExportResponse struct {
	Markdown   string `json:"markdown"`
	EmailCount int    `json:"email_count"`
	FileName   string `json:"file_name"`
}

type boardPosition struct {
	Stage     string
	SortOrder int
	Title     string
	Included  int
}

func exportBoardForAIHandler(dbpool *pgxpool.Pool, aiService AIAnalysisService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSuperAdmin(w, r); !ok {
			return
		}

		var request boardAIExportRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if len(request.EmailIDs) > maxBoardAIExportEmails {
			http.Error(w, fmt.Sprintf("at most %d emails can be exported at once", maxBoardAIExportEmails), http.StatusBadRequest)
			return
		}
		for _, id := range request.EmailIDs {
			if !uuidPattern.MatchString(id) {
				http.Error(w, "invalid email id", http.StatusBadRequest)
				return
			}
		}

		boardKey := chi.URLParam(r, "boardKey")
		boardName, stages, err := loadBoardForExport(r.Context(), dbpool, boardKey)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load board %s for ai export: %v\n", boardKey, err)
			http.Error(w, "failed to load board", http.StatusInternalServerError)
			return
		}

		var docs []emails.MarkdownDoc
		if len(request.EmailIDs) > 0 {
			// Emails sit under "## Emails" > "### Stage" > "#### Title".
			docs, err = emails.ExportEmailMarkdownDocs(r.Context(), dbpool, boardKey, request.EmailIDs, request.EmailSections, request.OpenCommentsOnly, 3)
			if err != nil {
				if strings.Contains(err.Error(), "unknown section") {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				fmt.Fprintf(os.Stderr, "failed to export emails of board %s: %v\n", boardKey, err)
				http.Error(w, "failed to export emails", http.StatusInternalServerError)
				return
			}
		}

		positions, err := loadBoardPositions(r.Context(), dbpool, boardKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load positions of board %s: %v\n", boardKey, err)
			http.Error(w, "failed to load board", http.StatusInternalServerError)
			return
		}

		var config BoardAIConfig
		if request.IncludeAISetup {
			var found bool
			config, found, err = loadBoardAIConfig(r.Context(), dbpool, boardKey)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to load ai config of board %s: %v\n", boardKey, err)
				http.Error(w, "failed to load ai config", http.StatusInternalServerError)
				return
			}
			if !found {
				config = defaultBoardAIConfig(boardKey, aiService)
			}
		}

		markdown := buildBoardAIDocument(boardAIDocumentInput{
			BoardKey:       boardKey,
			BoardName:      boardName,
			Stages:         stages,
			Docs:           docs,
			Positions:      positions,
			IncludeBoard:   request.IncludeBoard,
			IncludeAISetup: request.IncludeAISetup,
			Config:         config,
			Now:            time.Now().UTC(),
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(boardAIExportResponse{
			Markdown:   markdown,
			EmailCount: len(docs),
			FileName:   fmt.Sprintf("%s-ai-pack-%s.md", boardKey, time.Now().UTC().Format("20060102")),
		})
	}
}

func loadBoardForExport(ctx context.Context, dbpool *pgxpool.Pool, boardKey string) (string, []string, error) {
	var name string
	var stagesJSON []byte
	if err := dbpool.QueryRow(ctx, `SELECT name, stages FROM boards WHERE key = $1;`, boardKey).Scan(&name, &stagesJSON); err != nil {
		return "", nil, err
	}
	var stages []string
	if err := json.Unmarshal(stagesJSON, &stages); err != nil {
		return "", nil, err
	}
	return name, stages, nil
}

// loadBoardPositions lists each (stage, sort_order) slot of the board with a
// representative title, so the document can show which slots have no email.
func loadBoardPositions(ctx context.Context, dbpool *pgxpool.Pool, boardKey string) (map[string]*boardPosition, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT id, COALESCE(stage, ''), sort_order, title
		FROM emails
		WHERE sequence = $1 AND archived_at IS NULL
		ORDER BY sort_order, created_at;
	`, boardKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	positions := map[string]*boardPosition{}
	for rows.Next() {
		var id, stage, title string
		var sortOrder int
		if err := rows.Scan(&id, &stage, &sortOrder, &title); err != nil {
			return nil, err
		}
		key := positionKey(stage, sortOrder)
		if _, ok := positions[key]; !ok {
			positions[key] = &boardPosition{Stage: stage, SortOrder: sortOrder, Title: title}
		}
	}
	return positions, rows.Err()
}

func positionKey(stage string, sortOrder int) string {
	return fmt.Sprintf("%s/%d", stage, sortOrder)
}

type boardAIDocumentInput struct {
	BoardKey       string
	BoardName      string
	Stages         []string
	Docs           []emails.MarkdownDoc
	Positions      map[string]*boardPosition
	IncludeBoard   bool
	IncludeAISetup bool
	Config         BoardAIConfig
	Now            time.Time
}

func buildBoardAIDocument(in boardAIDocumentInput) string {
	var b strings.Builder

	stageIndex := map[string]int{}
	for i, stage := range in.Stages {
		stageIndex[stage] = i
	}
	orderOf := func(stage string) int {
		if i, ok := stageIndex[stage]; ok {
			return i
		}
		return len(in.Stages)
	}

	docs := append([]emails.MarkdownDoc(nil), in.Docs...)
	sort.SliceStable(docs, func(i, j int) bool {
		if a, b := orderOf(docs[i].Stage), orderOf(docs[j].Stage); a != b {
			return a < b
		}
		return docs[i].SortOrder < docs[j].SortOrder
	})

	for _, doc := range docs {
		if p, ok := in.Positions[positionKey(doc.Stage, doc.SortOrder)]; ok {
			p.Included++
		}
	}

	fmt.Fprintf(&b, "# %s — knowledge pack for AI\n\n", in.BoardName)
	fmt.Fprintf(&b, "Board key: `%s` · Generated: %s · Emails: %d\n\n", in.BoardKey, in.Now.Format("2006-01-02"), len(docs))

	if in.IncludeBoard {
		b.WriteString("## Board\n\n")
		b.WriteString("Stages in order, with the number of email slots on the board and how many are included below:\n\n")

		stageOrder := append([]string(nil), in.Stages...)
		for _, p := range in.Positions {
			if _, ok := stageIndex[p.Stage]; !ok {
				stageIndex[p.Stage] = len(stageOrder)
				stageOrder = append(stageOrder, p.Stage)
			}
		}
		for i, stage := range stageOrder {
			slots, included := 0, 0
			for _, p := range in.Positions {
				if p.Stage == stage {
					slots++
					if p.Included > 0 {
						included++
					}
				}
			}
			fmt.Fprintf(&b, "%d. **%s** — %d slots, %d included\n", i+1, displayStage(stage), slots, included)
		}

		missing := []*boardPosition{}
		for _, p := range in.Positions {
			if p.Included == 0 {
				missing = append(missing, p)
			}
		}
		if len(missing) > 0 {
			sort.Slice(missing, func(i, j int) bool {
				if a, b := orderOf(missing[i].Stage), orderOf(missing[j].Stage); a != b {
					return a < b
				}
				return missing[i].SortOrder < missing[j].SortOrder
			})
			b.WriteString("\nSlots with no email in this document (not selected, or no email matches the chosen language/version):\n\n")
			for _, p := range missing {
				fmt.Fprintf(&b, "- %s: %s\n", displayStage(p.Stage), oneLineText(p.Title))
			}
		}
		b.WriteString("\n")
	}

	if in.IncludeAISetup {
		writeAISetupMarkdown(&b, in.Config)
	}

	b.WriteString("## Emails\n\n")
	if len(docs) == 0 {
		b.WriteString("No emails selected.\n")
	}
	currentStage, started := "", false
	for _, doc := range docs {
		if !started || doc.Stage != currentStage {
			fmt.Fprintf(&b, "### Stage: %s\n\n", displayStage(doc.Stage))
			currentStage, started = doc.Stage, true
		}
		b.WriteString(doc.Markdown)
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func writeAISetupMarkdown(b *strings.Builder, config BoardAIConfig) {
	b.WriteString("## AI review setup\n\n")

	if config.Instruction != "" {
		b.WriteString("### Instruction\n\n" + demoteHeadings(config.Instruction, 3) + "\n\n")
	}

	roles := []AIRole{}
	for _, role := range config.Roles {
		if role.Enabled {
			roles = append(roles, role)
		}
	}
	if len(roles) > 0 {
		b.WriteString("### Roles\n\n")
		for _, role := range roles {
			fmt.Fprintf(b, "- **%s**: %s\n", role.Name, oneLineText(role.Description))
		}
		b.WriteString("\n")
	}

	rules := []AIRule{}
	for _, rule := range config.Rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	if len(rules) > 0 {
		b.WriteString("### Rules\n\n")
		for _, rule := range rules {
			if rule.Body == "" {
				fmt.Fprintf(b, "- %s\n", rule.Title)
			} else {
				fmt.Fprintf(b, "- **%s**: %s\n", rule.Title, oneLineText(rule.Body))
			}
		}
		b.WriteString("\n")
	}

	if config.SequenceContext != "" {
		b.WriteString("### Sequence context\n\n" + demoteHeadings(config.SequenceContext, 3) + "\n\n")
	}
}

// demoteHeadings pushes Markdown headings down so user-written text can't
// break the document outline. Fenced code blocks are left alone.
func demoteHeadings(text string, levels int) string {
	lines := strings.Split(text, "\n")
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(line, "#") {
			lines[i] = strings.Repeat("#", levels) + line
		}
	}
	return strings.Join(lines, "\n")
}

func displayStage(stage string) string {
	if stage == "" {
		return "Uncategorized"
	}
	words := strings.FieldsFunc(stage, func(r rune) bool { return r == '-' || r == '_' })
	for i, word := range words {
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func oneLineText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
