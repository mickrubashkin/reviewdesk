package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

const (
	maxAIInstructionLength = 4000
	maxAIContextLength     = 20000
	maxAIRules             = 50
	maxAIRuleTitleLength   = 120
	maxAIRuleBodyLength    = 2000
	maxAIRoles             = 20
	maxAIRoleNameLength    = 80
	maxAIRoleDescLength    = 1000
)

type AIRule struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	Enabled bool   `json:"enabled"`
}

type AIRole struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// BoardAIConfig is the per-board AI review setup. Boards without a stored
// config fall back to the file-based defaults loaded at startup.
type BoardAIConfig struct {
	BoardKey        string    `json:"board_key"`
	Instruction     string    `json:"instruction"`
	SequenceContext string    `json:"sequence_context"`
	Rules           []AIRule  `json:"rules"`
	Roles           []AIRole  `json:"roles"`
	IsDefault       bool      `json:"is_default"`
	UpdatedByEmail  string    `json:"updated_by_email"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type updateBoardAIConfigRequest struct {
	Instruction     string   `json:"instruction"`
	SequenceContext string   `json:"sequence_context"`
	Rules           []AIRule `json:"rules"`
	Roles           []AIRole `json:"roles"`
}

var errAIBoardNotFound = errors.New("board not found")

func registerBoardAIConfigRoutes(r chi.Router, dbpool *pgxpool.Pool, aiService AIAnalysisService) {
	r.Get("/api/boards/{boardKey}/ai-config", getBoardAIConfigHandler(dbpool, aiService))
	r.Put("/api/boards/{boardKey}/ai-config", putBoardAIConfigHandler(dbpool))
	r.Delete("/api/boards/{boardKey}/ai-config", deleteBoardAIConfigHandler(dbpool))
	r.Post("/api/boards/{boardKey}/ai-export", exportBoardForAIHandler(dbpool, aiService))
}

func requireSuperAdmin(w http.ResponseWriter, r *http.Request) (AuthUser, bool) {
	user, ok := auth.FromRequest(r)
	if !ok || !auth.IsSuperAdmin(user) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return AuthUser{}, false
	}
	return user, true
}

func getBoardAIConfigHandler(dbpool *pgxpool.Pool, aiService AIAnalysisService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSuperAdmin(w, r); !ok {
			return
		}

		boardKey := chi.URLParam(r, "boardKey")
		config, found, err := loadBoardAIConfig(r.Context(), dbpool, boardKey)
		if errors.Is(err, errAIBoardNotFound) {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load ai config for board %s: %v\n", boardKey, err)
			http.Error(w, "failed to load ai config", http.StatusInternalServerError)
			return
		}
		if !found {
			config = defaultBoardAIConfig(boardKey, aiService)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
	}
}

func putBoardAIConfigHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireSuperAdmin(w, r)
		if !ok {
			return
		}

		var request updateBoardAIConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := normalizeAIConfigRequest(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rulesJSON, _ := json.Marshal(request.Rules)
		rolesJSON, _ := json.Marshal(request.Roles)
		boardKey := chi.URLParam(r, "boardKey")

		tag, err := dbpool.Exec(r.Context(), `
			INSERT INTO board_ai_configs (board_id, instruction, sequence_context, rules, roles, updated_by_email)
			SELECT id, $2, $3, $4::jsonb, $5::jsonb, $6
			FROM boards
			WHERE key = $1
			ON CONFLICT (board_id) DO UPDATE SET
				instruction = EXCLUDED.instruction,
				sequence_context = EXCLUDED.sequence_context,
				rules = EXCLUDED.rules,
				roles = EXCLUDED.roles,
				updated_by_email = EXCLUDED.updated_by_email,
				updated_at = now();
		`, boardKey, request.Instruction, request.SequenceContext, string(rulesJSON), string(rolesJSON), user.Email)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to save ai config for board %s: %v\n", boardKey, err)
			http.Error(w, "failed to save ai config", http.StatusInternalServerError)
			return
		}
		if tag.RowsAffected() == 0 {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}

		config, _, err := loadBoardAIConfig(r.Context(), dbpool, boardKey)
		if err != nil {
			http.Error(w, "failed to load ai config", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
	}
}

func deleteBoardAIConfigHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSuperAdmin(w, r); !ok {
			return
		}

		boardKey := chi.URLParam(r, "boardKey")
		_, err := dbpool.Exec(r.Context(), `
			DELETE FROM board_ai_configs
			USING boards
			WHERE boards.id = board_ai_configs.board_id AND boards.key = $1;
		`, boardKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to reset ai config for board %s: %v\n", boardKey, err)
			http.Error(w, "failed to reset ai config", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func normalizeAIConfigRequest(request *updateBoardAIConfigRequest) error {
	request.Instruction = strings.TrimSpace(request.Instruction)
	request.SequenceContext = strings.TrimSpace(request.SequenceContext)

	if utf8.RuneCountInString(request.Instruction) > maxAIInstructionLength {
		return fmt.Errorf("instruction must be at most %d characters", maxAIInstructionLength)
	}
	if utf8.RuneCountInString(request.SequenceContext) > maxAIContextLength {
		return fmt.Errorf("sequence_context must be at most %d characters", maxAIContextLength)
	}
	if len(request.Rules) > maxAIRules {
		return fmt.Errorf("at most %d rules are allowed", maxAIRules)
	}
	if len(request.Roles) > maxAIRoles {
		return fmt.Errorf("at most %d roles are allowed", maxAIRoles)
	}

	rules := make([]AIRule, 0, len(request.Rules))
	for _, rule := range request.Rules {
		rule.Title = strings.TrimSpace(rule.Title)
		rule.Body = strings.TrimSpace(rule.Body)
		if rule.Title == "" && rule.Body == "" {
			continue
		}
		if rule.Title == "" {
			return errors.New("every rule needs a title")
		}
		if utf8.RuneCountInString(rule.Title) > maxAIRuleTitleLength || utf8.RuneCountInString(rule.Body) > maxAIRuleBodyLength {
			return errors.New("rule title or body is too long")
		}
		rules = append(rules, rule)
	}
	request.Rules = rules

	roles := make([]AIRole, 0, len(request.Roles))
	for _, role := range request.Roles {
		role.Name = strings.TrimSpace(role.Name)
		role.Description = strings.TrimSpace(role.Description)
		if role.Name == "" && role.Description == "" {
			continue
		}
		if role.Name == "" {
			return errors.New("every role needs a name")
		}
		if utf8.RuneCountInString(role.Name) > maxAIRoleNameLength || utf8.RuneCountInString(role.Description) > maxAIRoleDescLength {
			return errors.New("role name or description is too long")
		}
		roles = append(roles, role)
	}
	request.Roles = roles

	return nil
}

// loadBoardAIConfig returns found=false when the board exists but has no
// stored config, and errAIBoardNotFound when the board itself is missing.
func loadBoardAIConfig(ctx context.Context, dbpool *pgxpool.Pool, boardKey string) (BoardAIConfig, bool, error) {
	var config BoardAIConfig
	var rulesJSON, rolesJSON []byte
	var instruction, sequenceContext, updatedBy *string
	var updatedAt *time.Time

	err := dbpool.QueryRow(ctx, `
		SELECT
			boards.key,
			c.instruction,
			c.sequence_context,
			c.rules,
			c.roles,
			c.updated_by_email,
			c.updated_at
		FROM boards
		LEFT JOIN board_ai_configs c ON c.board_id = boards.id
		WHERE boards.key = $1;
	`, boardKey).Scan(&config.BoardKey, &instruction, &sequenceContext, &rulesJSON, &rolesJSON, &updatedBy, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BoardAIConfig{}, false, errAIBoardNotFound
	}
	if err != nil {
		return BoardAIConfig{}, false, err
	}
	if instruction == nil {
		return BoardAIConfig{BoardKey: boardKey}, false, nil
	}

	config.Instruction = *instruction
	config.SequenceContext = *sequenceContext
	config.UpdatedByEmail = *updatedBy
	config.UpdatedAt = *updatedAt
	config.Rules = []AIRule{}
	config.Roles = []AIRole{}
	if err := json.Unmarshal(rulesJSON, &config.Rules); err != nil {
		return BoardAIConfig{}, false, err
	}
	if err := json.Unmarshal(rolesJSON, &config.Roles); err != nil {
		return BoardAIConfig{}, false, err
	}

	return config, true, nil
}

// defaultBoardAIConfig expresses the file-based defaults in editable form so a
// super admin can start from what the AI uses today.
func defaultBoardAIConfig(boardKey string, aiService AIAnalysisService) BoardAIConfig {
	instruction, rules := splitDefaultReviewRules(aiService.ReviewRules)

	return BoardAIConfig{
		BoardKey:        boardKey,
		Instruction:     strings.TrimSpace(defaultAIInstructionHead + "\n" + instruction),
		SequenceContext: strings.TrimSpace(aiService.SequenceContext),
		Rules:           rules,
		Roles:           []AIRole{},
		IsDefault:       true,
	}
}

// splitDefaultReviewRules turns "- Title: body" bullets into rules and keeps
// the remaining prose as extra instruction text.
func splitDefaultReviewRules(markdown string) (string, []AIRule) {
	rules := []AIRule{}
	prose := []string{}

	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# "):
			continue
		case strings.HasPrefix(trimmed, "- "):
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			title, body, found := strings.Cut(item, ":")
			if !found {
				title, body = item, ""
			}
			rules = append(rules, AIRule{Title: strings.TrimSpace(title), Body: strings.TrimSpace(body), Enabled: true})
		case trimmed != "":
			prose = append(prose, trimmed)
		}
	}

	return strings.Join(prose, "\n"), rules
}

// withBoardConfig returns a copy of the service using the board's stored AI
// config, or the service itself when the board has none.
func (service AIAnalysisService) withBoardConfig(ctx context.Context, dbpool *pgxpool.Pool, boardKey string) AIAnalysisService {
	config, found, err := loadBoardAIConfig(ctx, dbpool, boardKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load ai config for board %s, using defaults: %v\n", boardKey, err)
		return service
	}
	if !found {
		return service
	}

	service.Instruction = config.Instruction
	service.SequenceContext = config.SequenceContext
	service.ReviewRules = renderAIRulesAndRoles(config)
	return service
}

func renderAIRulesAndRoles(config BoardAIConfig) string {
	var b strings.Builder

	roles := []AIRole{}
	for _, role := range config.Roles {
		if role.Enabled {
			roles = append(roles, role)
		}
	}
	if len(roles) > 0 {
		b.WriteString("Reviewer roles (review the email from each of these perspectives):\n\n")
		for _, role := range roles {
			fmt.Fprintf(&b, "- %s: %s\n", role.Name, role.Description)
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
		b.WriteString("Review rules:\n\n")
		for _, rule := range rules {
			if rule.Body == "" {
				fmt.Fprintf(&b, "- %s\n", rule.Title)
			} else {
				fmt.Fprintf(&b, "- %s: %s\n", rule.Title, rule.Body)
			}
		}
	}

	return strings.TrimSpace(b.String())
}
