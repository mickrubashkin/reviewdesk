package emails

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	auth "github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
)

func TestMarkEmailLiveRequiresSuperAdmin(t *testing.T) {
	router := chi.NewRouter()
	router.Patch("/api/emails/{id}/live", markEmailLiveHandler(nil))

	request := httptest.NewRequest(http.MethodPatch, "/api/emails/x/live", strings.NewReader(`{"live":true}`))
	request = request.WithContext(auth.WithUser(request.Context(), auth.AuthUser{ID: "a", Email: "a@example.com", Role: "admin"}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin, got %d", response.Code)
	}
}
