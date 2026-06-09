package summarizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaSummarizer calls the Ollama REST API.
type OllamaSummarizer struct {
	endpoint string
	model    string
	think    *bool
	timeout  time.Duration
}

type ollamaRequest struct {
	Model   string        `json:"model"`
	Prompt  string        `json:"prompt"`
	Stream  bool          `json:"stream"`
	Think   *bool         `json:"think,omitempty"`
	Options ollamaOptions `json:"options"`
}

type ollamaOptions struct {
	NumPredict int `json:"num_predict"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

func (o *OllamaSummarizer) Summarize(text string, opts SummarizeOptions) (SummarizeResult, error) {
	model := o.model
	if opts.Model != "" {
		model = opts.Model
	}

	combinedPrompt := resolvePrompt(text, opts)

	// Auto-scale num_predict when thinking is enabled, since Ollama's
	// num_predict controls the total budget for thinking + response combined.
	numPredict := opts.MaxTokens
	if o.think == nil || *o.think {
		numPredict = opts.MaxTokens * 4
	}

	reqBody := ollamaRequest{
		Model:  model,
		Prompt: combinedPrompt,
		Stream: false,
		Think:  o.think,
		Options: ollamaOptions{
			NumPredict: numPredict,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return SummarizeResult{}, fmt.Errorf("ollama: marshal request: %w", err)
	}

	timeout := o.timeout
	if timeout == 0 {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	url := o.endpoint + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return SummarizeResult{}, fmt.Errorf("ollama: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return SummarizeResult{}, fmt.Errorf("ollama: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return SummarizeResult{}, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		baseErr := fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode, string(respBody))
		if qe := quotaErrorFrom("ollama", baseErr, resp.StatusCode, string(respBody), resp.Header.Get("Retry-After"), time.Now()); qe != nil {
			return SummarizeResult{}, qe
		}
		return SummarizeResult{}, baseErr
	}

	var result ollamaResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return SummarizeResult{}, fmt.Errorf("ollama: parse response: %w", err)
	}

	return SummarizeResult{
		Text:     StripThinkingTags(result.Response),
		Provider: "ollama",
		Model:    model,
	}, nil
}
