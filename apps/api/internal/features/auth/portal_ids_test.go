package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestParsePortalUserIDs(t *testing.T) {
	ids, err := parsePortalUserIDs([]byte(`{"success":true,"data":[
		{"id":681146,"email":"Ann@Example.com","active":true},
		{"id":7,"email":"ann@example.com","active":false},
		{"id":8,"email":"ann.other@example.com","active":true}
	]}`), "ann@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "681146" {
		t.Errorf("ids %v", ids)
	}
}

func TestFillPortalUserIDs(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	const (
		byAlias   = "portal-fill-alias@example.com"
		aliasAddr = "portal-fill-alias@other.test"
		ambiguous = "portal-fill-twice@example.com"
		taken     = "portal-fill-taken@example.com"
		holder    = "portal-fill-holder@example.com"
		unknown   = "portal-fill-unknown@example.com"
	)
	for _, email := range []string{byAlias, ambiguous, taken, holder, unknown} {
		deleteTestUserByEmail(t, dbpool, email)
	}
	t.Cleanup(func() {
		_, _ = dbpool.Exec(context.Background(), `DELETE FROM user_email_aliases WHERE email = $1;`, aliasAddr)
	})
	var aliasUserID string
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ($1, 'reviewer') RETURNING id;`, byAlias).Scan(&aliasUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO user_email_aliases (email, user_id) VALUES ($1, $2);`, aliasAddr, aliasUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO users (email, role, portal_user_id) VALUES ($1, 'reviewer', NULL), ($2, 'reviewer', NULL), ($3, 'reviewer', '9200003'), ($4, 'reviewer', NULL);
	`, ambiguous, taken, holder, unknown); err != nil {
		t.Fatal(err)
	}

	found := map[string][]string{
		aliasAddr: {"9200001"},            // found by the alias only
		ambiguous: {"9200002", "9200004"}, // two employees: left alone
		taken:     {"9200003"},            // another user has it: left alone
	}
	findPortalUserIDs = func(_ context.Context, email string) ([]string, error) {
		return found[email], nil
	}
	t.Cleanup(func() { findPortalUserIDs = fetchPortalUserIDsByEmail })

	if _, err := fillPortalUserIDs(ctx, dbpool); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{byAlias: "9200001", ambiguous: "", taken: "", holder: "9200003", unknown: ""}
	for email, id := range want {
		var got *string
		if err := dbpool.QueryRow(ctx, `SELECT portal_user_id FROM users WHERE email = $1;`, email).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (got == nil && id != "") || (got != nil && *got != id) {
			t.Errorf("%s: portal_user_id %v, want %q", email, got, id)
		}
	}
}

func TestSetPortalUserIDByHand(t *testing.T) {
	dbpool := testDBPool(t)
	ctx := context.Background()
	admin := createTestUserWithRole(t, dbpool, "super_admin")
	const email, other = "portal-hand@example.com", "portal-hand-other@example.com"
	deleteTestUserByEmail(t, dbpool, email)
	deleteTestUserByEmail(t, dbpool, other)
	var userID string
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ($1, 'reviewer') RETURNING id;`, email).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO users (email, role, portal_user_id) VALUES ($1, 'reviewer', '9300002');`, other); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	RegisterAuthRoutes(router, dbpool, nil)
	set := func(as string, value string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+userID+"/portal-id", strings.NewReader(`{"portal_user_id":"`+value+`"}`))
		user := admin
		if as != "super_admin" {
			user.Role = as
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, withAuthUser(r, user))
		return response
	}

	if response := set("admin", "9300001"); response.Code != http.StatusForbidden {
		t.Errorf("as admin: %d", response.Code)
	}
	if response := set("super_admin", "12ab"); response.Code != http.StatusBadRequest {
		t.Errorf("not a number: %d", response.Code)
	}
	if response := set("super_admin", "9300002"); response.Code != http.StatusConflict {
		t.Errorf("taken: %d", response.Code)
	}
	response := set("super_admin", "9300001")
	var item UserAdminItem
	_ = json.NewDecoder(response.Body).Decode(&item)
	if response.Code != http.StatusOK || item.PortalUserID == nil || *item.PortalUserID != "9300001" {
		t.Errorf("set: %d %+v", response.Code, item.PortalUserID)
	}
	response = set("super_admin", "")
	item = UserAdminItem{}
	_ = json.NewDecoder(response.Body).Decode(&item)
	if response.Code != http.StatusOK || item.PortalUserID != nil {
		t.Errorf("clear: %d %+v", response.Code, item.PortalUserID)
	}
}
