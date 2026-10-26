package ai

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestAnalysisBlocksPointRecommendations(t *testing.T) {
	review := `<html><head><style>td{}</style></head><body><table>
		<tr><td data-review-block="hero_banner"><img src="x.png" alt=""></td></tr>
		<tr><td data-review-block="intro_body">Dear Partner,<br>welcome   aboard.</td></tr>
		<tr><td><a data-review-block="primary_cta" href="#">Open Portal</a></td></tr>
		<tr><td data-review-block="footer_legal">Legal text</td></tr>
		<tr><td data-review-block="privacy_policy_link">Privacy</td></tr>
	</table></body></html>`
	email := EmailDetail{Title: "Test", ReviewHTML: &review}

	blocks := analysisBlocks(email)
	want := []analysisBlock{{ID: "intro_body", Text: "Dear Partner, welcome aboard."}, {ID: "primary_cta", Text: "Open Portal"}}
	if !slices.Equal(blocks, want) {
		t.Fatalf("blocks with text, without the footer: got %+v", blocks)
	}

	var payload struct {
		Email struct {
			Blocks []analysisBlock `json:"blocks"`
		} `json:"email"`
	}
	if err := json.Unmarshal([]byte(buildEmailAnalysisInput("", "", "English", email)), &payload); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(payload.Email.Blocks, want) {
		t.Fatalf("the input carries the blocks: %+v", payload.Email.Blocks)
	}

	// The model may only name a block of this email, the subject or the
	// preheader, or none.
	body := AIAnalysisService{Model: "test", ResponseLanguage: "English"}.buildOpenAIAnalysisRequestBody(email)
	schema := body["text"].(map[string]any)["format"].(map[string]any)["schema"].(map[string]any)
	item := schema["properties"].(map[string]any)["recommendations"].(map[string]any)["items"].(map[string]any)
	enum := item["properties"].(map[string]any)["block"].(map[string]any)["enum"].([]string)
	if !slices.Equal(enum, []string{"", "subject", "preheader", "intro_body", "primary_cta"}) {
		t.Fatalf("unexpected block values: %v", enum)
	}
	if !slices.Contains(item["required"].([]string), "block") {
		t.Fatal("strict schemas need every property required")
	}
}
