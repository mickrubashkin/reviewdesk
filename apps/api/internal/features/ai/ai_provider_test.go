package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderFromEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("BITRIX_API_KEY", "vibe-test")
	t.Setenv("BITRIX_API_BASE_URL", "https://platform.test/v1/")

	t.Setenv("AI_PROVIDER", "")
	if p := providerFromEnv(); p.chat || p.url != "https://api.openai.com/v1/responses" || p.apiKey != "sk-test" {
		t.Errorf("default: %+v", p)
	}
	t.Setenv("AI_PROVIDER", "vibecode")
	if p := providerFromEnv(); !p.chat || p.url != "https://platform.test/v1/chat/completions" || p.apiKey != "vibe-test" {
		t.Errorf("vibecode: %+v", p)
	}
}

func TestChatCompletionsBody(t *testing.T) {
	schema := map[string]any{"type": "object"}
	body := chatCompletionsBody(map[string]any{
		"model":             "m",
		"stream":            true,
		"instructions":      "be brief",
		"input":             "the email",
		"max_output_tokens": 600,
		"reasoning":         map[string]any{"effort": "low"},
		"text": map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "email_analysis", "strict": true, "schema": schema,
		}},
	})

	got, _ := json.Marshal(body)
	want := `{"max_completion_tokens":600,"messages":[{"content":"be brief","role":"system"},{"content":"the email","role":"user"}],` +
		`"model":"m","reasoning_effort":"low","response_format":{"json_schema":{"name":"email_analysis","schema":{"type":"object"},"strict":true},"type":"json_schema"},` +
		`"stream":true,"stream_options":{"include_usage":true}}`
	if string(got) != want {
		t.Errorf("body:\n got %s\nwant %s", got, want)
	}
}

func TestReadChatStream(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"choices":[{"delta":{"content":"{\"a\":"}}]}`,
		`data: {"choices":[{"delta":{"content":"1}"},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14,"prompt_tokens_details":{"cached_tokens":2}}}`,
		`data: [DONE]`,
	}, "\n\n")
	var metrics AIAnalysisMetrics
	var deltas []string
	text, err := readChatStreamOutputText(strings.NewReader(stream), &metrics, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil || text != `{"a":1}` {
		t.Fatalf("text %q, err %v", text, err)
	}
	if len(deltas) != 2 || *metrics.InputTokens != 10 || *metrics.OutputTokens != 4 || *metrics.TotalTokens != 14 || *metrics.CachedTokens != 2 {
		t.Errorf("deltas %v, metrics %+v", deltas, metrics)
	}

	// The platform reports some errors inside a stream that began with 200.
	_, err = readChatStreamOutputText(strings.NewReader(`data: {"error":{"message":"model overloaded"}}`), &metrics, nil)
	if err == nil || !strings.Contains(err.Error(), "model overloaded") {
		t.Errorf("error in stream: %v", err)
	}

	_, err = readChatStreamOutputText(strings.NewReader(`data: {"choices":[{"delta":{},"finish_reason":"length"}]}`+"\n\ndata: [DONE]"), &metrics, nil)
	if err == nil || !strings.Contains(err.Error(), "length") {
		t.Errorf("cut off: %v", err)
	}
}

func TestExtractChatOutputText(t *testing.T) {
	var metrics AIAnalysisMetrics
	provider := aiProvider{chat: true}
	text, err := provider.readResponse([]byte(`{"choices":[{"message":{"content":"{\"fields\":[]}"}}],"usage":{"prompt_tokens":5}}`), &metrics)
	if err != nil || text != `{"fields":[]}` || *metrics.InputTokens != 5 {
		t.Errorf("text %q, err %v, metrics %+v", text, err, metrics)
	}
	if _, err := provider.readResponse([]byte(`{"error":{"message":"bad key"}}`), &metrics); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Errorf("error answer: %v", err)
	}
}

func TestAnalysisThroughTheVibecodeProvider(t *testing.T) {
	var received map[string]any
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("X-Api-Key") != "vibe-test" || r.Header.Get("Authorization") != "" {
			http.Error(w, "wrong request", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"{\"summary\":\"ok\"}"}}]}`+"\n\n"+`data: [DONE]`+"\n\n")
	}))
	defer platform.Close()

	t.Setenv("AI_PROVIDER", "vibecode")
	t.Setenv("BITRIX_API_KEY", "vibe-test")
	t.Setenv("BITRIX_API_BASE_URL", platform.URL+"/v1")
	service := AIAnalysisService{Provider: providerFromEnv(), APIKey: "vibe-test", Model: "bitrix/bitrixgpt-5.5", Client: platform.Client()}

	result, err := service.AnalyzeEmailStream(context.Background(), EmailDetail{}, nil)
	if err != nil {
		t.Fatalf("analysis: %v", err)
	}
	if result.Metrics.Status != "success" {
		t.Errorf("metrics: %+v", result.Metrics)
	}
	if received["model"] != "bitrix/bitrixgpt-5.5" || received["response_format"] == nil || received["input"] != nil {
		t.Errorf("platform got: %v", received)
	}
}
