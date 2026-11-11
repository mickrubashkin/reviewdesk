package portal

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// responsibleID reads the activity's responsible user; nil when missing.
func responsibleID(value string) *int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	return &id
}

// storeSenders saves the names and emails of the CRM portal users who sent
// the funnel's emails. A webhook without the "user" scope cannot read them:
// the sync goes on without names.
func storeSenders(ctx context.Context, db *pgxpool.Pool, client *Client, senders map[int64]bool) error {
	ids := make([]int64, 0, len(senders))
	for id := range senders {
		ids = append(ids, id)
	}
	for start := 0; start < len(ids); start += 50 {
		end := min(start+50, len(ids))
		response, err := client.Call(ctx, "user.get", map[string]any{"filter": map[string]any{"ID": ids[start:end]}})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return nil
		}
		var users []struct {
			ID       string `json:"ID"`
			Name     string `json:"NAME"`
			LastName string `json:"LAST_NAME"`
			Email    string `json:"EMAIL"`
		}
		if err := json.Unmarshal(response.Result, &users); err != nil {
			return nil
		}
		batch := &pgx.Batch{}
		for _, u := range users {
			id := responsibleID(u.ID)
			if id == nil {
				continue
			}
			batch.Queue(`
				INSERT INTO portal_users (user_id, name, email) VALUES ($1, $2, $3)
				ON CONFLICT (user_id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email, synced_at = now();
			`, *id, strings.TrimSpace(u.Name+" "+u.LastName), strings.TrimSpace(u.Email))
		}
		if err := sendBatch(ctx, db, batch); err != nil {
			return err
		}
	}
	return nil
}

// nameTokens lowercases a name, drops accents and splits it into words.
func nameTokens(name string) []string {
	plain, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), name)
	if err != nil {
		plain = name
	}
	return strings.FieldsFunc(strings.ToLower(plain), func(r rune) bool { return !unicode.IsLetter(r) })
}

// sameWord allows the ways a robot's name differs from the portal's: a short
// form ("Alex" for "Alexander") and one letter off in a longer word
// ("Bennet" for "Bennett", "Kovalska" for "Kowalska").
func sameWord(robot, user string) bool {
	if robot == user {
		return true
	}
	if len([]rune(robot)) >= 3 && strings.HasPrefix(user, robot) {
		return true
	}
	return min(len([]rune(robot)), len([]rune(user))) >= 5 && editDistanceAtMostOne(robot, user)
}

func editDistanceAtMostOne(a, b string) bool {
	x, y := []rune(a), []rune(b)
	if len(x) > len(y) {
		x, y = y, x
	}
	if len(y)-len(x) > 1 {
		return false
	}
	i, j, edits := 0, 0, 0
	for i < len(x) && j < len(y) {
		if x[i] == y[j] {
			i++
			j++
			continue
		}
		edits++
		if edits > 1 {
			return false
		}
		if len(x) == len(y) {
			i++
		}
		j++
	}
	return edits+(len(y)-j)+(len(x)-i) <= 1
}

type portalUser struct {
	ID    int64
	Name  string
	Email string
}

// suggestRobotUser picks the portal user a robot's name points to: every
// word of the robot's name matches a word of the user's name or of the part
// of the email before "@". Nothing when no user or more than one matches.
func suggestRobotUser(robot string, users []portalUser) *int64 {
	words := nameTokens(robot)
	if len(words) == 0 {
		return nil
	}
	var found *int64
	for _, u := range users {
		local, _, _ := strings.Cut(u.Email, "@")
		candidates := append(nameTokens(u.Name), nameTokens(local)...)
		all := true
		for _, word := range words {
			matched := false
			for _, candidate := range candidates {
				if sameWord(word, candidate) {
					matched = true
					break
				}
			}
			if !matched {
				all = false
				break
			}
		}
		if all {
			if found != nil {
				return nil
			}
			id := u.ID
			found = &id
		}
	}
	return found
}
