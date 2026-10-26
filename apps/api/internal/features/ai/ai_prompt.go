package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const aiAnalysisSchemaVersion = "email-analysis-schema-v5"

// Appended only when set so prompts without a board config keep their hash.
func instructionHashPart(instruction string) string {
	if instruction == "" {
		return ""
	}
	return "\x00" + instruction
}

func (service AIAnalysisService) PromptHash() string {
	hash := sha256.Sum256([]byte(strings.Join([]string{
		service.Model,
		service.ResponseLanguage,
		service.ReviewRules,
		service.SequenceContext,
		aiAnalysisSchemaVersion,
	}, "\x00") + instructionHashPart(service.Instruction)))

	return hex.EncodeToString(hash[:])
}

func (service AIAnalysisService) buildOpenAIAnalysisRequestBody(email EmailDetail) map[string]any {
	blocks := analysisBlocks(email)
	return map[string]any{
		"model":             service.Model,
		"stream":            true,
		"instructions":      service.analysisInstructions(),
		"input":             buildEmailAnalysisInput(service.ReviewRules, service.SequenceContext, service.ResponseLanguage, email),
		"max_output_tokens": 600,
		"text": map[string]any{
			"format": map[string]any{
				"type":        "json_schema",
				"name":        "email_analysis",
				"description": "Email text review result",
				"strict":      true,
				"schema":      service.emailAnalysisSchema(analysisBlockIDs(blocks)),
			},
		},
	}
}

const defaultAIInstructionHead = `You are an email copy reviewer for partner onboarding sequences.
Use the provided review rules and onboarding sequence context.
Review only email text and metadata.`

func (service AIAnalysisService) analysisInstructions() string {
	head := defaultAIInstructionHead
	if strings.TrimSpace(service.Instruction) != "" {
		head = strings.TrimSpace(service.Instruction)
	}

	return head + "\n" + fmt.Sprintf(strings.TrimSpace(`
The email text may be in any language. Do not use the email language for your response unless it is also the requested response language.
All human-readable output fields MUST be written in %s:
- summary
- every recommendation.title
- every recommendation.details
Do not write those fields in declared_language or in the email's own language when it differs from %s.
primary_cta is the actual button CTA.
If primary_cta is present, do not infer the main CTA from links or repeated body text.
Support links are not competing CTAs when they help complete primary_cta and are phrased as help.
Footer links are omitted from the input.
Use support and other links only as supporting context.
email.blocks are the parts of the email in order, each with an id and its text. Set each recommendation's block to the id of the part it is about, or subject or preheader; use "" only when it is about the whole email.
Return only JSON matching the schema.
Limits:
- summary: 1 short sentence
- score: integer from 1 to 10
- recommendations: max 3
- recommendation details: max 220 characters
`), service.ResponseLanguage, service.ResponseLanguage)
}

// emailAnalysisSchema is the answer the model gives; blockIDs are the values
// a recommendation's block may take (see analysisBlockIDs).
func (service AIAnalysisService) emailAnalysisSchema(blockIDs []string) map[string]any {
	responseLanguage := service.ResponseLanguage

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"summary", "score", "verdict", "checks", "recommendations"},
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": fmt.Sprintf("One short sentence written only in %s.", responseLanguage),
				"maxLength":   160,
			},
			"score": map[string]any{
				"type":    "integer",
				"minimum": 1,
				"maximum": 10,
			},
			"verdict": map[string]any{
				"type": "string",
				"enum": []string{"ready", "minor_fixes", "needs_work"},
			},
			"checks": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required": []string{
					"subject",
					"preheader",
					"focus",
					"cta",
					"stage_alignment",
					"readability",
				},
				"properties": map[string]any{
					"subject":         analysisStatusSchema(),
					"preheader":       analysisStatusSchema(),
					"focus":           analysisStatusSchema(),
					"cta":             analysisStatusSchema(),
					"stage_alignment": analysisStatusSchema(),
					"readability":     analysisStatusSchema(),
				},
			},
			"recommendations": map[string]any{
				"type":     "array",
				"maxItems": 3,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"priority", "block", "title", "details"},
					"properties": map[string]any{
						"block": map[string]any{
							"type":        "string",
							"description": "The id of the email block the recommendation is about, from email.blocks, or subject or preheader; empty for the whole email.",
							"enum":        blockIDs,
						},
						"priority": map[string]any{
							"type": "string",
							"enum": []string{"high", "medium", "low"},
						},
						"title": map[string]any{
							"type":        "string",
							"description": fmt.Sprintf("Recommendation title written only in %s.", responseLanguage),
							"maxLength":   80,
						},
						"details": map[string]any{
							"type":        "string",
							"description": fmt.Sprintf("Actionable recommendation details written only in %s.", responseLanguage),
							"maxLength":   220,
						},
					},
				},
			},
		},
	}
}

func analysisStatusSchema() map[string]any {
	return map[string]any{
		"type": "string",
		"enum": []string{"good", "weak", "bad"},
	}
}

func buildEmailAnalysisInput(reviewRules string, sequenceContext string, responseLanguage string, email EmailDetail) string {
	parts := emailContentParts(email)

	type emailInput struct {
		Subject    string             `json:"subject"`
		Preheader  string             `json:"preheader"`
		BannerText string             `json:"banner_text"`
		PrimaryCTA string             `json:"primary_cta"`
		BodyText   string             `json:"body_text"`
		Links      analysisLinkGroups `json:"links"`
		Blocks     []analysisBlock    `json:"blocks"`
	}

	type analysisInput struct {
		Rules            string     `json:"rules"`
		SequenceContext  string     `json:"sequence_context"`
		ResponseLanguage string     `json:"response_language"`
		Stage            string     `json:"stage"`
		Timing           string     `json:"timing"`
		TimingIntent     string     `json:"timing_intent"`
		DeclaredLanguage string     `json:"declared_language"`
		Email            emailInput `json:"email"`
	}

	input := analysisInput{
		Rules:            reviewRules,
		SequenceContext:  sequenceContext,
		ResponseLanguage: responseLanguage,
		Stage:            email.Stage,
		Timing:           stringValue(email.SendTiming),
		TimingIntent:     inferTimingIntent(email.Title, email.SendTiming),
		DeclaredLanguage: email.Language,
		Email: emailInput{
			Subject:    firstNonEmpty(parts.Subject, stringValue(email.Subject)),
			Preheader:  firstNonEmpty(parts.Preheader, stringValue(email.Preheader)),
			BannerText: parts.BannerText,
			PrimaryCTA: analysisPrimaryCTA(parts.PrimaryCTA),
			BodyText:   limitText(analysisBodyText(firstNonEmpty(parts.BodyText, emailBodyText(email))), 3000),
			Links:      emailLinkGroups(parts),
			Blocks:     analysisBlocks(email),
		},
	}

	inputBytes, err := json.Marshal(input)
	if err != nil {
		return "{}"
	}

	return string(inputBytes)
}

func emailContentParts(email EmailDetail) EmailContentParts {
	if email.ContentParts == nil || strings.TrimSpace(*email.ContentParts) == "" {
		return EmailContentParts{}
	}

	var parts EmailContentParts
	if err := json.Unmarshal([]byte(*email.ContentParts), &parts); err != nil {
		return EmailContentParts{}
	}

	return parts
}

type analysisLinkGroups struct {
	Primary []string `json:"primary"`
	Support []string `json:"support"`
	Other   []string `json:"other"`
}

func emailLinkGroups(parts EmailContentParts) analysisLinkGroups {
	if len(parts.LinkGroups.Primary) > 0 ||
		len(parts.LinkGroups.Support) > 0 ||
		len(parts.LinkGroups.Footer) > 0 ||
		len(parts.LinkGroups.Other) > 0 {
		return analysisLinkGroups{
			Primary: nonFooterLinkTexts(parts.LinkGroups.Primary),
			Support: nonFooterLinkTexts(parts.LinkGroups.Support),
			Other:   nonFooterLinkTexts(parts.LinkGroups.Other),
		}
	}

	return analysisLinkGroups{Other: nonFooterLinkTexts(parts.Links)}
}

func analysisPrimaryCTA(value string) string {
	if isFooterLinkText(value) {
		return ""
	}

	return value
}

func analysisBodyText(value string) string {
	trimmedValue := strings.TrimSpace(value)
	lowerValue := strings.ToLower(trimmedValue)
	footerStart := len(trimmedValue)

	if index := strings.Index(lowerValue, "all rights reserved"); index >= 0 {
		footerStart = copyrightStart(lowerValue, index)
	}

	for _, marker := range []string{
		"this is an automatically generated notification",
		"you received this email because",
		"to manage your email preferences",
	} {
		if index := strings.Index(lowerValue, marker); index >= 0 && index < footerStart {
			footerStart = index
		}
	}

	return strings.TrimSpace(trimmedValue[:footerStart])
}

// copyrightStart finds where the copyright line ending in "all rights
// reserved" (at index) begins: at a nearby "©", or else at the company name
// sentence right before it ("Acme. All rights reserved.").
func copyrightStart(lowerValue string, index int) int {
	prefix := lowerValue[:index]
	if sign := strings.LastIndex(prefix, "©"); sign >= 0 && index-sign <= 80 {
		return sign
	}

	companyName := strings.TrimSuffix(strings.TrimRight(prefix, " "), ".")
	if sentenceEnd := strings.LastIndex(companyName, ". "); sentenceEnd >= 0 {
		return sentenceEnd + 1
	}

	return 0
}

func nonFooterLinkTexts(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !isFooterLinkText(value) {
			filtered = append(filtered, value)
		}
	}

	return filtered
}

func isFooterLinkText(value string) bool {
	lowerValue := strings.ToLower(value)
	return strings.Contains(lowerValue, "privacy") ||
		strings.Contains(lowerValue, "unsubscribe") ||
		strings.Contains(lowerValue, "terms") ||
		strings.Contains(lowerValue, "telegram") ||
		strings.Contains(lowerValue, "linkedin") ||
		strings.Contains(lowerValue, "youtube") ||
		strings.Contains(lowerValue, "facebook")
}

func emailBodyText(email EmailDetail) string {
	if email.BodyText != nil && strings.TrimSpace(*email.BodyText) != "" {
		return *email.BodyText
	}

	return htmlToText(email.OriginalHTML)
}

func inferTimingIntent(title string, sendTiming *string) string {
	value := strings.ToLower(title + " " + stringValue(sendTiming))

	switch {
	case strings.Contains(value, "last call") || strings.Contains(value, "last-call") || strings.Contains(value, "14 days"):
		return "last-call"
	case strings.Contains(value, "follow up 2") || strings.Contains(value, "follow-up 2") || strings.Contains(value, "follow-up-2") || strings.Contains(value, "5 days"):
		return "follow-up-2"
	case strings.Contains(value, "follow up 1") || strings.Contains(value, "follow-up 1") || strings.Contains(value, "follow-up-1") || strings.Contains(value, "2 days"):
		return "follow-up-1"
	case strings.Contains(value, "next step") || strings.Contains(value, "next-step") || strings.Contains(value, "immediately"):
		return "next-step"
	default:
		return ""
	}
}

func loadAIContextFile(fileName string) (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for dir := workingDir; ; dir = filepath.Dir(dir) {
		candidates := []string{
			filepath.Join(dir, "ai", fileName),
			filepath.Join(dir, "apps", "api", "ai", fileName),
		}
		for _, candidate := range candidates {
			rulesBytes, err := os.ReadFile(candidate)
			if err != nil && !os.IsNotExist(err) {
				return "", err
			}
			if err == nil {
				return string(rulesBytes), nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	return "", fmt.Errorf("ai/%s was not found from %s or its parents", fileName, workingDir)
}
