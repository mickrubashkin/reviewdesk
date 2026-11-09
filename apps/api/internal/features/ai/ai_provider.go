package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// aiProvider is where model calls go. By default that is OpenAI's Responses
// API. With AI_PROVIDER=vibecode it is the VibeCode platform's router, which
// speaks Chat Completions only: the calls still build a Responses body, and
// the provider turns it into a Chat Completions one.
type aiProvider struct {
	chat   bool
	url    string
	apiKey string
}

func providerFromEnv() aiProvider {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("AI_PROVIDER")), "vibecode") {
		base := strings.TrimRight(strings.TrimSpace(os.Getenv("BITRIX_API_BASE_URL")), "/")
		if base == "" {
			base = "https://vibecode.bitrix24.tech/v1"
		}
		return aiProvider{chat: true, url: base + "/chat/completions", apiKey: strings.TrimSpace(os.Getenv("BITRIX_API_KEY"))}
	}
	return aiProvider{url: "https://api.openai.com/v1/responses", apiKey: os.Getenv("OPENAI_API_KEY")}
}

// keyName names the variable the key comes from, for "not configured" errors.
func (p aiProvider) keyName() string {
	if p.chat {
		return "BITRIX_API_KEY"
	}
	return "OPENAI_API_KEY"
}

func (p aiProvider) newRequest(ctx context.Context, responsesBody map[string]any) (*http.Request, error) {
	body := responsesBody
	if p.chat {
		body = chatCompletionsBody(responsesBody)
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if p.chat {
		request.Header.Set("X-Api-Key", p.apiKey)
	} else {
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	return request, nil
}

// readStream reads the streamed answer's text, passing each piece to onDelta.
func (p aiProvider) readStream(body io.Reader, metrics *AIAnalysisMetrics, onDelta func(string) error) (string, error) {
	if p.chat {
		return readChatStreamOutputText(body, metrics, onDelta)
	}
	return readOpenAIStreamOutputText(body, metrics, onDelta)
}

// readResponse reads a whole (not streamed) answer's text and usage.
func (p aiProvider) readResponse(responseBytes []byte, metrics *AIAnalysisMetrics) (string, error) {
	if p.chat {
		applyChatUsage(metrics, responseBytes)
		return extractChatOutputText(responseBytes)
	}
	applyOpenAIUsage(metrics, responseBytes)
	return extractOpenAIOutputText(responseBytes)
}

// chatCompletionsBody turns a Responses request into a Chat Completions one:
// instructions and input become the system and user messages, text.format
// becomes response_format, max_output_tokens becomes max_completion_tokens.
func chatCompletionsBody(responsesBody map[string]any) map[string]any {
	messages := []map[string]any{}
	if instructions, ok := responsesBody["instructions"].(string); ok && instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}
	messages = append(messages, map[string]any{"role": "user", "content": responsesBody["input"]})

	body := map[string]any{"model": responsesBody["model"], "messages": messages}
	if text, ok := responsesBody["text"].(map[string]any); ok {
		if format, ok := text["format"].(map[string]any); ok && format["type"] == "json_schema" {
			schema := map[string]any{}
			for _, key := range []string{"name", "description", "strict", "schema"} {
				if value, ok := format[key]; ok {
					schema[key] = value
				}
			}
			body["response_format"] = map[string]any{"type": "json_schema", "json_schema": schema}
		}
	}
	if limit, ok := responsesBody["max_output_tokens"]; ok {
		body["max_completion_tokens"] = limit
	}
	if reasoning, ok := responsesBody["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"]; ok {
			body["reasoning_effort"] = effort
		}
	}
	if stream, ok := responsesBody["stream"].(bool); ok && stream {
		body["stream"] = true
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	return body
}

type chatError struct {
	Message string `json:"message"`
}

type chatUsage struct {
	PromptTokens        *int `json:"prompt_tokens"`
	CompletionTokens    *int `json:"completion_tokens"`
	TotalTokens         *int `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (usage *chatUsage) apply(metrics *AIAnalysisMetrics) {
	if usage == nil {
		return
	}
	metrics.InputTokens = usage.PromptTokens
	metrics.OutputTokens = usage.CompletionTokens
	metrics.TotalTokens = usage.TotalTokens
	metrics.CachedTokens = usage.PromptTokensDetails.CachedTokens
}

// readChatStreamOutputText reads a Chat Completions stream. The platform can
// report an error inside a stream that started with HTTP 200.
func readChatStreamOutputText(body io.Reader, metrics *AIAnalysisMetrics, onDelta func(string) error) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	var output strings.Builder
	finishReason := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *chatUsage `json:"usage"`
			Error *chatError `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return "", fmt.Errorf("AI stream error: %s", chunk.Error.Message)
		}
		chunk.Usage.apply(metrics)
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil {
				finishReason = *choice.FinishReason
			}
			if choice.Delta.Content == "" {
				continue
			}
			output.WriteString(choice.Delta.Content)
			if onDelta != nil {
				if err := onDelta(choice.Delta.Content); err != nil {
					return "", err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	text := strings.TrimSpace(output.String())
	if text == "" {
		if finishReason != "" {
			// A reasoning model can spend the whole limit before writing text.
			return "", fmt.Errorf("AI response incomplete: %s", finishReason)
		}
		return "", fmt.Errorf("AI stream did not include output text")
	}
	return stripMarkdownCodeFence(text), nil
}

func extractChatOutputText(responseBytes []byte) (string, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error *chatError `json:"error"`
	}
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		return "", err
	}
	if response.Error != nil {
		return "", fmt.Errorf("AI error: %s", response.Error.Message)
	}
	if len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		if len(response.Choices) > 0 && response.Choices[0].FinishReason != "" {
			return "", fmt.Errorf("AI response incomplete: %s", response.Choices[0].FinishReason)
		}
		return "", fmt.Errorf("AI response did not include output text")
	}
	return stripMarkdownCodeFence(strings.TrimSpace(response.Choices[0].Message.Content)), nil
}

func applyChatUsage(metrics *AIAnalysisMetrics, responseBytes []byte) {
	var response struct {
		Usage *chatUsage `json:"usage"`
	}
	if json.Unmarshal(responseBytes, &response) == nil {
		response.Usage.apply(metrics)
	}
}
