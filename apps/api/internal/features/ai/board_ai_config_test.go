package ai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestBoardAIConfigRequiresSuperAdmin(t *testing.T) {
	router := chi.NewRouter()
	RegisterAIRoutes(router, nil, AIAnalysisService{})

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		request := httptest.NewRequest(method, "/api/boards/onboarding/ai-config", nil)
		request = withAuthUser(request, AuthUser{ID: "a", Email: "admin@example.com", Role: "admin"})
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: expected 403 for admin, got %d", method, response.Code)
		}
	}
}

func TestSplitDefaultReviewRules(t *testing.T) {
	prose, rules := splitDefaultReviewRules("# Rules\n\nReview text only.\n\nCheck:\n\n- Subject: clear reason.\n- Plain rule\n")

	if len(rules) != 2 || rules[0].Title != "Subject" || rules[0].Body != "clear reason." || !rules[0].Enabled {
		t.Fatalf("unexpected rules: %#v", rules)
	}
	if rules[1].Title != "Plain rule" || rules[1].Body != "" {
		t.Fatalf("unexpected bullet without colon: %#v", rules[1])
	}
	if prose != "Review text only.\nCheck:" {
		t.Fatalf("unexpected prose: %q", prose)
	}
}

func TestRenderAIRulesAndRolesSkipsDisabled(t *testing.T) {
	rendered := renderAIRulesAndRoles(BoardAIConfig{
		Roles: []AIRole{{Name: "Copywriter", Description: "tone", Enabled: true}, {Name: "Legal", Description: "claims", Enabled: false}},
		Rules: []AIRule{{Title: "Subject", Body: "short", Enabled: true}, {Title: "Hidden", Enabled: false}},
	})

	for _, want := range []string{"Copywriter: tone", "Subject: short"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("missing %q in %q", want, rendered)
		}
	}
	for _, unwanted := range []string{"Legal", "Hidden"} {
		if strings.Contains(rendered, unwanted) {
			t.Errorf("unexpected %q in %q", unwanted, rendered)
		}
	}
}

func TestNormalizeAIConfigRequest(t *testing.T) {
	request := updateBoardAIConfigRequest{
		Rules: []AIRule{{Title: " A ", Body: " b "}, {}},
		Roles: []AIRole{{Name: "R"}},
	}
	if err := normalizeAIConfigRequest(&request); err != nil {
		t.Fatal(err)
	}
	if len(request.Rules) != 1 || request.Rules[0].Title != "A" || request.Rules[0].Body != "b" {
		t.Fatalf("unexpected rules: %#v", request.Rules)
	}

	if err := normalizeAIConfigRequest(&updateBoardAIConfigRequest{Rules: []AIRule{{Body: "no title"}}}); err == nil {
		t.Error("expected error for rule without title")
	}
}

func TestCustomInstructionChangesPromptAndHash(t *testing.T) {
	base := AIAnalysisService{ResponseLanguage: "English"}
	custom := base
	custom.Instruction = "You are a strict compliance reviewer."

	if base.PromptHash() == custom.PromptHash() {
		t.Error("custom instruction must change the prompt hash")
	}
	if !strings.HasPrefix(custom.analysisInstructions(), "You are a strict compliance reviewer.") {
		t.Errorf("custom instruction not used: %s", custom.analysisInstructions())
	}
	if !strings.Contains(custom.analysisInstructions(), "Return only JSON matching the schema.") {
		t.Error("fixed output constraints must be kept")
	}
	if !strings.HasPrefix(base.analysisInstructions(), "You are an email copy reviewer") {
		t.Error("default instruction should be unchanged")
	}
}
