package ai

import (
	"strings"
	"testing"
)

func TestReadOpenAIStreamOutputTextSaysWhyItStopped(t *testing.T) {
	cases := map[string]string{
		`data: {"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`: "openai response incomplete: max_output_tokens",
		`data: {"type":"response.failed","response":{"status":"failed","error":{"message":"model overloaded"}}}`:                      "openai response failed: model overloaded",
		`data: {"type":"response.failed","response":{}}`:                                                                              "openai stream ended with response.failed",
	}
	for stream, want := range cases {
		body := strings.NewReader(`data: {"type":"response.created"}` + "\n\n" + stream + "\n\n")
		_, err := readOpenAIStreamOutputText(body, &AIAnalysisMetrics{}, nil)
		if err == nil || err.Error() != want {
			t.Errorf("got %v, want %q", err, want)
		}
	}
}
