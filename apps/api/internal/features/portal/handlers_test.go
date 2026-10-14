package portal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

func TestPortalRoutesRequireSuperAdmin(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router, nil)

	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/portal/categories", nil),
		httptest.NewRequest(http.MethodPost, "/api/boards/onboarding/portal/sync", strings.NewReader(`{"category_id":10,"days":90}`)),
		httptest.NewRequest(http.MethodGet, "/api/boards/onboarding/portal", nil),
		httptest.NewRequest(http.MethodGet, "/api/portal/groups/x", nil),
		httptest.NewRequest(http.MethodPatch, "/api/portal/groups/x", strings.NewReader(`{"decision":"ignored"}`)),
		httptest.NewRequest(http.MethodGet, "/api/portal/groups/x/reconstruct", nil),
		httptest.NewRequest(http.MethodPost, "/api/boards/onboarding/portal/import-missing", nil),
	}
	for _, request := range requests {
		request = request.WithContext(auth.WithUser(request.Context(), auth.AuthUser{ID: "a", Email: "a@example.com", Role: "admin"}))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403 for admin, got %d", request.Method, request.URL.Path, response.Code)
		}
	}
}
