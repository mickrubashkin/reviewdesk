package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

const (
	maxVariables          = 50
	maxVariableLabel      = 80
	maxVariableExpression = 300
	maxVariableExample    = 500
	maxVariableNote       = 300
)

// Variable is a Bitrix24 expression that the CRM robot fills in when it sends
// an email, e.g. the responsible manager's booking link. Emails keep the
// expression as is; the preview shows Example in its place.
type Variable struct {
	Label      string `json:"label"`
	Expression string `json:"expression"`
	Example    string `json:"example"`
	Note       string `json:"note"`
}

type variablesPayload struct {
	Variables []Variable `json:"variables"`
}

func getVariablesHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.FromRequest(r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		variables, err := loadVariables(r.Context(), dbpool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load bitrix variables: %v\n", err)
			http.Error(w, "failed to load variables", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(variablesPayload{Variables: variables})
	}
}

func putVariablesHandler(dbpool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireSuperAdmin(w, r)
		if !ok {
			return
		}
		var request variablesPayload
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		variables, err := normalizeVariables(request.Variables)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveVariables(r.Context(), dbpool, variables, user.Email); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save bitrix variables: %v\n", err)
			http.Error(w, "failed to save variables", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(variablesPayload{Variables: variables})
	}
}

func loadVariables(ctx context.Context, dbpool *pgxpool.Pool) ([]Variable, error) {
	rows, err := dbpool.Query(ctx, `
		SELECT label, expression, example, note FROM bitrix_variables ORDER BY position;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	variables := []Variable{}
	for rows.Next() {
		var variable Variable
		if err := rows.Scan(&variable.Label, &variable.Expression, &variable.Example, &variable.Note); err != nil {
			return nil, err
		}
		variables = append(variables, variable)
	}
	return variables, rows.Err()
}

func saveVariables(ctx context.Context, dbpool *pgxpool.Pool, variables []Variable, updatedBy string) error {
	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM bitrix_variables;`); err != nil {
		return err
	}
	for position, variable := range variables {
		if _, err := tx.Exec(ctx, `
			INSERT INTO bitrix_variables (position, label, expression, example, note, updated_by_email)
			VALUES ($1, $2, $3, $4, $5, $6);
		`, position, variable.Label, variable.Expression, variable.Example, variable.Note, updatedBy); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func normalizeVariables(input []Variable) ([]Variable, error) {
	if len(input) > maxVariables {
		return nil, fmt.Errorf("at most %d variables", maxVariables)
	}
	variables := make([]Variable, 0, len(input))
	seen := map[string]bool{}
	for _, variable := range input {
		variable.Label = strings.TrimSpace(variable.Label)
		variable.Expression = strings.TrimSpace(variable.Expression)
		variable.Example = strings.TrimSpace(variable.Example)
		variable.Note = strings.TrimSpace(variable.Note)
		switch {
		case variable.Label == "":
			return nil, errors.New("every variable needs a name")
		case utf8.RuneCountInString(variable.Label) > maxVariableLabel:
			return nil, fmt.Errorf("name %q is too long", variable.Label)
		case !emailedit.IsBitrixExpression(variable.Expression):
			return nil, fmt.Errorf("%q: the expression must be a single Bitrix24 expression, e.g. {{Responsible person > Booking link}}", variable.Label)
		case utf8.RuneCountInString(variable.Expression) > maxVariableExpression:
			return nil, fmt.Errorf("%q: the expression is too long", variable.Label)
		case utf8.RuneCountInString(variable.Example) > maxVariableExample:
			return nil, fmt.Errorf("%q: the example is too long", variable.Label)
		case utf8.RuneCountInString(variable.Note) > maxVariableNote:
			return nil, fmt.Errorf("%q: the note is too long", variable.Label)
		case seen[variable.Expression]:
			return nil, fmt.Errorf("%q: this expression is already in the list", variable.Label)
		}
		seen[variable.Expression] = true
		variables = append(variables, variable)
	}
	return variables, nil
}
