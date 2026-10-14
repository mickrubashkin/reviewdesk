package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (service AIAnalysisService) requestOpenAIAnalysis(ctx context.Context, bodyBytes []byte, metrics *AIAnalysisMetrics, onDelta func(string) error) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+service.APIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := service.Client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBytes, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			return "", readErr
		}

		return "", fmt.Errorf("openai returned %d: %s", response.StatusCode, string(responseBytes))
	}

	return readOpenAIStreamOutputText(response.Body, metrics, onDelta)
}

func readOpenAIStreamOutputText(body io.Reader, metrics *AIAnalysisMetrics, onDelta func(string) error) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	var output strings.Builder
	var finalResponseBytes []byte

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var event struct {
			Type     string          `json:"type"`
			Delta    string          `json:"delta"`
			Response json.RawMessage `json:"response"`
			Error    *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "response.output_text.delta":
			output.WriteString(event.Delta)
			if onDelta != nil && event.Delta != "" {
				if err := onDelta(event.Delta); err != nil {
					return "", err
				}
			}
		case "response.completed":
			if len(event.Response) > 0 {
				finalResponseBytes = event.Response
			}
		case "error":
			if event.Error != nil && strings.TrimSpace(event.Error.Message) != "" {
				return "", fmt.Errorf("openai stream error: %s", event.Error.Message)
			}
			return "", fmt.Errorf("openai stream error")
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(finalResponseBytes) > 0 {
		applyOpenAIUsage(metrics, finalResponseBytes)
	}

	text := strings.TrimSpace(output.String())
	if text == "" && len(finalResponseBytes) > 0 {
		return extractOpenAIOutputText(finalResponseBytes)
	}
	if text == "" {
		return "", fmt.Errorf("openai stream did not include output text")
	}

	return stripMarkdownCodeFence(text), nil
}

func applyOpenAIUsage(metrics *AIAnalysisMetrics, responseBytes []byte) {
	var response struct {
		Usage struct {
			InputTokens  *int `json:"input_tokens"`
			OutputTokens *int `json:"output_tokens"`
			TotalTokens  *int `json:"total_tokens"`
			// Chat Completions and some Responses payloads report cached prompt tokens here.
			PromptTokensDetails struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			// Keep this for Responses usage payloads if they expose input token details.
			InputTokensDetails struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		return
	}

	metrics.InputTokens = response.Usage.InputTokens
	metrics.OutputTokens = response.Usage.OutputTokens
	metrics.TotalTokens = response.Usage.TotalTokens
	metrics.CachedTokens = firstInt(
		response.Usage.InputTokensDetails.CachedTokens,
		response.Usage.PromptTokensDetails.CachedTokens,
	)
}

func firstInt(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}

	return nil
}

func extractOpenAIOutputText(responseBytes []byte) (string, error) {
	var response struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		return "", err
	}

	if strings.TrimSpace(response.OutputText) != "" {
		return strings.TrimSpace(response.OutputText), nil
	}

	var builder strings.Builder
	for _, output := range response.Output {
		for _, content := range output.Content {
			if strings.TrimSpace(content.Text) != "" {
				builder.WriteString(content.Text)
			}
		}
	}

	text := strings.TrimSpace(builder.String())
	if text == "" {
		return "", fmt.Errorf("openai response did not include output text")
	}

	return stripMarkdownCodeFence(text), nil
}

func stripMarkdownCodeFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}

	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```JSON")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")

	return strings.TrimSpace(text)
}

func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end < start {
		return text
	}

	return text[start : end+1]
}
