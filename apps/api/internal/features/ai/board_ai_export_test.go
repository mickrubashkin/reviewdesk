package ai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

func TestBoardAIExportRequiresSuperAdmin(t *testing.T) {
	router := chi.NewRouter()
	RegisterAIRoutes(router, nil, AIAnalysisService{})

	request := httptest.NewRequest(http.MethodPost, "/api/boards/onboarding/ai-export", strings.NewReader(`{}`))
	request = withAuthUser(request, AuthUser{ID: "a", Email: "admin@example.com", Role: "admin"})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin, got %d", response.Code)
	}
}

func TestBuildBoardAIDocument(t *testing.T) {
	positions := map[string]*boardPosition{
		"registered/1": {Stage: "registered", SortOrder: 1, Title: "Welcome"},
		"approved/2":   {Stage: "approved", SortOrder: 2, Title: "Set up workspace"},
		"approved/3":   {Stage: "approved", SortOrder: 3, Title: "Reminder"},
	}
	doc := buildBoardAIDocument(boardAIDocumentInput{
		BoardKey:  "onboarding",
		BoardName: "Onboarding",
		Stages:    []string{"registered", "approved"},
		Docs: []emails.MarkdownDoc{
			{Stage: "approved", SortOrder: 2, Markdown: "#### Workspace email\n"},
			{Stage: "registered", SortOrder: 1, Markdown: "#### Welcome email\n"},
		},
		Positions:      positions,
		IncludeBoard:   true,
		IncludeAISetup: true,
		Config: BoardAIConfig{
			Instruction:     "Be strict.",
			Roles:           []AIRole{{Name: "Copywriter", Description: "tone", Enabled: true}},
			Rules:           []AIRule{{Title: "Subject", Body: "short", Enabled: true}, {Title: "Off", Enabled: false}},
			SequenceContext: "# Context\nGoal.",
		},
		Now: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
	})

	for _, want := range []string{
		"# Onboarding — knowledge pack for AI",
		"Emails: 2",
		"1. **Registered** — 1 slots, 1 included",
		"2. **Approved** — 2 slots, 1 included",
		"- Approved: Reminder",
		"- **Copywriter**: tone",
		"- **Subject**: short",
		"#### Context",
		"### Stage: Registered",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %q in:\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "Off") {
		t.Errorf("disabled rule leaked:\n%s", doc)
	}
	if strings.Index(doc, "Welcome email") > strings.Index(doc, "Workspace email") {
		t.Error("emails must follow board stage order")
	}
}

func TestDemoteHeadingsSkipsCodeFences(t *testing.T) {
	got := demoteHeadings("# A\n```\n# not a heading\n```", 2)
	if got != "### A\n```\n# not a heading\n```" {
		t.Fatalf("unexpected: %q", got)
	}
}
