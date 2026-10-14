package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type AIAnalysisService struct {
	APIKey           string
	Model            string
	ResponseLanguage string
	Instruction      string
	ReviewRules      string
	SequenceContext  string
	Client           *http.Client
}

func NewAIAnalysisService() (AIAnalysisService, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENAI_API_KEY is not set, AI analysis will be disabled")
	}

	reviewRules, err := loadAIContextFile("email_review_rules.md")
	if err != nil {
		return AIAnalysisService{}, err
	}

	sequenceContext, err := loadAIContextFile("onboarding-sequence.md")
	if err != nil {
		return AIAnalysisService{}, err
	}

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-5-nano"
	}
	responseLanguage := os.Getenv("AI_RESPONSE_LANGUAGE")
	if responseLanguage == "" {
		responseLanguage = "Russian"
	}

	return AIAnalysisService{
		APIKey:           os.Getenv("OPENAI_API_KEY"),
		Model:            model,
		ResponseLanguage: responseLanguage,
		ReviewRules:      reviewRules,
		SequenceContext:  sequenceContext,
		Client:           &http.Client{Timeout: 45 * time.Second},
	}, nil
}

func (service AIAnalysisService) AnalyzeEmail(ctx context.Context, email EmailDetail) (AIAnalysisResult, error) {
	return service.analyzeEmail(ctx, email, nil)
}

func (service AIAnalysisService) AnalyzeEmailStream(ctx context.Context, email EmailDetail, onDelta func(string) error) (AIAnalysisResult, error) {
	return service.analyzeEmail(ctx, email, onDelta)
}

func (service AIAnalysisService) analyzeEmail(ctx context.Context, email EmailDetail, onDelta func(string) error) (AIAnalysisResult, error) {
	startedAt := time.Now()
	metrics := AIAnalysisMetrics{
		Model:  service.Model,
		Status: "error",
	}

	requestBody := service.buildOpenAIAnalysisRequestBody(email)
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return AIAnalysisResult{Metrics: finishAIAnalysisMetrics(metrics, startedAt, err)}, err
	}

	outputText, err := service.requestOpenAIAnalysis(ctx, bodyBytes, &metrics, onDelta)
	if err != nil {
		return AIAnalysisResult{Metrics: finishAIAnalysisMetrics(metrics, startedAt, err)}, err
	}

	var analysis EmailAnalysis
	if err := json.Unmarshal([]byte(extractJSONObject(outputText)), &analysis); err != nil {
		err := fmt.Errorf("failed to parse AI analysis JSON: %w", err)
		return AIAnalysisResult{Metrics: finishAIAnalysisMetrics(metrics, startedAt, err)}, err
	}

	metrics.Status = "success"
	return AIAnalysisResult{
		Analysis: analysis,
		Metrics:  finishAIAnalysisMetrics(metrics, startedAt, nil),
	}, nil
}

func finishAIAnalysisMetrics(metrics AIAnalysisMetrics, startedAt time.Time, err error) AIAnalysisMetrics {
	metrics.LatencyMS = int(time.Since(startedAt).Milliseconds())
	if err != nil {
		message := err.Error()
		metrics.ErrorMessage = &message
	}

	return metrics
}
