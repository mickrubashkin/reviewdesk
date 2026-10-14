package ai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

func withAuthUser(r *http.Request, user AuthUser) *http.Request {
	return r.WithContext(auth.WithUser(r.Context(), user))
}

func TestAIAnalysisLogsRequireAdmin(t *testing.T) {
	router := chi.NewRouter()
	RegisterAIRoutes(router, nil, AIAnalysisService{})

	request := httptest.NewRequest(http.MethodGet, "/api/ai-analysis-logs", nil)
	request = withAuthUser(request, AuthUser{
		ID:    "reviewer-id",
		Email: "reviewer@example.com",
		Role:  "reviewer",
	})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected GET status 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAIAnalysisDebugRequiresAdminWhenEnabled(t *testing.T) {
	t.Setenv("AI_DEBUG_ENABLED", "true")

	router := chi.NewRouter()
	RegisterAIRoutes(router, nil, AIAnalysisService{})

	request := httptest.NewRequest(http.MethodGet, "/api/emails/email-id/ai-analysis-debug", nil)
	request = withAuthUser(request, AuthUser{
		ID:    "reviewer-id",
		Email: "reviewer@example.com",
		Role:  "reviewer",
	})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected GET status 403, got %d: %s", response.Code, response.Body.String())
	}
}
